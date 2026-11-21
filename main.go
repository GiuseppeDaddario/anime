package main

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"os"
)

const usage = `Usage: anime [options]

  (no options)   Open the home menu and account settings
  --continue     Continue the last anime you watched
  --anime, --tui Browse the catalogue and select an anime
  --info         Open your history and the full database
  --add          Search for and add an anime
  --ep N         Play episode N; -1 moves progress back one episode
  --player ...   Open a file or URL with mpv (following options go to mpv)
  -v, --verbose  Show backend diagnostics
  --version      Show the installed version
  -h, --help     Show this help

Link your Telegram account from the anime home menu. No Python setup is needed.`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	options, err := parseOptions(args)
	if err != nil {
		return err
	}
	Verbose = options.verbose
	if options.version {
		fmt.Println("anime " + version)
		return nil
	}
	if options.help {
		fmt.Println(usage)
		return nil
	}
	if options.action == actionPlayer {
		return runMPV(options.playerArgs...)
	}
	if options.action != actionHome {
		return runAction(options)
	}
	// Every completed/cancelled flow returns to the home menu.
	var lastError error
	for {
		result, err := tea.NewProgram(initHomeModel(lastError), tea.WithAltScreen()).Run()
		if err != nil {
			return fmt.Errorf("home menu: %w", err)
		}
		home := result.(homeModel)
		if home.action == actionHome {
			return nil
		}
		lastError = runAction(cliOptions{action: home.action, verbose: options.verbose})
	}
}

func runAction(options cliOptions) error {
	if options.action == actionLink {
		return linkTelegramAccount()
	}
	if options.action == actionLogout {
		return logoutAccount()
	}
	play := func(title, language string) error {
		return remotePlay(context.Background(), title, language, options.episode)
	}
	switch options.action {
	case actionContinue:
		return play("", "")
	case actionAdd:
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, err := tea.NewProgram(initAddModel(ctx), tea.WithAltScreen()).Run()
		return err
	case actionInfo:
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m := initInfoModel()
		go readInfoStream(ctx, m.events)
		result, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
		cancel()
		for range m.events {
		} // Wait until the background HTTP stream stops.
		if err != nil {
			return err
		}
		if info := result.(infoModel); info.selected != nil {
			return play(info.selected.Title, info.selected.Language)
		}
		return nil
	case actionBrowse:
		fmt.Println("Caricamento catalogo... (Attendere)")
		items, err := fetchAnimeList(context.Background())
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return fmt.Errorf("The catalogue is empty")
		}
		result, err := tea.NewProgram(initModel(items), tea.WithAltScreen()).Run()
		if err != nil {
			return err
		}
		if selected := result.(model).selected; selected != "" {
			return play(selected, "sub")
		}
		return nil
	}
	return fmt.Errorf("unknown action")
}
