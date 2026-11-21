package main

import "testing"

func TestCommandRouting(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		action  cliAction
		episode string
	}{
		{nil, actionHome, ""},
		{[]string{"-v"}, actionHome, ""},
		{[]string{"--continue"}, actionContinue, ""},
		{[]string{"--info"}, actionInfo, ""},
		{[]string{"--anime"}, actionBrowse, ""},
		{[]string{"--tui"}, actionBrowse, ""},
		{[]string{"--add"}, actionAdd, ""},
		{[]string{"--ep", "3"}, actionContinue, "3"},
		{[]string{"--continue", "--ep=3"}, actionContinue, "3"},
		{[]string{"--ep", "-1"}, actionContinue, "-1"},
		{[]string{"--info", "--ep", "3"}, actionInfo, "3"},
	} {
		got, err := parseOptions(tc.args)
		if err != nil || got.action != tc.action || got.episode != tc.episode {
			t.Fatalf("%v: got %+v, %v", tc.args, got, err)
		}
	}
}

func TestInvalidFlagsFailInsteadOfStartingPlayback(t *testing.T) {
	for _, args := range [][]string{
		{"--unknown"}, {"--ep"}, {"--ep="}, {"--ep", "oops"}, {"--ep", "0"},
		{"--ep", "-2"}, {"--add", "--ep", "3"}, {"--info", "--continue"},
		{"--player"}, {"--info", "--player", "video.mp4"}, {"--user-id", "12345"},
	} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("Accepted invalid flags: %v", args)
		}
	}
}

func TestPlayerPassesItsArgumentsThroughWithoutAnAccount(t *testing.T) {
	got, err := parseOptions([]string{"--player", "--pause", "--ep", "video with spaces.mp4"})
	if err != nil || got.action != actionPlayer || len(got.playerArgs) != 3 || got.playerArgs[2] != "video with spaces.mp4" {
		t.Fatalf("Player options were changed: %+v, %v", got, err)
	}
}
