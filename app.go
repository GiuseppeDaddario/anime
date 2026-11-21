package main

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// screenDoneMsg tells the app that the current screen has finished; the app
// reads the screen's final state to decide what comes next.
type screenDoneMsg struct{}

func screenDone() tea.Msg { return screenDoneMsg{} }

// appModel is the single Bubble Tea program: it shows one screen at a time and
// never leaves the alternate screen until the user quits.
type appModel struct {
	screen     tea.Model
	cancel     context.CancelFunc // stops the current screen's background work
	standalone bool               // started from a flag: quit instead of going home
	episode    string             // --ep, applied to the next playback
	size       tea.WindowSizeMsg
	err        error  // reported on exit in standalone mode
	notice     string // printed on exit in standalone mode
}

type logoutMsg struct{ err error }

func newApp(action cliAction, episode string) appModel {
	a := appModel{standalone: action != actionHome, episode: episode}
	a.screen, _ = a.open(action)
	return a
}

func (a appModel) Init() tea.Cmd { return a.screen.Init() }

// open builds the screen for an action and returns it with its context set up.
func (a *appModel) open(action cliAction) (tea.Model, tea.Cmd) {
	ctx := a.newContext()
	switch action {
	case actionContinue:
		return a.play(ctx, "", ""), nil
	case actionBrowse:
		m := initModel(nil)
		m.state = stateLoading
		return m, nil
	case actionInfo:
		m := initInfoModel()
		go readInfoStream(ctx, m.events)
		return m, nil
	case actionAdd:
		return initAddModel(ctx), nil
	case actionLink:
		return newLinkModel(ctx), nil
	case actionLogout:
		return initHomeModel(nil), func() tea.Msg { return logoutMsg{logoutAccount()} }
	}
	return initHomeModel(nil), nil
}

func (a *appModel) play(ctx context.Context, title, language string) tea.Model {
	m := newPlayModel(ctx, title, language, a.episode)
	a.episode = ""
	return m
}

// show switches to a screen, starting it and giving it the current size.
func (a appModel) show(screen tea.Model, extra tea.Cmd) (tea.Model, tea.Cmd) {
	a.screen = screen
	cmds := []tea.Cmd{screen.Init(), extra}
	if a.size.Width > 0 {
		var cmd tea.Cmd
		a.screen, cmd = a.screen.Update(a.size)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// goHome returns to the home menu with an optional error or notice, or quits
// when the app was started directly on a screen.
func (a appModel) goHome(err error, notice string) (tea.Model, tea.Cmd) {
	if a.standalone {
		if a.cancel != nil {
			a.cancel()
		}
		a.err, a.notice = err, notice
		return a, tea.Quit
	}
	screen, cmd := a.open(actionHome)
	home := screen.(homeModel)
	home.err = err
	if notice != "" {
		home.notice = notice
	}
	return a.show(home, cmd)
}

func (a appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.size = msg
	case logoutMsg:
		return a.goHome(msg.err, "")
	case screenDoneMsg:
		return a.next()
	}
	var cmd tea.Cmd
	a.screen, cmd = a.screen.Update(msg)
	return a, cmd
}

// next decides where to go when the current screen finishes.
func (a appModel) next() (tea.Model, tea.Cmd) {
	switch s := a.screen.(type) {
	case homeModel:
		if s.action == actionHome {
			return a, tea.Quit
		}
		screen, cmd := a.open(s.action)
		return a.show(screen, cmd)
	case model:
		if s.selected != "" {
			// Two statements: play updates a, and show must see that update.
			screen := a.play(a.newContext(), s.selected, "sub")
			return a.show(screen, nil)
		}
		return a.goHome(s.err, "")
	case infoModel:
		if s.selected != nil {
			screen := a.play(a.newContext(), s.selected.Title, s.selected.Language)
			return a.show(screen, nil)
		}
	case playModel:
		return a.goHome(s.err, s.notice)
	}
	return a.goHome(nil, "")
}

// newContext cancels the previous screen's background work and starts fresh.
func (a *appModel) newContext() context.Context {
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	return ctx
}

func (a appModel) View() string { return a.screen.View() }

// playModel asks the service for the episode, then waits while mpv plays it
// in its own window.
type playModel struct {
	ctx     context.Context
	title   string
	result  playResult
	playing bool
	spinner spinner.Model
	notice  string
	err     error
	request tea.Cmd
}

// Both messages carry the screen's context so a newer play screen ignores
// results that belong to one the user already left.
type playResolvedMsg struct {
	ctx    context.Context
	result playResult
	err    error
}
type playFinishedMsg struct {
	ctx context.Context
	err error
}

func newPlayModel(ctx context.Context, title, language, episode string) playModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return playModel{ctx: ctx, title: title, spinner: s, request: func() tea.Msg {
		result, err := requestPlay(ctx, title, language, episode)
		return playResolvedMsg{ctx, result, err}
	}}
}

func (m playModel) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.request) }

func (m playModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case playResolvedMsg:
		if msg.ctx != m.ctx {
			return m, nil
		}
		if msg.err != nil || msg.result.Rewound {
			m.err = msg.err
			if msg.err == nil {
				m.notice = msg.result.rewoundText()
			}
			return m, screenDone
		}
		m.result, m.playing = msg.result, true
		ctx := m.ctx
		return m, func() tea.Msg { return playFinishedMsg{ctx, playInBackground(ctx, msg.result.mpvArgs()...)} }
	case playFinishedMsg:
		if msg.ctx != m.ctx {
			return m, nil
		}
		m.err = msg.err
		if msg.err == nil {
			m.notice = fmt.Sprintf("Watched %s, episode %d", m.result.Title, m.result.Episode)
		}
		return m, screenDone
	case tea.KeyMsg:
		if s := msg.String(); s == "esc" || s == "q" || s == "ctrl+c" {
			return m, screenDone // the app cancels the context, which closes mpv
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m playModel) View() string {
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	if !m.playing {
		what := "the next episode"
		if m.title != "" {
			what = m.title
		}
		return renderHeader() + m.spinner.View() + " Loading " + what + "…\n\n" + muted.Render("Esc Cancel")
	}
	return renderHeader() + accent.Render(fmt.Sprintf("▶ %s, episode %d", m.result.Title, m.result.Episode)) +
		"\n\n" + muted.Render("Playing in the mpv window. Close it to come back here.") +
		"\n\n" + muted.Render("Esc Stop playback")
}
