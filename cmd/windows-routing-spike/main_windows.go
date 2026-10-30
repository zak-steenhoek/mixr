//go:build windows
// +build windows

// Command windows-routing-spike probes Windows playback routing without using
// or modifying mixr's production audio-session code.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	ps "github.com/mitchellh/go-ps"
	wca "github.com/moutend/go-wca"
	"github.com/zak-steenhoek/mixr/internal/wincom"
)

const (
	deviceStateActive     = 0x1
	deviceStateDisabled   = 0x2
	deviceStateNotPresent = 0x4
	deviceStateUnplugged  = 0x8
	deviceStateAll        = 0xf
	eRoleConsole          = 0
	eRoleMultimedia       = 1
	eRoleCommunications   = 2
	eRender               = 0
	stgmRead              = 0
	clsctxAll             = 23
	roInitMultithreaded   = 1
	audioRenderClassID    = "{e6327cad-dcec-4949-ae8a-991e976a79d2}"
)

var (
	// Undocumented PolicyConfig COM contract used by current Windows Sound UI.
	// The older Vista class/interface pair returned S_OK but made no change on
	// Windows build 26200, so every mutation is also verified through MMDevice.
	clsidPolicyConfigClient = ole.NewGUID("{870AF99C-171D-4F9E-AF0D-E63DF40C2BC9}")
	iidPolicyConfig         = ole.NewGUID("{F8679F50-850A-41CF-9C72-430F290290C8}")

	// Undocumented WinRT policy factory. This is intentionally isolated here.
	iidAudioPolicyConfigFactory = ole.NewGUID("{AB3D4648-E242-459F-B02F-541C70306324}")
	combase                     = syscall.NewLazyDLL("combase.dll")
	procRoInitialize            = combase.NewProc("RoInitialize")
	procRoUninitialize          = combase.NewProc("RoUninitialize")
	procRoGetActivationFactory  = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString     = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString     = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuf  = combase.NewProc("WindowsGetStringRawBuffer")
)

type iAudioPolicyConfigFactory struct{ rawVTable *audioPolicyConfigFactoryVtbl }
type audioPolicyConfigFactoryVtbl struct {
	queryInterface, addRef, release             uintptr
	getIids, getRuntimeClassName, getTrustLevel uintptr
	// This internal interface has 19 methods before the persisted-endpoint
	// methods. Keeping the placeholders is essential to its version-sensitive ABI.
	incomplete                                   [19]uintptr
	setPersistedDefaultAudioEndpoint             uintptr
	getPersistedDefaultAudioEndpoint             uintptr
	clearAllPersistedApplicationDefaultEndpoints uintptr
}

type iPolicyConfig struct{ rawVTable *policyConfigVtbl }
type policyConfigVtbl struct {
	queryInterface, addRef, release                                uintptr
	getMixFormat, getDeviceFormat, resetDeviceFormat               uintptr
	setDeviceFormat, getProcessingPeriod, setProcessingPeriod      uintptr
	getShareMode, setShareMode, getPropertyValue, setPropertyValue uintptr
	setDefaultEndpoint, setEndpointVisibility                      uintptr
}

