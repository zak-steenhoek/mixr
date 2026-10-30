//go:build windows
// +build windows

// Package wincom contains the small set of corrected Windows Core Audio calls
// that mixr cannot safely make through its pinned go-wca version.
package wincom

import (
	"fmt"
	"reflect"
	"runtime"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca"
)

const maxEndpointIDCodeUnits = 1 << 20

// Activate invokes IMMDevice::Activate using the native COM signature.
func Activate(device *wca.IMMDevice, iid *ole.GUID, ctx uint32, params *wca.PROPVARIANT, out interface{}) error {
	if device == nil || iid == nil {
		return fmt.Errorf("IMMDevice::Activate: nil device or interface ID")
	}

	outValue := reflect.ValueOf(out)
	if !outValue.IsValid() || outValue.Kind() != reflect.Ptr || outValue.IsNil() {
		return fmt.Errorf("IMMDevice::Activate: output must be a non-nil pointer")
	}

	hr, _, _ := syscall.Syscall6(
		device.VTable().Activate,
		5,
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(iid)),
		uintptr(ctx),
		uintptr(unsafe.Pointer(params)),
		outValue.Pointer(),
		0,
	)
	runtime.KeepAlive(device)
	runtime.KeepAlive(iid)
	runtime.KeepAlive(params)
	runtime.KeepAlive(out)

	return hresult("IMMDevice::Activate", hr)
}

// DeviceID invokes IMMDevice::GetId without truncating the returned LPWSTR.
func DeviceID(device *wca.IMMDevice) (string, error) {
	if device == nil {
		return "", fmt.Errorf("IMMDevice::GetId: nil device")
	}

	var id *uint16
	hr, _, _ := syscall.Syscall(
		device.VTable().GetId,
		2,
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(&id)),
		0,
	)
	runtime.KeepAlive(device)

	if err := hresult("IMMDevice::GetId", hr); err != nil {
		if id != nil {
			ole.CoTaskMemFree(uintptr(unsafe.Pointer(id)))
		}
		return "", err
	}
	if id == nil {
		return "", fmt.Errorf("IMMDevice::GetId: success returned a nil endpoint ID")
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(id)))

	codeUnits := (*[maxEndpointIDCodeUnits]uint16)(unsafe.Pointer(id))
	for length, value := range codeUnits {
		if value == 0 {
			return syscall.UTF16ToString(codeUnits[:length:length]), nil
		}
	}

	return "", fmt.Errorf("IMMDevice::GetId: endpoint ID is not null-terminated")
}

// AudioSessionCount invokes IAudioSessionEnumerator::GetCount with the native
// 32-bit int output type instead of Go's architecture-dependent int.
func AudioSessionCount(enumerator *wca.IAudioSessionEnumerator) (int, error) {
	if enumerator == nil {
		return 0, fmt.Errorf("IAudioSessionEnumerator::GetCount: nil enumerator")
	}

	var count int32
	hr, _, _ := syscall.Syscall(
		enumerator.VTable().GetCount,
		2,
		uintptr(unsafe.Pointer(enumerator)),
		uintptr(unsafe.Pointer(&count)),
		0,
	)
	runtime.KeepAlive(enumerator)

	if err := hresult("IAudioSessionEnumerator::GetCount", hr); err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, fmt.Errorf("IAudioSessionEnumerator::GetCount: invalid negative count %d", count)
	}
	return int(count), nil
}

// UnregisterEndpointNotificationCallback fills the E_NOTIMPL gap in the pinned
// go-wca wrapper while preserving the wrapper's public object types.
func UnregisterEndpointNotificationCallback(enumerator *wca.IMMDeviceEnumerator, client *wca.IMMNotificationClient) error {
	if enumerator == nil || client == nil {
		return nil
	}

	hr, _, _ := syscall.Syscall(
		enumerator.VTable().UnregisterEndpointNotificationCallback,
		2,
		uintptr(unsafe.Pointer(enumerator)),
		uintptr(unsafe.Pointer(client)),
		0,
	)
	runtime.KeepAlive(enumerator)
	runtime.KeepAlive(client)

	return hresult("IMMDeviceEnumerator::UnregisterEndpointNotificationCallback", hr)
}

func hresult(operation string, hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("%s: %w", operation, ole.NewError(hr))
	}
	return nil
}
