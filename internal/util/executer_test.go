package util

import (
	"os"
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
