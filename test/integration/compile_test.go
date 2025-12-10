//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Norgate-AV/vtpc/internal/compiler"
	"github.com/Norgate-AV/vtpc/internal/logger"
	"github.com/Norgate-AV/vtpc/internal/timeouts"
	"github.com/Norgate-AV/vtpc/internal/vtpro"
	"github.com/Norgate-AV/vtpc/internal/windows"
)

// TestIntegration_SimpleCompile tests end-to-end compilation of a simple .vtp file
func TestIntegration_SimpleCompile(t *testing.T) {
	if !windows.IsElevated() {
		t.Skip("Integration tests require administrator privileges")
	}

	// Get fixture path
	fixturePath := getFixturePath(t, "simple.vtp")
	require.FileExists(t, fixturePath, "Fixture file should exist")

	// Run compilation
	result, cleanup := compileFile(t, fixturePath)
	defer cleanup()

	// Verify successful compilation
	assert.False(t, result.HasErrors, "Simple file should compile without errors")
	assert.Equal(t, 0, result.Errors, "Should have 0 errors")
	assert.GreaterOrEqual(t, result.Warnings, 0, "Warnings should be non-negative")

	// Ensure we didn't hit a timeout
	for _, msg := range result.ErrorMessages {
		assert.NotContains(t, msg, "timeout", "Should not have timed out")
	}
}

// TestIntegration_CompileWithWarnings tests compilation of a file that produces warnings
func TestIntegration_CompileWithWarnings(t *testing.T) {
	t.Skip()

	if !windows.IsElevated() {
		t.Skip("Integration tests require administrator privileges")
	}

	fixturePath := getFixturePath(t, "warnings.vtp")
	require.FileExists(t, fixturePath, "Fixture file should exist")

	result, cleanup := compileFile(t, fixturePath)
	defer cleanup()

	// Verify compilation with warnings
	assert.False(t, result.HasErrors, "Should compile successfully despite warnings")
	assert.Equal(t, 0, result.Errors, "Should have 0 errors")
	assert.Greater(t, result.Warnings, 0, "Should have at least 1 warning")
	assert.Len(t, result.WarningMessages, result.Warnings, "Warning count should match messages")

	// Ensure we didn't hit a timeout
	for _, msg := range result.ErrorMessages {
		assert.NotContains(t, msg, "timeout", "Should not have timed out")
	}
}

// TestIntegration_CompileWithErrors tests compilation of a file that produces errors
func TestIntegration_CompileWithErrors(t *testing.T) {
	t.Skip()

	if !windows.IsElevated() {
		t.Skip("Integration tests require administrator privileges")
	}

	fixturePath := getFixturePath(t, "error.vtp")
	require.FileExists(t, fixturePath, "Fixture file should exist")

	result, cleanup := compileFile(t, fixturePath)
	defer cleanup()

	// Verify compilation failed with errors
	assert.True(t, result.HasErrors, "Should fail compilation with errors")
	assert.Greater(t, result.Errors, 1, "Should have at least 2 errors (not just a timeout)")
	assert.Len(t, result.ErrorMessages, result.Errors, "Error count should match messages")

	// Ensure we didn't hit a timeout
	for _, msg := range result.ErrorMessages {
		assert.NotContains(t, msg, "timeout", "Should not have timed out - actual compilation errors expected")
		assert.NotContains(t, msg, "Compile Complete", "Should not have timed out - actual compilation errors expected")
	}
}

// TestIntegration_FileValidation tests the file validation that should occur before compilation
func TestIntegration_FileValidation(t *testing.T) {
	// This test doesn't require admin privileges - it's just file validation

	nonExistentPath := filepath.Join(os.TempDir(), "nonexistent.vtp")

	// Ensure file doesn't exist
	os.Remove(nonExistentPath)

	// Create a minimal logger for the test
	testLog, err := logger.NewLogger(logger.LoggerOptions{Verbose: false})
	require.NoError(t, err, "Should create logger")
	defer testLog.Close()

	// Test that file validation (as done in cmd/root.go) catches non-existent files
	_, err = os.Stat(nonExistentPath)

	// Should detect the file doesn't exist
	assert.True(t, os.IsNotExist(err), "Should detect non-existent file")
}

// Helper Functions

// getFixturePath returns the absolute path to a test fixture
func getFixturePath(t *testing.T, filename string) string {
	// Get the current working directory
	cwd, err := os.Getwd()
	require.NoError(t, err, "Should get current directory")

	// Construct path to fixtures directory
	// Assuming tests run from project root
	fixturePath := filepath.Join(cwd, "test", "integration", "fixtures", filename)

	// If not found, try from test/integration directory
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		fixturePath = filepath.Join(cwd, "fixtures", filename)
	}

	return fixturePath
}

// compileFile performs end-to-end compilation and returns result with cleanup function
func compileFile(t *testing.T, filePath string) (*compiler.CompileResult, func()) {
	require.FileExists(t, filePath, "File should exist before compilation")

	// Convert to absolute path
	absPath, err := filepath.Abs(filePath)
	require.NoError(t, err, "Should resolve absolute path")

	// Create a minimal logger for the test (discard output)
	testLog, err := logger.NewLogger(logger.LoggerOptions{Verbose: false})
	require.NoError(t, err, "Should create logger")
	defer testLog.Close()

	// Create SIMPL client
	vtproClient := vtpro.NewClient(testLog)

	// Open file with VTPro
	t.Logf("Opening VTPro with file: %s", absPath)
	pid, err := windows.ShellExecuteEx(0, "open", vtpro.GetVTProPath(), absPath, "", 1, testLog)
	require.NoError(t, err, "Should launch VTPro")
	t.Logf("VTPro process started with PID: %d", pid)

	// Start background window monitor with the exact PID we just launched
	stopMonitor := vtproClient.StartMonitoring(pid)

	// Wait for process to start
	time.Sleep(timeouts.WindowMessageDelay)

	// Wait for window to appear
	t.Log("Waiting for VTPro to appear...")
	hwnd, found := vtproClient.WaitForAppear(pid, timeouts.WindowAppearTimeout)
	require.True(t, found, "VTPro should appear within timeout")
	require.NotZero(t, hwnd, "Should have valid window handle")

	// Wait for window to be ready
	t.Log("Waiting for window to be ready...")
	ready := vtproClient.WaitForReady(hwnd, timeouts.WindowReadyTimeout)
	require.True(t, ready, "VTPro should be ready within timeout")

	// Allow UI to settle
	time.Sleep(timeouts.UISettlingDelay)

	// Use the PID from ShellExecuteEx for compilation
	vtproPid := pid

	// Cleanup function
	cleanup := func() {
		t.Log("Cleaning up VTPro...")
		stopMonitor()
		if hwnd != 0 {
			vtproClient.Cleanup(hwnd, pid)
		}
		// Give it time to close
		time.Sleep(timeouts.FocusVerificationDelay)
	}

	// Run compilation
	t.Log("Starting compilation...")

	// Create compiler with logger
	comp := compiler.NewCompiler(testLog)

	result, err := comp.Compile(compiler.CompileOptions{
		FilePath:    absPath,
		Hwnd:        hwnd,
		VTProPid:    vtproPid,
		VTProPidPtr: &vtproPid,
	})
	// Note: We don't require NoError here because some tests expect compilation to fail
	if err != nil {
		t.Logf("Compilation returned error: %v", err)
	}

	require.NotNil(t, result, "Should always return a result")

	t.Logf("Compilation complete - Errors: %d, Warnings: %d",
		result.Errors, result.Warnings)

	return result, cleanup
}
