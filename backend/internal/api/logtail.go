package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"golang.org/x/sync/errgroup"
)

// logTailReadLimit はブラウザ側 WebSocket からの 1 メッセージあたりの読み取り上限バイト数。
// ブラウザから backend へは切断検知のためのメッセージしか届かない想定のため小さくてよい。
const logTailReadLimit = 1 << 12 // 4KiB

// logTailControlMessage は Live Tail 終了時にブラウザへ送る制御メッセージ
// (session/bridge.go の exit 通知に倣った形)。
type logTailControlMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason,omitempty"`
}

// serveLogTail は Live Tail の WebSocket 中継の共通処理。GCP Cloud Logging と AWS
// CloudWatch Logs で共有する。produce は send コールバックへ各ログエントリの JSON バイト列を
// 渡すプロデューサで、ブラウザ切断や ctx キャンセル時に send がエラーを返して終了する。
//
// フレーム規約: ログエントリは TEXT フレームの JSON で 1 件ずつ push する。終了時は
// {"type":"end","reason":"..."} を送ってからクローズする。
func (s *Server) serveLogTail(w http.ResponseWriter, r *http.Request, produce func(ctx context.Context, send func(payload []byte) error) error) {
	// OriginPatterns は cfg.WebOrigins に従う。DNS rebinding 対策のためここにのみ渡し、
	// InsecureSkipVerify は使わない (handlers_session.go と同じ規約)。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.cfg.WebOrigins,
	})
	if err != nil {
		slog.Warn("failed to accept log tail websocket", "err", err)
		return
	}
	conn.SetReadLimit(logTailReadLimit)

	// errgroup + 明示的な cancel で、tail ストリームの終了とブラウザ切断のどちらが先に
	// 起きても確実にもう片方を止める (session/bridge.go の Bridge.Run を片方向 push 用に
	// 簡略化したもの)。
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer cancel()
		return produce(ctx, func(payload []byte) error {
			return conn.Write(ctx, websocket.MessageText, payload)
		})
	})
	g.Go(func() error {
		defer cancel()
		return discardLogTailBrowserMessages(ctx, conn)
	})

	err = g.Wait()

	reason := "stream ended"
	switch {
	case err == nil:
		// discardLogTailBrowserMessages がブラウザの正常切断で nil を返したケース。
	case errors.Is(err, context.Canceled):
		// もう片方の goroutine 終了に伴う cancel。ブラウザへは汎用メッセージのみ通知する。
	default:
		reason = err.Error()
		slog.Warn("log tail ended with error", "err", err)
	}
	notifyLogTailEnd(conn, reason)
	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		slog.Warn("failed to close log tail websocket", "err", err)
	}
}

// discardLogTailBrowserMessages はブラウザ→backend 方向を切断検知のためだけに読み捨てる。
// ブラウザの正常切断 (タブ/画面を閉じる等) は nil を返し、それ以外はエラーを返す。
func discardLogTailBrowserMessages(ctx context.Context, conn *websocket.Conn) error {
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			switch websocket.CloseStatus(err) {
			case websocket.StatusNormalClosure, websocket.StatusGoingAway:
				return nil
			}
			return err
		}
	}
}

// notifyLogTailEnd はセッション終了をブラウザへ通知する。呼び出し時点で r.Context() は
// すでにキャンセルされている可能性があるため、専用の短命 context を使う
// (session/bridge.go の cleanup と同じ規約)。送信エラーはログに残すのみで処理は継続する。
func notifyLogTailEnd(conn *websocket.Conn, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionTerminateTimeout)
	defer cancel()

	payload, err := json.Marshal(logTailControlMessage{Type: "end", Reason: reason})
	if err != nil {
		slog.Warn("failed to marshal log tail end message", "err", err)
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		slog.Warn("failed to notify browser of log tail end", "err", err)
	}
}
