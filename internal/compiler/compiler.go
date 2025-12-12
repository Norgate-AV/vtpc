// Package compiler provides VTPro file compilation orchestration and result parsing.
package compiler

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/Norgate-AV/vtpc/internal/interfaces"
	"github.com/Norgate-AV/vtpc/internal/logger"
	"github.com/Norgate-AV/vtpc/internal/timeouts"
	"github.com/Norgate-AV/vtpc/internal/vtpro"
	"github.com/Norgate-AV/vtpc/internal/windows"
)

const (
	// Dialog title constants
	dialogCompiling    = "VisionTools Pro-e Compiling..."
	dialogVTProWarning = "VisionTools(R) Pro-e"
	dialogAddressBook  = "Address Book"
)

// CompileResult holds the results of a compilation
type CompileResult struct {
	Warnings        int
	Errors          int
	ErrorMessages   []string
	WarningMessages []string
	HasErrors       bool
	Size            string // Output file size (e.g., "18,588,092 bytes")
	ProjectSize     string // Project size (e.g., "0 Kb")
}

// CompileOptions holds options for the compilation
type CompileOptions struct {
	FilePath                      string
	Hwnd                          uintptr
	VTProPid                      uint32        // Known PID from ShellExecuteEx (preferred over searching)
	VTProPidPtr                   *uint32       // Pointer to store PID for signal handlers
	SkipPreCompilationDialogCheck bool          // For testing - skip the pre-compilation dialog check
	CompilationTimeout            time.Duration // Override default timeout (0 = use default 5 minutes)
}

// CompileDependencies holds all external dependencies for testing
type CompileDependencies struct {
	ProcessMgr    interfaces.ProcessManager
	WindowMgr     interfaces.WindowManager
	Keyboard      interfaces.KeyboardInjector
	ControlReader interfaces.ControlReader
}

// Compiler orchestrates the compilation process with injected dependencies
type Compiler struct {
	log           logger.LoggerInterface
	processMgr    interfaces.ProcessManager
	windowMgr     interfaces.WindowManager
	keyboard      interfaces.KeyboardInjector
	controlReader interfaces.ControlReader
}

// NewCompiler creates a new Compiler with the provided logger and default dependencies
func NewCompiler(log logger.LoggerInterface) *Compiler {
	windowsAPI := windows.NewWindowsAPI(log)
	vtproAPI := vtpro.VTProProcessAPI{}

	return &Compiler{
		log:           log,
		processMgr:    vtproAPI,
		windowMgr:     windowsAPI,
		keyboard:      windowsAPI,
		controlReader: windowsAPI,
	}
}

// NewCompilerWithDeps creates a new Compiler with custom dependencies for testing
func NewCompilerWithDeps(log logger.LoggerInterface, deps *CompileDependencies) *Compiler {
	return &Compiler{
		log:           log,
		processMgr:    deps.ProcessMgr,
		windowMgr:     deps.WindowMgr,
		keyboard:      deps.Keyboard,
		controlReader: deps.ControlReader,
	}
}

