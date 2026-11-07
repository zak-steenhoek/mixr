package util

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxn/win"
	ps "github.com/mitchellh/go-ps"
)

// Go exposes no supported callback-allocation counter. Pin the registration
// structure as well as checking the pointer used by actual enumerations below.
// Counting selector references also catches saving NewCallback into an alias.
func TestForegroundCallbackRegistrationStructure(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := packages["util"].Files["util_windows.go"]
	if file == nil {
		t.Fatal("foreground resolver source not found")
	}
	var registration *ast.CallExpr
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			if len(value.Names) == 1 && value.Names[0].Name == "enumChildWindowsCallback" && len(value.Values) == 1 {
				registration, _ = value.Values[0].(*ast.CallExpr)
			}
		}
	}
	if registration == nil || len(registration.Args) != 1 {
		t.Fatal("register enumChildWindowsCallback once in a package variable initializer")
	}
	allocator, ok := registration.Fun.(*ast.SelectorExpr)
	if !ok || allocator.Sel.Name != "NewCallback" {
		t.Fatal("expected one NewCallback registration")
	}
	callback, ok := registration.Args[0].(*ast.Ident)
	if !ok || callback.Name != "collectChildWindowProcess" {
		t.Fatal("registration must use the shared non-capturing collectChildWindowProcess function")
	}
	for _, source := range packages["util"].Files {
		ast.Inspect(source, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && (selector.Sel.Name == "NewCallback" || selector.Sel.Name == "NewCallbackCDecl") && selector != allocator {
				t.Errorf("%s: callback allocation/reference outside the single package initializer", fset.Position(selector.Pos()))
			}
			return true
		})
	}
}

func TestForegroundCallbackReuseAcrossTransitions(t *testing.T) {
	fakeForeground(t)
	registered := enumChildWindowsCallback
	if registered == 0 {
		t.Fatal("callback is not registered")
	}
	invoke := enumerateChildWindows
	enumerations := 0
	enumerateChildWindows = func(hwnd win.HWND, callback, param uintptr) bool {
		enumerations++ // serialized by currentWindowMu, including concurrent callers
		if callback != registered || enumChildWindowsCallback != registered {
			t.Errorf("enumeration uses callback %#x, shared %#x, originally %#x", callback, enumChildWindowsCallback, registered)
		}
		return invoke(hwnd, callback, param)
	}
	invalidate := func() {
		currentWindowMu.Lock()
		lastGetCurrentWindowCall = time.Time{}
		currentWindowMu.Unlock()
	}
	lookup := func(want []string, wantError bool) {
		t.Helper()
		names, err := getCurrentWindowProcessNames()
		if (err != nil) != wantError || !reflect.DeepEqual(names, want) {
			t.Errorf("lookup = %q, %v; want %q, error=%v", names, err, want, wantError)
		}
	}
	initial := []string{"process-10.exe", "process-20.exe"}
	lookup(initial, false)
	// Force a cache hit without depending on elapsed wall-clock time.
	lastGetCurrentWindowCall = time.Now().Add(time.Hour)
	lookup(initial, false)
	if enumerations != 1 {
		t.Fatalf("cached lookup enumerated: %d calls", enumerations)
	}
	foregroundWindow = func() win.HWND { return 40 }
	invalidate()
	lookup([]string{"process-40.exe", "process-10.exe", "process-20.exe"}, false)
	find := findWindowProcess
	for _, vanished := range []struct {
		name string
		find func(int) (ps.Process, error)
	}{
		{"missing", func(int) (ps.Process, error) { return nil, nil }},
		{"exited", func(int) (ps.Process, error) { return nil, errors.New("process exited") }},
	} {
		t.Run(vanished.name, func(t *testing.T) {
			findWindowProcess = vanished.find
			invalidate()
			lookup(nil, vanished.name == "exited")
			lastGetCurrentWindowCall = time.Now().Add(time.Hour)
			lookup(nil, false)
		})
	}
	if enumerations != 2 {
		t.Fatalf("missing parent enumerated: %d calls", enumerations)
	}
	findWindowProcess = find
	foregroundWindow = func() win.HWND { return 10 }
	// Each worker invalidates before calling. Individual calls may coalesce,
	// but each round must enumerate at least once, regardless of scheduling.
	for round := 0; round < 4; round++ {
		before := enumerations
		var workers sync.WaitGroup
		for worker := 0; worker < 8; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				invalidate()
				lookup(initial, false) // includes a child that disappears
			}()
		}
		workers.Wait()
		if enumerations <= before {
			t.Fatal("concurrent round did not exercise an uncached enumeration")
		}
	}
	if activeWindowEnumeration != nil {
		t.Fatal("enumeration context retained after lookup")
	}
}
