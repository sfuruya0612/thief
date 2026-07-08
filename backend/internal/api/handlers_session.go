package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/session"
)

// ecsExecDefaultCommand は ECS Exec でコマンド未指定時に実行するデフォルトコマンド。
// 旧 CLI (cmd/ecs.go の ecsExecCmd) のデフォルト値に揃える。
const ecsExecDefaultCommand = "/bin/sh"

// sessionTerminateTimeout はセッション確立に失敗した際の後始末 (TerminateSSMSession 呼び出し) に使うタイムアウト。
const sessionTerminateTimeout = 5 * time.Second

// wsOriginPatterns はブラウザからの WebSocket アップグレードを許可するオリジンパターン。
// DNS rebinding 対策のため websocket.Accept の OriginPatterns にのみ渡し、InsecureSkipVerify は使わない。
// frontend dev server (mise run frontend:run) のポートに合わせる。
var wsOriginPatterns = []string{"localhost:8082", "127.0.0.1:8082"}

// handleEC2Session は EC2 インスタンスへの SSM Session Manager セッションを開始し、
// ブラウザ WebSocket とデータチャネルの間をブリッジする。
func (s *Server) handleEC2Session(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	instance := r.PathValue("instance")

	result, err := awsinternal.StartSSMSession(r.Context(), profile, region, instance)
	if err != nil {
		writeAWSError(w, err)
		return
	}

	s.runSessionBridge(w, r, result, func(ctx context.Context) error {
		return awsinternal.TerminateSSMSession(ctx, profile, region, result.SessionID)
	})
}

// handleECSExec は ECS タスクコンテナへの ExecuteCommand セッションを開始し、
// ブラウザ WebSocket とデータチャネルの間をブリッジする。
func (s *Server) handleECSExec(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	task := r.PathValue("task")

	container := r.URL.Query().Get("container")
	if container == "" {
		writeBadRequest(w, "container query parameter is required")
		return
	}
	command := r.URL.Query().Get("command")
	if command == "" {
		command = ecsExecDefaultCommand
	}

	result, err := awsinternal.ExecuteECSCommand(r.Context(), profile, region, cluster, task, container, command)
	if err != nil {
		writeAWSError(w, err)
		return
	}

	s.runSessionBridge(w, r, result, func(ctx context.Context) error {
		return awsinternal.TerminateSSMSession(ctx, profile, region, result.SessionID)
	})
}

// runSessionBridge はデータチャネルへ接続し、ブラウザ WebSocket をアップグレードした上でブリッジを開始する。
// StartSSMSession/ExecuteECSCommand が成功した直後に呼ぶこと。
// データチャネル接続やアップグレードに失敗した場合は、AWS 側のセッションが残らないよう terminate を呼んでから
// 通常の HTTP エラーを返す (アップグレード前なので通常のレスポンスがまだ書ける)。
func (s *Server) runSessionBridge(w http.ResponseWriter, r *http.Request, result *awsinternal.StartSessionResult, terminate session.TerminateFunc) {
	ctx := r.Context()

	dc, err := session.OpenDataChannel(ctx, result.StreamURL, result.TokenValue, result.SessionID)
	if err != nil {
		writeInternalError(w, "open data channel: "+err.Error())
		terminateBestEffort(terminate)
		return
	}

	browser, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: wsOriginPatterns,
	})
	if err != nil {
		slog.Warn("failed to accept browser websocket", "err", err)
		if closeErr := dc.Close(); closeErr != nil {
			slog.Warn("failed to close data channel after accept failure", "err", closeErr)
		}
		terminateBestEffort(terminate)
		return
	}

	bridge := &session.Bridge{DataChannel: dc, Browser: browser, Terminate: terminate}
	if err := bridge.Run(ctx); err != nil {
		slog.Error("session bridge ended with error", "err", err)
	}
}

// terminateBestEffort はアップグレード前のセットアップ失敗時に、専用の短命 context で
// セッション終了処理を試みる。エラーはログに残すのみで呼び出し元には伝播しない。
func terminateBestEffort(terminate session.TerminateFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionTerminateTimeout)
	defer cancel()
	if err := terminate(ctx); err != nil {
		slog.Warn("failed to terminate session after setup failure", "err", err)
	}
}
