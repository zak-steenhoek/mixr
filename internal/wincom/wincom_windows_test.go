//go:build windows
// +build windows

package wincom

import (
	"math"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca"
)

var (
	fakeOutputMoved      bool
	fakeActivationDevice *wca.IMMDevice
	fakeActivationIID    *ole.GUID
	fakeActivationCtx    uintptr
	fakeActivationParams *wca.PROPVARIANT
	fakeActivationResult unsafe.Pointer
	fakeEndpointID       *uint16
	fakeSimpleVolume     float32
	fakeEndpointVolume   float32
	fakeMuteValue        uintptr
	fakeMuteContext      uintptr
	fakeUnregistered     uintptr
	fakeGrowStack        bool
	fakeStackMoved       bool
)

func TestActivateUsesNativeSignature(t *testing.T) {
	fakeActivationDevice = nil
	fakeActivationIID = nil
	fakeActivationCtx = 0
	fakeActivationParams = nil
	fakeActivationResult = unsafe.Pointer(&wca.IAudioSessionManager2{})

	vtable := &wca.IMMDeviceVtbl{}
	vtable.Activate = syscall.NewCallback(fakeActivate)
	device := fakeDevice(vtable)
	params := &wca.PROPVARIANT{}
	var result unsafe.Pointer

	if err := Activate(device, wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, params, &result); err != nil {
		t.Fatal(err)
	}
	if fakeActivationDevice != device {
		t.Fatalf("activation device = %p, want %p", fakeActivationDevice, device)
	}
	if fakeActivationIID != wca.IID_IAudioSessionManager2 {
		t.Fatalf("activation IID = %p, want %p", fakeActivationIID, wca.IID_IAudioSessionManager2)
	}
	if fakeActivationCtx != uintptr(wca.CLSCTX_ALL) {
		t.Fatalf("activation context = %#x, want %#x", fakeActivationCtx, uintptr(wca.CLSCTX_ALL))
	}
	if fakeActivationParams != params {
		t.Fatalf("activation parameters = %p, want %p", fakeActivationParams, params)
	}
	if result != fakeActivationResult {
		t.Fatalf("activation result = %p, want %p", result, fakeActivationResult)
	}
}

func TestDeviceIDUsesNativePointerWidth(t *testing.T) {
	text, err := syscall.UTF16FromString("{0.0.0.00000000}.{native-pointer-test}")
	if err != nil {
		t.Fatal(err)
	}

	ole32 := syscall.NewLazyDLL("ole32.dll")
	alloc := ole32.NewProc("CoTaskMemAlloc")
	ptr, _, callErr := alloc.Call(uintptr(len(text) * 2))
	if ptr == 0 {
		t.Fatalf("CoTaskMemAlloc failed: %v", callErr)
	}
	buffer := (*[1 << 20]uint16)(unsafe.Pointer(ptr))[:len(text):len(text)]
	copy(buffer, text)
	fakeEndpointID = (*uint16)(unsafe.Pointer(ptr))

	vtable := &wca.IMMDeviceVtbl{}
	vtable.GetId = syscall.NewCallback(fakeGetID)
	got, err := DeviceID(fakeDevice(vtable))
	if err != nil {
		t.Fatal(err)
	}
	want := syscall.UTF16ToString(text)
	if got != want {
		t.Fatalf("endpoint ID = %q, want %q", got, want)
	}
}

func TestAudioSessionCountUsesInt32Output(t *testing.T) {
	vtable := &wca.IAudioSessionEnumeratorVtbl{}
	vtable.GetCount = syscall.NewCallback(fakeGetCount)
	enumerator := &wca.IAudioSessionEnumerator{
		IUnknown: ole.IUnknown{RawVTable: (*interface{})(unsafe.Pointer(vtable))},
	}

	got, err := AudioSessionCount(enumerator)
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("session count = %d, want 7", got)
	}
}

