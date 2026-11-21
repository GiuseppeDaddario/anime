package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func updateInfo(m infoModel, msg tea.Msg) (infoModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(infoModel), cmd
}

func infoFixture() []InfoRecord {
	seen := 3
	return []InfoRecord{
		{Title: "Tower of God", Language: "sub", Status: "in progress", Total: "12", LastSeen: &seen, Next: 4, Watching: true, AvailabilityState: "pending"},
		{Title: "Tower of God", Language: "ita", Status: "in progress", Total: "12", LastSeen: &seen, Next: 4, AvailabilityState: "unchecked", HistoryShared: true},
		{Title: "Unstarted", Language: "ita", Status: "ended", Total: "24", AvailabilityState: "listed"},
	}
}

func TestInfoOpensBeforeDataAndTabsWorkWhileChecking(t *testing.T) {
	m := initInfoModel()
	if !strings.Contains(m.View(), "Caricamento del database") || m.Init() == nil {
		t.Fatal("The panel must render before any backend work")
	}
	m, _ = updateInfo(m, infoEvent{Type: "snapshot", Items: infoFixture(), UserLanguage: "sub"})
	if !m.loaded || !m.checking || len(m.lists[0].Items()) != 1 || len(m.lists[1].Items()) != 3 {
		t.Fatal("Expected immediate watching and full catalogue tabs")
	}
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.tab != 1 || !m.checking {
		t.Fatal("Tab switching should work during slow link checks")
	}
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.detailKey != "ita\x00Tower of God" || !strings.Contains(m.viewport.View(), "ITA") {
		t.Fatal("Enter should open details for the exact language selected")
	}
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.selected == nil || m.selected.Language != "ita" {
		t.Fatal("Playback must keep the selected catalogue language")
	}
}

func TestInfoUpdatesDoNotResetSearchOrOtherLanguage(t *testing.T) {
	m := initInfoModel()
	m, _ = updateInfo(m, infoEvent{Type: "snapshot", Items: infoFixture()})
	m.tab = 1
	m.lists[1].SetFilterState(list.Filtering)
	m.lists[1].FilterInput.SetValue("Tower")
	updated := infoFixture()[0]
	available := 2
	updated.Available, updated.AvailabilityState = &available, "checked"
	m, _ = updateInfo(m, infoEvent{Type: "availability", Item: updated})
	if m.tab != 1 || m.lists[1].FilterInput.Value() != "Tower" || m.lists[1].FilterState() != list.Filtering {
		t.Fatal("Live updates must preserve the active filter and tab")
	}
	if m.records[0].Available == nil || *m.records[0].Available != 2 || m.records[1].Available != nil {
		t.Fatal("Updates must target the matching title and language only")
	}
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !strings.Contains(m.lists[1].FilterInput.Value(), "q") {
		t.Fatal("Typing q in a search must not quit")
	}
}

func TestInfoFilterCommandsStayWithTheirOwnTab(t *testing.T) {
	m := initInfoModel()
	m, _ = updateInfo(m, infoEvent{Type: "snapshot", Items: infoFixture()})
	m.lists[0].SetFilterText("Tower")
	m.lists[1].SetFilterText("Tower")
	m.lists[1].Select(1)
	updated := infoFixture()[0]
	available := 2
	updated.Available, updated.AvailabilityState = &available, "checked"
	cmd := m.updateRecord(updated)
	var apply func(tea.Msg)
	apply = func(msg tea.Msg) {
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				if child != nil {
					apply(child())
				}
			}
			return
		}
		var next tea.Cmd
		m, next = updateInfo(m, msg)
		if next != nil {
			apply(next())
		}
	}
	apply(cmd())
	if len(m.lists[0].VisibleItems()) != 1 || len(m.lists[1].VisibleItems()) != 2 {
		t.Fatal("Background filter results must not cross between tabs")
	}
	selected := m.lists[1].SelectedItem().(infoListItem).record
	if selected.Language != "ita" || m.lists[1].FilterInput.Value() != "Tower" {
		t.Fatal("Background checks must preserve selection and search")
	}
}

func TestInfoDetailsAndRawMetadata(t *testing.T) {
	m := initInfoModel()
	items := infoFixture()
	items[0].Metadata = map[string]any{"fillers": []int{4}, "extra": "preserved", "link_ID": "source"}
	m, _ = updateInfo(m, infoEvent{Type: "snapshot", Items: items})
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.selected != nil || !strings.Contains(m.viewport.View(), "Ultimo visto: 3") {
		t.Fatal("Enter should inspect progress without launching playback")
	}
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.raw || !strings.Contains(m.viewport.View(), "preserved") {
		t.Fatal("All stored metadata should be inspectable")
	}
	m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.detailKey != "" || m.raw {
		t.Fatal("Escape should return to the list")
	}
	m, _ = updateInfo(m, infoEvent{Type: "done"})
	if m.checking {
		t.Fatal("Completed checks should clear the progress indicator")
	}
}

func TestInfoRetainsCatalogueAfterCheckFailure(t *testing.T) {
	m := initInfoModel()
	m, _ = updateInfo(m, infoEvent{Type: "snapshot", Items: infoFixture()})
	m, _ = updateInfo(m, infoEvent{err: fmt.Errorf("Check process failed")})
	if !m.loaded || len(m.lists[1].Items()) != 3 || m.err == nil {
		t.Fatal("A check failure must not hide the loaded catalogue")
	}
}

func TestInfoFitsTypicalTerminalSizes(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 120, Height: 32}} {
		m := initInfoModel()
		m, _ = updateInfo(m, size)
		m, _ = updateInfo(m, infoEvent{Type: "snapshot", Items: infoFixture(), UserLanguage: "sub"})
		if lipgloss.Height(m.View()) > size.Height || lipgloss.Width(m.View()) > size.Width {
			for _, line := range strings.Split(m.View(), "\n") {
				if lipgloss.Width(line) > size.Width {
					t.Logf("Overflow (%d columns): %q", lipgloss.Width(line), line)
				}
			}
			t.Fatalf("Panel exceeds %dx%d: got %dx%d", size.Width, size.Height, lipgloss.Width(m.View()), lipgloss.Height(m.View()))
		}
		m, _ = updateInfo(m, tea.KeyMsg{Type: tea.KeyEnter})
		if lipgloss.Height(m.View()) > size.Height || lipgloss.Width(m.View()) > size.Width {
			t.Fatalf("Detail panel exceeds %dx%d", size.Width, size.Height)
		}
	}
}
