package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/session"
)

// ecsExecDefaultCommand は ECS Exec でコマンド未指定時に実行するデフォルトコマンド。
// 旧 CLI (cmd/ecs.go の ecsExecCmd) のデフォルト値に揃える。
const ecsExecDefaultCommand = "/bin/sh"

// sessionErrorMessage はアップグレード後にセッションの確立へ失敗したことをブラウザへ伝える
// TEXT メッセージ。frontend (components/Terminal/Terminal.tsx) は {"type":"error","message":...}
// を解釈して画面に表示する (session/bridge.go の controlMessage と同じ形)。
type sessionErrorMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// handleEC2Session は EC2 インスタンスへの SSM Session Manager セッションを開始し、
// ブラウザ WebSocket とデータチャネルの間をブリッジする。
//
// AWS のセッションを開始する前に websocket.Accept で Origin とアップグレードの検査を行う。
// 順序を逆にすると、他サイトからの単純 GET (<img src="..."> など) が Origin の検査を受けずに
// AWS の API を呼べてしまう (GET は corsMiddleware と hostCheckMiddleware を通る)。
func (s *Server) handleEC2Session(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	instance := r.PathValue("instance")

	browser, ok := s.acceptSessionWebSocket(w, r)
	if !ok {
		return
	}

	result, err := s.startSSMSession(r.Context(), profile, region, instance)
	if err != nil {
		slog.Warn("failed to start ssm session", "err", err)
		failSessionSetup(browser, err.Error())
		return
	}

	s.runSessionBridge(r.Context(), browser, result, func(ctx context.Context) error {
		return s.terminateSSMSession(ctx, profile, region, result.SessionID)
	})
}

// handleECSExec は ECS タスクコンテナへの ExecuteCommand セッションを開始し、
// ブラウザ WebSocket とデータチャネルの間をブリッジする。
//
// handleEC2Session と同じく、AWS の呼び出しより前に websocket.Accept を呼ぶ。
// container の欠落は AWS を呼ばないため、従来どおりアップグレードより前に 400 で返す。
func (s *Server) handleECSExec(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	task := r.PathValue("task")

	container, ok := requireQueryParam(w, r, "container")
	if !ok {
		return
	}
	command := r.URL.Query().Get("command")
	if command == "" {
		command = ecsExecDefaultCommand
	}

	browser, ok := s.acceptSessionWebSocket(w, r)
	if !ok {
		return
	}

	result, err := s.executeECSCommand(r.Context(), profile, region, cluster, task, container, command)
	if err != nil {
		slog.Warn("failed to execute ecs command", "err", err)
		failSessionSetup(browser, err.Error())
		return
	}

	s.runSessionBridge(r.Context(), browser, result, func(ctx context.Context) error {
		return s.terminateSSMSession(ctx, profile, region, result.SessionID)
	})
}

// acceptSessionWebSocket はセッション用の WebSocket アップグレードを受け付ける。
// Origin の検査 (cfg.WebOrigins) とアップグレードのリクエストであることの検査は
// websocket.Accept が行う。失敗時は Accept が書いた応答をそのまま返し、ok=false を返す。
// AWS のセッションを開始する前に必ずこれを呼ぶこと。
func (s *Server) acceptSessionWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, bool) {
	// OriginPatterns は cfg.WebOrigins (デフォルト localhost:8088/127.0.0.1:8088、環境変数
	// THIEF_WEB_ORIGINS で上書き可能) に従う。DNS rebinding 対策のためここにのみ渡し、
	// InsecureSkipVerify は使わない。
	browser, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.cfg.WebOrigins,
	})
	if err != nil {
		slog.Warn("failed to accept session websocket", "err", err)
		return nil, false
	}
	return browser, true
}

// runSessionBridge はデータチャネルへ接続し、ブリッジを開始する。呼び出し時点でブラウザ
// WebSocket はアップグレード済みであること。
// データチャネルへの接続に失敗した場合は、AWS 側のセッションが残らないよう terminate を
// 呼んでから、ブラウザへエラーメッセージを送って閉じる (アップグレード後は通常の HTTP
// エラーを返せないため)。
func (s *Server) runSessionBridge(ctx context.Context, browser *websocket.Conn, result *awsinternal.StartSessionResult, terminate session.TerminateFunc) {
	dc, err := s.openDataChannel(ctx, result.StreamURL, result.TokenValue, result.SessionID)
	if err != nil {
		terminateBestEffort(terminate)
		failSessionSetup(browser, "open data channel: "+err.Error())
		return
	}

	bridge := &session.Bridge{DataChannel: dc, Browser: browser, Terminate: terminate}
	if err := bridge.Run(ctx); err != nil {
		slog.Error("session bridge ended with error", "err", err)
	}
}

// failSessionSetup はアップグレード後のセッション確立失敗をブラウザへ通知し、WebSocket を
// 閉じる。アップグレード後は HTTP のエラーを返せないため、TEXT メッセージで原因を伝える。
// 送信・クローズのエラーはログに残すのみで呼び出し元には伝播しない (この直後にハンドラが
// 終了するため。notifyLogTailEnd と同じ規約)。
func failSessionSetup(browser *websocket.Conn, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)
	defer cancel()

	payload, err := json.Marshal(sessionErrorMessage{Type: "error", Message: message})
	if err != nil {
		slog.Warn("failed to marshal session error message", "err", err)
	} else if err := browser.Write(ctx, websocket.MessageText, payload); err != nil {
		slog.Warn("failed to notify browser of session error", "err", err)
	}

	if err := browser.Close(websocket.StatusInternalError, ""); err != nil {
		slog.Warn("failed to close browser websocket after session setup failure", "err", err)
	}
}

// terminateBestEffort はセッション確立の途中で失敗したときに、専用の短命 context で
// セッション終了処理を試みる。エラーはログに残すのみで呼び出し元には伝播しない。
func terminateBestEffort(terminate session.TerminateFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)
	defer cancel()
	if err := terminate(ctx); err != nil {
		slog.Warn("failed to terminate session after setup failure", "err", err)
	}
}
