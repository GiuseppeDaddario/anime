# Homebrew releases

Move this folder into a separate repository, e.g. `GiuseppeDaddario/anime`.
Keep `go.mod` at the root. The optional `.github/workflows/` automate releases
for this standalone repository; they are not bot deployment workflows.
Do not commit ignored `bin/`, `dist/`, session settings or credentials.
Licensed under MIT (`LICENSE`).

The backend is the deployed Telegram bot (`GiuseppeDaddario/AnimeWatcherBot`); the
CLI never has a service of its own. Verify sign-in with `/login` and the one-time code.

Create an initialized public tap repository, e.g. `GiuseppeDaddario/homebrew-tap`.
Set these GitHub Actions variables in the client repository:

| Variable | Value |
| --- | --- |
| `ANIME_API_URL` | `https://animewatcherbot-izrakwm6rq-ew.a.run.app` (the bot's service) |
| `HOMEBREW_TAP_REPOSITORY` | `GiuseppeDaddario/homebrew-tap` |

For automatic tap updates, add the Actions secret `HOMEBREW_TAP_TOKEN`: a scoped
GitHub token with Contents write permission on the tap. Alternatively leave the
tap variable unset and copy the release's `anime.rb` into `Formula/anime.rb`.

Push a semantic version tag such as `v0.1.0`. The release workflow:

1. Checks the deployed API's health/protocol.
2. Runs client and Lua tests and builds natively on Apple Silicon and Intel.
3. Generates the formula from the actual archive SHA-256 checksums.
4. Installs and tests both archives through temporary Homebrew taps, including
   a headless mpv playback check.
5. Publishes the archives/formula, then optionally updates the tap.

No client release is published before those checks pass. Each archive contains
only `anime`; the formula installs mpv. Users then run:

```sh
brew install GiuseppeDaddario/tap/anime
anime
```

Future releases use `brew upgrade anime`. End users never set the API URL or run
pip/browser installation commands. They only link their Telegram account in the
home menu.

## Without workflow automation

The same release tools can be run manually from this folder:

```sh
python3 scripts/check-api.py https://animewatcherbot-izrakwm6rq-ew.a.run.app
python3 scripts/build-release.py --version v0.1.0 --api-url https://animewatcherbot-izrakwm6rq-ew.a.run.app --arch arm64
python3 scripts/build-release.py --version v0.1.0 --api-url https://animewatcherbot-izrakwm6rq-ew.a.run.app --arch amd64
python3 scripts/generate-formula.py --repository OWNER/REPOSITORY --version v0.1.0
```

Upload the two archives under `dist/` to that version's GitHub release, and put
`dist/anime.rb` under `Formula/anime.rb` in the tap repository. This is maintainer
packaging; users still only run `brew install` and `anime`.
