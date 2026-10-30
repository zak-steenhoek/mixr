//go:build windows && 386
// +build windows,386

package deej

// PROPERTYKEY is passed by value. On 386 it occupies five stack words after
// the receiver and device-ID pointer.
func (sf *wcaSessionFinder) propertyValueChangedCallback(
	this, deviceID, key0, key1, key2, key3, key4 uintptr,
) uintptr {
	return 0
}
