//go:build windows
// +build windows

package main

import "testing"

func TestAppRoutingDeviceID(t *testing.T) {
	endpoint := "{0.0.0.00000000}.{example}"
	want := `\\?\SWD#MMDEVAPI#` + endpoint + "#" + audioRenderClassID
	got, err := appRoutingDeviceID(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	got, err = appRoutingDeviceID("default")
	if err != nil || got != "" {
		t.Fatalf("default: got %q, err %v", got, err)
	}
}
