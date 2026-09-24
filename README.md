# ytplay

TUI YouTube search & play built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). Search YouTube from your terminal, preview the results with inline thumbnails, and play the selected video in mpv.

## Screenshots

<table width="100%">
  <!-- Top Row: Two images side-by-side -->
  <tr>
    <td width="50%"><img src="./Screenshots/Screenshot_2026-09-23_18-19-06.png" width="100%" alt="Top Left Image"></td>
    <td width="50%"><img src="./Screenshots/Screenshot_2026-09-23_18-19-22.png" width="100%" alt="Top Right Image"></td>
  </tr>
  <!-- Bottom Row: One image taking up the full width -->
  <tr>
    <td colspan="2" width="100%"><img src="./Screenshots/Screenshot_2026-09-23_18-19-28.png" width="100%" alt="Bottom Full Image"></td>
  </tr>
</table>

## Features

- Two-pane results view: keyboard-scrollable list, plus a preview pane with a rendered thumbnail and channel/video stats
- Infinite paging: keep scrolling near the bottom to fetch more results
- `Enter` on a result spawns mpv detached — the TUI stays open so you can queue the next video
- Play queue: `a` stages videos, `q` opens a queue page that reuses the same two-pane list + thumbnail/stats preview to reorder (`K`/`J`), remove (`x`), or play a single entry (`Enter`)
- Channel browsing: browse a channel's videos in a two-pane list with thumbnails and stats (`Enter` on a channel result, `C` or `Tab` + `Enter` on a video)
- Search history: past queries are persisted and recalled with `Ctrl+P` / `Ctrl+N`
- `c` copies the selected video's URL, `o` opens the video directly in your browser
- `/` always opens the search prompt, and `Esc` always navigates back to the previous view
- Native image rendering when supported (kitty unicode placeholders / sixel), half-block ANSI art as fallback

## Usage

```
ytplay # Open TUI
ytplay <query> # Search directly
```

### Keys

| Key                 | Prompt view          |
| ------------------- | -------------------- |
| `Enter`             | search               |
| `Ctrl+P` / `Ctrl+N` | recall past searches |
| `Esc`               | cancel / go back     |

| Key                                   | Results view                                      |
| ------------------------------------- | ------------------------------------------------- |
| `j` / `k` / `↓` / `↑`                 | move selection                                    |
| `Tab` / `Shift+Tab`                   | jump between list and channel preview             |
| `PgUp` / `PgDn` / `j`/`k` near bottom | page & load more results                          |
| `Enter`                               | play selected video (or browse channel if chosen) |
| `C`                                   | view selected video's channel                     |
| `a`                                   | add selected video to play queue                  |
| `q`                                   | open the queue view                               |
| `c`                                   | copy video URL to clipboard                       |
| `o`                                   | open video in browser                             |
| `/`                                   | open search prompt                                |
| `Esc`                                 | back to previous view (or prompt)                 |
| `Ctrl+C` / `Ctrl+D`                   | quit                                              |

| Key                                   | Channel view                        |
| ------------------------------------- | ----------------------------------- |
| `j` / `k` / `↓` / `↑`                 | move selection                      |
| `PgUp` / `PgDn` / `j`/`k` near bottom | page & load more channel videos     |
| `Enter`                               | play selected video (mpv, detached) |
| `a`                                   | add selected video to play queue    |
| `q`                                   | open the queue view                 |
| `c`                                   | copy video URL to clipboard         |
| `o`                                   | open video in browser               |
| `/`                                   | open search prompt                  |
| `Esc`                                 | back to previous view               |
| `Ctrl+C` / `Ctrl+D`                   | quit                                |

| Key                   | Queue view                                   |
| --------------------- | -------------------------------------------- |
| `j` / `k` / `↓` / `↑` | move selection                               |
| `K` / `J`             | move selected video up / down                |
| `x`                   | remove selected video from queue             |
| `Enter`               | play selected video and remove it from queue |
| `p`                   | play all queued videos in sequence           |
| `c`                   | copy selected video's URL                    |
| `o`                   | open video in browser                        |
| `/`                   | open search prompt                           |
| `Esc`                 | back to results / previous view              |
| `Ctrl+C` / `Ctrl+D`   | quit                                         |

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

Requires `yt-dlp` (`ytsearch`), `mpv`, and `xdg-open` (for the channel shortcut) at runtime.
