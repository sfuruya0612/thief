package util

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecCommand_Simple(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Skip on Windows as 'echo' command is different
		t.Skip("Skipping test on Windows")
	}

	// Execute a simple command that should succeed
	err := ExecCommand("echo", "test")

	// Verify no error is returned
	assert.NoError(t, err)
}

func TestExecCommand_Error(t *testing.T) {
	// Execute a command that should fail (non-existent command)
	err := ExecCommand("nonexistentcommand123456789")

	// Verify error is returned
	assert.Error(t, err)
}

func TestExecCommand_WithArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Skip on Windows as file system commands are different
		t.Skip("Skipping test on Windows")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "executer-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a simple test file
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	assert.NoError(t, err)

	// Execute command with arguments to list the file
	err = ExecCommand("ls", "-l", tempDir)

	// Verify no error is returned
	assert.NoError(t, err)
}
