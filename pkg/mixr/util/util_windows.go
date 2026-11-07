package util

import (
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/lxn/win"
	"github.com/mitchellh/go-ps"
)

const getCurrentWindowInternalCooldown = time.Millisecond * 350

var (
	currentWindowMu            sync.Mutex
	lastGetCurrentWindowResult []string
	lastGetCurrentWindowCall   time.Time

	foregroundWindow      = win.GetForegroundWindow
	windowProcessID       = win.GetWindowThreadProcessId
	findWindowProcess     = ps.FindProcess
	enumerateChildWindows = win.EnumChildWindows

	// NewCallback retains its function forever. Register one non-capturing
	// callback for the process, never a closure on each foreground poll.
	enumChildWindowsCallback = syscall.NewCallback(collectChildWindowProcess)
	activeWindowEnumeration  *windowProcessEnumeration
)

type windowProcessEnumeration struct {
	ownerPID uint32
	names    []string
}

// EnumChildWindows is synchronous. currentWindowMu serializes enumerations and
// keeps this context alive until every callback has returned. LPARAM carries no
// Go pointer, and HWND/LPARAM have the native pointer width on both architectures.
func collectChildWindowProcess(hwnd, lParam uintptr) uintptr {
	state := activeWindowEnumeration
	if state == nil {
		return 1
	}
	var childPID uint32
	windowProcessID(win.HWND(hwnd), &childPID)
	if childPID != 0 && childPID != state.ownerPID {
		process, err := findWindowProcess(int(childPID))
		// A process can disappear between reading its PID and looking it up.
		if err == nil && process != nil {
			state.names = append(state.names, process.Executable())
		}
	}
	return 1
}

func getCurrentWindowProcessNames() ([]string, error) {
	currentWindowMu.Lock()
	defer currentWindowMu.Unlock()

	now := time.Now()
	if lastGetCurrentWindowCall.Add(getCurrentWindowInternalCooldown).After(now) {
		return append([]string(nil), lastGetCurrentWindowResult...), nil
	}
	lastGetCurrentWindowCall = now
	// Do not retain another application's result if the window/process vanished.
	lastGetCurrentWindowResult = nil

	hwnd := foregroundWindow()
	var ownerPID uint32
	windowProcessID(hwnd, &ownerPID)
	if ownerPID == 0 {
		return nil, nil
	}
	process, err := findWindowProcess(int(ownerPID))
	if err != nil {
		return nil, fmt.Errorf("get parent process for pid %d: %w", ownerPID, err)
	}
	if process == nil {
		return nil, nil
	}

	// Include descendants for container/UWP windows whose audio process differs
	// from the process owning the foreground window.
	state := &windowProcessEnumeration{ownerPID: ownerPID, names: []string{process.Executable()}}
	activeWindowEnumeration = state
	defer func() { activeWindowEnumeration = nil }()
	enumerateChildWindows(hwnd, enumChildWindowsCallback, 0)
	lastGetCurrentWindowResult = state.names
	return append([]string(nil), state.names...), nil
}
