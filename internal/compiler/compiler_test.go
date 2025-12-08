package compiler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Norgate-AV/vtpc/internal/logger"
	"github.com/Norgate-AV/vtpc/internal/testutil"
	"github.com/Norgate-AV/vtpc/internal/windows"
)

func TestCompiler_SuccessfulCompilation(t *testing.T) {
	// Setup monitor channel for event-driven testing
	testutil.SetupMonitorChannel()
	defer testutil.CleanupMonitorChannel()

	// VTPro format: Message Log window with compilation results
	vtproOutput := "---------- Compiling for TSW-770: [test.vtp] ---------\nBoot\nMain\n---------- Successful ---------\n0 warning(s), 0 error(s)"

	mockWin := testutil.NewMockWindowManager().
		WithChildInfosForHwnd(0x9999, // Main VTPro window
			windows.ChildInfo{ClassName: "ListBox", Text: vtproOutput},
		).
		WithWindowValid(0x1111, false) // Compiling dialog will be invalid after compilation

	mockKbd := testutil.NewMockKeyboardInjector()
	mockCtrl := testutil.NewMockControlReader()
	mockProc := testutil.NewMockProcessManager().WithPid(1234)

	log := logger.NewNoOpLogger()
	deps := &CompileDependencies{
		ProcessMgr:    mockProc,
		WindowMgr:     mockWin,
		Keyboard:      mockKbd,
		ControlReader: mockCtrl,
	}

	compiler := NewCompilerWithDeps(log, deps)
	opts := CompileOptions{
		Hwnd:                          0x9999,
		VTProPid:                      1234,
		SkipPreCompilationDialogCheck: true,
	}

	// Send VTPro compiling dialog event
	testutil.SendEventsToMonitor(
		windows.WindowEvent{Hwnd: 0x1111, Title: "VisionTools Pro-e Compiling..."},
	)

	result, err := compiler.Compile(opts)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.HasErrors)
	assert.Equal(t, 0, result.Errors)
	assert.Equal(t, 0, result.Warnings)

	// Verify F12 was sent
	assert.True(t, mockKbd.SendF12WithSendInputCalled)

	// Verify window was set to foreground
	assert.Len(t, mockWin.SetForegroundCalls, 1)
	assert.Equal(t, uintptr(0x9999), mockWin.SetForegroundCalls[0])

	// Verify VTPro was closed
	assert.Len(t, mockWin.CloseWindowCalls, 1)
	assert.Equal(t, uintptr(0x9999), mockWin.CloseWindowCalls[0].Hwnd)
	assert.Equal(t, "VTPro", mockWin.CloseWindowCalls[0].Title)
}

func TestCompiler_WithWarnings(t *testing.T) {
	testutil.SetupMonitorChannel()
	defer testutil.CleanupMonitorChannel()

	vtproOutput := "---------- Compiling for TSW-770: [test.vtp] ---------\nBoot\nMain\n---------- Successful ---------\n2 warning(s), 0 error(s)"

	mockWin := testutil.NewMockWindowManager().
		WithChildInfosForHwnd(0x9999,
			windows.ChildInfo{ClassName: "ListBox", Text: vtproOutput},
		).
		WithWindowValid(0x1111, false)

	mockKbd := testutil.NewMockKeyboardInjector()
	mockCtrl := testutil.NewMockControlReader()
	mockProc := testutil.NewMockProcessManager().WithPid(1234)

	log := logger.NewNoOpLogger()
	deps := &CompileDependencies{
		ProcessMgr:    mockProc,
		WindowMgr:     mockWin,
		Keyboard:      mockKbd,
		ControlReader: mockCtrl,
	}

	compiler := NewCompilerWithDeps(log, deps)

	opts := CompileOptions{
		Hwnd:                          0x9999,
		VTProPid:                      1234,
		SkipPreCompilationDialogCheck: true,
	}

	testutil.SendEventsToMonitor(
		windows.WindowEvent{Hwnd: 0x1111, Title: "VisionTools Pro-e Compiling..."},
	)

	result, err := compiler.Compile(opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.HasErrors)
	assert.Equal(t, 0, result.Errors)
	assert.Equal(t, 2, result.Warnings)
}

func TestCompiler_WithErrors(t *testing.T) {
	testutil.SetupMonitorChannel()
	defer testutil.CleanupMonitorChannel()

	vtproOutput := "---------- Compiling for TSW-770: [test.vtp] ---------\nBoot\nMain\n---------- Failed ---------\n0 warning(s), 3 error(s)"

	mockWin := testutil.NewMockWindowManager().
		WithChildInfosForHwnd(0x9999,
			windows.ChildInfo{ClassName: "ListBox", Text: vtproOutput},
		).
		WithWindowValid(0x1111, false)

	mockKbd := testutil.NewMockKeyboardInjector()
	mockCtrl := testutil.NewMockControlReader()
	mockProc := testutil.NewMockProcessManager().WithPid(1234)

	log := logger.NewNoOpLogger()
	deps := &CompileDependencies{
		ProcessMgr:    mockProc,
		WindowMgr:     mockWin,
		Keyboard:      mockKbd,
		ControlReader: mockCtrl,
	}

	compiler := NewCompilerWithDeps(log, deps)

	opts := CompileOptions{
		Hwnd:                          0x9999,
		VTProPid:                      1234,
		SkipPreCompilationDialogCheck: true,
	}

	testutil.SendEventsToMonitor(
		windows.WindowEvent{Hwnd: 0x1111, Title: "VisionTools Pro-e Compiling..."},
	)

	result, err := compiler.Compile(opts)

	// Compile returns an error when there are compile errors
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compilation failed")
	assert.NotNil(t, result)
	assert.True(t, result.HasErrors)
	assert.Equal(t, 3, result.Errors)
	assert.Equal(t, 0, result.Warnings)
}

