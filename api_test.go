package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeAPI(t *testing.T, handler func(*http.Request) (int, string)) {
	t.Helper()
	isolatedUserConfig(t)
	t.Setenv("ANIMEWATCHER_API_URL", "https://api.example.com")
	old := apiHTTP
	copyClient := *old
	copyClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		status, body := handler(req)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	})
	apiHTTP = &copyClient
	t.Cleanup(func() { apiHTTP = old })
}
func TestRemoteLoginStoresPrivateSessionAndVerifiedUser(t *testing.T) {
	fakeAPI(t, func(r *http.Request) (int, string) {
		if r.URL.Path != "/v1/auth/redeem" {
			t.Fatalf("Unexpected request: %s", r.URL.Path)
		}
		return 200, `{"token":"abcdefghijklmnopqrstuvwxyz0123456789","user_id":777,"expires_at":"2026-12-09T12:00:00Z"}`
	})
	m := newLinkModel(context.Background())
	m.input.SetValue("ABCD-EFGH-JKLM-NPQR-STUV")
	updated, submit := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(linkModel)
	if submit == nil {
		t.Fatal("A valid Telegram code must be submitted")
	}
	updated, _ = m.Update(submit())
	m = updated.(linkModel)
	config, err := readUserConfig()
	if err != nil || !m.linked || config.UserID != 777 || config.Token == "" || config.APIURL != "https://api.example.com" {
		t.Fatalf("Session not stored: linked=%v, error=%v", m.linked, err)
	}
	path, _ := userConfigPath()
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("Session file must not be world-readable")
	}
	home := initHomeModel(nil)
	if home.userID != 777 {
		t.Fatal("Home should show the authenticated ID")
	}
	home.cursor = accountEntry
	updated, cmd := home.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || updated.(homeModel).action != actionLink {
		t.Fatal("Account changes must authenticate, not edit an ID")
	}
}
func TestRemoteCallsNeverTrustARawUserIDOrLeakTokenToAnotherHost(t *testing.T) {
	calls := 0
	fakeAPI(t, func(r *http.Request) (int, string) { calls++; return 200, `{"user_id":7}` })
	writeUserConfig(userConfig{UserID: 7})
	var result any
	if err := apiJSON(context.Background(), "GET", "/v1/me", nil, true, &result); err == nil || calls != 0 {
		t.Fatal("An ID alone must not authenticate")
	}
	writeUserConfig(userConfig{UserID: 7, Token: "private-token", APIURL: "https://different.example.com"})
	if err := apiJSON(context.Background(), "GET", "/v1/me", nil, true, &result); err == nil || calls != 0 {
		t.Fatal("Tokens must be bound to their service origin")
	}
}
func TestRemoteContinueAndStreamNeedNoPython(t *testing.T) {
	fakeAPI(t, func(r *http.Request) (int, string) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("Missing session")
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if _, exists := payload["user_id"]; exists {
			t.Fatal("Client must not choose an identity")
		}
		switch r.URL.Path {
		case "/v1/rpc/play":
			if payload["episode"] != float64(3) {
				t.Fatal("Episode not passed")
			}
			return 200, `{"url":"https://example.com/video.mp4","title":"Naruto","episode":3,"mal_id":20,"intro_episode":3}`
		case "/v1/info/stream":
			return 200, "{\"type\":\"snapshot\",\"items\":[]}\n{\"type\":\"done\"}\n"
		}
		t.Fatalf("Unexpected operation %s", r.URL.Path)
		return 500, ""
	})
	writeUserConfig(userConfig{UserID: 7, Token: "secret-token", APIURL: "https://api.example.com"})
	directory := t.TempDir()
	player := filepath.Join(directory, "mpv")
	args := filepath.Join(directory, "args")
	os.WriteFile(player, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ANIME_TEST_ARGS\"\n"), 0700)
	t.Setenv("ANIMEWATCHER_MPV", player)
	t.Setenv("ANIME_TEST_ARGS", args)
	if err := run([]string{"--continue", "--ep", "3"}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(args)
	if !strings.Contains(string(content), "https://example.com/video.mp4") {
		t.Fatal("Player never received the video")
	}
	events := make(chan infoEvent, 4)
	readInfoStream(context.Background(), events)
	first := <-events
	second := <-events
	if first.Type != "snapshot" || second.Type != "done" {
		t.Fatalf("Invalid event stream: %+v %+v", first, second)
	}
}
func TestRemoteInvalidatedSessionExplainsRelinking(t *testing.T) {
	fakeAPI(t, func(*http.Request) (int, string) { return 401, `{"error":"expired"}` })
	writeUserConfig(userConfig{UserID: 7, Token: "expired-token", APIURL: "https://api.example.com"})
	var result any
	err := apiJSON(context.Background(), "GET", "/v1/me", nil, true, &result)
	if err == nil || !strings.Contains(err.Error(), "link your Telegram account again") {
		t.Fatalf("Unreadable expiry error: %v", err)
	}
}

