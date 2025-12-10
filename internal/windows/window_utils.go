//go:build windows

package windows

import (
	"fmt"
	"log/slog"
	"os"
	"syscall"
	"unsafe"

	"github.com/Norgate-AV/vtpc/internal/logger"
)

// ShellExecute executes a file using the Windows shell
func ShellExecute(hwnd uintptr, verb, file, args, cwd string, showCmd int) error {
	var verbPtr, filePtr, argsPtr, cwdPtr *uint16
	var err error

	if verb != "" {
		verbPtr, err = syscall.UTF16PtrFromString(verb)
		if err != nil {
			return err
		}
	}

	filePtr, err = syscall.UTF16PtrFromString(file)
	if err != nil {
		return err
	}

	if args != "" {
		argsPtr, err = syscall.UTF16PtrFromString(args)
		if err != nil {
			return err
		}
	}

	if cwd != "" {
		cwdPtr, err = syscall.UTF16PtrFromString(cwd)
		if err != nil {
			return err
		}
	}

	ret, _, _ := procShellExecute.Call(
		hwnd,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(argsPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(showCmd),
	)

	// ShellExecute returns a value > 32 on success
	if ret <= 32 {
		return fmt.Errorf("shell execute failed with error code: %d", ret)
	}

	return nil
}

// ShellExecuteEx executes a file using the Windows shell and returns the process ID
// This is more reliable than ShellExecute when you need to track the launched process
// func ShellExecuteEx(hwnd uintptr, verb, file, args, cwd string, showCmd int, log logger.LoggerInterface) (uint32, error) {
// 	const SEE_MASK_NOCLOSEPROCESS = 0x00000040

// 	var verbPtr, filePtr, argsPtr, cwdPtr *uint16
// 	var err error

// 	if verb != "" {
// 		verbPtr, err = syscall.UTF16PtrFromString(verb)
// 		if err != nil {
// 			return 0, err
// 		}
// 	}

// 	filePtr, err = syscall.UTF16PtrFromString(file)
// 	if err != nil {
// 		return 0, err
// 	}

// 	if args != "" {
// 		argsPtr, err = syscall.UTF16PtrFromString(args)
// 		if err != nil {
// 			return 0, err
// 		}
// 	}

// 	if cwd != "" {
// 		cwdPtr, err = syscall.UTF16PtrFromString(cwd)
// 		if err != nil {
// 			return 0, err
// 		}
// 	}

// 	// Initialize SHELLEXECUTEINFO structure
// 	sei := SHELLEXECUTEINFO{
// 		CbSize:       uint32(unsafe.Sizeof(SHELLEXECUTEINFO{})),
// 		FMask:        SEE_MASK_NOCLOSEPROCESS,
// 		Hwnd:         hwnd,
// 		LpVerb:       verbPtr,
// 		LpFile:       filePtr,
// 		LpParameters: argsPtr,
// 		LpDirectory:  cwdPtr,
// 		NShow:        int32(showCmd),
// 	}

// 	// Call ShellExecuteExW
// 	ret, _, _ := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&sei)))
// 	if ret == 0 {
// 		return 0, fmt.Errorf("shell execute ex failed")
// 	}

// 	// Get process ID from the process handle
// 	if sei.HProcess == 0 {
// 		return 0, fmt.Errorf("shell execute ex did not return a process handle")
// 	}

// 	pid, _, _ := procGetProcessId.Call(sei.HProcess)
// 	if pid == 0 {
// 		// Clean up the process handle before returning error
// 		if ret, _, err := ProcCloseHandle.Call(sei.HProcess); ret == 0 {
// 			log.Debug("Failed to close process handle in error path", slog.Any("error", err))
// 		}

// 		return 0, fmt.Errorf("failed to get process ID from handle")
// 	}

// 	// Close the process handle - we only need the PID
// 	if ret, _, err := ProcCloseHandle.Call(sei.HProcess); ret == 0 {
// 		log.Debug("Failed to close process handle after getting PID", slog.Any("error", err))
// 	}

// 	return uint32(pid), nil
// }

// CreateProcessSimple launches an executable with arguments and returns the process ID
// This provides direct control over the command line, unlike ShellExecuteEx which
// may modify arguments based on shell integration and file associations.
func CreateProcessSimple(exePath, args string, showCmd int, log logger.LoggerInterface) (uint32, error) {
	// Validate that the executable exists before attempting to launch
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return 0, fmt.Errorf("executable not found: %s", exePath)
	} else if err != nil {
		return 0, fmt.Errorf("error checking executable at %s: %w", exePath, err)
	}

	// Build the command line: "executable" "arguments"
	// Windows CreateProcess requires the full command line including the executable
	var cmdLine string
	if args != "" {
		cmdLine = fmt.Sprintf("%q %s", exePath, args)
	} else {
		cmdLine = fmt.Sprintf("%q", exePath)
	}

	cmdLinePtr, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return 0, fmt.Errorf("failed to convert command line to UTF16: %w", err)
	}

	// Initialize STARTUPINFO
	si := STARTUPINFO{
		Cb:          uint32(unsafe.Sizeof(STARTUPINFO{})),
		DwFlags:     STARTF_USESHOWWINDOW,
		WShowWindow: uint16(showCmd),
	}

	// Initialize PROCESS_INFORMATION
	var pi PROCESS_INFORMATION

	// Call CreateProcessW
	// lpApplicationName = nil (use command line instead)
	// lpCommandLine = full command line
	ret, _, err := procCreateProcessW.Call(
		0,                                   // lpApplicationName (nil)
		uintptr(unsafe.Pointer(cmdLinePtr)), // lpCommandLine
		0,                                   // lpProcessAttributes (nil)
		0,                                   // lpThreadAttributes (nil)
		0,                                   // bInheritHandles (FALSE)
		0,                                   // dwCreationFlags
		0,                                   // lpEnvironment (nil)
		0,                                   // lpCurrentDirectory (nil)
		uintptr(unsafe.Pointer(&si)),        // lpStartupInfo
		uintptr(unsafe.Pointer(&pi)),        // lpProcessInformation
	)

	if ret == 0 {
		return 0, fmt.Errorf("CreateProcessW failed: %w", err)
	}

	// Close handles - we only need the PID
	if pi.HThread != 0 {
		if ret, _, err := ProcCloseHandle.Call(pi.HThread); ret == 0 {
			log.Debug("Failed to close thread handle", slog.Any("error", err))
		}
	}

	if pi.HProcess != 0 {
		if ret, _, err := ProcCloseHandle.Call(pi.HProcess); ret == 0 {
			log.Debug("Failed to close process handle", slog.Any("error", err))
		}
	}

	return pi.DwProcessId, nil
}

