# AnimeWatcher: CLI + bot

Two local repos make up one product. Read this whole file before changing either of them.

| Repo | Path | What it is |
|---|---|---|
| CLI | `/Users/giuseppedaddario/Documents/AnimeWatcherCLI` | Go TUI `anime` + mpv player (`player/skip_intro.lua`). Shipped via Homebrew. |
| Bot | `/Users/giuseppedaddario/Documents/AnimeWatcherBot` | Telegram bot (Python). **Already deployed and working.** Git remote `GiuseppeDaddario/AnimeWatcherBot`, branch `main`. |

## Ground rules (from the owner)

1. **The bot is the one and only backend.** It is already deployed, it already has the
   database, Playwright/Chromium and the scraping code. The CLI must use *that*.
2. **No new services.** Never deploy `anime-cli-backend` or any other Cloud Run
   service, function, project, or database. Never run `backend/scripts/deploy-backend.sh`.
3. **No duplicated code.** Python server code lives only in the bot repo. The CLI repo
   holds only Go/Lua client code and its packaging.
4. **Never commit or push the bot repo unless explicitly asked.** A push to the bot's
   `main` triggers its Google-managed deployment, so it changes the live bot. Leave bot
   changes uncommitted for the owner to review. The CLI repo is `GiuseppeDaddario/anime`
   (public); a `v*` tag push runs the Homebrew release into `GiuseppeDaddario/homebrew-tap`.
5. Don't change GCP config (env vars, secrets, IAM) yourself. Tell the owner the exact
   command, or ask first.

## Production facts

- GCP/Firebase project: `botanime-b8262`, region `europe-west1`.
- Bot Cloud Run service: `animewatcherbot`, URL `https://animewatcherbot-izrakwm6rq-ew.a.run.app`.
  (The local default gcloud project is an unrelated one, `vacanzapp-4bc14`. Always pass `--project botanime-b8262`.)
- Bot image: `mcr.microsoft.com/playwright/python` (`Dockerfile`), entry point
  `functions-framework --target=telegram_bot --port=8080`, so `main.py:telegram_bot(request)`
  receives **every** HTTP request on any path, as a Flask `request`.
  - `GET` re-registers the Telegram webhook at `/telegram_bot`.
  - `POST` with a Telegram update gets dispatched to python-telegram-bot handlers.
- DB access in the bot: `includes/firebase.py` initialises `firebase_admin` with
  `./certificate.json` and exposes `db_firestore` (Firestore) and `db_realtime` (RTDB).
  Reuse these, and never create a second client or credential.
  - ⚠️ `certificate.json` is tracked in the bot's git. If the repo is public, that is a leaked
    service-account key. Mention it to the owner, but don't change it unasked.
- Scraping: bot `includes/data.py` (animeworld, Playwright). It downloads Chromium to
  `/tmp/pw-browsers` at runtime. That works in production, so leave it.
- Login: bot command `/login` (`src/commands/CliLogin.py`, registered in `main.py`)
  writes the hash of a one-time code to Firestore `cli_logins/{sha256(code)}`, rate-limited
  via `cli_limits`. The code is `XXXX-XXXX-XXXX-XXXX-XXXX` and expires after 10 minutes.
  - **Pending, uncommitted change in the bot repo:** `/login` now sends two messages: an
    explanation, then the bare code alone so it can be copied with one tap.

## How the CLI talks to the backend

- `api.go`: the base URL is `ANIMEWATCHER_API_URL` env, else `defaultAPIURL`, which is injected at
  build time (`make build API_URL=...`; release: `scripts/build-release.py --api-url`).
  Empty base gives the error *"The service address is not configured in this build"*. HTTPS
  is required except on loopback. Session stored in `~/.config/animewatcher/config.json`,
  bound to the base URL.
- Endpoints the client calls (contract, protocol 1):
  - `GET  /v1/healthz` (Cloud Run reserves `/healthz`) returns `{"status":"ok","protocol":1}` (checked by `scripts/check-api.py`, release CI).
  - `POST /v1/auth/redeem` `{code}` returns a session token (no auth).
  - `GET  /v1/me`, `POST /v1/logout`
  - `POST /v1/rpc/{catalog|search|add|play|info}`
  - `POST /v1/info/stream` returns NDJSON stream.
  - All except redeem/healthz send `Authorization: Bearer <token>`. The server derives the user
    from the session, and payloads must not contain `user_id`.
- The server implementation lives in the bot: `src/cli_api/` (`app.py` `create_app(database, secret, operations)`,
  `auth.py`, `operations.py`) + `includes/terminal_info.py`, routed from `main.py:telegram_bot` for
  `/v1/*` (503 if `CLI_SESSION_SECRET` is unset). Bot tests:
  `python -m unittest discover -s tests` from the bot root, in a scratchpad venv. Firestore/RTDB rules: bot `firebase/`.
- Bot changes are kept minimal (owner decision): the scraper `includes/data.py` stays untouched,
  and the CLI API adapts to it.
- Migration deployed 2026-11-21: bot revision `animewatcherbot-00040-57v` has `CLI_SESSION_SECRET`,
  `check-api.py` passes. Release CI variable `ANIME_API_URL` = `https://animewatcherbot-izrakwm6rq-ew.a.run.app`.

## Running the CLI locally

`bin/` isn't on PATH. Use `./bin/anime`, or once:
`ln -sf "$PWD/bin/anime" /opt/homebrew/bin/anime`.

## Other context

- Skip intro: `player/skip_intro.lua` asks AniSkip (MAL id + episode). Many anime are missing
  there. Proposed fallback (not built yet): compare the audio of two episodes of the same season
  (ffmpeg on the first ~5 minutes of the stream + Chromaprint `fpcalc -raw`, longest common run of 60–100 s),
  cache the result, and pass `intro_start`/`intro_end` to the Lua script.
