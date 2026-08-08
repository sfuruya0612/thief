//go:build unix

package util

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// executerHelperReadyVar はヘルパープロセスとして動くことを示す環境変数である。
// 値は子プロセスが起動を知らせるために作るファイルのパスとして使う。
const executerHelperReadyVar = "THIEF_TEST_EXEC_COMMAND_READY_FILE"

const (
	// ヘルパープロセスの子プロセスが各シグナルの trap で使う終了コード。
	// INT と TERM で別の値にして、どちらが届いたかを終了コードで見分ける。
	// シグナルで落ちた場合の ExitCode は -1 になるため、trap を通ったかどうかも区別できる。
	childSIGINTExitCode  = 43
	childSIGTERMExitCode = 42

	// ヘルパープロセスが観測結果を返せなかった場合の終了コード。
	// trap が動かず子プロセスがシグナルで落ちた場合と、ExecCommand が
	// *exec.ExitError 以外のエラーを返した場合を区別する。
	helperChildSignaledExitCode = 92
	helperFailureExitCode       = 91

	// 子プロセスのループの 1 周期。trap は実行中の sleep が終わってから走るため、
	// シグナルが届いてから子プロセスが終了するまでの遅れの上限になる。
	helperChildTickSeconds = 1

	// 親だけに SIGINT を送った後、子プロセスに届いていないことを確かめるために待つ時間。
	// 転送されていれば上記の遅れの範囲で子プロセスが終了するため、それより長く取る。
	sigintNotForwardedGrace = 3 * time.Second
)

// helperChildScript はヘルパープロセスが起動する子プロセスのスクリプトを返す。
// 目印のファイルの作成は trap を張った後に行う。親はこのファイルの出現を待ってシグナルを
// 送るため、先に作ると trap が無い状態でシグナルを受けうる。
// ループに上限を置いているのは、シグナルが届かなかった場合にこのプロセスがテストの終了後も
// 残り続けないための保険である。
func helperChildScript(ready string) string {
	return fmt.Sprintf(`trap 'exit %d' INT
trap 'exit %d' TERM
: > '%s'
i=0
while [ "$i" -lt 30 ]; do
	sleep %d
	i=$((i + 1))
done`, childSIGINTExitCode, childSIGTERMExitCode, ready, helperChildTickSeconds)
}

// TestExecCommandSignalHelper は executerHelperReadyVar が設定されているときだけ動く
// ヘルパーである。ExecCommand のシグナルの扱いは、それを呼んでいるプロセス自身が
// シグナルを受け取ったときの挙動であり、テストプロセスに送って観測するとテストプロセス
// ごと落ちうる。そのためテストバイナリを別プロセスとして起動し、そのプロセスにシグナルを
// 送って結果を終了コードで受け取る。
//
// 結果を終了コードで返すため、テストの報告機構ではなく os.Exit で終了する。
func TestExecCommandSignalHelper(t *testing.T) {
	ready := os.Getenv(executerHelperReadyVar)
	if ready == "" {
		t.Skip("this test only runs as a helper process")
	}

	err := ExecCommand("sh", "-c", helperChildScript(ready))

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			// 子プロセスが trap を通らずシグナルで落ちた場合。
			os.Exit(helperChildSignaledExitCode)
		}
		os.Exit(code)
	}
	if err != nil {
		t.Logf("ExecCommand returned an unexpected error: %v", err)
		os.Exit(helperFailureExitCode)
	}
	os.Exit(0)
}

// TestExecCommand_SignalHandling は ExecCommand を呼んでいるプロセスがシグナルを受けたとき、
// 子プロセスがそれを処理して終わることを検証する。
//
// SIGINT は端末からフォアグラウンドのプロセスグループ全体へ配送されるため、親は購読して
// 握りつぶすだけでよい (子プロセスは親を経由せず受け取る)。親が購読を止めると、親自身が
// SIGINT の既定の動作で落ち、子プロセスの終了コードを持ち帰れなくなる。
//
// SIGTERM は送られた 1 プロセスにしか届かないため、親が子プロセスへ転送する必要がある。
// 転送しないと親は call.Wait の中で子プロセスの終了を待ち続け、SIGTERM で終了できない。
func TestExecCommand_SignalHandling(t *testing.T) {
	tests := []struct {
		name string
		// toProcessGroup は端末からの Ctrl-C を模して、プロセスグループ全体へ送るかを示す。
		toProcessGroup bool
		signal         syscall.Signal
		wantExitCode   int
	}{
		{
			name:           "SIGINT is left to the child process",
			toProcessGroup: true,
			signal:         syscall.SIGINT,
			wantExitCode:   childSIGINTExitCode,
		},
		{
			name:         "SIGTERM is forwarded to the child process",
			signal:       syscall.SIGTERM,
			wantExitCode: childSIGTERMExitCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := startExecCommandHelper(t)
			helper.send(t, tt.signal, tt.toProcessGroup)

			err := helper.waitForExit(t)

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("helper exited with %v, want exit code %d; output:\n%s", err, tt.wantExitCode, helper.output)
			}
			if got := exitErr.ExitCode(); got != tt.wantExitCode {
				t.Errorf("helper exit code = %d, want %d; output:\n%s", got, tt.wantExitCode, helper.output)
			}
		})
	}
}

