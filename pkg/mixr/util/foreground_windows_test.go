package util

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/lxn/win"
	ps "github.com/mitchellh/go-ps"
)

// Opt-in read-only Windows smoke test; normal CI uses the deterministic tests.
func TestForegroundLiveCallbackBudget(t *testing.T) {
	if os.Getenv("MIXR_TEST_LIVE_FOREGROUND") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	liveLookups := 0
	for i := 0; i < 2500; i++ {
		currentWindowMu.Lock()
		lastGetCurrentWindowCall = time.Time{}
		currentWindowMu.Unlock()
		names, err := getCurrentWindowProcessNames()
		if err == nil && len(names) > 0 {
			liveLookups++
		}
	}
	if liveLookups < 2000 {
		t.Fatalf("only %d live lookups exercised the callback path", liveLookups)
	}
	t.Logf("completed %d real foreground lookups without callback exhaustion", liveLookups)
}

type foregroundTestProcess string

func (p foregroundTestProcess) Pid() int           { return 10 }
func (p foregroundTestProcess) PPid() int          { return 0 }
func (p foregroundTestProcess) Executable() string { return string(p) }

func fakeForeground(t *testing.T) {
	t.Helper()
	oldForeground, oldPID, oldFind, oldEnum := foregroundWindow, windowProcessID, findWindowProcess, enumerateChildWindows
	t.Cleanup(func() {
		foregroundWindow, windowProcessID, findWindowProcess, enumerateChildWindows = oldForeground, oldPID, oldFind, oldEnum
		lastGetCurrentWindowCall, lastGetCurrentWindowResult = time.Time{}, nil
	})
	lastGetCurrentWindowCall, lastGetCurrentWindowResult = time.Time{}, nil
	foregroundWindow = func() win.HWND { return 10 }
	windowProcessID = func(hwnd win.HWND, pid *uint32) uint32 { *pid = uint32(hwnd); return 1 }
	findWindowProcess = func(pid int) (ps.Process, error) {
		if pid == 30 {
			return nil, nil
		} // child exited during enumeration
		return foregroundTestProcess(fmt.Sprintf("process-%d.exe", pid)), nil
	}
	enumerateChildWindows = func(hwnd win.HWND, callback, param uintptr) bool {
		// Invoke the registered callback through its native ABI on 386 and amd64.
		for _, child := range []uintptr{10, 20, 30} {
			result, _, _ := syscall.SyscallN(callback, child, param)
			if result != 1 {
				t.Errorf("callback stopped enumeration: %d", result)
			}
		}
		return true
	}
}

func TestForegroundLookupCallbackBudget(t *testing.T) {
	fakeForeground(t)
	for i := 0; i < 10000; i++ {
		lastGetCurrentWindowCall = time.Time{}
		names, err := getCurrentWindowProcessNames()
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"process-10.exe", "process-20.exe"}; !reflect.DeepEqual(names, want) {
			t.Fatalf("lookup %d = %q, want %q", i, names, want)
		}
	}
}

func TestForegroundProcessDisappeared(t *testing.T) {
	fakeForeground(t)
	names, err := getCurrentWindowProcessNames()
	if err != nil || len(names) != 2 {
		t.Fatalf("initial lookup = %q, %v", names, err)
	}
	lastGetCurrentWindowCall = time.Time{}
	findWindowProcess = func(int) (ps.Process, error) { return nil, nil }
	names, err = getCurrentWindowProcessNames()
	if err != nil || len(names) != 0 {
		t.Fatalf("exited parent lookup = %q, %v", names, err)
	}
	names, err = getCurrentWindowProcessNames()
	if err != nil || len(names) != 0 {
		t.Fatalf("cached exited parent = %q, %v", names, err)
	}
}

func TestForegroundConcurrentCacheIsolation(t *testing.T) {
	fakeForeground(t)
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				names, err := getCurrentWindowProcessNames()
				if err != nil || len(names) != 2 || names[0] != "process-10.exe" {
					t.Errorf("lookup = %q, %v", names, err)
					return
				}
				names[0] = "caller-mutated.exe"
			}
		}()
	}
	workers.Wait()
}
