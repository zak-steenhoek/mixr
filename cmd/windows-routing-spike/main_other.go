//go:build !windows
// +build !windows

package main

import "fmt"

func main() {
	fmt.Println("windows-routing-spike only runs on Windows")
}
