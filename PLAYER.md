# Terminal interface

Build from the standalone CLI root with `make build`.
The executable is `bin/anime`; commands below assume it is on your PATH.

Run `anime --info` to open the information panel. It opens immediately and loads
your history and the SUB/ITA catalogues without waiting for video link checks.
Availability checks for ongoing watched anime run in the background and update
the screen as each anime finishes. Closing the panel stops outstanding checks.

- **Tab**, **1**, **2**: switch between **In visione** and **Tutto il database**.
- **/**: search the current section by title, language, or status. Press Enter to
  apply the filter, then Enter again to inspect an anime.
- **Enter**: open details, including status, episode total, last watched episode,
  next episode, availability, fillers, joint episodes, source page, and supervisor.
- **D** in details: inspect all original database fields, including custom fields.
- **Up/Down**, **Page Up/Page Down**: scroll the details.
- **Esc** in details: return to the list.
- **P**: play the highlighted anime in its selected language.
- **Space** in a list: preview the cover with macOS Quick Look.
- **Q** or **Ctrl+C**: exit.

Completed and unstarted anime remain searchable in the full database section.
Totals come from stored episode ranges, not from testing every episode. Unknown
totals (including the scraper's `999` placeholder) are shown as unknown; open
ranges are marked with `+`. Availability from the catalogue is labelled
separately from verified consecutive links starting after the last watched episode.
Checks stop at the first unavailable link, and counts capped at 100 link requests
are shown with `+`.

Progress is the last watched episode, not a count of individually watched
episodes. The current database stores history by title, without a language key:
identical titles in SUB and ITA therefore share progress. The details panel marks
these entries explicitly.

`anime --add` opens the add flow: enter `Nome Anime - lingua`, select a search
result, and press Enter to add it.

Rebuild the executable after source changes with `make build` from `cli/`.

## Video playback

Playback now adds the project's `player/skip_intro.lua` overlay to the installed
mpv. Its configuration, normal on-screen controls, keyboard shortcuts, subtitles,
and video output remain controlled by mpv.

The **Skip intro** button sits in the lower-right video area above the normal
controls, and only during the episode's opening. A click jumps exactly to the end
of the opening and dismisses the button; seeking back into the opening offers it
again. When the opening starts the button stays up for eight seconds, then returns
whenever the mouse moves, for four seconds, and while hovered or paused. It leaves
two seconds before the opening ends and has no mouse input area while hidden.
Audio-only files and unseekable streams are not skipped.

### Where the intro times come from

[AniSkip](https://api.aniskip.com/docs) is a free, community-made database of
anime opening times, keyed by MyAnimeList id. Each catalogue anime's id is read
once from the MyAnimeList link on its AnimeWorld page (`link_ID`) and saved as
`mal_id` on its Firestore document; 0 means the page links none. `anime` passes
the id and episode to mpv, and the overlay asks AniSkip with `curl` once the video
has loaded:

- episodes numbered on from an earlier season use AniSkip's relation rules, so
  Attack on Titan episode 30 becomes season 2 episode 5;
- a joint file (`Ep_003-004`) skips the opening of its first episode;
- times measured on a video of the same length come first, then any release's,
  and an opening that would end after the video is ignored.

mpv's terminal output reports the opening found, or that AniSkip has none. Without
one there is no button. Missing episodes can be added on AniSkip itself.

### Look

The button is [glisse](https://github.com/0kzh/glisse)'s dark Liquid Glass button:
its 40 px pill, 14 px label and 20 px icon, tint, hover and press colours, rim
light, press glow and press spring, scaled to the window. As glisse's
`backdrop-filter` does, the video under the button is blurred, bent by glisse's
lens along the rim (red further than blue) and saturated, by a GPU shader on mpv's
output. mpv's renderer takes no live shader parameters, so the overlay writes one
shader for the button's place and swaps it in only while the button is shown and
still. With `refraction=no`, or a video output without user shaders, the button
keeps its tint and rim over the plain video. The label uses SF Pro Text; without
it libass falls back to its default sans-serif.

### Options

Normal `anime` playback uses the overlay automatically. Open a local file or URL
directly, without touching watch history, with:

```sh
anime --player /path/to/video.mp4
anime --player --pause /path/to/video.mp4
```

Arguments after `--player` are passed to mpv, including the overlay's options:

```sh
# Look the intro up on AniSkip, as anime does
anime --player --script-opts-append=animewatcher-mal_id=59978 \
    --script-opts-append=animewatcher-episode=4 /path/to/video.mp4
# Or give it in seconds
anime --player --script-opts-append=animewatcher-intro_start=95 \
    --script-opts-append=animewatcher-intro_end=185 /path/to/video.mp4
```

| `animewatcher-…` | Default | Meaning |
| --- | --- | --- |
| `mal_id`, `episode` | 0 | MyAnimeList episode to ask AniSkip about |
| `intro_start`, `intro_end` | -1 | Intro in seconds, instead of AniSkip |
| `prompt` | 8 | Seconds the button stays up once the intro starts |
| `hide_after` | 4 | Seconds it stays after the mouse last moved |
| `always_visible` | no | Keep it up for the whole intro |
| `refraction` | yes | Bend and blur the video under the glass |
| `font` | SF Pro Text | Label font |

Set `ANIMEWATCHER_MPV` to the executable path if using a different mpv build.
The Lua source is embedded in the `anime` executable, so playback does not depend on
the source folder or install anything into the global mpv scripts directory.

Overlay checks:

```sh
lua tests/test_skip_intro.lua                     # From cli/
go test ./...
python3 -m unittest discover -s tests     # From cli/
```

`tests/check_mpv_overlay.py` also tests the rendered button against a running mpv
IPC socket and a disposable local clip (at least 30 seconds long) with a 2s → 12s
intro; its docstring has the command to start the player. It checks that the
button and its glass appear only in the intro, an exact 3s → 12s mouse seek,
dismissal and re-arming, outside clicks, pause, double-click handling, and normal
spacebar controls. It changes that test player's position.