func TestExpiredSessionCanBeSignedOutAndRemovedLocally(t *testing.T) {
	fakeAPI(t, func(*http.Request) (int, string) { return 401, `{"error":"expired"}` })
	writeUserConfig(userConfig{UserID: 7, Token: "expired-token", APIURL: "https://api.example.com"})
	if err := logoutAccount(); err != nil {
		t.Fatal(err)
	}
	config, err := readUserConfig()
	if err != nil || config.Token != "" || config.UserID != 0 {
		t.Fatal("Expired token was not removed")
	}
}
func TestSavedSessionIsNotFollowedThroughRedirects(t *testing.T) {
	fakeAPI(t, func(*http.Request) (int, string) { return 500, "unused" })
	writeUserConfig(userConfig{UserID: 7, Token: "secret-token", APIURL: "https://api.example.com"})
	calls := 0
	apiHTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{"https://other.example.com/steal"}}, Request: r}, nil
	})
	var result any
	if err := apiJSON(context.Background(), "GET", "/v1/me", nil, true, &result); err == nil || calls != 1 {
		t.Fatal("Authenticated redirects must not be followed")
	}
}

func TestSearchAndAddUseAuthenticatedHTTPSelection(t *testing.T) {
	fakeAPI(t, func(r *http.Request) (int, string) {
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Fatal("Missing session")
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		switch r.URL.Path {
		case "/v1/rpc/search":
			if payload["query"] != "Naruto" || payload["language"] != "ita" {
				t.Fatal("Search selection changed")
			}
			return 200, `[{"title":"Chosen season","href":"/chosen","img_url":"cover","language":"ita"}]`
		case "/v1/rpc/add":
			if payload["href"] != "/chosen" || payload["title"] != "Chosen season" || payload["language"] != "ita" {
				t.Fatal("Add must preserve the exact selected result")
			}
			return 200, `{"added":true,"title":"Chosen season","language":"ita"}`
		}
		return 400, `{"error":"Unexpected operation"}`
	})
	writeUserConfig(userConfig{UserID: 7, Token: "session", APIURL: "https://api.example.com"})
	result := searchAnimeCmd(context.Background(), "Naruto", "ita")().(searchResultsMsg)
	if result.err != nil || len(result.results) != 1 {
		t.Fatalf("Search: %v", result.err)
	}
	added := addAnimeCmd(context.Background(), result.results[0])().(animeAddedMsg)
	if added.err != nil || !added.result.Added || added.result.Title != "Chosen season" {
		t.Fatalf("Add: %v", added.err)
	}
}
func TestProgressRewindDoesNotLaunchMPV(t *testing.T) {
	fakeAPI(t, func(*http.Request) (int, string) { return 200, `{"rewound":true,"title":"Naruto","episode":2}` })
	writeUserConfig(userConfig{UserID: 7, Token: "session", APIURL: "https://api.example.com"})
	t.Setenv("ANIMEWATCHER_MPV", "/missing/player")
	if err := run([]string{"--ep", "-1"}); err != nil {
		t.Fatal("A rewind should only update progress:", err)
	}
}