func TestCompiler_CompileDialogTimeout(t *testing.T) {
	testutil.SetupMonitorChannel()
	defer testutil.CleanupMonitorChannel()

	mockWin := testutil.NewMockWindowManager()

	mockKbd := testutil.NewMockKeyboardInjector()
	mockCtrl := testutil.NewMockControlReader()
	mockProc := testutil.NewMockProcessManager().WithPid(1234)

	log := logger.NewNoOpLogger()
	deps := &CompileDependencies{
		ProcessMgr:    mockProc,
		WindowMgr:     mockWin,
		Keyboard:      mockKbd,
		ControlReader: mockCtrl,
	}

	compiler := NewCompilerWithDeps(log, deps)

	opts := CompileOptions{
		Hwnd:                          0x9999,
		VTProPid:                      1234,
		SkipPreCompilationDialogCheck: true,
		CompilationTimeout:            1 * time.Second, // Fast timeout for testing
	}

	// Don't send any events to trigger timeout

	result, err := compiler.Compile(opts)

	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, err.Error(), "compilation did not complete")
	assert.True(t, result.HasErrors)
	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.ErrorMessages, 1)
}

func TestCompiler_NoPid(t *testing.T) {
	testutil.SetupMonitorChannel()
	defer testutil.CleanupMonitorChannel()

	// When PID is 0, dialog monitoring should be skipped but compilation should still proceed
	compilingDialogHwnd := uintptr(0x1111)
	mockWin := testutil.NewMockWindowManager().
		WithWindowValid(compilingDialogHwnd, false). // Dialog starts invalid since we skip monitoring
		WithChildInfosForHwnd(0x9999,
			windows.ChildInfo{ClassName: "ListBox", Items: []string{"0 warning(s), 0 error(s)"}},
		)

	mockKbd := testutil.NewMockKeyboardInjector()
	mockCtrl := testutil.NewMockControlReader()
	mockProc := testutil.NewMockProcessManager().WithPid(0) // PID not available

	log := logger.NewNoOpLogger()
	deps := &CompileDependencies{
		ProcessMgr:    mockProc,
		WindowMgr:     mockWin,
		Keyboard:      mockKbd,
		ControlReader: mockCtrl,
	}

	compiler := NewCompilerWithDeps(log, deps)

	opts := CompileOptions{
		Hwnd:                          0x9999,
		VTProPid:                      0, // No PID available
		SkipPreCompilationDialogCheck: true,
	}

	// PID=0 means no monitoring, so don't send events
	testutil.SendEventsToMonitor(
		windows.WindowEvent{Hwnd: compilingDialogHwnd, Title: "VisionTools Pro-e Compiling..."},
	)

	result, err := compiler.Compile(opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.HasErrors)

	// Verify F12 was still sent even without PID (new SendInput method should be called)
	assert.True(t, mockKbd.SendF12WithSendInputCalled)
}

func TestCompiler_WithSavePrompts(t *testing.T) {
	testutil.SetupMonitorChannel()
	defer testutil.CleanupMonitorChannel()

	compilingDialogHwnd := uintptr(0x1111)
	mockWin := testutil.NewMockWindowManager().
		WithWindowValid(compilingDialogHwnd, true). // Start valid
		WithChildInfosForHwnd(0x9999,
			windows.ChildInfo{ClassName: "ListBox", Items: []string{"0 warning(s), 0 error(s)"}},
		)

	mockKbd := testutil.NewMockKeyboardInjector()
	mockCtrl := testutil.NewMockControlReader()
	mockProc := testutil.NewMockProcessManager().WithPid(1234)

	log := logger.NewNoOpLogger()
	deps := &CompileDependencies{
		ProcessMgr:    mockProc,
		WindowMgr:     mockWin,
		Keyboard:      mockKbd,
		ControlReader: mockCtrl,
	}

	compiler := NewCompilerWithDeps(log, deps)

	opts := CompileOptions{
		Hwnd:                          0x9999,
		VTProPid:                      1234,
		SkipPreCompilationDialogCheck: true,
	}

	// Simulate compilation workflow with VTPro warning dialog
	go func() {
		time.Sleep(100 * time.Millisecond)
		testutil.SendEventsToMonitor(
			windows.WindowEvent{Hwnd: compilingDialogHwnd, Title: "VisionTools Pro-e Compiling..."},
		)
		time.Sleep(200 * time.Millisecond)
		mockWin.WithWindowValid(compilingDialogHwnd, false) // Dialog closes when compilation complete
	}()

	result, err := compiler.Compile(opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.HasErrors)

	// Verify F12 was sent to start compilation (using SendInput method)
	assert.True(t, mockKbd.SendF12WithSendInputCalled)
}
