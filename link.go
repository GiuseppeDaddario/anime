package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type loginResultMsg struct {
	config userConfig
	err    error
}
type linkModel struct {
	ctx    context.Context
	input  textinput.Model
	saving bool
	linked bool
	userID int64
	err    error
}

func newLinkModel(ctx context.Context) linkModel {
	input := textinput.New()
	input.Placeholder = "XXXX-XXXX-XXXX-XXXX-XXXX"
	input.Width = 30
	input.CharLimit = 64
	input.Focus()
	return linkModel{ctx: ctx, input: input}
}
func (m linkModel) Init() tea.Cmd { return textinput.Blink }
func normalizeLoginCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(code), "-", ""), " ", ""))
}
func (m linkModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case loginResultMsg:
		m.saving = false
		m.err = msg.err
		if msg.err == nil {
			m.linked = true
			m.userID = msg.config.UserID
			m.input.SetValue("")
			m.input.Blur()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.linked {
				return m, tea.Quit
			}
			if m.saving {
				return m, nil
			}
			code := normalizeLoginCode(m.input.Value())
			if len(code) != 20 || strings.IndexFunc(code, func(c rune) bool { return !strings.ContainsRune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", c) }) >= 0 {
				m.err = fmt.Errorf("Paste the complete code sent by /login in Telegram")
				return m, nil
			}
			m.saving = true
			m.err = nil
			return m, func() tea.Msg {
				var response struct {
					Token     string `json:"token"`
					UserID    int64  `json:"user_id"`
					ExpiresAt string `json:"expires_at"`
				}
				err := apiJSON(m.ctx, http.MethodPost, "/v1/auth/redeem", map[string]string{"code": code}, false, &response)
				config := userConfig{}
				if err == nil {
					base, _ := apiBaseURL()
					if response.UserID <= 0 || len(response.Token) < 32 {
						err = fmt.Errorf("The service returned an invalid session")
					} else {
						config = userConfig{UserID: response.UserID, Token: response.Token, APIURL: base, ExpiresAt: response.ExpiresAt}
						err = writeUserConfig(config)
					}
				}
				return loginResultMsg{config, err}
			}
		}
	}
	if !m.saving && !m.linked {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(message)
		return m, cmd
	}
	return m, nil
}
func (m linkModel) View() string {
	text := renderHeader() + "Link Telegram account\n\n"
	if m.linked {
		return text + fmt.Sprintf("Linked to Telegram ID %d. Your history is ready.\n\nEnter Return home\n", m.userID)
	}
	text += "1. Send /login in your private chat with the bot.\n2. Paste the one-time code here.\n\n" + m.input.View() + "\n\n"
	if m.err != nil {
		text += m.err.Error() + "\n\n"
	}
	if m.saving {
		return text + "Linking… · Esc Cancel\n"
	}
	return text + "Enter Link · Esc Back\n"
}
func linkTelegramAccount() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := tea.NewProgram(newLinkModel(ctx), tea.WithAltScreen()).Run()
	return err
}
func logoutAccount() error {
	config, err := readUserConfig()
	if err != nil {
		return err
	}
	if config.Token == "" {
		return writeUserConfig(userConfig{})
	}
	var response any
	if err := apiJSON(context.Background(), http.MethodPost, "/v1/logout", map[string]any{}, true, &response); err != nil && !unauthorized(err) {
		return err
	}
	return writeUserConfig(userConfig{})
}
