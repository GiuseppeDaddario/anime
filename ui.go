package main

import (
	"context"
	"fmt"
	"io"
	"strings"

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
	stateLoading sessionState = iota
	stateList
)

type model struct {
	state    sessionState
	list     list.Model
	selected string
	err      error
}

type catalogMsg struct {
	items []list.Item
	err   error
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

func (m model) Init() tea.Cmd {
	if m.state != stateLoading {
		return nil
	}
	return func() tea.Msg {
		items, err := fetchAnimeList(context.Background())
		if err == nil && len(items) == 0 {
			err = fmt.Errorf("The catalogue is empty")
		}
		return catalogMsg{items, err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, screenDone
		}
		m.state = stateList
		return m, m.list.SetItems(msg.items)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, screenDone
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
				return m, screenDone
			}
		}
	case tea.WindowSizeMsg:
		noteScreen(msg.Width, msg.Height)
		m.list.SetSize(msg.Width, msg.Height-11-headerExtra())
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.state != stateList {
		return renderHeader() + "Caricamento catalogo... (Attendere)"
	}

	header := renderHeader()

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(1).
		Render("↑↓  |  Enter Play  |  Space Preview  |  / Search  |  Q Quit")

	return header + m.list.View() + footer
}

// Banner "Anime.cmd". The big 6-row version is used when the terminal has room
// for it; otherwise the compact 2-row version keeps 80x24 layouts intact.
var (
	bigAnime = [][]string{
		{" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},                   // A
		{"███╗   ██╗", "████╗  ██║", "██╔██╗ ██║", "██║╚██╗██║", "██║ ╚████║", "╚═╝  ╚═══╝"},       // N
		{"██╗", "██║", "██║", "██║", "██║", "╚═╝"},                                                 // I
		{"███╗   ███╗", "████╗ ████║", "██╔████╔██║", "██║╚██╔╝██║", "██║ ╚═╝ ██║", "╚═╝     ╚═╝"}, // M
		{"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},                   // E
	}
	bigCmd = [][]string{
		{"   ", "   ", "   ", "   ", "██╗", "╚═╝"}, // .
		{" ██████╗", "██╔════╝", "██║     ", "██║     ", "╚██████╗", " ╚═════╝"},                   // C
		{"███╗   ███╗", "████╗ ████║", "██╔████╔██║", "██║╚██╔╝██║", "██║ ╚═╝ ██║", "╚═╝     ╚═╝"}, // M
		{"██████╗ ", "██╔══██╗", "██║  ██║", "██║  ██║", "██████╔╝", "╚═════╝ "},                   // D
	}
	smallAnime = [][]string{{"▄▀█", "█▀█"}, {"█▄ █", "█ ▀█"}, {"█", "█"}, {"█▀▄▀█", "█ ▀ █"}, {"█▀▀", "██▄"}}
	smallCmd   = [][]string{{" ", "▄"}, {"█▀▀", "█▄▄"}, {"█▀▄▀█", "█ ▀ █"}, {"█▀▄", "█▄▀"}}
	// Orange-to-pink gradient for "ANIME", one colour per letter.
	bannerGradient = []string{"#FF8C00", "#FF7A1A", "#FF6633", "#FF5050", "#FF3D6E"}

	screenWidth, screenHeight int
)

// noteScreen records the terminal size so renderHeader can pick a banner.
func noteScreen(width, height int) { screenWidth, screenHeight = width, height }

func bigHeader() bool { return screenWidth >= 74 && screenHeight >= 30 }

// headerExtra is how many more rows the big banner takes than the compact one.
func headerExtra() int {
	if bigHeader() {
		return 4
	}
	return 0
}

func renderHeader() string {
	anime, cmd, gap := smallAnime, smallCmd, " "
	if bigHeader() {
		anime, cmd, gap = bigAnime, bigCmd, ""
	}
	shadow := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	paint := func(rows []string, glyphs [][]string, style lipgloss.Style) {
		for _, glyph := range glyphs {
			for r := range rows {
				for _, c := range glyph[r] {
					if strings.ContainsRune("╗║╝═╔╚", c) {
						rows[r] += shadow.Render(string(c))
					} else {
						rows[r] += style.Render(string(c))
					}
				}
				rows[r] += gap
			}
		}
	}
	rows := make([]string, len(anime[0]))
	for i, glyph := range anime {
		paint(rows, [][]string{glyph}, lipgloss.NewStyle().Foreground(lipgloss.Color(bannerGradient[i])).Bold(true))
	}
	paint(rows, cmd, lipgloss.NewStyle().Foreground(lipgloss.Color("245")))
	return strings.Join(rows, "\n") + "\n\n"
}
