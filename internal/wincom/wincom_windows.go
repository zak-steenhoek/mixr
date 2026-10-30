//go:build windows
// +build windows

// Package wincom contains the small set of corrected Windows Core Audio calls
// that mixr cannot safely make through its pinned go-wca version.
package wincom

import (
	"fmt"
	"math"
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

// SimpleAudioVolumeMute invokes ISimpleAudioVolume::GetMute with a Win32 BOOL
// output. BOOL is always 32 bits; using Go bool here lets the native call write
// past the one-byte value used by the pinned go-wca wrapper.
func SimpleAudioVolumeMute(volume *wca.ISimpleAudioVolume) (bool, error) {
	if volume == nil {
		return false, fmt.Errorf("ISimpleAudioVolume::GetMute: nil volume")
	}

	var muted int32
	hr, _, _ := syscall.Syscall(
		volume.VTable().GetMute,
		2,
		uintptr(unsafe.Pointer(volume)),
		uintptr(unsafe.Pointer(&muted)),
		0,
	)
	runtime.KeepAlive(volume)

	if err := hresult("ISimpleAudioVolume::GetMute", hr); err != nil {
		return false, err
	}
	return muted != 0, nil
}

// SetSimpleAudioVolumeMute invokes ISimpleAudioVolume::SetMute without using
// the pinned wrapper's Go-bool signature.
func SetSimpleAudioVolumeMute(volume *wca.ISimpleAudioVolume, muted bool, eventContext *ole.GUID) error {
	if volume == nil {
		return fmt.Errorf("ISimpleAudioVolume::SetMute: nil volume")
	}

	var nativeMuted uint32
	if muted {
		nativeMuted = 1
	}
	hr, _, _ := syscall.Syscall(
		volume.VTable().SetMute,
		3,
		uintptr(unsafe.Pointer(volume)),
		uintptr(nativeMuted),
		uintptr(unsafe.Pointer(eventContext)),
	)
	runtime.KeepAlive(volume)
	runtime.KeepAlive(eventContext)

	return hresult("ISimpleAudioVolume::SetMute", hr)
}

// EndpointVolumeMute invokes IAudioEndpointVolume::GetMute with a 32-bit BOOL
// output instead of the unsafe Go bool used by the pinned dependency.
func EndpointVolumeMute(volume *wca.IAudioEndpointVolume) (bool, error) {
	if volume == nil {
		return false, fmt.Errorf("IAudioEndpointVolume::GetMute: nil volume")
	}

	var muted int32
	hr, _, _ := syscall.Syscall(
		volume.VTable().GetMute,
		2,
		uintptr(unsafe.Pointer(volume)),
		uintptr(unsafe.Pointer(&muted)),
		0,
	)
	runtime.KeepAlive(volume)

	if err := hresult("IAudioEndpointVolume::GetMute", hr); err != nil {
		return false, err
	}
	return muted != 0, nil
}

// SetEndpointVolumeMute invokes IAudioEndpointVolume::SetMute using a native
// 32-bit BOOL value and the caller's event context.
func SetEndpointVolumeMute(volume *wca.IAudioEndpointVolume, muted bool, eventContext *ole.GUID) error {
	if volume == nil {
		return fmt.Errorf("IAudioEndpointVolume::SetMute: nil volume")
	}

	var nativeMuted uint32
	if muted {
		nativeMuted = 1
	}
	hr, _, _ := syscall.Syscall(
		volume.VTable().SetMute,
		3,
		uintptr(unsafe.Pointer(volume)),
		uintptr(nativeMuted),
		uintptr(unsafe.Pointer(eventContext)),
	)
	runtime.KeepAlive(volume)
	runtime.KeepAlive(eventContext)

	return hresult("IAudioEndpointVolume::SetMute", hr)
}

// EndpointVolumeLevelScalar reads an endpoint's normalized 0..1 volume using
// the native FLOAT output type.
func EndpointVolumeLevelScalar(volume *wca.IAudioEndpointVolume) (float32, error) {
	if volume == nil {
		return 0, fmt.Errorf("IAudioEndpointVolume::GetMasterVolumeLevelScalar: nil volume")
	}

	var level float32
	hr, _, _ := syscall.Syscall(
		volume.VTable().GetMasterVolumeLevelScalar,
		2,
		uintptr(unsafe.Pointer(volume)),
		uintptr(unsafe.Pointer(&level)),
		0,
	)
	runtime.KeepAlive(volume)

	if err := hresult("IAudioEndpointVolume::GetMasterVolumeLevelScalar", hr); err != nil {
		return 0, err
	}
	return level, nil
}

// SetEndpointVolumeLevelScalar writes a normalized endpoint volume using the
// native FLOAT argument representation and the caller's event context.
func SetEndpointVolumeLevelScalar(volume *wca.IAudioEndpointVolume, level float32, eventContext *ole.GUID) error {
	if volume == nil {
		return fmt.Errorf("IAudioEndpointVolume::SetMasterVolumeLevelScalar: nil volume")
	}

	hr, _, _ := syscall.Syscall(
		volume.VTable().SetMasterVolumeLevelScalar,
		3,
		uintptr(unsafe.Pointer(volume)),
		uintptr(math.Float32bits(level)),
		uintptr(unsafe.Pointer(eventContext)),
	)
	runtime.KeepAlive(volume)
	runtime.KeepAlive(eventContext)

	return hresult("IAudioEndpointVolume::SetMasterVolumeLevelScalar", hr)
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
