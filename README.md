# ytplay

TUI YouTube search & play built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). Search YouTube from your terminal, preview the results with inline thumbnails, and play the selected video in mpv.

## Features

- Two-pane results view: keyboard-scrollable list with a right-aligned duration column, plus a preview pane with a rendered thumbnail and channel/video stats
- Infinite paging: keep scrolling near the bottom to fetch more results (deduped by ID)
- `Enter` on a result spawns mpv detached — the TUI stays open so you can queue the next video
- Play queue: `a` stages videos, `p` plays the queue in sequence
- Search history: past queries are persisted and recalled with `Ctrl+P` / `Ctrl+N`
- `c` copies the selected video's URL, `o` opens its channel in your browser
- Native image rendering when supported (kitty unicode placeholders / sixel), half-block ANSI art as fallback

## Usage

```
nix run .# -- <query>
```

or, once installed as a system package:

```
ytplay <query>
```

### Keys

| Key | Prompt view |
| --- | --- |
| `Enter` | search |
| `Ctrl+P` / `Ctrl+N` | recall past searches |

| Key | Results view |
| --- | --- |
| `j` / `k` / `↓` / `↑` | move selection |
| `Tab` / `Shift+Tab` | jump between panes |
| `PgUp` / `PgDn` / `j`/`k` near bottom | page & load more results |
| `Enter` | play selected video (mpv, detached) |
| `a` | add selected video to play queue |
| `p` | play all queued videos in sequence |
| `c` | copy video URL to clipboard |
| `o` | open channel in browser |
| `Esc` | back to prompt (new search) |
| `Ctrl+C` / `Ctrl+D` | quit |

Search history is stored in `~/.config/ytplay/history`.

## Nix

`flake.nix` exposes `packages.<system>.default` (a `buildGoModule`), wrapping the binary so `yt-dlp` and mpv are on `PATH`. Standalone:

```sh
nix build .
```

Consumed as a flake input in your main configuration (`inputs.ytplay.packages.${system}.default`).

## Development

```sh
go test ./...
nix run .# -- "some query"
```

Requires `yt-dlp` (`ytsearch`), `mpv`, and `xdg-open` (for the channel
shortcut) at runtime.
