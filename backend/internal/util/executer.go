package util

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// ExecCommand は外部プロセスを標準入出力を引き継いで実行する。
// 実行中の SIGINT は子プロセス側 (例: session-manager-plugin) に処理を委ねるため、
// 親プロセスでは無視する。
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

	if err := call.Run(); err != nil {
		return fmt.Errorf("%v", err)
	}

	return nil
}
