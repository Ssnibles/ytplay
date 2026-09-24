package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// searchMsg delivers one yt-dlp search batch. A "more" batch refetches the
// query with a larger limit and is merged onto the results already shown, so
// scrolling can page deeper into the channel the way a web search does.
type searchMsg struct {
	videos []video
	err    error
	limit  int
	more   bool
}

type thumbMsg struct {
	id   string
	art  string
	cols int
	rows int
	err  error
}

type detailMsg struct {
	id  string
	det videoDetail
	err error
}

type copyMsg struct {
	url string
	err error
}

type openMsg struct {
	url string
	err error
}

// searchCmd fetches results via yt-dlp's ytsearchN: syntax. N is the total
// count asked for: "more" batches re-ask with a bigger N (yt-dlp pages
// internally) and the results are deduplicated by ID on merge.
func searchCmd(query string, limit int, more bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "yt-dlp",
			"--flat-playlist", "--skip-download", "--no-warnings", "--no-playlist",
			"-J", fmt.Sprintf("ytsearch%d:%s", limit, query))
		out, err := cmd.Output()
		if err != nil {
			return searchMsg{err: fmt.Errorf("yt-dlp: %w", err), limit: limit, more: more}
		}

		var res struct {
			Entries []video `json:"entries"`
		}
		if err := json.Unmarshal(out, &res); err != nil {
			return searchMsg{err: fmt.Errorf("parse: %w", err), limit: limit, more: more}
		}

		videos := make([]video, 0, len(res.Entries))
		seen := make(map[string]bool, len(res.Entries))
		for _, v := range res.Entries {
			if v.ID == "" || v.Title == "" || seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			videos = append(videos, v)
		}
		if len(videos) == 0 {
			// a follow-up page hitting the end comes back empty — that's not
			// an error, it just means there is nothing more to load.
			if more {
				return searchMsg{limit: limit, more: more}
			}
			return searchMsg{err: fmt.Errorf("no results"), limit: limit, more: more}
		}
		return searchMsg{videos: videos, limit: limit, more: more}
	}
}

// mergeResults appends the videos in extra that aren't already in base,
// keeping base's order (and YouTube's order within extra).
func mergeResults(base, extra []video) []video {
	if len(base) == 0 {
		return extra
	}
	seen := make(map[string]bool, len(base)+len(extra))
	for _, v := range base {
		seen[v.ID] = true
	}
	merged := make([]video, len(base), len(base)+len(extra))
	copy(merged, base)
	for _, v := range extra {
		if !seen[v.ID] {
			seen[v.ID] = true
			merged = append(merged, v)
		}
	}
	return merged
}

// detailCmd extracts channel/video stats for one video. A full (non-flat)
// extraction is needed, so this runs for the selected video only.
func detailCmd(v video) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, "yt-dlp",
			"--no-playlist", "--skip-download", "--no-warnings",
			"-J", v.watchURL()).Output()
		if err != nil {
			return detailMsg{v.ID, videoDetail{}, err}
		}

		var d struct {
			Subs       *int64 `json:"channel_follower_count"`
			ChViews    *int64 `json:"channel_view_count"`
			Views      *int64 `json:"view_count"`
			Likes      *int64 `json:"like_count"`
			Date       string `json:"upload_date"`
			ChannelURL string `json:"channel_url"`
			ChannelID  string `json:"channel_id"`
		}
		if err := json.Unmarshal(out, &d); err != nil {
			return detailMsg{v.ID, videoDetail{}, err}
		}
		return detailMsg{v.ID, videoDetail{
			subs: d.Subs, chViews: d.ChViews, views: d.Views, likes: d.Likes,
			uploaded: d.Date, channelURL: d.ChannelURL, channelID: d.ChannelID,
		}, nil}
	}
}

func thumbCmd(v video, cols, rows int, proto imgProto) tea.Cmd {
	return func() tea.Msg {
		art, err := renderThumb(v, cols, rows, proto)
		if err != nil {
			return thumbMsg{v.ID, "", cols, rows, err}
		}
		return thumbMsg{v.ID, art, cols, rows, nil}
	}
}

// copyURLCmd writes url to the system clipboard (yank) and reports back so the
// TUI can confirm without blocking on the clipboard service.
func copyURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(url); err != nil {
			return copyMsg{url, err}
		}
		return copyMsg{url, nil}
	}
}

// openChannelCmd opens a URL in the system browser (via xdg-open) off the TUI.
func openChannelCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return openMsg{"", fmt.Errorf("no channel available")}
		}
		devnull, err := os.Open(os.DevNull)
		if err != nil {
			return openMsg{url, err}
		}
		defer devnull.Close()
		cmd := exec.Command("xdg-open", url)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
		if err := cmd.Run(); err != nil {
			return openMsg{url, err}
		}
		return openMsg{url, nil}
	}
}

// openChannel returns the browser command for the selected video, using the
// detail lookup for its channel URL when the flat search entry lacks one.
func (m model) openChannel(v video) tea.Cmd {
	if v.ChannelID != "" {
		return openChannelCmd("https://www.youtube.com/channel/" + v.ChannelID)
	}
	if d, ok := m.details[v.ID]; ok {
		if url := d.channelLink(); url != "" {
			return openChannelCmd(url)
		}
	}
	return openChannelCmd("")
}

func playInMPV(urls ...string) error {
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("mpv: %w", err)
	}
	defer devnull.Close()

	cmd := exec.Command("mpv", urls...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Detach mpv from the TUI's streams: stdin would otherwise receive the
	// keystrokes the TUI is listening for, and mpv's own output would print
	// into the alternate screen and desync the cell-anchored image layout.
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mpv: %w", err)
	}
	return nil
}