// Compile orchestrates the compilation process for a VTPro file
// This includes:
// - Handling pre-compilation dialogs
// - Triggering the compile
// - Monitoring compilation progress
// - Parsing results
// - Closing dialogs
func (c *Compiler) Compile(opts CompileOptions) (*CompileResult, error) {
	result := &CompileResult{}

	// Use the exact PID from ShellExecuteEx - no searching, no guessing
	pid := opts.VTProPid
	if pid == 0 {
		c.log.Warn("No PID provided - dialog monitoring will be disabled")
		c.log.Info("Warning: Could not determine VTPro process PID; dialog detection may be limited")
	} else {
		c.log.Debug("Using VTPro PID from launch", slog.Uint64("pid", uint64(pid)))
		if opts.VTProPidPtr != nil {
			*opts.VTProPidPtr = pid // Store for signal handler
		}
	}

	// Confirm elevation before sending keystrokes
	if c.windowMgr.IsElevated() {
		c.log.Debug("Process is elevated, proceeding with keystroke injection")
	} else {
		c.log.Warn("Process is NOT elevated, keystroke injection may fail")
	}

	// Bring window to foreground and send compile keystroke
	c.log.Debug("Bringing window to foreground")
	focusSuccess := c.windowMgr.SetForeground(opts.Hwnd)
	if !focusSuccess {
		c.log.Warn("SetForeground failed on first attempt, retrying...")
		time.Sleep(500 * time.Millisecond)

		focusSuccess = c.windowMgr.SetForeground(opts.Hwnd)
		if !focusSuccess {
			c.log.Error("Failed to bring window to foreground after retry")
			return &CompileResult{
				Errors:        1,
				HasErrors:     true,
				ErrorMessages: []string{"Failed to bring VTPro to foreground - cannot send keystrokes"},
			}, fmt.Errorf("failed to bring VTPro to foreground - cannot send keystrokes")
		}
	}

	time.Sleep(timeouts.FocusVerificationDelay)

	// Verify the window is in the foreground before sending keystrokes
	c.log.Debug("Verifying foreground window")
	verified := c.windowMgr.VerifyForegroundWindow(opts.Hwnd, pid)
	if !verified {
		c.log.Error("Could not verify correct window is in foreground")
		return &CompileResult{
			Errors:        1,
			HasErrors:     true,
			ErrorMessages: []string{"Wrong window in foreground - cannot safely send keystrokes"},
		}, fmt.Errorf("wrong window in foreground - cannot safely send keystrokes")
	}

	// Handle any pre-compilation dialogs (like "Operation Complete") that may be blocking
	// Skip this in test mode since tests send all events upfront
	if pid != 0 && !opts.SkipPreCompilationDialogCheck {
		if err := c.handlePreCompilationDialogs(); err != nil {
			c.log.Warn("Error handling pre-compilation dialogs", slog.Any("error", err))
		}

		// Drain any stale events from pre-compilation phase BEFORE triggering compilation
		// This ensures we start with a clean channel and don't miss the Compiling dialog
		c.drainMonitorChannel()
	}

	// Try SendInput first (modern API, atomic operation)
	success := c.keyboard.SendF12WithSendInput()
	if !success {
		c.log.Warn("SendF12WithSendInput failed, falling back to keybd_event")
		c.keyboard.SendF12()
	} else {
		c.log.Debug("SendF12WithSendInput succeeded")
	}

	c.log.Debug("Starting compile monitoring")

	// Only attempt dialog handling if we have a valid PID
	if pid != 0 {
		// Use event-driven dialog handling
		var err error
		var eventResult *CompileResult

		eventResult, err = c.handleCompilationEvents(opts)
		if err != nil {
			// Return the result even on error so caller can see what happened
			return eventResult, err
		}

		// Copy event result into our result
		result = eventResult
	}

	// Close dialogs and handle post-compilation events
	c.log.Debug("Closing dialogs and VTPro...")

	// Close main window and handle any confirmation dialogs via events
	if opts.Hwnd != 0 {
		c.windowMgr.CloseWindow(opts.Hwnd, "VTPro")

		// Handle confirmation dialog that may appear when closing
		if pid != 0 {
			if err := c.handlePostCompilationEvents(); err != nil {
				// Return the result we have so far, even if cleanup failed
				return result, err
			}
		}

		if !opts.SkipPreCompilationDialogCheck {
			time.Sleep(timeouts.CleanupDelay)
		}
	}

	if result.HasErrors {
		return result, fmt.Errorf("compilation failed with %d error(s)", result.Errors)
	}

	return result, nil
}

