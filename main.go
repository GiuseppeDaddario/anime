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
	if options.action == actionContinue {
		// Plain command: print progress here and run mpv in this terminal.
		return remotePlay(context.Background(), "", "", options.episode)
	}
	// Everything else runs in one full-screen program until the user quits.
	result, err := tea.NewProgram(newApp(options.action, options.episode), tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	app := result.(appModel)
	if app.notice != "" {
		fmt.Println(app.notice)
	}
	return app.err
}
