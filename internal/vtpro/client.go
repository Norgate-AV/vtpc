// Package vtpro provides VTPro process management and interaction.
package vtpro

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unsafe"

	"github.com/Norgate-AV/vtpc/internal/logger"
	"github.com/Norgate-AV/vtpc/internal/timeouts"
	"github.com/Norgate-AV/vtpc/internal/windows"
)

// Client provides methods for interacting with VTPro processes
type Client struct {
	log logger.LoggerInterface
	win *windows.Client
}

// NewClient creates a new VTPro client
func NewClient(log logger.LoggerInterface) *Client {
	return &Client{
		log: log,
		win: windows.NewClient(log),
	}
}

// FindWindow searches for the VTPro main window belonging to a specific process
// targetPid must be a valid process ID - passing 0 will return no results
func (c *Client) FindWindow(targetPid uint32, debug bool) (uintptr, string) {
	result := c.findWindowWithTracking(targetPid, debug, nil)
	return result.mainHwnd, result.mainTitle
}

// windowSearchResult contains the results of a window search
type windowSearchResult struct {
	mainHwnd    uintptr
	mainTitle   string
	foundSplash bool
}

// findWindowWithTracking is the internal implementation that supports window tracking
// Returns the main window handle and title if found, or indicates if only splash screen was detected
func (c *Client) findWindowWithTracking(targetPid uint32, debug bool, seenWindows map[uintptr]bool) windowSearchResult {
	result := windowSearchResult{}

	// Must have a valid PID to search for windows
	if targetPid == 0 {
		if debug {
			c.log.Debug("No PID provided for window search")
		}
		return result
	}

	// Enumerate windows (thread-safe)
	windowsList := windows.EnumerateWindows()

	// Look for windows belonging to our process
	var mainWindow windows.WindowInfo
	var splashWindow windows.WindowInfo

	for _, w := range windowsList {
		if w.Pid == targetPid {
			// Only log if debug is enabled AND we haven't seen this window before
			shouldLog := debug && (seenWindows == nil || !seenWindows[w.Hwnd])
			if shouldLog {
				c.log.Debug("Window found",
					slog.String("title", w.Title),
					slog.Uint64("hwnd", uint64(w.Hwnd)),
				)
				if seenWindows != nil {
					seenWindows[w.Hwnd] = true
				}
			}

			// Get window class name and lowercase title for identification
			className := windows.GetClassName(w.Hwnd)
			title := strings.ToLower(w.Title)

			// Priority 1: Window with .vtp in title - file is definitely loaded
			if strings.Contains(w.Title, ".vtp") {
				mainWindow = w
				break
			}

			// Priority 2: VWT32AppClass - this is the main application window
			if className == "VWT32AppClass" {
				mainWindow = w
				break
			}

			// Skip progress dialogs
			if strings.Contains(title, "progress") {
				continue
			}

			// Skip splash screen - remember it but keep looking
			if w.Title == "VTPro" {
				splashWindow = w
				continue
			}

			// Skip common dialog window class (#32770)
			if className == "#32770" {
				continue
			}
		}
	}

	// If we found a main window with a more specific title, use it
	if mainWindow.Hwnd != 0 {
		if debug {
			c.log.Debug("Found main window", slog.String("title", mainWindow.Title))
		}

		result.mainHwnd = mainWindow.Hwnd
		result.mainTitle = mainWindow.Title
		return result
	}

	// If we only found the generic splash screen, indicate it but return no handle
	if splashWindow.Hwnd != 0 {
		result.foundSplash = true
	}

	return result
}

// WaitForReady waits for a window to become fully responsive
func (c *Client) WaitForReady(hwnd uintptr, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	elapsed := 0

	c.log.Debug("Waiting for window ready state",
		slog.Uint64("hwnd", uint64(hwnd)),
		slog.String("timeout", timeout.String()),
	)

	for time.Now().Before(deadline) {
		debug := elapsed%30 == 0 // Debug every 3 seconds

		if c.isWindowResponsive(hwnd, debug) {
			// Window is responsive, wait a bit more to ensure stability
			consecutiveResponses := 0
			for range 3 {
				time.Sleep(timeouts.StabilityCheckInterval)
				if c.isWindowResponsive(hwnd, false) {
					consecutiveResponses++
				}
			}

			if consecutiveResponses >= 2 {
				c.log.Debug("Window is stable and ready")
				return true
			}
		}

		time.Sleep(timeouts.StatePollingInterval)
		elapsed++
	}

	c.log.Debug("Timeout waiting for window to be ready")
	return false
}

