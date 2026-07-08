package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/errgroup"
)

// terminateTimeout はセッション終了時に TerminateSSMSession を呼ぶ際のタイムアウト。
// ブリッジ終了処理はリクエストの ctx とは無関係に完了させる必要があるため、専用の短いタイムアウトを持つ。
const terminateTimeout = 5 * time.Second

// browserReadLimit はブラウザ側 WebSocket からの 1 メッセージあたりの読み取り上限バイト数。
// リサイズ制御用の JSON は小さいため、この上限で十分。
const browserReadLimit = 1 << 20 // 1MiB

// controlMessageType はブラウザ backend 間の TEXT (JSON) 制御メッセージの種別。
type controlMessageType string

const (
	controlTypeResize controlMessageType = "resize"
	controlTypeError  controlMessageType = "error"
	controlTypeExit   controlMessageType = "exit"
)

// resizeMessage はブラウザ→backend のリサイズ通知 (TEXT メッセージ) を表す。
type resizeMessage struct {
	Type controlMessageType `json:"type"`
	Cols uint32             `json:"cols"`
	Rows uint32             `json:"rows"`
}

// controlMessage は backend→ブラウザ の制御通知 (TEXT メッセージ) を表す。
type controlMessage struct {
	Type    controlMessageType `json:"type"`
	Message string             `json:"message,omitempty"`
}

// TerminateFunc はブリッジ終了時に呼び出すセッション終了処理 (SSM TerminateSession 等)。
type TerminateFunc func(ctx context.Context) error

// Bridge はデータチャネルとブラウザ WebSocket の間で双方向にバイト列を中継する。
type Bridge struct {
	DataChannel *DataChannel
	Browser     *websocket.Conn
	// Terminate はブリッジ終了時に呼び出す (省略可)。SSM セッションのクリーンアップに使う。
	Terminate TerminateFunc
}

// Run はブリッジを開始し、いずれかの方向が終了するまでブロックする。
// 片方の goroutine が終了すると ctx がキャンセルされ、もう片方も終了する。
// 戻り値は通信そのもののエラーであり、正常な切断 (channel_closed やクライアント切断) では nil を返す。
func (b *Bridge) Run(ctx context.Context) error {
	b.Browser.SetReadLimit(browserReadLimit)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// errgroup は non-nil error を返した場合のみ ctx をキャンセルするため、
	// 片方が nil で正常終了した場合 (channel_closed 等) はもう片方が読み取りをブロックし続けてしまう。
	// そのため両方の goroutine で明示的に外側の cancel を呼び、どちらの終了でも確実に相方を止める。
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer cancel()
		return b.pumpDataChannelToBrowser(ctx)
	})
	g.Go(func() error {
		defer cancel()
		return b.pumpBrowserToDataChannel(ctx)
	})

	err := g.Wait()

	b.cleanup()

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("session bridge: %w", err)
	}
	return nil
}

// cleanup はブリッジ終了後の後始末を行う。リクエストの ctx がすでにキャンセルされている可能性があるため、
// 専用の短命 context を使う。
func (b *Bridge) cleanup() {
	if err := b.DataChannel.Close(); err != nil {
		slog.Warn("failed to close data channel", "err", err)
	}

	if b.Terminate == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), terminateTimeout)
	defer cancel()
	if err := b.Terminate(ctx); err != nil {
		slog.Warn("failed to terminate ssm session", "err", err)
	}
}

// pumpDataChannelToBrowser はデータチャネルからの出力をブラウザへ BINARY メッセージとして転送する。
func (b *Bridge) pumpDataChannelToBrowser(ctx context.Context) error {
	for {
		result, err := b.DataChannel.Read(ctx)
		if err != nil {
			return err
		}

		if len(result.Output) > 0 {
			if err := b.Browser.Write(ctx, websocket.MessageBinary, result.Output); err != nil {
				return fmt.Errorf("write to browser: %w", err)
			}
		}

		if result.Closed {
			b.notifyExit(ctx, result.CloseMessage)
			return nil
		}
	}
}

// pumpBrowserToDataChannel はブラウザからの入力をデータチャネルへ転送する。
// BINARY メッセージは端末入力バイト列、TEXT メッセージはリサイズ等の JSON 制御として扱う。
func (b *Bridge) pumpBrowserToDataChannel(ctx context.Context) error {
	for {
		typ, data, err := b.Browser.Read(ctx)
		if err != nil {
			// ブラウザ側の正常切断 (Drawer を閉じる等) はここに到達する。上位で context.Canceled 相当として扱う。
			return err
		}

		switch typ {
		case websocket.MessageBinary:
			if err := b.DataChannel.SendInput(ctx, PayloadTypeOutput, data); err != nil {
				return fmt.Errorf("send input to data channel: %w", err)
			}
		case websocket.MessageText:
			if err := b.handleBrowserControlMessage(ctx, data); err != nil {
				return err
			}
		}
	}
}

// handleBrowserControlMessage はブラウザからの TEXT (JSON) 制御メッセージを処理する。
func (b *Bridge) handleBrowserControlMessage(ctx context.Context, data []byte) error {
	var msg resizeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("failed to unmarshal browser control message", "err", err)
		return nil
	}

	switch msg.Type {
	case controlTypeResize:
		if err := b.DataChannel.SendSize(ctx, msg.Cols, msg.Rows); err != nil {
			return fmt.Errorf("send size to data channel: %w", err)
		}
	default:
		slog.Warn("unknown browser control message type", "type", msg.Type)
	}
	return nil
}

// notifyExit はセッション終了をブラウザへ通知する。送信エラーはログに残すのみで処理は継続する
// (この直後にブリッジ全体が終了するため)。
func (b *Bridge) notifyExit(ctx context.Context, message string) {
	payload, err := json.Marshal(controlMessage{Type: controlTypeExit, Message: message})
	if err != nil {
		slog.Warn("failed to marshal exit notification", "err", err)
		return
	}
	if err := b.Browser.Write(ctx, websocket.MessageText, payload); err != nil {
		slog.Warn("failed to notify browser of session exit", "err", err)
	}
}
