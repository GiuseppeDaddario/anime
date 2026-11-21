package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed player/skip_intro.lua
var skipIntroScript []byte

// Add our one overlay to the real mpv, preserving its configuration and controls.
func runMPV(args ...string) error {
	directory, err := os.MkdirTemp("", "animewatcher-mpv-")
	if err != nil {
		return fmt.Errorf("prepare mpv overlay: %w", err)
	}
	defer os.RemoveAll(directory)
	script := filepath.Join(directory, "skip_intro.lua")
	if err := os.WriteFile(script, skipIntroScript, 0600); err != nil {
		return fmt.Errorf("write mpv overlay: %w", err)
	}
	executable := os.Getenv("ANIMEWATCHER_MPV")
	if executable == "" {
		executable = "mpv"
	}
	command := exec.Command(executable, append([]string{"--script=" + script}, args...)...)
	command.Stdout, command.Stderr, command.Stdin = os.Stdout, os.Stderr, os.Stdin
	return command.Run()
}
