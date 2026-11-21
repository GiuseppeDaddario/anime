package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type InfoRecord struct {
	Title             string         `json:"title"`
	Language          string         `json:"language"`
	Status            string         `json:"status"`
	Total             string         `json:"total"`
	ListedEpisodes    int            `json:"listed_episodes"`
	OpenEnded         bool           `json:"open_ended"`
	UnknownTotal      bool           `json:"unknown_total"`
	LastSeen          *int           `json:"last_seen"`
	Next              int            `json:"next"`
	Completed         bool           `json:"completed"`
	Watching          bool           `json:"watching"`
	Current           bool           `json:"current"`
	HistoryShared     bool           `json:"history_shared"`
	Cover             string         `json:"cover"`
	Available         *int           `json:"available"`
	AvailabilityState string         `json:"availability_state"`
	AvailableAtLeast  bool           `json:"available_at_least"`
	CheckError        string         `json:"check_error"`
	Metadata          map[string]any `json:"metadata"`
}

func (i InfoRecord) key() string { return i.Language + "\x00" + i.Title }

func (i InfoRecord) progress() string {
	if i.LastSeen == nil {
		return "Mai iniziato"
	}
	if i.Completed {
		return fmt.Sprintf("Completato · ultimo visto: %d", *i.LastSeen)
	}
	return fmt.Sprintf("Ultimo visto: %d", *i.LastSeen)
}

func (i InfoRecord) availability() string {
	switch i.AvailabilityState {
	case "pending":
		return "Verifica dei link in corso…"
	case "error":
		return "Verifica fallita: " + i.CheckError
	case "listed", "checked":
		if i.Available != nil {
			suffix := "da catalogo"
			if i.AvailabilityState == "checked" {
				suffix = "link consecutivi verificati"
			}
			plus := ""
			if i.AvailableAtLeast {
				plus = "+"
			}
			return fmt.Sprintf("%d%s %s", *i.Available, plus, suffix)
		}
	}
	return "Link non verificati"
}

type infoListItem struct{ record InfoRecord }

func (i infoListItem) Title() string {
	return fmt.Sprintf("%s [%s]", i.record.Title, strings.ToUpper(i.record.Language))
}
func (i infoListItem) Description() string {
	return fmt.Sprintf("%s · Totale: %s · %s", i.record.progress(), i.record.Total, i.record.availability())
}
func (i infoListItem) FilterValue() string {
	return i.record.Title + " " + i.record.Language + " " + i.record.Status
}

type infoEvent struct {
	Type         string       `json:"type"`
	Items        []InfoRecord `json:"items"`
	Item         InfoRecord   `json:"item"`
	UserLanguage string       `json:"user_language"`
	LastAnime    string       `json:"last_anime"`
	err          error
}

type infoListMsg struct {
	tab int
	msg tea.Msg
}

func infoListCmd(tab int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			commands := make([]tea.Cmd, len(batch))
			for i, child := range batch {
				commands[i] = infoListCmd(tab, child)
			}
			return tea.Batch(commands...)()
		}
		return infoListMsg{tab: tab, msg: msg}
	}
}

func sendInfoEvent(ctx context.Context, events chan<- infoEvent, event infoEvent) {
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

func readInfoStream(ctx context.Context, events chan<- infoEvent) {
	defer close(events)
	response, err := apiRequest(ctx, "POST", "/v1/info/stream", map[string]any{}, true)
	if err != nil {
		sendInfoEvent(ctx, events, infoEvent{err: err})
		return
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	gotSnapshot, gotDone := false, false
	for scanner.Scan() {
		var event infoEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			sendInfoEvent(ctx, events, infoEvent{err: fmt.Errorf("Invalid service event")})
			return
		}
		gotSnapshot = gotSnapshot || event.Type == "snapshot"
		gotDone = gotDone || event.Type == "done"
		sendInfoEvent(ctx, events, event)
	}
	if ctx.Err() != nil {
		return
	}
	if err := scanner.Err(); err != nil {
		sendInfoEvent(ctx, events, infoEvent{err: err})
	} else if !gotSnapshot {
		sendInfoEvent(ctx, events, infoEvent{err: fmt.Errorf("The service returned no catalogue")})
	} else if !gotDone {
		sendInfoEvent(ctx, events, infoEvent{err: fmt.Errorf("The availability connection ended early; the loaded catalogue is still usable")})
	}
}

func waitInfoEvent(events <-chan infoEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return infoEvent{Type: "closed"}
		}
		return event
	}
}

type infoModel struct {
	events       chan infoEvent
	lists        [2]list.Model
	spinner      spinner.Model
	viewport     viewport.Model
	records      []InfoRecord
	tab          int
	loaded       bool
	checking     bool
	detailKey    string
	raw          bool
	userLanguage string
	lastAnime    string
	err          error
	width        int
	height       int
	selected     *InfoRecord
}

