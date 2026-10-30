//go:build windows
// +build windows

package wincom

import (
	"math"
	"syscall"
	"testing"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca"
)

var (
	fakeActivationCtx    uintptr
	fakeActivationParams uintptr
	fakeActivationResult uintptr
	fakeEndpointID       uintptr
	fakeSimpleVolume     float32
	fakeEndpointVolume   float32
	fakeUnregistered     uintptr
)

func TestActivateUsesNativeSignature(t *testing.T) {
	fakeActivationCtx = 0
	fakeActivationParams = 0
	fakeActivationResult = 0x1234

	vtable := &wca.IMMDeviceVtbl{}
	vtable.Activate = syscall.NewCallback(fakeActivate)
	device := fakeDevice(vtable)
	params := &wca.PROPVARIANT{}
	var result unsafe.Pointer

	if err := Activate(device, wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, params, &result); err != nil {
		t.Fatal(err)
	}
	if fakeActivationCtx != uintptr(wca.CLSCTX_ALL) {
		t.Fatalf("activation context = %#x, want %#x", fakeActivationCtx, uintptr(wca.CLSCTX_ALL))
	}
	if fakeActivationParams != uintptr(unsafe.Pointer(params)) {
		t.Fatalf("activation parameters = %#x, want %#x", fakeActivationParams, uintptr(unsafe.Pointer(params)))
	}
	if uintptr(result) != fakeActivationResult {
		t.Fatalf("activation result = %#x, want %#x", uintptr(result), fakeActivationResult)
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
	fakeEndpointID = ptr

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
	var simpleLevel float32
	if err := simple.GetMasterVolume(&simpleLevel); err != nil {
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

	if err := endpoint.SetMasterVolumeLevelScalar(0.64, nil); err != nil {
		t.Fatal(err)
	}
	if fakeEndpointVolume != float32(0.64) {
		t.Fatalf("endpoint set volume = %f, want 0.64", fakeEndpointVolume)
	}
	var endpointLevel float32
	if err := endpoint.GetMasterVolumeLevelScalar(&endpointLevel); err != nil {
		t.Fatal(err)
	}
	if endpointLevel != float32(0.73) {
		t.Fatalf("endpoint get volume = %f, want 0.73", endpointLevel)
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

func fakeActivate(_, _, ctx, params, out uintptr) uintptr {
	fakeActivationCtx = ctx
	fakeActivationParams = params
	*(*uintptr)(unsafe.Pointer(out)) = fakeActivationResult
	return 0
}

func fakeGetID(_, out uintptr) uintptr {
	*(*uintptr)(unsafe.Pointer(out)) = fakeEndpointID
	return 0
}

func fakeGetCount(_, out uintptr) uintptr {
	*(*int32)(unsafe.Pointer(out)) = 7
	return 0
}

func fakeSetSimpleVolume(_, level, _ uintptr) uintptr {
	fakeSimpleVolume = math.Float32frombits(uint32(level))
	return 0
}

func fakeGetSimpleVolume(_, out uintptr) uintptr {
	*(*float32)(unsafe.Pointer(out)) = 0.37
	return 0
}

func fakeSetEndpointVolume(_, level, _ uintptr) uintptr {
	fakeEndpointVolume = math.Float32frombits(uint32(level))
	return 0
}

func fakeGetEndpointVolume(_, out uintptr) uintptr {
	*(*float32)(unsafe.Pointer(out)) = 0.73
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
