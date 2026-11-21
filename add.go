package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type SearchResult struct {
	Title    string `json:"title"`
	Href     string `json:"href"`
	ImageURL string `json:"img_url"`
	Language string `json:"language"`
}

type addResult struct {
	Added    bool   `json:"added"`
	Title    string `json:"title"`
	Language string `json:"language"`
}

type searchResultsMsg struct {
	results []SearchResult
	err     error
}

type animeAddedMsg struct {
	result addResult
	err    error
}

func searchAnimeCmd(ctx context.Context, query, language string) tea.Cmd {
	return func() tea.Msg {
		var results []SearchResult
		err := apiJSON(ctx, "POST", "/v1/rpc/search", map[string]string{"query": query, "language": language}, true, &results)
		return searchResultsMsg{results: results, err: err}
	}
}

func addAnimeCmd(ctx context.Context, selected SearchResult) tea.Cmd {
	return func() tea.Msg {
		var result addResult
		err := apiJSON(ctx, "POST", "/v1/rpc/add", selected, true, &result)
		return animeAddedMsg{result: result, err: err}
	}
}

func parseAnimeSearch(value string) (string, string, error) {
	separator := strings.LastIndex(value, "-")
	if separator < 0 {
		return "", "", fmt.Errorf("Scrivi Nome Anime - lingua (es. Naruto - ita).")
	}
	query := strings.TrimSpace(value[:separator])
	language := strings.ToLower(strings.TrimSpace(value[separator+1:]))
	if query == "" {
		return "", "", fmt.Errorf("Specifica il nome dell'anime.")
	}
	if language != "sub" && language != "ita" {
		return "", "", fmt.Errorf("Lingua non valida. Usa sub o ita.")
	}
	return query, language, nil
}

type addState int

const (
	addInput addState = iota
	addSearching
	addChoosing
	addSaving
	addDone
)

type addModel struct {
	ctx      context.Context
	state    addState
	input    textinput.Model
	list     list.Model
	spinner  spinner.Model
	query    string
	language string
	selected string
	result   addResult
	err      error
}

func initAddModel(ctx context.Context) addModel {
	input := textinput.New()
	input.Placeholder = "Naruto - ita"
	input.CharLimit = 250
	input.Width = 60
	input.Focus()
	l := initModel(nil).list
	l.SetSize(80, 12)
	s := spinner.New()
	s.Spinner = spinner.Dot
	return addModel{ctx: ctx, input: input, list: l, spinner: s}
}

func (m addModel) Init() tea.Cmd { return textinput.Blink }

func (m addModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, max(3, msg.Height-13))
		m.input.Width = max(10, min(60, msg.Width-6))
	case searchResultsMsg:
		if m.state != addSearching {
			return m, nil
		}
		if msg.err != nil || len(msg.results) == 0 {
			m.state = addInput
			m.err = msg.err
			if m.err == nil {
				m.err = fmt.Errorf("Nessun risultato trovato. Prova un altro nome.")
			}
			return m, m.input.Focus()
		}
		items := make([]list.Item, len(msg.results))
		for i, result := range msg.results {
			items[i] = myListItem{
				title: result.Title, desc: strings.ToUpper(result.Language),
				coverUrl: result.ImageURL, searchResult: &msg.results[i],
			}
		}
		m.state = addChoosing
		m.list.ResetFilter()
		m.list.Select(0)
		return m, m.list.SetItems(items)
	case animeAddedMsg:
		if m.state != addSaving {
			return m, nil
		}
		if msg.err != nil {
			m.state, m.err = addChoosing, msg.err
			return m, nil
		}
		m.state, m.result = addDone, msg.result
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.state {
		case addInput:
			switch key {
			case "esc":
				return m, tea.Quit
			case "enter":
				query, language, err := parseAnimeSearch(m.input.Value())
				m.err = err
				if err != nil {
					return m, nil
				}
				m.query, m.language, m.state = query, language, addSearching
				m.input.Blur()
				return m, tea.Batch(m.spinner.Tick, searchAnimeCmd(m.ctx, query, language))
			}
		case addChoosing:
			if m.list.FilterState() != list.Filtering {
				switch key {
				case "q":
					return m, tea.Quit
				case "esc":
					m.state, m.err = addInput, nil
					return m, m.input.Focus()
				case "enter":
					item, ok := m.list.SelectedItem().(myListItem)
					if ok && item.searchResult != nil {
						m.state, m.selected, m.err = addSaving, item.title, nil
						return m, tea.Batch(m.spinner.Tick, addAnimeCmd(m.ctx, *item.searchResult))
					}
				case " ":
					item, ok := m.list.SelectedItem().(myListItem)
					if ok && item.coverUrl != "" {
						showQuickLook(item.coverUrl)
					}
					return m, nil
				}
			}
		case addDone:
			if key == "enter" || key == "q" || key == "esc" {
				return m, tea.Quit
			}
			if key == "n" {
				m.state, m.err = addInput, nil
				m.input.SetValue("")
				return m, m.input.Focus()
			}
		case addSearching, addSaving:
			if key == "esc" {
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	switch m.state {
	case addInput:
		m.input, cmd = m.input.Update(msg)
	case addChoosing:
		m.list, cmd = m.list.Update(msg)
	case addSearching, addSaving:
		m.spinner, cmd = m.spinner.Update(msg)
	}
	return m, cmd
}

func (m addModel) View() string {
	var body, help string
	switch m.state {
	case addInput:
		body = "Aggiungi un anime\n\nChe anime vuoi aggiungere? (formato: Nome Anime - lingua)\n\n" + m.input.View()
		help = "Enter Cerca  |  Esc Annulla"
	case addSearching:
		body = fmt.Sprintf("%s Cerco %s (%s)...", m.spinner.View(), m.query, m.language)
		help = "Esc Annulla"
	case addChoosing:
		body = fmt.Sprintf("Risultati per %s (%s)\n\n%s", m.query, m.language, m.list.View())
		help = "↑↓ Scegli  |  Enter Aggiungi  |  Space Anteprima  |  / Filtra  |  Esc Nuova ricerca  |  Q Esci"
	case addSaving:
		body = fmt.Sprintf("%s Recupero i link e aggiungo %s...", m.spinner.View(), m.selected)
		help = "Esc Annulla"
	case addDone:
		if m.result.Added {
			body = fmt.Sprintf("✓ Anime '%s' aggiunto con successo (%s).", m.result.Title, m.result.Language)
		} else {
			body = fmt.Sprintf("Anime '%s' già presente nel catalogo %s.", m.result.Title, m.result.Language)
		}
		help = "Enter Esci  |  N Aggiungi un altro anime"
	}
	if m.err != nil {
		body += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err.Error())
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(help + "  |  Ctrl+C Esci")
	return renderHeader() + body + "\n\n" + footer
}
