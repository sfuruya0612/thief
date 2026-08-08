package util

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
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
