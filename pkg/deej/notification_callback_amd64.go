//go:build windows && amd64
// +build windows,amd64

package deej

// Windows x64 passes the 20-byte PROPERTYKEY by reference.
func (sf *wcaSessionFinder) propertyValueChangedCallback(this, deviceID, key uintptr) uintptr {
	return 0
}
