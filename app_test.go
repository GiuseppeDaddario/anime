package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// drive feeds msg to the app and runs every resulting command until none are
// left, reporting whether the program asked to quit.
func drive(a tea.Model, msg tea.Msg) (tea.Model, bool) {
	queue, quit := []tea.Msg{msg}, false
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		switch next := next.(type) {
		case tea.QuitMsg:
			quit = true
			continue
		case spinner.TickMsg:
			continue // animation only
		case tea.BatchMsg:
			for _, cmd := range next {
				if cmd != nil {
					queue = append(queue, cmd())
				}
			}
			continue
		}
		var cmd tea.Cmd
		a, cmd = a.Update(next)
		if cmd != nil {
			queue = append(queue, cmd())
		}
	}
	return a, quit
}

func TestBrowseAndPlayStayInsideOneProgram(t *testing.T) {
	fakeAPI(t, func(r *http.Request) (int, string) {
		switch r.URL.Path {
		case "/v1/rpc/catalog":
			return 200, `{"Naruto":""}`
		case "/v1/rpc/play":
			return 200, `{"url":"https://video.example.com/naruto-3.mp4","title":"Naruto","episode":3}`
		}
		t.Fatalf("Unexpected request: %s", r.URL.Path)
		return 500, ""
	})
	writeUserConfig(userConfig{UserID: 7, Token: strings.Repeat("t", 32), APIURL: "https://api.example.com"})
	dir := t.TempDir()
	program, played := filepath.Join(dir, "mpv"), filepath.Join(dir, "played")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+played+"\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANIMEWATCHER_MPV", program)

	var a tea.Model = newApp(actionHome, "")
	a, _ = drive(a, a.Init())
	a, _ = drive(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	a, _ = drive(a, tea.KeyMsg{Type: tea.KeyDown}) // Browse anime
	a, quit := drive(a, tea.KeyMsg{Type: tea.KeyEnter})
	if browse, ok := a.(appModel).screen.(model); quit || !ok || len(browse.list.Items()) != 1 {
		t.Fatalf("Browse must open with the catalogue inside the same program, got %T", a.(appModel).screen)
	}
	a, quit = drive(a, tea.KeyMsg{Type: tea.KeyEnter})
	home, ok := a.(appModel).screen.(homeModel)
	if quit || !ok || home.err != nil || home.notice != "Watched Naruto, episode 3" {
		t.Fatalf("After playback the app must return home, got %T %+v", a.(appModel).screen, home)
	}
	if args, _ := os.ReadFile(played); !strings.Contains(string(args), "https://video.example.com/naruto-3.mp4") {
		t.Fatal("mpv did not receive the episode URL")
	}
	if _, quit = drive(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); !quit {
		t.Fatal("Q on the home menu must quit")
	}
}

func TestEscDuringPlaybackClosesMPV(t *testing.T) {
	fakeAPI(t, func(r *http.Request) (int, string) {
		return 200, `{"url":"https://video.example.com/naruto-3.mp4","title":"Naruto","episode":3}`
	})
	writeUserConfig(userConfig{UserID: 7, Token: strings.Repeat("t", 32), APIURL: "https://api.example.com"})
	program := filepath.Join(t.TempDir(), "mpv")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANIMEWATCHER_MPV", program)

	a := newApp(actionHome, "")
	a.screen = model{selected: "Naruto"}
	next, _ := a.Update(screenDoneMsg{})
	var resolved tea.Msg
	for _, cmd := range next.(appModel).screen.Init()().(tea.BatchMsg) {
		if msg, ok := cmd().(playResolvedMsg); ok {
			resolved = msg
		}
	}
	next, startMPV := next.Update(resolved)
	finished := make(chan tea.Msg, 1)
	go func() { finished <- startMPV() }()
	next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next.Update(screenDoneMsg{})
	select {
	case msg := <-finished:
		if msg.(playFinishedMsg).err != nil {
			t.Fatalf("Stopping playback is not an error: %v", msg.(playFinishedMsg).err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Esc must close mpv")
	}
}
