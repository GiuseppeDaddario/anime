package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(myListItem)
	if !ok {
		return
	}

	titleWithNum := fmt.Sprintf("%d. %s", index+1, i.title)
	titlePadded := fmt.Sprintf("%-55.55s", titleWithNum)

	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	if index == m.Index() {
		fmt.Fprint(w, accentStyle.Render("> "+titlePadded)+"  "+accentStyle.Render(i.desc))
	} else {
		fmt.Fprint(w, whiteStyle.Render("  "+titlePadded)+"  "+descStyle.Render(i.desc))
	}
}

type sessionState int

const (
	stateMenu sessionState = iota
	stateList
)

type model struct {
	state    sessionState
	list     list.Model
	selected string
}

func initModel(listData []list.Item) model {
	l := list.New(listData, itemDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	return model{
		state: stateList,
		list:  l,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

		if msg.String() == " " && m.state == stateList {
			i, ok := m.list.SelectedItem().(myListItem)
			if ok && i.coverUrl != "" {
				showQuickLook(i.coverUrl)
			}
			return m, nil
		}

		if msg.String() == "enter" && m.state == stateList {
			i, ok := m.list.SelectedItem().(myListItem)
			if ok {
				m.selected = i.title
				return m, tea.Quit
			}
		}
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-10)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.state != stateList {
		return "Caricamento in corso..."
	}

	header := renderHeader()

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(1).
		Render("↑↓  |  Enter Play  |  Space Preview  |  / Search  |  Q Quit")

	return header + m.list.View() + footer
}

func renderHeader() string {
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render("ANIME")
	return title + "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Watch, discover, and keep your place.") + "\n\n"
}