// WaitForAppear waits for the VTPro main window to appear for a specific process
// targetPid must be a valid process ID - passing 0 will immediately return failure
func (c *Client) WaitForAppear(targetPid uint32, timeout time.Duration) (uintptr, bool) {
	deadline := time.Now().Add(timeout)
	seenWindows := make(map[uintptr]bool) // Track windows we've already logged
	loggedSplashOnly := false             // Track if we've logged "splash screen detected" message

	c.log.Debug("Searching for window", slog.Uint64("pid", uint64(targetPid)))

	for time.Now().Before(deadline) {
		// Check for the main VTPro window, passing seenWindows for tracking
		result := c.findWindowWithTracking(targetPid, true, seenWindows)

		if result.mainHwnd != 0 {
			return result.mainHwnd, true
		}

		// If we detected a splash screen but no main window yet, log it once
		if result.foundSplash && !loggedSplashOnly {
			c.log.Debug("Found splash screen, continuing to wait for main window")
			loggedSplashOnly = true
		}

		time.Sleep(timeouts.StatePollingInterval)
	}

	c.log.Debug("Timeout reached, performing final detailed check")
	result := c.findWindowWithTracking(targetPid, true, seenWindows)
	if result.mainHwnd != 0 {
		c.log.Debug("Found window at timeout", slog.String("title", result.mainTitle))
		return result.mainHwnd, true
	}

	return 0, false
}

// Cleanup ensures VTPro is properly closed, with fallback to force termination
func (c *Client) Cleanup(hwnd uintptr, pid uint32) {
	if hwnd == 0 {
		return
	}

	// Check if the window still exists before attempting cleanup
	if !windows.IsWindow(hwnd) {
		return
	}

	c.log.Debug("Cleaning up...")

	// Try to close gracefully
	c.win.Window.CloseWindow(hwnd, "VTPro")

	// Poll for up to 3 seconds to see if window closes
	maxWait := 3 * time.Second
	pollInterval := 200 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		if !windows.IsWindow(hwnd) {
			c.log.Debug("Window closed successfully")
			return
		}

		time.Sleep(pollInterval)
	}

	// Window still exists after waiting - force terminate
	c.log.Warn("VTPro did not close properly after waiting")
	if pid != 0 {
		c.log.Debug("Attempting to force terminate process", slog.Uint64("pid", uint64(pid)))
		_ = windows.TerminateProcess(pid)
	}
}

// ForceCleanup attempts to forcefully close VTPro using the known PID.
// It tries two approaches in order:
// 1. Use hwnd if available (graceful close with PID for force termination)
// 2. Use known PID (forced termination)
func (c *Client) ForceCleanup(hwnd uintptr, knownPid uint32) {
	// Strategy 1: Use hwnd if available for graceful close
	if hwnd != 0 {
		c.Cleanup(hwnd, knownPid)
		return
	}

	// Strategy 2: Use known PID for forced termination
	if knownPid != 0 {
		c.log.Debug("Force terminating with known PID", slog.Uint64("pid", uint64(knownPid)))
		_ = windows.TerminateProcess(knownPid)
		return
	}

	c.log.Warn("Unable to cleanup VTPro - no hwnd or PID provided")
}

// WaitForFileLoaded waits for VTPro to complete loading the file by monitoring
// the "VisionTools Pro-e" and "Progress [xx%]" dialogs that appear during file load.
// Returns true if file loading completed within timeout, false otherwise.
func (c *Client) WaitForFileLoaded(pid uint32, timeout time.Duration) bool {
	if pid == 0 {
		c.log.Warn("No PID provided for file load monitoring")
		return false
	}

	deadline := time.Now().Add(timeout)
	const (
		dialogFileLoading = "VisionTools Pro-e"
		dialogProgress    = "Progress"
	)

	// Track dialog states
	seenFileLoadingDialog := false
	seenProgressDialog := false
	lastDialogSeenTime := time.Time{}

	c.log.Info("Waiting for file to fully load...")
	c.log.Debug("Monitoring for file loading dialogs",
		slog.String("fileLoadingDialog", dialogFileLoading),
		slog.String("progressDialog", dialogProgress))

	for time.Now().Before(deadline) {
		select {
		case ev := <-windows.MonitorCh:
			// Check for file loading dialog
			if ev.Title == dialogFileLoading {
				if !seenFileLoadingDialog {
					c.log.Debug("Detected file loading dialog", slog.String("title", ev.Title))
					seenFileLoadingDialog = true
				}
				lastDialogSeenTime = time.Now()
			}

			// Check for progress dialog (can appear multiple times with % in title)
			if strings.Contains(ev.Title, dialogProgress) {
				if !seenProgressDialog {
					c.log.Debug("Detected progress dialog", slog.String("title", ev.Title))
					seenProgressDialog = true
				}
				lastDialogSeenTime = time.Now()
			}

		default:
			// No events in channel
			// If we've seen loading dialogs and haven't seen any for 2 seconds, they're likely closed
			if seenFileLoadingDialog && time.Since(lastDialogSeenTime) > 2*time.Second {
				c.log.Debug("File loading dialogs appear to have closed")

				// Wait a bit more to ensure stability
				time.Sleep(1 * time.Second)
				c.log.Info("File loading complete")
				return true
			}

			// Small sleep to avoid tight loop
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Timeout - if we never saw loading dialogs, file might be small/cached
	if !seenFileLoadingDialog {
		c.log.Debug("Timeout waiting for file loading dialogs - file may be small or cached")
		return true // Proceed anyway for small files
	}

	c.log.Warn("Timeout waiting for file loading to complete")
	return false
}

// StartMonitoring starts a background goroutine that monitors VTPro dialogs for a specific PID
// Returns a function to stop the monitoring
func (c *Client) StartMonitoring(pid uint32) func() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Init channel with larger buffer to handle bursts of events
		// (startup splash screens, progress dialogs, warnings, compiling dialog, etc.)
		windows.MonitorCh = make(chan windows.WindowEvent, 256)

		if pid == 0 {
			c.log.Warn("Window monitor started with PID=0, monitoring all processes (not recommended)")
			c.win.Monitor.StartWindowMonitor(ctx, 0, timeouts.MonitorPollingInterval)
		} else {
			c.log.Debug("Window monitor targeting SIMPL PID", slog.Uint64("pid", uint64(pid)))
			c.win.Monitor.StartWindowMonitor(ctx, pid, timeouts.MonitorPollingInterval)
		}

		// Wait for cancellation
		<-ctx.Done()
	}()

	return func() {
		cancel()
	}
}

