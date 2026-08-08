package util

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// ExecCommand は外部プロセスを標準入出力を引き継いで実行する。
// 実行中の SIGINT は子プロセス側 (例: session-manager-plugin) に処理を委ねるため、
// 親プロセスでは無視する。SIGTERM は子プロセスへ転送する。
//
// SIGINT と SIGTERM で扱いが違うのは、届く相手が違うためである。端末からの Ctrl-C は
// フォアグラウンドのプロセスグループ全体に配送されるため、子プロセスは親が何もしなくても
// 受け取る。SIGTERM は明示的に送られた 1 プロセスにしか届かないため、転送しない限り
// 子プロセスは知らないままとなり、親は本関数の中で子の終了を待ち続ける。
// 呼び出し側の main は signal.NotifyContext でシグナルの既定の動作 (プロセスの即時終了) を
// 止めているため、転送しなければ SIGTERM で終了できないプロセスになる。
//
// 失敗時は exec が返したエラーをラップせずそのまま返す。何を実行しようとしたかを
// 述べられるのは呼び出し側だけであり、この関数がラップしても文脈は増えない。
// ラップしないことで、呼び出し側は errors.As で *exec.ExitError を取り出して
// 終了コードで分岐できる。
func ExecCommand(process string, args ...string) error {
	call := exec.Command(process, args...)
	call.Stderr = os.Stderr
	call.Stdout = os.Stdout
	call.Stdin = os.Stdin

	// 購読は Start より先に始める。後にすると、その間に届いたシグナルが既定の動作に
	// 落ちる。チャネルはバッファ 1 なので、goroutine を起動する前に届いた分も残る。
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	if err := call.Start(); err != nil {
		return err
	}

	// goroutine の起動は Start の後にする。call.Process が埋まるのは Start の中であり、
	// 先に起動すると転送のための参照が書き込みと競合する。
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case sig := <-sigs:
				if sig != syscall.SIGTERM {
					continue
				}
				// 転送の失敗は無視する。起こるのは子プロセスが既に終了している場合で、
				// そのとき call.Wait は戻っており転送する相手がいない。
				_ = call.Process.Signal(syscall.SIGTERM)
			case <-done:
				return
			}
		}
	}()

	return call.Wait()
}
