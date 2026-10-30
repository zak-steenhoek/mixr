//go:build windows
// +build windows

// Command windows-audio-control exercises mixr's transport-independent audio
// commands against live Windows Core Audio sessions.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zak-steenhoek/mixr/pkg/mixr"
	"go.uber.org/zap"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	operation := mixr.AudioCommandOperation(os.Args[1])
	flags := flag.NewFlagSet("windows-audio-control "+os.Args[1], flag.ContinueOnError)
	target := flags.String("target", "master", "master, deej.current, or an application executable such as chrome.exe")
	steps := flags.Int("steps", 0, "signed number of relative volume steps")
	increment := flags.Float64("increment", 0.02, "volume scalar per relative step (0 < value <= 1)")
	if err := flags.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 0 {
		usage()
		os.Exit(2)
	}

	logger := zap.NewNop()

	executor, err := mixr.NewSystemAudioCommandExecutor(logger.Sugar())
	if err != nil {
		fatal(err)
	}
	defer func() {
		if err := executor.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "close warning:", err)
		}
	}()

	result, err := executor.ExecuteAudioCommand(mixr.AudioCommand{
		Target:    *target,
		Operation: operation,
		Steps:     *steps,
		Increment: float32(*increment),
	})
	printResult(result)
	if err != nil {
		fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Live Windows audio command test interface

Usage:
  windows-audio-control <operation> [flags]

Operations:
  relative-volume  --target <name> --steps <signed-int> [--increment 0.02]
  mute             --target <name>
  unmute           --target <name>
  toggle-mute      --target <name>
  current-volume   --target <name>
  current-mute     --target <name>

Targets use existing mapping semantics: master, deej.current, or an exact
case-insensitive session key such as chrome.exe. This tool changes live audio
state for relative-volume, mute, unmute, and toggle-mute.`)
}

func printResult(result mixr.AudioCommandResult) {
	fmt.Printf("target: %s\n", result.Target)
	for index, state := range result.Volumes {
		fmt.Printf("volume[%d] %s: %.4f\n", index+1, state.SessionKey, state.Volume)
	}
	if len(result.Volumes) > 1 {
		fmt.Printf("mixed-volume: %t\n", result.MixedVolume)
	}
	for index, state := range result.Mutes {
		fmt.Printf("mute[%d] %s: %t\n", index+1, state.SessionKey, state.Muted)
	}
	if len(result.Mutes) > 0 {
		fmt.Printf("mute-state: %s\n", result.MuteState)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
