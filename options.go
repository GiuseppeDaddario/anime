package main

import (
	"fmt"
	"strconv"
	"strings"
)

type cliAction int

const (
	actionHome cliAction = iota
	actionContinue
	actionBrowse
	actionInfo
	actionAdd
	actionPlayer
	actionLink
	actionLogout
)

type cliOptions struct {
	action     cliAction
	episode    string
	verbose    bool
	help       bool
	version    bool
	playerArgs []string
}

func parseOptions(args []string) (cliOptions, error) {
	options := cliOptions{}
	episodeSet := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		action := actionHome
		switch arg {
		case "--version":
			options.version = true
		case "-h", "--help":
			options.help = true
		case "-v", "--verbose":
			options.verbose = true
		case "--continue":
			action = actionContinue
		case "--anime", "--tui":
			action = actionBrowse
		case "--info":
			action = actionInfo
		case "--add":
			action = actionAdd
		case "--player":
			if options.action != actionHome || episodeSet {
				return options, fmt.Errorf("--player cannot be combined with other actions or --ep")
			}
			if i+1 == len(args) {
				return options, fmt.Errorf("Usage: anime --player [mpv options] <video file or URL>")
			}
			options.action, options.playerArgs = actionPlayer, args[i+1:]
			return options, nil
		case "--ep":
			if i+1 == len(args) {
				return options, fmt.Errorf("--ep requires an episode number")
			}
			i++
			options.episode, episodeSet = args[i], true
		default:
			if strings.HasPrefix(arg, "--ep=") {
				options.episode, episodeSet = strings.TrimPrefix(arg, "--ep="), true
			} else {
				return options, fmt.Errorf("Unknown option %q. Run anime --help", arg)
			}
		}
		if action != actionHome {
			if options.action != actionHome && options.action != action {
				return options, fmt.Errorf("Choose one action: --continue, --anime, --info, or --add")
			}
			options.action = action
		}
	}
	if episodeSet {
		ep, err := strconv.Atoi(options.episode)
		if err != nil || (ep < 1 && ep != -1) {
			return options, fmt.Errorf("--ep must be a positive episode number or -1")
		}
		if options.action == actionAdd {
			return options, fmt.Errorf("--ep cannot be combined with --add")
		}
		if options.action == actionHome {
			options.action = actionContinue
		}
	}
	return options, nil
}