// handleCompilationEvents uses an event-driven approach to respond to dialogs as they appear
func (c *Compiler) handleCompilationEvents(opts CompileOptions) (*CompileResult, error) {
	// Maximum time to wait for compilation to complete
	// Use custom timeout if specified, otherwise use default 5 minutes
	compilationTimeout := timeouts.CompilationCompleteTimeout
	if opts.CompilationTimeout > 0 {
		compilationTimeout = opts.CompilationTimeout
	}

	timeout := time.NewTimer(compilationTimeout)
	defer timeout.Stop()

	result := &CompileResult{}

	// Track what we've seen and what we're waiting for
	var (
		compilingDetected       bool
		compileCompleteDetected bool
		compilingDialogHwnd     uintptr
	)

	// Create a ticker to periodically check if compiling dialog has disappeared
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	c.log.Debug("Entering event-driven dialog monitoring loop")

	// Event loop - respond to dialogs as they appear in real-time
	for {
		select {
		case ev := <-windows.MonitorCh:
			c.log.Debug("Received window event",
				slog.String("title", ev.Title),
				slog.Uint64("hwnd", uint64(ev.Hwnd)),
			)

			// Handle each dialog type as it appears
			if ev.Title == dialogCompiling {
				// Compilation in progress
				if !compilingDetected {
					c.log.Debug("Detected 'VisionTools Pro-e Compiling...' dialog")
					c.log.Info("Compiling program...")
					compilingDetected = true
					compilingDialogHwnd = ev.Hwnd
				}
			}

		case <-ticker.C:
			// Periodically check if compiling dialog has disappeared (VTPro-specific)
			if compilingDetected && !compileCompleteDetected && compilingDialogHwnd != 0 {
				// Poll to see if the compiling dialog still exists
				if !c.windowMgr.IsWindowValid(compilingDialogHwnd) {
					c.log.Debug("Compiling dialog disappeared - compilation complete")
					c.log.Info("Gathering details...")

					// Give UI a moment to update (skip in test mode for speed)
					if !opts.SkipPreCompilationDialogCheck {
						time.Sleep(500 * time.Millisecond)
					}

					// Read Message Log from main window
					logText := c.readMessageLog(opts.Hwnd)
					if logText != "" {
						c.parseVTProOutput(logText, result)

						// Log any warning/error messages
						if len(result.ErrorMessages) > 0 || len(result.WarningMessages) > 0 {
							c.logCompilationMessages(result.ErrorMessages, result.WarningMessages)
						}
					} else {
						c.log.Warn("Could not read Message Log contents")
					}

					compileCompleteDetected = true
				}
			}

			// If we have detected compilation complete, we're done
			if compileCompleteDetected {
				// Set HasErrors flag
				result.HasErrors = result.Errors > 0 || len(result.ErrorMessages) > 0

				// Compilation complete
				return result, nil
			}

		case <-timeout.C:
			c.log.Error("Compilation timeout: compilation did not complete within 5 minutes")
			return &CompileResult{
				Errors:    1,
				HasErrors: true,
				ErrorMessages: []string{
					"Compilation timeout: compilation did not complete within 5 minutes",
				},
			}, fmt.Errorf("compilation timeout: compilation did not complete within 5 minutes")
		}
	}
}

// logCompilationMessages logs error/warning/notice messages with proper formatting
func (c *Compiler) logCompilationMessages(errorMsgs, warningMsgs []string) {
	if len(errorMsgs) > 0 {
		c.log.Info("")
		c.log.Info("Error messages:")
		for i, msg := range errorMsgs {
			c.log.Info(fmt.Sprintf("  %d. %s", i+1, msg),
				slog.Int("number", i+1),
				slog.String("type", "error"),
				slog.String("message", msg),
			)
		}
	}

	if len(warningMsgs) > 0 {
		c.log.Info("")
		c.log.Info("Warning messages:")
		for i, msg := range warningMsgs {
			c.log.Info(fmt.Sprintf("  %d. %s", i+1, msg),
				slog.Int("number", i+1),
				slog.String("type", "warning"),
				slog.String("message", msg),
			)
		}
	}

	// Add trailing blank line if any messages were displayed
	if len(errorMsgs) > 0 || len(warningMsgs) > 0 {
		c.log.Info("")
	}
}

// handlePreCompilationDialogs checks for and dismisses dialogs that may block compilation
// This includes "Operation Complete" dialog that can appear during VTPro startup
func (c *Compiler) handlePreCompilationDialogs() error {
	// Short timeout - check if there are any dialogs already present
	timeout := time.NewTimer(timeouts.WindowMessageDelay)
	defer timeout.Stop()

	for {
		select {
		case ev := <-windows.MonitorCh:
			c.log.Trace("Received pre-compilation event",
				slog.String("title", ev.Title),
				slog.Uint64("hwnd", uint64(ev.Hwnd)))

			// Enumerate and log all child controls for this dialog
			c.enumerateDialogControls(ev.Hwnd, ev.Title)

			// Handle dialogs that may block compilation
			switch ev.Title {
			case dialogVTProWarning:
				c.log.Trace("Detected VTPro warning dialog - closing")
				c.log.Debug("Handling pre-compilation warning dialog")
				c.windowMgr.CloseWindow(ev.Hwnd, dialogVTProWarning)

			default:
				// Log but don't handle other dialogs here
				c.log.Trace("Ignoring pre-compilation dialog", slog.String("title", ev.Title))
			}

		case <-timeout.C:
			// Timeout is fine - no blocking dialogs present
			return nil
		}
	}
}

