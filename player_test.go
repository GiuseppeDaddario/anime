package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlayerAddsOnlyOverlayAndPreservesMPVArguments(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(dir, "custom-mpv")
	arguments, scriptCopy := filepath.Join(dir, "args"), filepath.Join(dir, "overlay.lua")
	source := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MPV_TEST_ARGS\"\ncp \"${1#--script=}\" \"$MPV_TEST_SCRIPT\"\nexit \"${MPV_TEST_EXIT:-0}\"\n"
	if err := os.WriteFile(program, []byte(source), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANIMEWATCHER_MPV", program)
	t.Setenv("MPV_TEST_ARGS", arguments)
	t.Setenv("MPV_TEST_SCRIPT", scriptCopy)
	for _, failure := range []bool{false, true} {
		t.Setenv("MPV_TEST_EXIT", "0")
		if failure {
			t.Setenv("MPV_TEST_EXIT", "7")
		}
		err := runMPV("--pause", "--", "video with spaces.mp4")
		if (err != nil) != failure {
			t.Fatalf("Unexpected mpv exit: %v", err)
		}
		data, err := os.ReadFile(arguments)
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(args) != 4 || !strings.HasPrefix(args[0], "--script=") || args[1] != "--pause" || args[2] != "--" || args[3] != "video with spaces.mp4" {
			t.Fatalf("mpv arguments changed: %#v", args)
		}
		loaded, err := os.ReadFile(scriptCopy)
		if err != nil || string(loaded) != string(skipIntroScript) {
			t.Fatalf("Embedded overlay was not loaded: %v", err)
		}
		if _, err := os.Stat(strings.TrimPrefix(args[0], "--script=")); !os.IsNotExist(err) {
			t.Fatal("Temporary script must be cleaned up after mpv exits")
		}
	}
}