func initInfoModel() infoModel {
	m := infoModel{
		events:  make(chan infoEvent, 8),
		spinner: spinner.New(), viewport: viewport.New(80, 16), width: 100, height: 30,
	}
	m.spinner.Spinner = spinner.Dot
	for i := range m.lists {
		delegate := list.NewDefaultDelegate()
		delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("208")).BorderForeground(lipgloss.Color("208"))
		delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.BorderForeground(lipgloss.Color("208"))
		l := list.New(nil, delegate, 55, 16)
		l.SetShowTitle(false)
		l.SetShowHelp(false)
		l.SetStatusBarItemName("anime", "anime")
		m.lists[i] = l
	}
	m.resize()
	return m
}

func (m infoModel) Init() tea.Cmd {
	return tea.Batch(waitInfoEvent(m.events), m.spinner.Tick)
}

func (m *infoModel) resize() {
	width, height := m.width, max(4, m.height-11-headerExtra())
	if m.width >= 100 {
		width = m.width / 2
	} else {
		height = max(4, m.height-20-headerExtra())
	}
	for i := range m.lists {
		m.lists[i].SetSize(width, height)
	}
	m.viewport.Width = max(20, m.width-4)
	m.viewport.Height = max(4, m.height-13-headerExtra())
}

func (m *infoModel) rebuildLists() tea.Cmd {
	watching, all := []list.Item{}, []list.Item{}
	for _, record := range m.records {
		item := infoListItem{record: record}
		all = append(all, item)
		if record.Watching {
			watching = append(watching, item)
		}
	}
	sort.SliceStable(watching, func(i, j int) bool {
		return watching[i].(infoListItem).record.Current && !watching[j].(infoListItem).record.Current
	})
	return tea.Batch(infoListCmd(0, m.lists[0].SetItems(watching)), infoListCmd(1, m.lists[1].SetItems(all)))
}

func (m *infoModel) updateRecord(record InfoRecord) tea.Cmd {
	for i := range m.records {
		if m.records[i].key() == record.key() {
			m.records[i] = record
			break
		}
	}
	var commands []tea.Cmd
	for tab := range m.lists {
		for i, item := range m.lists[tab].Items() {
			if item.(infoListItem).record.key() == record.key() {
				commands = append(commands, infoListCmd(tab, m.lists[tab].SetItem(i, infoListItem{record: record})))
				break
			}
		}
	}
	return tea.Batch(commands...)
}

func (m infoModel) highlighted() (InfoRecord, bool) {
	if m.detailKey != "" {
		for _, record := range m.records {
			if record.key() == m.detailKey {
				return record, true
			}
		}
	}
	item, ok := m.lists[m.tab].SelectedItem().(infoListItem)
	return item.record, ok
}

func (m *infoModel) updateDetail() {
	if record, ok := m.highlighted(); ok {
		content := record.details()
		if m.raw {
			data, _ := json.MarshalIndent(record.Metadata, "", "  ")
			content = record.Title + " [" + strings.ToUpper(record.Language) + "]\n\nDati del database (JSON)\n\n" + string(data)
		}
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(content))
	}
}

