package main

import (
	"context"
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestInterruptContextIsCanceledBySignals は interruptContext が返す context が
// SIGINT と SIGTERM の両方でキャンセルされることを、実際にシグナルを送って検証する。
//
// これが Ctrl-C を実行中の AWS 呼び出しへ届ける唯一の経路である。signal.NotifyContext を
// 外して context.Background を返しても、コンパイルも lint も通る。
//
// 自プロセスへシグナルを送るが、signal.NotifyContext がチャネルを登録した後に送るため
// 既定の動作 (プロセス終了) は起きない。stop を呼ぶと購読が外れて既定の動作に戻るため、
// 送信とキャンセルの確認は必ず stop より前に済ませる。
func TestInterruptContextIsCanceledBySignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal supports only Kill on Windows")
	}

	tests := []struct {
		name string
		sig  os.Signal
	}{
		{name: "SIGINT", sig: syscall.SIGINT},
		{name: "SIGTERM", sig: syscall.SIGTERM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, stop := interruptContext()
			// stop は Cleanup で登録する。以降の送信は必ず購読中に行われる。
			t.Cleanup(stop)

			if err := ctx.Err(); err != nil {
				t.Fatalf("ctx.Err() = %v before any signal, want nil", err)
			}

			self, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatalf("find own process: %v", err)
			}
			if err := self.Signal(tt.sig); err != nil {
				t.Fatalf("send %v to self: %v", tt.sig, err)
			}

			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
				t.Fatalf("ctx was not canceled within 10s after %v", tt.sig)
			}

			// 中断は context.Canceled として観測される。cli.Run はこれを見て
			// 終了コード 130 に振り分ける。
			if err := ctx.Err(); !errors.Is(err, context.Canceled) {
				t.Errorf("ctx.Err() = %v, want %v", err, context.Canceled)
			}
		})
	}
}
