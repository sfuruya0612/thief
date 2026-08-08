package util

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// ExecCommand は外部プロセスを標準入出力を引き継いで実行する。
// 実行中の SIGINT は子プロセス側 (例: session-manager-plugin) に処理を委ねるため、
// 親プロセスでは無視する。
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

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigs:
			case <-done:
				signal.Stop(sigs)
				return
			}
		}
	}()
	defer close(done)

	return call.Run()
}
