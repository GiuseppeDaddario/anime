package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"testing"
)

func updateHome(m homeModel, msg tea.Msg) (homeModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(homeModel), cmd
}
func TestHomeOpensWithoutCredentialsAndRoutesToLinking(t *testing.T) {
	isolatedUserConfig(t)
	m := initHomeModel(nil)
	for _, text := range []string{"ANIME", "Not linked", "Continue watching", "Link Telegram account", "anime --info"} {
		if !strings.Contains(m.View(), text) {
			t.Fatalf("Home missing %q", text)
		}
	}
	m, cmd := updateHome(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.action != actionLink {
		t.Fatal("An unlinked account must authenticate before accessing data")
	}
}
func TestRawNumericIDCannotReplaceTelegramLinking(t *testing.T) {
	isolatedUserConfig(t)
	t.Setenv("ANIMEWATCHER_API_URL", "https://api.example.com")
	t.Setenv("ANIMEWATCHER_USER_ID", "12345")
	writeUserConfig(userConfig{UserID: 12345})
	if initHomeModel(nil).userID != 0 {
		t.Fatal("An ID alone must never sign a user in")
	}
}
func TestHomeRoutesActionsAndFitsCommonTerminals(t *testing.T) {
	isolatedUserConfig(t)
	t.Setenv("ANIMEWATCHER_API_URL", "https://api.example.com")
	writeUserConfig(userConfig{UserID: 12345, Token: "session", APIURL: "https://api.example.com"})
	for i, entry := range homeEntries {
		m := initHomeModel(nil)
		m.cursor = i
		m, cmd := updateHome(m, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil || m.action != entry.action {
			t.Fatalf("Wrong action for %s", entry.title)
		}
	}
	for _, width := range []int{60, 80, 120} {
		m, _ := updateHome(initHomeModel(nil), tea.WindowSizeMsg{Width: width, Height: 24})
		if lipgloss.Width(m.View()) > width || lipgloss.Height(m.View()) > 24 {
			t.Fatalf("Home exceeds %dx24", width)
		}
	}
}