type endpoint struct {
	id, name  string
	state     uint32
	isDefault [3]bool
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}

	var err error
	switch flag.Arg(0) {
	case "list":
		err = listEndpoints()
	case "default":
		err = printDefaults()
	case "sessions":
		err = listSessions()
	case "set-default":
		err = commandSetDefault(flag.Args()[1:])
	case "get-app":
		err = commandGetApp(flag.Args()[1:])
	case "set-app":
		err = commandSetApp(flag.Args()[1:])
	case "clear-apps":
		err = clearApps()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Windows routing feasibility spike

Public Core Audio probes:
  windows-routing-spike list
  windows-routing-spike default
  windows-routing-spike sessions

EXPERIMENTAL, undocumented/version-sensitive policy calls:
  windows-routing-spike set-default <endpoint-id>
  windows-routing-spike get-app <pid>
  windows-routing-spike set-app <pid> <endpoint-id|default>
  windows-routing-spike clear-apps

set-default applies console, multimedia, and communications roles. The per-app
commands require a running process; Windows derives the persisted app identity
from that PID. Run without elevation first, as preferences are per user.`)
}

func withCOM(fn func(*wca.IMMDeviceEnumerator) error) error {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		var oe *ole.OleError
		if !errors.As(err, &oe) || oe.Code() != 1 {
			return err
		}
	}
	defer ole.CoUninitialize()
	var e *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, clsctxAll, wca.IID_IMMDeviceEnumerator, &e); err != nil {
		return err
	}
	defer e.Release()
	return fn(e)
}

func enumerate(e *wca.IMMDeviceEnumerator) ([]endpoint, error) {
	defaults := map[string][3]bool{}
	for role := uint32(0); role < 3; role++ {
		var d *wca.IMMDevice
		if err := e.GetDefaultAudioEndpoint(eRender, role, &d); err == nil {
			if id, idErr := wincom.DeviceID(d); idErr == nil {
				x := defaults[id]
				x[role] = true
				defaults[id] = x
			}
			d.Release()
		}
	}
	var c *wca.IMMDeviceCollection
	if err := e.EnumAudioEndpoints(eRender, deviceStateAll, &c); err != nil {
		return nil, err
	}
	defer c.Release()
	var n uint32
	if err := c.GetCount(&n); err != nil {
		return nil, err
	}
	out := make([]endpoint, 0, n)
	for i := uint32(0); i < n; i++ {
		var d *wca.IMMDevice
		if err := c.Item(i, &d); err != nil {
			return nil, err
		}
		var state uint32
		id, err := wincom.DeviceID(d)
		if err == nil {
			err = d.GetState(&state)
		}
		name := "<unknown>"
		if err == nil {
			var ps *wca.IPropertyStore
			if psErr := d.OpenPropertyStore(stgmRead, &ps); psErr == nil {
				var pv wca.PROPVARIANT
				if ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv) == nil {
					name = pv.String()
				}
				ps.Release()
			}
		}
		d.Release()
		if err != nil {
			return nil, err
		}
		out = append(out, endpoint{id: id, name: name, state: state, isDefault: defaults[id]})
	}
	return out, nil
}

func listEndpoints() error {
	return withCOM(func(e *wca.IMMDeviceEnumerator) error {
		items, err := enumerate(e)
		if err != nil {
			return err
		}
		for _, x := range items {
			fmt.Printf("%s\n  id: %s\n  state: %s\n  default roles: %s\n", x.name, x.id, stateName(x.state), roleNames(x.isDefault))
		}
		return nil
	})
}

func printDefaults() error {
	return withCOM(func(e *wca.IMMDeviceEnumerator) error {
		for role, name := range []string{"console", "multimedia", "communications"} {
			var d *wca.IMMDevice
			if err := e.GetDefaultAudioEndpoint(eRender, uint32(role), &d); err != nil {
				return err
			}
			id, err := wincom.DeviceID(d)
			d.Release()
			if err != nil {
				return err
			}
			fmt.Printf("%s: %s\n", name, id)
		}
		return nil
	})
}

func listSessions() error {
	return withCOM(func(e *wca.IMMDeviceEnumerator) error {
		var c *wca.IMMDeviceCollection
		if err := e.EnumAudioEndpoints(eRender, deviceStateActive, &c); err != nil {
			return err
		}
		defer c.Release()
		var n uint32
		if err := c.GetCount(&n); err != nil {
			return err
		}
		for i := uint32(0); i < n; i++ {
			var d *wca.IMMDevice
			if err := c.Item(i, &d); err != nil {
				return err
			}
			if err := printEndpointSessions(d); err != nil {
				d.Release()
				return err
			}
			d.Release()
		}
		return nil
	})
}

func printEndpointSessions(d *wca.IMMDevice) error {
	id, err := wincom.DeviceID(d)
	if err != nil {
		return err
	}
	name := "<unknown>"
	var store *wca.IPropertyStore
	if err := d.OpenPropertyStore(stgmRead, &store); err == nil {
		var value wca.PROPVARIANT
		if store.GetValue(&wca.PKEY_Device_FriendlyName, &value) == nil {
			name = value.String()
		}
		store.Release()
	}

	var manager *wca.IAudioSessionManager2
	if err := wincom.Activate(d, wca.IID_IAudioSessionManager2, clsctxAll, nil, &manager); err != nil {
		return fmt.Errorf("activate session manager for %s: %w", name, err)
	}
	defer manager.Release()
	var sessions *wca.IAudioSessionEnumerator
	if err := manager.GetSessionEnumerator(&sessions); err != nil {
		return err
	}
	defer sessions.Release()
	count, err := wincom.AudioSessionCount(sessions)
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		var control *wca.IAudioSessionControl
		if err := sessions.GetSession(i, &control); err != nil {
			return err
		}
		dispatch, err := control.QueryInterface(wca.IID_IAudioSessionControl2)
		control.Release()
		if err != nil {
			return err
		}
		control2 := (*wca.IAudioSessionControl2)(unsafe.Pointer(dispatch))
		var pid uint32
		err = control2.GetProcessId(&pid)
		control2.Release()
		if err != nil || pid == 0 {
			continue
		}
		processName := "<exited>"
		if process, findErr := ps.FindProcess(int(pid)); findErr == nil && process != nil {
			processName = process.Executable()
		}
		fmt.Printf("endpoint: %s\n  endpoint-id: %s\n  pid: %d\n  process: %s\n", name, id, pid, processName)
	}
	return nil
}

func commandSetDefault(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: set-default <endpoint-id>")
	}
	fmt.Fprintln(os.Stderr, "WARNING: using undocumented IPolicyConfig; the command will verify every role by reading it back.")
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		var oe *ole.OleError
		if !errors.As(err, &oe) || oe.Code() != 1 {
			return err
		}
	}
	defer ole.CoUninitialize()
	var p *iPolicyConfig
	if err := wca.CoCreateInstance(clsidPolicyConfigClient, 0, clsctxAll, iidPolicyConfig, &p); err != nil {
		return err
	}
	defer release(unsafe.Pointer(p), p.rawVTable.release)
	id, err := syscall.UTF16PtrFromString(args[0])
	if err != nil {
		return err
	}
	for role := uintptr(0); role < 3; role++ {
		hr, _, _ := syscall.Syscall(p.rawVTable.setDefaultEndpoint, 3, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(id)), role)
		if int32(hr) < 0 {
			return fmt.Errorf("SetDefaultEndpoint role %d HRESULT 0x%08x", role, uint32(hr))
		}
	}
	return verifyDefaultRoles(args[0])
}

func verifyDefaultRoles(want string) error {
	return withCOM(func(e *wca.IMMDeviceEnumerator) error {
		for role := uint32(0); role < 3; role++ {
			var d *wca.IMMDevice
			if err := e.GetDefaultAudioEndpoint(eRender, role, &d); err != nil {
				return fmt.Errorf("verify default role %d: %w", role, err)
			}
			got, err := wincom.DeviceID(d)
			d.Release()
			if err != nil {
				return fmt.Errorf("verify default role %d ID: %w", role, err)
			}
			if got != want {
				return fmt.Errorf("SetDefaultEndpoint returned success but role %d remained %q", role, got)
			}
		}
		return nil
	})
}

func commandGetApp(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: get-app <pid>")
	}
	pid, err := parsePID(args[0])
	if err != nil {
		return err
	}
	return withPolicyFactory(func(p *iAudioPolicyConfigFactory) error {
		for role, name := range []string{"console", "multimedia", "communications"} {
			var h uintptr
			hr, _, _ := syscall.Syscall6(p.rawVTable.getPersistedDefaultAudioEndpoint, 5, uintptr(unsafe.Pointer(p)), uintptr(pid), eRender, uintptr(role), uintptr(unsafe.Pointer(&h)), 0)
			if int32(hr) < 0 {
				fmt.Printf("%s: <not set> (HRESULT 0x%08x)\n", name, uint32(hr))
				continue
			}
			fmt.Printf("%s: %s\n", name, hstringValue(h))
			deleteHString(h)
		}
		return nil
	})
}

func commandSetApp(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: set-app <pid> <endpoint-id|default>")
	}
	pid, err := parsePID(args[0])
	if err != nil {
		return err
	}
	value, err := appRoutingDeviceID(args[1])
	if err != nil {
		return err
	}
	var h uintptr
	if value != "" {
		h, err = newHString(value)
		if err != nil {
			return err
		}
		defer deleteHString(h)
	}
	fmt.Fprintln(os.Stderr, "WARNING: using undocumented Windows.Media.Internal.AudioPolicyConfig; runtime behavior is Windows-build dependent.")
	return withPolicyFactory(func(p *iAudioPolicyConfigFactory) error {
		for role := uintptr(eRoleConsole); role <= uintptr(eRoleCommunications); role++ {
			hr, _, _ := syscall.Syscall6(p.rawVTable.setPersistedDefaultAudioEndpoint, 5, uintptr(unsafe.Pointer(p)), uintptr(pid), eRender, role, h, 0)
			if int32(hr) < 0 {
				return fmt.Errorf("SetPersistedDefaultAudioEndpoint role %d HRESULT 0x%08x", role, uint32(hr))
			}
			var got uintptr
			hr, _, _ = syscall.Syscall6(p.rawVTable.getPersistedDefaultAudioEndpoint, 5, uintptr(unsafe.Pointer(p)), uintptr(pid), eRender, role, uintptr(unsafe.Pointer(&got)), 0)
			if int32(hr) < 0 {
				return fmt.Errorf("verify GetPersistedDefaultAudioEndpoint role %d HRESULT 0x%08x", role, uint32(hr))
			}
			gotValue := hstringValue(got)
			deleteHString(got)
			if gotValue != value {
				return fmt.Errorf("persisted role %d endpoint read-back %q, want %q", role, gotValue, value)
			}
		}
		return nil
	})
}

func appRoutingDeviceID(value string) (string, error) {
	if value == "default" {
		// The internal API expects a null HSTRING, not an allocated empty one.
		return "", nil
	}
	if strings.HasPrefix(value, `\\?\SWD#MMDEVAPI#`) {
		return value, nil
	}
	if strings.HasPrefix(value, "{0.0.0.") {
		// Windows 25H2 persists the render interface path rather than the shorter
		// IMMDevice endpoint ID returned by GetId.
		return `\\?\SWD#MMDEVAPI#` + value + "#" + audioRenderClassID, nil
	}
	return "", fmt.Errorf("unsupported app-routing device ID %q", value)
}

