# anime

The native TUI/player and its Homebrew release tooling. The backend is the
deployed Telegram bot (`GiuseppeDaddario/AnimeWatcherBot`), which serves the CLI API
on `/v1/*` next to its webhook.

## Install and sign in

After the first release is published:

```sh
brew install GiuseppeDaddario/tap/anime
anime
```

Homebrew installs mpv. Users never install Python, Chromium or Firebase keys.
Choose **Link Telegram account**, send `/login` privately to the bot, and paste
the returned one-time code into the TUI. Use the menu again to change accounts;
**Sign out** revokes the current session.

| Command | Action |
| --- | --- |
| `anime` | Home and account settings |
| `anime --continue` | Resume the current anime |
| `anime --anime` / `anime --tui` | Browse the catalogue |
| `anime --info` | History, database and details |
| `anime --add` | Search and add an anime |
| `anime --ep 3` | Play episode 3 of the current anime |
| `anime --ep -1` | Rewind progress one episode |
| `anime --player FILE` | Play a file or URL without signing in |
| `anime --help` / `anime --version` | Help / version |

Video plays locally in mpv. Scraping, Chromium and database access are handled by
the bot's service. See [PLAYER.md](PLAYER.md) for playback controls.

## Build, check and release

```sh
make build
./bin/anime
make test
```

Building requires Go. Maintainer tests also use Python's standard library and
Lua; those are not client runtime requirements. `make build` points at the bot by
default; override with `make build API_URL=...`. Server code, its tests and the
Firebase rules live in the bot repository. Homebrew release instructions are in
[RELEASING.md](RELEASING.md).

The `.github/` workflows are optional checks/release automation for this repository.
`bin/`, `dist/`, caches
and private settings are ignored by Git. Release archives contain only `anime`;
Homebrew supplies mpv, and macOS supplies curl/Quick Look.

The backend address is embedded in releases. `ANIMEWATCHER_API_URL` overrides it
for development; HTTPS is required except on loopback. Session settings are
stored privately under `~/.config/animewatcher/config.json` (or `XDG_CONFIG_HOME`).
Tokens are bound to their backend origin.