// GetWindowText retrieves the text of a window
func GetWindowText(hwnd uintptr) string {
	buf := make([]uint16, 256)

	ret, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}

	return syscall.UTF16ToString(buf)
}

// GetClassName retrieves the class name of a window
func GetClassName(hwnd uintptr) string {
	buf := make([]uint16, 256)

	ret, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}

	return syscall.UTF16ToString(buf)
}

// IsWindow checks if a window handle is valid
func IsWindow(hwnd uintptr) bool {
	ret, _, _ := procIsWindow.Call(hwnd)
	return ret != 0
}

// IsWindowVisible checks if a window is visible
func IsWindowVisible(hwnd uintptr) bool {
	ret, _, _ := procIsWindowVisible.Call(hwnd)
	return ret != 0
}

// GetWindowPid retrieves the process ID of a window
func GetWindowPid(hwnd uintptr) uint32 {
	var pid uint32

	ret, _, _ := procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if ret == 0 {
		return 0
	}

	return pid
}

// TerminateProcess forcefully terminates a process by its PID
func TerminateProcess(pid uint32) error {
	const PROCESS_TERMINATE = 0x0001

	// Open the process with terminate rights
	hProcess, _, err := procOpenProcess.Call(
		uintptr(PROCESS_TERMINATE),
		uintptr(0),
		uintptr(pid),
	)

	if hProcess == 0 {
		return fmt.Errorf("failed to open process: %w", err)
	}

	defer func() {
		if ret, _, err := ProcCloseHandle.Call(hProcess); ret == 0 {
			// Handle leak - log for diagnostics
			_ = err // CloseHandle failed
		}
	}()

	// Terminate the process
	ret, _, err := procTerminateProcess.Call(hProcess, uintptr(1))
	if ret == 0 {
		return fmt.Errorf("failed to terminate process: %w", err)
	}

	return nil
}