// handlePostCompilationEvents waits for and handles any post-compilation dialogs (like Address Book)
func (c *Compiler) handlePostCompilationEvents() error {
	// Short timeout - if no confirmation dialog appears, that's fine
	timeout := time.NewTimer(timeouts.DialogConfirmationTimeout)
	defer timeout.Stop()

	select {
	case ev := <-windows.MonitorCh:
		c.log.Trace("Received post-compilation event",
			slog.String("title", ev.Title),
			slog.Uint64("hwnd", uint64(ev.Hwnd)))

		// Handle Address Book dialog if it appears
		if ev.Title == dialogAddressBook {
			c.log.Trace("Detected 'Address Book' dialog - closing")
			c.log.Debug("Handling Address Book dialog")
			c.windowMgr.CloseWindow(ev.Hwnd, dialogAddressBook)
		}

	case <-timeout.C:
		// Timeout is fine - dialog may not appear
	}

	return nil
}

// enumerateDialogControls enumerates and logs all child controls in a dialog window
func (c *Compiler) enumerateDialogControls(hwnd uintptr, title string) {
	c.log.Trace("Enumerating dialog controls",
		slog.String("title", title),
		slog.Uint64("hwnd", uint64(hwnd)))

	// Get the main window text (dialog body text, if any)
	windowText := c.windowMgr.GetWindowText(hwnd)
	if windowText != "" {
		c.log.Trace("Dialog window text",
			slog.String("title", title),
			slog.String("text", windowText))
	}

	// Collect all child controls
	childInfos := c.windowMgr.CollectChildInfos(hwnd)

	if len(childInfos) == 0 {
		c.log.Trace("No child controls found in dialog",
			slog.String("title", title))
		return
	}

	c.log.Trace("Found child controls in dialog",
		slog.String("title", title),
		slog.Int("count", len(childInfos)))

	// Log details for each child control
	for i, ci := range childInfos {
		logAttrs := []any{
			slog.String("title", title),
			slog.Int("index", i),
			slog.Uint64("childHwnd", uint64(ci.Hwnd)),
			slog.String("className", ci.ClassName),
		}

		if ci.Text != "" {
			logAttrs = append(logAttrs, slog.String("text", ci.Text))
		}

		if len(ci.Items) > 0 {
			logAttrs = append(logAttrs, slog.Any("items", ci.Items))
		}

		c.log.Trace("Child control", logAttrs...)
	}
}

// readMessageLog finds and reads the Message Log child window in VTPro
func (c *Compiler) readMessageLog(mainHwnd uintptr) string {
	c.log.Trace("Reading Message Log from main window")

	childInfos := c.windowMgr.CollectChildInfos(mainHwnd)

	// Look for a child control with "Message Log" text or that contains compilation output
	for _, ci := range childInfos {
		c.log.Trace("Checking child control",
			slog.String("class", ci.ClassName),
			slog.String("text", ci.Text),
		)

		// Look for compilation output markers
		if strings.Contains(ci.Text, "Compiling for") ||
			strings.Contains(ci.Text, "Successful") ||
			strings.Contains(ci.Text, "error(s)") {
			c.log.Trace("Found Message Log content",
				slog.String("className", ci.ClassName),
				slog.Int("textLength", len(ci.Text)),
			)
			return ci.Text
		}
	}

	c.log.Warn("Could not find Message Log control")
	return ""
}