func clearApps() error {
	fmt.Fprintln(os.Stderr, "WARNING: clearing all per-application endpoint preferences for the current user.")
	return withPolicyFactory(func(p *iAudioPolicyConfigFactory) error {
		hr, _, _ := syscall.Syscall(p.rawVTable.clearAllPersistedApplicationDefaultEndpoints, 1, uintptr(unsafe.Pointer(p)), 0, 0)
		if int32(hr) < 0 {
			return fmt.Errorf("ClearAllPersistedApplicationDefaultEndpoints HRESULT 0x%08x", uint32(hr))
		}
		return nil
	})
}

func withPolicyFactory(fn func(*iAudioPolicyConfigFactory) error) error {
	hr, _, _ := procRoInitialize.Call(roInitMultithreaded)
	if int32(hr) < 0 {
		return fmt.Errorf("RoInitialize HRESULT 0x%08x", uint32(hr))
	}
	defer procRoUninitialize.Call()
	class, err := newHString("Windows.Media.Internal.AudioPolicyConfig")
	if err != nil {
		return err
	}
	defer deleteHString(class)
	var p *iAudioPolicyConfigFactory
	hr, _, _ = procRoGetActivationFactory.Call(class, uintptr(unsafe.Pointer(iidAudioPolicyConfigFactory)), uintptr(unsafe.Pointer(&p)))
	if int32(hr) < 0 {
		return fmt.Errorf("RoGetActivationFactory HRESULT 0x%08x (interface unavailable on this Windows build)", uint32(hr))
	}
	defer release(unsafe.Pointer(p), p.rawVTable.release)
	return fn(p)
}

