package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestParseAnimeSearch(t *testing.T) {
	for _, tc := range []struct {
		input, query, language string
		valid                  bool
	}{
		{"Naruto - ita", "Naruto", "ita", true},
		{"  Re-Zero - SUB  ", "Re-Zero", "sub", true},
		{"", "", "", false},
		{"Naruto", "", "", false},
		{"Naruto - en", "", "", false},
		{" - sub", "", "", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			query, language, err := parseAnimeSearch(tc.input)
			if (err == nil) != tc.valid || query != tc.query || language != tc.language {
				t.Fatalf("got (%q, %q, %v)", query, language, err)
			}
		})
	}
}

func updateAdd(t *testing.T, m addModel, msg tea.Msg) (addModel, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	return updated.(addModel), cmd
}

func TestAddFlowSelectsExactResult(t *testing.T) {
	m := initAddModel(context.Background())
	m.input.SetValue("Naruto - ita")
	m, cmd := updateAdd(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != addSearching || cmd == nil || m.language != "ita" {
		t.Fatal("Enter should start the search")
	}
	results := []SearchResult{
		{Title: "Naruto", Href: "/first", Language: "ita"},
		{Title: "Naruto Shippuden", Href: "/second", Language: "ita"},
	}
	m, _ = updateAdd(t, m, searchResultsMsg{results: results})
	if m.state != addChoosing || len(m.list.Items()) != 2 {
		t.Fatal("Expected the search result picker")
	}
	m.list.Select(1)
	selected := m.list.SelectedItem().(myListItem).searchResult
	if selected.Href != "/second" {
		t.Fatal("Selection lost the source URL")
	}
	m, cmd = updateAdd(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != addSaving || cmd == nil || m.selected != "Naruto Shippuden" {
		t.Fatal("Enter should save the selected result")
	}
	m, _ = updateAdd(t, m, animeAddedMsg{result: addResult{Added: true, Title: m.selected, Language: "ita"}})
	if m.state != addDone || !strings.Contains(m.View(), "aggiunto con successo") {
		t.Fatal("Expected a success screen")
	}
}

func TestSearchValidationAndRecovery(t *testing.T) {
	m := initAddModel(context.Background())
	m.input.SetValue("Missing language")
	m, cmd := updateAdd(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != addInput || m.err == nil || cmd != nil {
		t.Fatal("Invalid input should stay in the form without starting a command")
	}
	for _, searchErr := range []error{nil, fmt.Errorf("Network unavailable")} {
		m.state = addSearching
		m, _ = updateAdd(t, m, searchResultsMsg{err: searchErr})
		if m.state != addInput || m.err == nil || !m.input.Focused() {
			t.Fatal("Empty searches and errors should return to an editable form")
		}
	}
	m.state = addSaving
	m, _ = updateAdd(t, m, animeAddedMsg{err: fmt.Errorf("Save failed")})
	if m.state != addChoosing || m.err == nil {
		t.Fatal("Save failure should allow retrying the selection")
	}
}

func TestFilteringDoesNotAddOrQuit(t *testing.T) {
	m := initAddModel(context.Background())
	m.state = addChoosing
	m.list.SetItems([]list.Item{myListItem{title: "Query", searchResult: &SearchResult{Title: "Query"}}})
	m.list.SetFilterState(list.Filtering)
	m, _ = updateAdd(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.state != addChoosing || m.list.FilterInput.Value() != "q" {
		t.Fatal("q should be editable in the filter")
	}
	m, _ = updateAdd(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state == addSaving {
		t.Fatal("The first Enter should only apply the filter")
	}
}
