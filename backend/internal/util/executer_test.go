package util

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExecCommand_Simple(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	err := ExecCommand("echo", "test")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecCommand_Error(t *testing.T) {
	err := ExecCommand("nonexistentcommand123456789")

	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestExecCommand_PreservesExitErrorChain は ExecCommand が exec のエラーをラップせずに
// 返すことを検証する。ここで fmt.Errorf("%v", err) のようにラップすると Unwrap の連鎖が
// 切れ、呼び出し側がいくら %w を積んでも errors.As は *exec.ExitError へ到達できない。
// 終了コードで分岐する呼び出し側 (internal/cli の session-manager-plugin 起動) が
// 依存している性質である。
func TestExecCommand_PreservesExitErrorChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}

	err := ExecCommand("sh", "-c", "exit 3")

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(*exec.ExitError) = false, want true; the chain is severed: %v", err)
	}
	if code := exitErr.ExitCode(); code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	// 接頭辞も付けない。呼び出し側が自前の文脈を前置するため、ここで足すと重なる。
	if msg := err.Error(); msg != "exit status 3" {
		t.Errorf("error = %q, want %q", msg, "exit status 3")
	}
}

// TestExecCommand_DoesNotLeakTheSignalGoroutine は ExecCommand がシグナルを待つ goroutine を
// 残さないことを検証する。終了の合図を送らないと、この goroutine は届かないシグナルを
// 待ったまま残る。CLI は 1 回の起動で数回しか呼ばないため実害は出にくいが、
// 「起動した goroutine には必ず終了経路を用意する」という前提が崩れていることは
// テストでしか分からない。
func TestExecCommand_DoesNotLeakTheSignalGoroutine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}

	// 1 度目の呼び出しは数に入れない。os/signal は最初の signal.Notify でパッケージ内の
	// goroutine を 1 つ起動し、それはプロセスが終わるまで残る。ExecCommand が残したものと
	// 混ざるため、数える前に起動させておく (このテストを単体で実行した場合に効く)。
	execCommandNoop(t)
	before := settledGoroutineCount(t)

	execCommandNoop(t)

	if after := settledGoroutineCount(t); after > before {
		t.Errorf("goroutines = %d, want it back to %d; the signal goroutine is still waiting", after, before)
	}
}

// execCommandNoop は何もせず正常終了する子プロセスで ExecCommand を 1 度呼ぶ。
func execCommandNoop(t *testing.T) {
	t.Helper()

	if err := ExecCommand("sh", "-c", "exit 0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// settledGoroutineCount は goroutine の数が落ち着くまで待ってその数を返す。
// goroutine の終了は非同期であり、呼び出しから戻った直後の数は多めに出る。
// 落ち着くのを待たずに基準値を取ると、残っている goroutine を基準に含めてしまい、
// リークを見逃す。
func settledGoroutineCount(t *testing.T) int {
	t.Helper()

	const (
		interval      = 10 * time.Millisecond
		stableSamples = 3
	)

	deadline := time.Now().Add(2 * time.Second)
	count := runtime.NumGoroutine()
	stable := 0
	for stable < stableSamples && time.Now().Before(deadline) {
		time.Sleep(interval)
		got := runtime.NumGoroutine()
		if got == count {
			stable++
			continue
		}
		count = got
		stable = 0
	}
	return count
}

func TestExecCommand_WithArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	tempDir, err := os.MkdirTemp("", "executer-test")
	if err != nil {
		t.Fatalf("unexpected error creating temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("unexpected error writing test file: %v", err)
	}

	err = ExecCommand("ls", "-l", tempDir)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