// TestExecCommand_DoesNotForwardSIGINT は親プロセスだけに送られた SIGINT が子プロセスへ
// 転送されないことを検証する。SIGINT を転送すると、端末から配送された分と合わせて
// 子プロセスは同じ中断を 2 回受け取る。
//
// 親が SIGINT を購読していない場合は既定の動作で落ちるため、この検証は
// 「親が生き残り、かつ子プロセスも動き続ける」ことを同時に確かめている。
func TestExecCommand_DoesNotForwardSIGINT(t *testing.T) {
	helper := startExecCommandHelper(t)
	helper.send(t, syscall.SIGINT, false)

	select {
	case err := <-helper.exited:
		t.Fatalf("helper exited with %v; SIGINT sent only to the parent must neither kill it nor reach the child process; output:\n%s", err, helper.output)
	case <-time.After(sigintNotForwardedGrace):
	}
}

// execCommandHelper は起動済みのヘルパープロセスと、その終了を受け取る口をまとめる。
type execCommandHelper struct {
	cmd    *exec.Cmd
	exited <-chan error
	output *bytes.Buffer
}

// startExecCommandHelper はテストバイナリを TestExecCommandSignalHelper だけを走らせる
// ヘルパープロセスとして起動し、その子プロセスが trap を張り終えるまで待つ。
func startExecCommandHelper(t *testing.T) *execCommandHelper {
	t.Helper()

	ready := filepath.Join(t.TempDir(), "ready")

	cmd := exec.Command(os.Args[0], "-test.run=TestExecCommandSignalHelper")
	cmd.Env = append(os.Environ(), executerHelperReadyVar+"="+ready)
	// テストプロセスと同じプロセスグループに入れない。プロセスグループへ送るシグナルが
	// テストプロセス自身に届くと、テスト全体が落ちる。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// 出力は失敗時の手がかりとして残す。
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		t.Fatalf("start the helper process: %v", err)
	}
	t.Cleanup(func() {
		// ヘルパーとその子プロセスをまとめて落とす。既に終了している場合は
		// エラーが返るだけで、後始末として困ることは無い。
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	helper := &execCommandHelper{cmd: cmd, exited: exited, output: &out}
	helper.waitForChildReady(t, ready)

	return helper
}

// send はヘルパープロセス、または端末からの配送を模してそのプロセスグループ全体に
// シグナルを送る。
func (h *execCommandHelper) send(t *testing.T, sig syscall.Signal, toProcessGroup bool) {
	t.Helper()

	// 負の pid はプロセスグループ全体を指す。Setpgid でヘルパーを新しいプロセスグループの
	// 長にしているため、pid とグループ ID は同じ値になる。
	target := h.cmd.Process.Pid
	if toProcessGroup {
		target = -h.cmd.Process.Pid
	}
	if err := syscall.Kill(target, sig); err != nil {
		t.Fatalf("send %v to the helper process: %v", sig, err)
	}
}

// waitForChildReady はヘルパープロセスの子プロセスが目印のファイルを作るまで待つ。
// trap を張る前にシグナルを送ると、届いたかどうかを終了コードで見分けられなくなる。
func (h *execCommandHelper) waitForChildReady(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case err := <-h.exited:
			t.Fatalf("the helper process exited before its child was ready: %v; output:\n%s", err, h.output)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child process did not start: %s was never created; output:\n%s", path, h.output)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitForExit はヘルパープロセスの終了を待つ。待ち続けないよう上限を置く。上限に達した
// 場合、シグナルが子プロセスに届かず ExecCommand が待ち続けている。
func (h *execCommandHelper) waitForExit(t *testing.T) error {
	t.Helper()

	select {
	case err := <-h.exited:
		return err
	case <-time.After(30 * time.Second):
		t.Fatalf("the helper process did not exit; the signal never reached the child process; output:\n%s", h.output)
		return nil
	}
}