func TestVolumeOperationsUseFloat32ABI(t *testing.T) {
	simpleVTable := &wca.ISimpleAudioVolumeVtbl{}
	simpleVTable.SetMasterVolume = syscall.NewCallback(fakeSetSimpleVolume)
	simpleVTable.GetMasterVolume = syscall.NewCallback(fakeGetSimpleVolume)
	simple := &wca.ISimpleAudioVolume{
		IUnknown: ole.IUnknown{RawVTable: (*interface{})(unsafe.Pointer(simpleVTable))},
	}

	if err := simple.SetMasterVolume(0.42, nil); err != nil {
		t.Fatal(err)
	}
	if fakeSimpleVolume != float32(0.42) {
		t.Fatalf("simple set volume = %f, want 0.42", fakeSimpleVolume)
	}
	simpleLevel, err := SimpleAudioVolumeLevel(simple)
	if err != nil {
		t.Fatal(err)
	}
	if simpleLevel != float32(0.37) {
		t.Fatalf("simple get volume = %f, want 0.37", simpleLevel)
	}

	endpointVTable := &wca.IAudioEndpointVolumeVtbl{}
	endpointVTable.SetMasterVolumeLevelScalar = syscall.NewCallback(fakeSetEndpointVolume)
	endpointVTable.GetMasterVolumeLevelScalar = syscall.NewCallback(fakeGetEndpointVolume)
	endpoint := &wca.IAudioEndpointVolume{
		IUnknown: ole.IUnknown{RawVTable: (*interface{})(unsafe.Pointer(endpointVTable))},
	}

	if err := SetEndpointVolumeLevelScalar(endpoint, 0.64, nil); err != nil {
		t.Fatal(err)
	}
	if fakeEndpointVolume != float32(0.64) {
		t.Fatalf("endpoint set volume = %f, want 0.64", fakeEndpointVolume)
	}
	endpointLevel, err := EndpointVolumeLevelScalar(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if endpointLevel != float32(0.73) {
		t.Fatalf("endpoint get volume = %f, want 0.73", endpointLevel)
	}
}

func TestMuteOperationsUseWin32BOOL(t *testing.T) {
	context := ole.NewGUID("{8b65d71d-3223-45bc-963a-118f613b5f89}")

	simpleVTable := &wca.ISimpleAudioVolumeVtbl{}
	simpleVTable.GetMute = syscall.NewCallback(fakeGetMute)
	simpleVTable.SetMute = syscall.NewCallback(fakeSetMute)
	simple := &wca.ISimpleAudioVolume{
		IUnknown: ole.IUnknown{RawVTable: (*interface{})(unsafe.Pointer(simpleVTable))},
	}

	muted, err := SimpleAudioVolumeMute(simple)
	if err != nil {
		t.Fatal(err)
	}
	if !muted {
		t.Fatal("simple mute = false, want true")
	}
	fakeMuteValue = 0
	fakeMuteContext = 0
	if err := SetSimpleAudioVolumeMute(simple, true, context); err != nil {
		t.Fatal(err)
	}
	assertNativeMuteCall(t, context)

	endpointVTable := &wca.IAudioEndpointVolumeVtbl{}
	endpointVTable.GetMute = syscall.NewCallback(fakeGetMute)
	endpointVTable.SetMute = syscall.NewCallback(fakeSetMute)
	endpoint := &wca.IAudioEndpointVolume{
		IUnknown: ole.IUnknown{RawVTable: (*interface{})(unsafe.Pointer(endpointVTable))},
	}

	muted, err = EndpointVolumeMute(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if !muted {
		t.Fatal("endpoint mute = false, want true")
	}
	fakeMuteValue = 1
	fakeMuteContext = 1
	if err := SetEndpointVolumeMute(endpoint, false, context); err != nil {
		t.Fatal(err)
	}
	if fakeMuteValue != 0 {
		t.Fatalf("endpoint BOOL = %#x, want 0", fakeMuteValue)
	}
	if fakeMuteContext != uintptr(unsafe.Pointer(context)) {
		t.Fatalf("endpoint context = %#x, want %#x", fakeMuteContext, uintptr(unsafe.Pointer(context)))
	}
}

func assertNativeMuteCall(t *testing.T, context *ole.GUID) {
	t.Helper()
	if fakeMuteValue != 1 {
		t.Fatalf("mute BOOL = %#x, want 1", fakeMuteValue)
	}
	if fakeMuteContext != uintptr(unsafe.Pointer(context)) {
		t.Fatalf("mute context = %#x, want %#x", fakeMuteContext, uintptr(unsafe.Pointer(context)))
	}
}

func TestUnregisterEndpointNotificationCallbackUsesVTableSlot(t *testing.T) {
	vtable := &wca.IMMDeviceEnumeratorVtbl{}
	vtable.UnregisterEndpointNotificationCallback = syscall.NewCallback(fakeUnregister)
	enumerator := &wca.IMMDeviceEnumerator{
		IUnknown: ole.IUnknown{RawVTable: (*interface{})(unsafe.Pointer(vtable))},
	}
	client := &wca.IMMNotificationClient{VTable: &wca.IMMNotificationClientVtbl{}}
	fakeUnregistered = 0

	if err := UnregisterEndpointNotificationCallback(enumerator, client); err != nil {
		t.Fatal(err)
	}
	if fakeUnregistered != uintptr(unsafe.Pointer(client)) {
		t.Fatalf("unregistered client = %#x, want %#x", fakeUnregistered, uintptr(unsafe.Pointer(client)))
	}
}

// Native pointer arguments stay pointers across the callback boundary. Receiving
// them as uintptr and converting back loses the provenance required by checkptr.
// These pointer-sized arguments preserve the stdcall/fastcall ABI on 386/amd64.
func fakeActivate(device *wca.IMMDevice, iid *ole.GUID, ctx uintptr, params *wca.PROPVARIANT, out *unsafe.Pointer) uintptr {
	growCallbackStack(unsafe.Pointer(out))
	fakeActivationDevice = device
	fakeActivationIID = iid
	fakeActivationCtx = ctx
	fakeActivationParams = params
	*out = fakeActivationResult
	return 0
}

func fakeGetID(_ uintptr, out **uint16) uintptr {
	growCallbackStack(unsafe.Pointer(out))
	*out = fakeEndpointID
	return 0
}

func fakeGetCount(_ uintptr, out *int32) uintptr {
	growCallbackStack(unsafe.Pointer(out))
	*out = 7
	return 0
}

func fakeSetSimpleVolume(_, level, _ uintptr) uintptr {
	fakeSimpleVolume = math.Float32frombits(uint32(level))
	return 0
}

func fakeGetSimpleVolume(_ uintptr, out *float32) uintptr {
	growCallbackStack(unsafe.Pointer(out))
	*out = 0.37
	return 0
}

func fakeSetEndpointVolume(_, level, _ uintptr) uintptr {
	fakeEndpointVolume = math.Float32frombits(uint32(level))
	return 0
}

func fakeGetEndpointVolume(_ uintptr, out *float32) uintptr {
	growCallbackStack(unsafe.Pointer(out))
	*out = 0.73
	return 0
}

func fakeGetMute(_ uintptr, out *int32) uintptr {
	growCallbackStack(unsafe.Pointer(out))
	*out = -1
	return 0
}

func fakeSetMute(_, muted, context uintptr) uintptr {
	fakeMuteValue = muted
	fakeMuteContext = context
	return 0
}

func fakeUnregister(_, client uintptr) uintptr {
	fakeUnregistered = client
	return 0
}

func fakeDevice(vtable *wca.IMMDeviceVtbl) *wca.IMMDevice {
	return &wca.IMMDevice{
		IUnknown: ole.IUnknown{RawVTable: (*interface{})(unsafe.Pointer(vtable))},
	}
}

// Each native invocation starts on a fresh goroutine stack. The callback grows
// that same stack before writing its output; native uintptr copies cannot be
// relocated by Go. Tests are sequential because the fixtures share state.
func TestNativeOutputsSurviveCallbackStackGrowth(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"Activate", TestActivateUsesNativeSignature},
		{"DeviceID", TestDeviceIDUsesNativePointerWidth},
		{"Count", TestAudioSessionCountUsesInt32Output},
		{"Volume", TestVolumeOperationsUseFloat32ABI},
		{"Mute", TestMuteOperationsUseWin32BOOL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeGrowStack, fakeStackMoved, fakeOutputMoved = true, false, false
			defer func() { fakeGrowStack = false }()
			done := make(chan struct{})
			go func() {
				defer close(done)
				tc.run(t)
			}()
			<-done
			if fakeOutputMoved {
				t.Error("native output address changed during callback")
			}
			if !fakeStackMoved {
				t.Fatal("callback did not relocate its Go stack")
			}
		})
	}
}

//go:noinline
func growCallbackStack(out unsafe.Pointer) {
	if !fakeGrowStack {
		return
	}
	// Model the address retained by native code. Compare it with the typed
	// pointer after growth; never reconstruct or dereference this integer.
	beforeOutput := uintptr(out)
	var marker byte
	before := uintptr(unsafe.Pointer(&marker))
	consumeCallbackStack(128)
	fakeStackMoved = fakeStackMoved || before != uintptr(unsafe.Pointer(&marker))
	fakeOutputMoved = fakeOutputMoved || beforeOutput != uintptr(out)
}

//go:noinline
func consumeCallbackStack(depth int) {
	var frame [1024]byte
	frame[0] = byte(depth)
	if depth > 0 {
		consumeCallbackStack(depth - 1)
	}
	runtime.KeepAlive(&frame)
}