// HandlePostLoadDialogs checks for and dismisses warning dialogs that may appear after file load
// This includes the "VisionTools(R) Pro-e" warning dialog containing messages like path limitation warnings.
// This MUST be called BEFORE bringing the window to foreground to ensure dialogs don't interfere.
func (c *Client) HandlePostLoadDialogs() error {
	const dialogVTProWarning = "VisionTools(R) Pro-e"

	// Longer timeout to catch warning dialogs that may appear after file load
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()

	dialogCount := 0
	maxDialogs := 5 // Prevent infinite loop

	for dialogCount < maxDialogs {
		select {
		case ev := <-windows.MonitorCh:
			c.log.Debug("Received post-load dialog event",
				slog.String("title", ev.Title),
				slog.Uint64("hwnd", uint64(ev.Hwnd)))

			dialogCount++

			// Enumerate and log all child controls for this dialog (for debugging)
			c.enumerateDialogControls(ev.Hwnd, ev.Title)

			// Handle warning dialogs that may appear after file load
			if ev.Title == dialogVTProWarning {
				c.log.Debug("Detected VTPro warning dialog - closing")
				c.log.Info("Handling post-load warning dialog")
				c.win.Window.CloseWindow(ev.Hwnd, dialogVTProWarning)

				// Give time for dialog to close
				time.Sleep(500 * time.Millisecond)
			} else {
				// Log but don't handle other dialogs here
				c.log.Trace("Ignoring post-load dialog", slog.String("title", ev.Title))
			}

		case <-timeout.C:
			// Timeout is fine - no blocking dialogs present
			c.log.Debug("Post-load dialog check complete", slog.Int("dialogsHandled", dialogCount))
			return nil
		}
	}

	c.log.Warn("Stopped checking for post-load dialogs after handling maximum count",
		slog.Int("maxDialogs", maxDialogs))
	return nil
}

// enumerateDialogControls enumerates and logs all child controls in a dialog window
func (c *Client) enumerateDialogControls(hwnd uintptr, title string) {
	c.log.Trace("Enumerating dialog controls",
		slog.String("title", title),
		slog.Uint64("hwnd", uint64(hwnd)))

	// Get the main window text (dialog body text, if any)
	windowText := windows.GetWindowText(hwnd)
	if windowText != "" {
		c.log.Trace("Dialog window text",
			slog.String("title", title),
			slog.String("text", windowText))
	}

	// Collect all child controls
	childInfos := windows.CollectChildInfos(hwnd)

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

// isWindowResponsive checks if a window is responding to messages
func (c *Client) isWindowResponsive(hwnd uintptr, debug bool) bool {
	var result uintptr

	// Send WM_NULL message with 1 second timeout
	ret, _, _ := windows.ProcSendMessageTimeoutW.Call(
		hwnd,
		windows.WM_NULL,
		0,
		0,
		windows.SMTO_ABORTIFHUNG,
		1000, // 1 second timeout in milliseconds
		uintptr(unsafe.Pointer(&result)),
	)

	responsive := ret != 0
	if debug {
		if responsive {
			c.log.Debug("Window is responsive")
		} else {
			c.log.Debug("Window is not responding")
		}
	}

	return responsive
}