// parseVTProOutput parses VTPro compilation output format
// Example format:
// ---------- Compiling for TSW-770: [...] ---------
// Boot
// ~DummyFlashPage - [ not compiled ]
// somepage
//
//	[ warning ]: Object "..." on Page "..." has an unassigned Smart Object ID.
//	[ error ]: Some error message
//
// ...
// ---------- Successful ---------
// 1 warning(s), 0 error(s)
func (c *Compiler) parseVTProOutput(text string, result *CompileResult) {
	c.log.Trace("Parsing VTPro output", slog.Int("textLength", len(text)))

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	// Process lines, handling multi-line messages (VTPro wraps long lines)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Look for warning messages: [ warning ]: ...
		if strings.Contains(line, "[ warning ]") {
			// Extract the warning message after "[ warning ]:"
			if idx := strings.Index(line, "[ warning ]:"); idx != -1 {
				msg := strings.TrimSpace(line[idx+len("[ warning ]:"):])

				// Check if next lines are continuations (indented or part of wrapped message)
				// Limit continuation to prevent unbounded growth
				maxContinuations := 5
				continuations := 0
				for i+1 < len(lines) && continuations < maxContinuations {
					nextLine := lines[i+1]
					// If next line doesn't start with a marker, it's a continuation
					if !strings.Contains(nextLine, "[ error ]") &&
						!strings.Contains(nextLine, "[ warning ]") &&
						!strings.Contains(nextLine, "----------") &&
						!strings.Contains(nextLine, "warning(s)") &&
						strings.TrimSpace(nextLine) != "" {
						i++
						continuations++
						msg += " " + strings.TrimSpace(nextLine)
					} else {
						break
					}
				}

				if msg != "" {
					result.WarningMessages = append(result.WarningMessages, msg)
					c.log.Trace("Found warning message", slog.String("message", msg))
				}
			}
		}

		// Look for error messages: [ error ]: ...
		if strings.Contains(line, "[ error ]") {
			// Extract the error message after "[ error ]:"
			if idx := strings.Index(line, "[ error ]:"); idx != -1 {
				msg := strings.TrimSpace(line[idx+len("[ error ]:"):])

				// Check if next lines are continuations (indented or part of wrapped message)
				// Limit continuation to prevent unbounded growth
				maxContinuations := 5
				continuations := 0
				for i+1 < len(lines) && continuations < maxContinuations {
					nextLine := lines[i+1]
					// If next line doesn't start with a marker, it's a continuation
					if !strings.Contains(nextLine, "[ error ]") &&
						!strings.Contains(nextLine, "[ warning ]") &&
						!strings.Contains(nextLine, "----------") &&
						!strings.Contains(nextLine, "warning(s)") &&
						strings.TrimSpace(nextLine) != "" {
						i++
						continuations++
						msg += " " + strings.TrimSpace(nextLine)
					} else {
						break
					}
				}

				if msg != "" {
					result.ErrorMessages = append(result.ErrorMessages, msg)
					c.log.Trace("Found error message", slog.String("message", msg))
				}
			}
		}

		// Look for size: [ size ]: 18,588,092 bytes
		if strings.Contains(line, "[ size ]") {
			if idx := strings.Index(line, "[ size ]:"); idx != -1 {
				size := strings.TrimSpace(line[idx+len("[ size ]:"):])
				if size != "" {
					result.Size = size
					c.log.Trace("Found size", slog.String("size", size))
				}
			}
		}

		// Look for project size: [ project size ]: 0 Kb
		if strings.Contains(line, "[ project size ]") {
			if idx := strings.Index(line, "[ project size ]:"); idx != -1 {
				projectSize := strings.TrimSpace(line[idx+len("[ project size ]:"):])
				if projectSize != "" {
					result.ProjectSize = projectSize
					c.log.Trace("Found project size", slog.String("projectSize", projectSize))
				}
			}
		}

		// Look for the summary line: "0 warning(s), 0 error(s)"
		// or "X warning(s), Y error(s)"
		if strings.Contains(line, "warning(s)") && strings.Contains(line, "error(s)") {
			c.log.Trace("Found summary line", slog.String("line", line))

			// Parse: "0 warning(s), 0 error(s)"
			pattern := regexp.MustCompile(`(\d+)\s+warning\(s\),\s+(\d+)\s+error\(s\)`)
			matches := pattern.FindStringSubmatch(line)

			if len(matches) >= 3 {
				if warnings, err := fmt.Sscanf(matches[1], "%d", &result.Warnings); err == nil {
					c.log.Trace("Parsed warnings", slog.Int("warnings", result.Warnings))
					_ = warnings
				}

				if errors, err := fmt.Sscanf(matches[2], "%d", &result.Errors); err == nil {
					c.log.Trace("Parsed errors", slog.Int("errors", result.Errors))
					_ = errors
				}
			}
		}
	}

	c.log.Trace("Parse complete",
		slog.Int("warnings", result.Warnings),
		slog.Int("errors", result.Errors),
		slog.String("size", result.Size),
		slog.String("projectSize", result.ProjectSize),
	)
}

// drainMonitorChannel drains any pending events from the monitor channel
// to ensure we don't miss critical events during compilation monitoring.
// This clears any stale pre-compilation events that may have accumulated.
func (c *Compiler) drainMonitorChannel() {
	if windows.MonitorCh == nil {
		return
	}

	c.log.Debug("Draining channel before compilation monitoring")
	drained := 0
	draining := true
	for draining {
		select {
		case ev := <-windows.MonitorCh:
			drained++
			c.log.Trace("Drained pre-compilation event",
				slog.String("title", ev.Title),
				slog.Uint64("hwnd", uint64(ev.Hwnd)))
		default:
			// Channel empty, ready to start monitoring
			draining = false
		}
	}

	if drained > 0 {
		c.log.Debug("Channel drained", slog.Int("events", drained))
	}
}
