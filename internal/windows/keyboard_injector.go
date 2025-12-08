//go:build windows

package windows

import (
	"log/slog"
	"time"
	"unsafe"

	"github.com/Norgate-AV/vtpc/internal/logger"
	"github.com/Norgate-AV/vtpc/internal/timeouts"
)

// keyboardInjector implements the KeyboardInjector interface
type keyboardInjector struct {
	log logger.LoggerInterface
}

// newKeyboardInjector creates a new keyboard injector
func newKeyboardInjector(log logger.LoggerInterface) *keyboardInjector {
	return &keyboardInjector{log: log}
}

// SendF12 sends the F12 key
func (k *keyboardInjector) SendF12() {
	// VK_F12 = 0x7B
	vkCode := uintptr(0x7B)

	// keybd_event(vk, scan, flags, extraInfo)
	// Note: keybd_event has void return type, no error checking needed
	k.log.Debug("Sending F12 KEYDOWN")
	_, _, _ = procKeybd_event.Call(vkCode, 0, 0x1, 0) // KEYEVENTF_EXTENDEDKEY

	time.Sleep(timeouts.KeystrokeDelay)

	k.log.Debug("Sending F12 KEYUP")
	_, _, _ = procKeybd_event.Call(vkCode, 0, 0x1|0x2, 0) // KEYEVENTF_EXTENDEDKEY | KEYEVENTF_KEYUP
}

// SendEnter sends the Enter key
func (k *keyboardInjector) SendEnter() {
	// VK_RETURN = 0x0D
	vkCode := uintptr(0x0D)

	// Note: keybd_event has void return type, no error checking needed
	k.log.Debug("Sending Enter KEYDOWN")
	_, _, _ = procKeybd_event.Call(vkCode, 0, 0x1, 0)
	time.Sleep(timeouts.KeystrokeDelay)

	k.log.Debug("Sending Enter KEYUP")
	_, _, _ = procKeybd_event.Call(vkCode, 0, 0x1|0x2, 0)
}

// SendF12ToWindow sends F12 key directly to a specific window using SendMessage
func (k *keyboardInjector) SendF12ToWindow(hwnd uintptr) bool {
	k.log.Debug("Sending F12 to window via PostMessage", slog.Uint64("hwnd", uint64(hwnd)))

	// lParam construction for F12:
	// Bits 0-15: Repeat count (1)
	// Bits 16-23: Scan code for F12 (0x58)
	// Bit 24: Extended key flag (1 for F12)
	// Bits 25-28: Reserved (0)
	// Bit 29: Context code (0 for non-Alt)
	// Bit 30: Previous key state (0 for key down)
	// Bit 31: Transition state (0 for key down, 1 for key up)
	const scanCodeF12 = 0x58
	lParamDown := uintptr(1 | (scanCodeF12 << 16) | (1 << 24))                       // Extended key flag set
	lParamUp := uintptr(1 | (scanCodeF12 << 16) | (1 << 24) | (1 << 30) | (1 << 31)) // Previous state + transition

	// Try SendMessage first (synchronous)
	k.log.Debug("Trying SendMessage for F12")
	ret, _, _ := procSendMessageW.Call(hwnd, WM_KEYDOWN, VK_F12, lParamDown)
	k.log.Debug("SendMessage WM_KEYDOWN returned", slog.Uint64("ret", uint64(ret)))
	time.Sleep(timeouts.KeystrokeDelay)

	ret, _, _ = procSendMessageW.Call(hwnd, WM_KEYUP, VK_F12, lParamUp)
	k.log.Debug("SendMessage WM_KEYUP returned", slog.Uint64("ret", uint64(ret)))

	k.log.Debug("F12 sent via SendMessage (synchronous)")
	return true
}

// SendF12WithSendInput sends F12 key using SendInput API (more modern than keybd_event)
func (k *keyboardInjector) SendF12WithSendInput() bool {
	k.log.Debug("Sending F12 via SendInput")

	// Create INPUT structure for keydown
	inputs := make([]INPUT, 2)

	// F12 KEYDOWN
	inputs[0].Type = INPUT_KEYBOARD
	kb := (*KEYBDINPUT)(unsafe.Pointer(&inputs[0].Data[0]))
	kb.WVk = VK_F12
	kb.DwFlags = KEYEVENTF_EXTENDEDKEY

	// F12 KEYUP
	inputs[1].Type = INPUT_KEYBOARD
	kb2 := (*KEYBDINPUT)(unsafe.Pointer(&inputs[1].Data[0]))
	kb2.WVk = VK_F12
	kb2.DwFlags = KEYEVENTF_EXTENDEDKEY | KEYEVENTF_KEYUP

	// Send the input
	ret, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		uintptr(unsafe.Sizeof(INPUT{})),
	)

	if ret != uintptr(len(inputs)) {
		k.log.Warn("SendInput failed", slog.Uint64("expected", uint64(len(inputs))), slog.Uint64("sent", uint64(ret)))
		return false
	}

	k.log.Debug("F12 sent via SendInput successfully")
	return true
}
