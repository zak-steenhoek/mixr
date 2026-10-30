//go:build !windows
// +build !windows

package main

import "fmt"

func main() {
	fmt.Println("windows-audio-control is only available on Windows")
}