func newHString(s string) (uintptr, error) {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h uintptr
	hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	if int32(hr) < 0 {
		return 0, fmt.Errorf("WindowsCreateString HRESULT 0x%08x", uint32(hr))
	}
	return h, nil
}
func deleteHString(h uintptr) {
	if h != 0 {
		procWindowsDeleteString.Call(h)
	}
}
func hstringValue(h uintptr) string {
	var n uint32
	p, _, _ := procWindowsGetStringRawBuf.Call(h, uintptr(unsafe.Pointer(&n)))
	if p == 0 {
		return ""
	}
	return syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(p))[:n:n])
}
func release(p unsafe.Pointer, fn uintptr) { syscall.Syscall(fn, 1, uintptr(p), 0, 0) }
func parsePID(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid PID %q", s)
	}
	return uint32(n), nil
}
func stateName(s uint32) string {
	switch s {
	case deviceStateActive:
		return "active"
	case deviceStateDisabled:
		return "disabled"
	case deviceStateNotPresent:
		return "not present"
	case deviceStateUnplugged:
		return "unplugged"
	default:
		return fmt.Sprintf("0x%x", s)
	}
}
func roleNames(r [3]bool) string {
	names := []string{}
	for i, ok := range r {
		if ok {
			names = append(names, []string{"console", "multimedia", "communications"}[i])
		}
	}
	if len(names) == 0 {
		return "none"
	}
	out := names[0]
	for _, n := range names[1:] {
		out += ", " + n
	}
	return out
}
