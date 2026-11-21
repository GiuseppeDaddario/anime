package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed player/skip_intro.lua
var skipIntroScript []byte

// runMPV runs mpv attached to this terminal (anime --player, anime --continue).
func runMPV(args ...string) error {
	return mpv(context.Background(), os.Stdin, os.Stdout, os.Stderr, args...)
}

// playInBackground runs mpv in its own window without touching the terminal,
// so the TUI stays on screen. Cancelling ctx closes mpv.
func playInBackground(ctx context.Context, args ...string) error {
	var stderr bytes.Buffer
	err := mpv(ctx, nil, io.Discard, &stderr, args...)
	if err != nil && ctx.Err() == nil {
		if lines := strings.Split(strings.TrimSpace(stderr.String()), "\n"); lines[len(lines)-1] != "" {
			return fmt.Errorf("mpv: %s", lines[len(lines)-1])
		}
		return fmt.Errorf("mpv: %w", err)
	}
	return nil
}

// Add our one overlay to the real mpv, preserving its configuration and controls.
func mpv(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
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
	command := exec.CommandContext(ctx, executable, append([]string{"--script=" + script}, args...)...)
	command.Stdout, command.Stderr, command.Stdin = stdout, stderr, stdin
	return command.Run()
}
