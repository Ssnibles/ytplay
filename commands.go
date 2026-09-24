package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// searchMsg delivers one yt-dlp search batch. A "more" batch refetches the
// query with a larger limit and is merged onto the results already shown, so
// scrolling can page deeper into the channel the way a web search does.
type searchMsg struct {
	query  string
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

type channelMsg struct {
	channelTitle string
	channelURL   string
	videos       []video
	err          error
	limit        int
	more         bool
}

// prioritizeChannels returns a slice where any channel entries appear first,
// preserving the relative order of channels and non-channel videos.
func prioritizeChannels(videos []video) []video {
	if len(videos) <= 1 {
		return videos
	}
	channels := make([]video, 0, len(videos))
	nonChannels := make([]video, 0, len(videos))
	for _, v := range videos {
		if v.isChannel() {
			channels = append(channels, v)
		} else {
			nonChannels = append(nonChannels, v)
		}
	}
	if len(channels) == 0 || len(nonChannels) == 0 {
		return videos
	}
	return append(channels, nonChannels...)
}

// searchCmd fetches results via yt-dlp. It queries YouTube search results
// (falling back to ytsearch syntax) up to limit entries, and ensures that any
// channel found in the results is prioritized at the top.
func searchCmd(query string, limit int, more bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		q := strings.TrimSpace(query)
		searchTarget := q
		if !strings.HasPrefix(q, "http://") && !strings.HasPrefix(q, "https://") {
			searchTarget = "https://www.youtube.com/results?search_query=" + url.QueryEscape(q)
		}

		cmd := exec.CommandContext(ctx, "yt-dlp",
			"--flat-playlist", "--skip-download", "--no-warnings",
			fmt.Sprintf("--playlist-end=%d", limit),
			"-J", searchTarget)
		out, err := cmd.Output()
		if err != nil {
			cmd = exec.CommandContext(ctx, "yt-dlp",
				"--flat-playlist", "--skip-download", "--no-warnings", "--no-playlist",
				"-J", fmt.Sprintf("ytsearch%d:%s", limit, query))
			out, err = cmd.Output()
			if err != nil {
				return searchMsg{query: query, err: fmt.Errorf("yt-dlp: %w", err), limit: limit, more: more}
			}
		}

		var res struct {
			Entries []video `json:"entries"`
		}
		if err := json.Unmarshal(out, &res); err != nil {
			return searchMsg{query: query, err: fmt.Errorf("parse: %w", err), limit: limit, more: more}
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
		videos = prioritizeChannels(videos)
		if len(videos) == 0 {
			// a follow-up page hitting the end comes back empty — that's not
			// an error, it just means there is nothing more to load.
			if more {
				return searchMsg{query: query, limit: limit, more: more}
			}
			return searchMsg{query: query, err: fmt.Errorf("no results"), limit: limit, more: more}
		}
		return searchMsg{query: query, videos: videos, limit: limit, more: more}
	}
}

// mergeResults appends the videos in extra that aren't already in base,
// keeping base's order (and YouTube's order within extra), and always
// prioritizing any channels at the top.
func mergeResults(base, extra []video) []video {
	if len(base) == 0 {
		return prioritizeChannels(extra)
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
	return prioritizeChannels(merged)
}

// detailCmd extracts channel/video stats for one video. A full (non-flat)
// extraction is needed, so this runs for the selected video only.
func detailCmd(v video) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var cmd *exec.Cmd
		if v.isChannel() {
			cmd = exec.CommandContext(ctx, "yt-dlp",
				"--flat-playlist", "--playlist-end", "1", "--skip-download", "--no-warnings",
				"-J", v.channelTargetURL())
		} else {
			cmd = exec.CommandContext(ctx, "yt-dlp",
				"--no-playlist", "--skip-download", "--no-warnings",
				"-J", v.watchURL())
		}
		out, err := cmd.Output()
		if err != nil {
			return detailMsg{v.ID, videoDetail{}, err}
		}

		var d struct {
			Subs        *int64 `json:"channel_follower_count"`
			ChViews     *int64 `json:"channel_view_count"`
			Views       *int64 `json:"view_count"`
			Likes       *int64 `json:"like_count"`
			Date        string `json:"upload_date"`
			ChannelURL  string `json:"channel_url"`
			ChannelID   string `json:"channel_id"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(out, &d); err != nil {
			return detailMsg{v.ID, videoDetail{}, err}
		}
		desc := d.Description
		if desc == "" {
			desc = v.Description
		}
		subs := d.Subs
		if subs == nil {
			subs = v.Followers
		}
		return detailMsg{v.ID, videoDetail{
			subs: subs, chViews: d.ChViews, views: d.Views, likes: d.Likes,
			uploaded: d.Date, channelURL: d.ChannelURL, channelID: d.ChannelID,
			description: desc,
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

// channelVideosURL formats a channel URL, handle, or ID into its /videos endpoint.
func channelVideosURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "UC") {
		return "https://www.youtube.com/channel/" + u + "/videos"
	}
	if strings.HasPrefix(u, "@") {
		return "https://www.youtube.com/" + u + "/videos"
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return "https://www.youtube.com/" + u + "/videos"
	}
	u = strings.TrimRight(u, "/")
	if strings.HasSuffix(u, "/videos") {
		return u
	}
	return u + "/videos"
}

// channelCmd fetches a batch of videos from a channel using yt-dlp.
func channelCmd(channelTitle, channelURL string, limit int, more bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		vURL := channelVideosURL(channelURL)
		cmd := exec.CommandContext(ctx, "yt-dlp",
			"--flat-playlist", "--skip-download", "--no-warnings",
			"-J", fmt.Sprintf("--playlist-end=%d", limit), vURL)
		out, err := cmd.Output()
		if err != nil {
			return channelMsg{channelTitle: channelTitle, channelURL: channelURL, err: fmt.Errorf("yt-dlp: %w", err), limit: limit, more: more}
		}

		var res struct {
			Title   string  `json:"title"`
			Channel string  `json:"channel"`
			Entries []video `json:"entries"`
		}
		if err := json.Unmarshal(out, &res); err != nil {
			return channelMsg{channelTitle: channelTitle, channelURL: channelURL, err: fmt.Errorf("parse: %w", err), limit: limit, more: more}
		}

		title := channelTitle
		if title == "" {
			if res.Channel != "" {
				title = res.Channel
			} else if res.Title != "" {
				title = strings.TrimSuffix(res.Title, " - Videos")
			}
		}

		videos := make([]video, 0, len(res.Entries))
		seen := make(map[string]bool, len(res.Entries))
		for _, v := range res.Entries {
			if v.ID == "" || v.Title == "" || seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			if v.Channel == "" {
				v.Channel = title
			}
			videos = append(videos, v)
		}

		if len(videos) == 0 {
			if more {
				return channelMsg{channelTitle: title, channelURL: channelURL, limit: limit, more: more}
			}
			return channelMsg{channelTitle: title, channelURL: channelURL, err: fmt.Errorf("no videos found"), limit: limit, more: more}
		}

		return channelMsg{channelTitle: title, channelURL: channelURL, videos: videos, limit: limit, more: more}
	}
}

// openURLCmd opens a URL in the system browser (via xdg-open) off the TUI.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return openMsg{"", fmt.Errorf("no URL available")}
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

// openVideo returns the browser command to open the selected video directly in the browser.
func (m model) openVideo(v video) tea.Cmd {
	return openURLCmd(v.watchURL())
}

// resolveChannelURL finds the canonical channel URL for a video.
func (m model) resolveChannelURL(v video) string {
	if u := v.channelTargetURL(); u != "" {
		return u
	}
	if d, ok := m.details[v.ID]; ok {
		if url := d.channelLink(); url != "" {
			return url
		}
	}
	return ""
}

// openChannel returns the browser command for the selected video's channel.
func (m model) openChannel(v video) tea.Cmd {
	return openURLCmd(m.resolveChannelURL(v))
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