func (m infoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case infoListMsg:
		var cmd tea.Cmd
		m.lists[msg.tab], cmd = m.lists[msg.tab].Update(msg.msg)
		return m, infoListCmd(msg.tab, cmd)
	case tea.WindowSizeMsg:
		m.width, m.height = max(25, msg.Width), max(15, msg.Height)
		noteScreen(msg.Width, msg.Height)
		m.resize()
		m.updateDetail()
	case infoEvent:
		if msg.err != nil {
			m.err, m.checking = msg.err, false
			return m, waitInfoEvent(m.events)
		}
		var update tea.Cmd
		switch msg.Type {
		case "snapshot":
			m.records, m.userLanguage, m.lastAnime = msg.Items, msg.UserLanguage, msg.LastAnime
			m.loaded, m.checking, m.err = true, true, nil
			update = m.rebuildLists()
		case "availability":
			update = m.updateRecord(msg.Item)
			m.updateDetail()
		case "done":
			m.checking = false
		case "closed":
			m.checking = false
			return m, nil
		}
		return m, tea.Batch(update, waitInfoEvent(m.events))
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, screenDone
		}
		if m.detailKey != "" {
			switch key {
			case "q":
				return m, screenDone
			case "esc", "enter":
				m.detailKey, m.raw = "", false
				return m, nil
			case "d":
				m.raw = !m.raw
				m.viewport.GotoTop()
				m.updateDetail()
				return m, nil
			case "p":
				if record, ok := m.highlighted(); ok {
					m.selected = &record
					return m, screenDone
				}
			}
		} else if m.lists[m.tab].FilterState() != list.Filtering {
			switch key {
			case "q", "esc":
				return m, screenDone
			case "tab", "shift+tab", "1", "2":
				m.tab = 1 - m.tab
				if key == "1" {
					m.tab = 0
				} else if key == "2" {
					m.tab = 1
				}
				return m, nil
			case "enter":
				if record, ok := m.highlighted(); ok {
					m.detailKey = record.key()
					m.viewport.GotoTop()
					m.updateDetail()
				}
				return m, nil
			case "p":
				if record, ok := m.highlighted(); ok {
					m.selected = &record
					return m, screenDone
				}
			case " ":
				if record, ok := m.highlighted(); ok && record.Cover != "" {
					showQuickLook(record.Cover)
				}
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	if m.detailKey != "" {
		m.viewport, cmd = m.viewport.Update(msg)
	} else {
		m.lists[m.tab], cmd = m.lists[m.tab].Update(msg)
		cmd = infoListCmd(m.tab, cmd)
	}
	if !m.loaded || m.checking {
		var tick tea.Cmd
		m.spinner, tick = m.spinner.Update(msg)
		cmd = tea.Batch(cmd, tick)
	}
	return m, cmd
}

func metadataValue(value any) string {
	if value == nil {
		return "—"
	}
	if text, ok := value.(string); ok {
		return text
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func (i InfoRecord) summary() string {
	total := i.Total
	if i.UnknownTotal {
		total = "Sconosciuto"
	}
	return fmt.Sprintf("%s [%s]\n\nStato: %s\nEpisodi totali: %s\n%s\nDisponibili: %s", i.Title, strings.ToUpper(i.Language), i.Status, total, i.progress(), i.availability())
}

func (i InfoRecord) details() string {
	content := i.summary()
	if !i.Completed {
		content += fmt.Sprintf("\nProssimo episodio: %d", i.Next)
	}
	if i.Current {
		content += "\nUltimo anime selezionato"
	}
	if i.HistoryShared {
		content += "\n\nLa cronologia è salvata per titolo e condivisa tra le versioni SUB e ITA con lo stesso nome."
	}
	if i.UnknownTotal {
		content += fmt.Sprintf("\n\nTotale non noto; limite salvato: %d.", i.ListedEpisodes)
	}
	if i.OpenEnded {
		content += "\nIl catalogo contiene un intervallo di episodi senza limite finale."
	}
	content += "\n\nFillers: " + metadataValue(i.Metadata["fillers"])
	content += "\nEpisodi uniti: " + metadataValue(i.Metadata["joint_episodes"])
	var ranges []any
	if templates, ok := i.Metadata["url_template"].([]any); ok {
		for _, template := range templates {
			if template, ok := template.(map[string]any); ok {
				ranges = append(ranges, template["range"])
			}
		}
	}
	content += "\nIntervalli episodi: " + metadataValue(ranges)
	content += "\nPagina anime: " + metadataValue(i.Metadata["link_ID"])
	content += "\nSupervisore: " + metadataValue(i.Metadata["supervisor_ID"])
	content += "\n\nIl progresso indica l'ultimo episodio visto, non il numero di episodi effettivamente guardati."
	if i.AvailabilityState == "checked" {
		content += "\nLa verifica parte dal prossimo episodio e si ferma al primo link non disponibile o non raggiungibile."
	}
	return content
}

func (m infoModel) View() string {
	tabs := []string{"[1] In visione", "[2] Tutto il database"}
	tabs[m.tab] = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render(tabs[m.tab])
	status := ""
	if m.loaded {
		status = "Preferita: " + strings.ToUpper(m.userLanguage)
		if m.checking {
			status += "  ·  " + m.spinner.View() + " Verifica link in background"
		}
	}
	body := ""
	help := "Tab Cambia sezione  |  / Cerca  |  Enter Dettagli  |  P Play  |  Space Anteprima  |  Q Esci"
	if m.width < 100 {
		help = "Tab Sezione | / Cerca | Enter Info | P Play | Q Esci"
	}
	if m.detailKey != "" {
		body = m.viewport.View()
		help = "↑↓ Scorri | D Dati completi | Esc Lista | P Play | Q Esci"
	} else if !m.loaded {
		if m.err == nil {
			body = m.spinner.View() + " Caricamento del database…"
		} else {
			body = "Impossibile caricare il database."
		}
	} else {
		body = m.lists[m.tab].View()
		if record, ok := m.highlighted(); ok {
			if m.width >= 100 {
				panel := lipgloss.NewStyle().Width(m.width - m.lists[m.tab].Width() - 4).MaxHeight(m.lists[m.tab].Height()).PaddingLeft(2).Render(record.summary() + "\n\nEnter per tutti i dettagli")
				body = lipgloss.JoinHorizontal(lipgloss.Top, body, panel)
			} else {
				body += "\n" + lipgloss.NewStyle().Width(m.width).MaxHeight(8).Render(record.summary())
			}
		}
	}
	if m.err != nil {
		body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err.Error())
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(help)
	return renderHeader() + strings.Join(tabs, "    ") + "\n" + status + "\n\n" + body + "\n\n" + footer
}
