package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type homeEntry struct {
	title, hint string
	action      cliAction
}

var homeEntries = []homeEntry{
	{"Continue watching", "anime --continue", actionContinue},
	{"Browse anime", "anime --anime", actionBrowse},
	{"History & database", "anime --info", actionInfo},
	{"Add an anime", "anime --add", actionAdd},
	{"Link Telegram account", "Sign in or change account", actionLink},
	{"Sign out", "Revoke this device session", actionLogout},
	{"Quit", "", actionHome},
}

const accountEntry = 4

type homeModel struct {
	cursor int
	userID int64
	action cliAction
	err    error
	notice string
	width  int
}

func initHomeModel(lastError error) homeModel {
	m := homeModel{width: 80, err: lastError}
	config, err := readUserConfig()
	base, baseErr := apiBaseURL()
	if err == nil && baseErr == nil && config.Token != "" && config.APIURL == base {
		m.userID = config.UserID
	}
	if m.userID == 0 {
		m.notice = "Choose Link Telegram account to connect your watch history."
	}
	return m
}
func (m homeModel) Init() tea.Cmd { return nil }
func (m homeModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.cursor = (m.cursor + len(homeEntries) - 1) % len(homeEntries)
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % len(homeEntries)
		case "enter":
			m.action = homeEntries[m.cursor].action
			if m.action != actionHome && m.action != actionLogout && m.userID == 0 {
				m.action = actionLink
			}
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m homeModel) View() string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	id := "Not linked"
	if m.userID > 0 {
		id = strconv.FormatInt(m.userID, 10)
	}
	var body strings.Builder
	body.WriteString(renderHeader() + "Telegram account: " + id + "\n\n")
	for i, entry := range homeEntries {
		line := fmt.Sprintf("  %-22s", entry.title)
		if i == m.cursor {
			line = accent.Render("› " + fmt.Sprintf("%-22s", entry.title))
		}
		if m.width >= 65 {
			line += muted.Render(entry.hint)
		}
		body.WriteString(line + "\n")
	}
	body.WriteString("\n" + muted.Render("↑↓ Choose · Enter Open · Q Quit") + "\n" + muted.Render("Tip: anime --ep N plays a specific episode.") + "\n")
	if m.notice != "" {
		body.WriteString("\n" + muted.Render(m.notice))
	}
	if m.err != nil {
		body.WriteString("\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err.Error()))
	}
	return lipgloss.NewStyle().Width(max(20, m.width-2)).Render(body.String())
}
