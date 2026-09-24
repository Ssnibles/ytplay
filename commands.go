package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
		} else if v.isPlaylist() {
			cmd = exec.CommandContext(ctx, "yt-dlp",
				"--flat-playlist", "--playlist-end", "1", "--skip-download", "--no-warnings",
				"-J", v.watchURL())
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

var (
	customMPVSocket string
	mpvCheckMu      sync.Mutex
	lastMPVCheck    time.Time
	lastMPVState    bool
)

// resetMPVRunningCache clears the cached mpv state so the next check performs a fresh probe.
func resetMPVRunningCache() {
	mpvCheckMu.Lock()
	lastMPVCheck = time.Time{}
	lastMPVState = false
	mpvCheckMu.Unlock()
}

// mpvSocketPath returns the path to the UNIX domain socket used for mpv IPC.
func mpvSocketPath() string {
	if customMPVSocket != "" {
		return customMPVSocket
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "ytplay-mpv.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("ytplay-mpv-%d.sock", os.Getuid()))
}

// isMPVRunning checks whether an mpv instance is actively listening on the IPC socket.
// It caches results for 250ms to prevent flooding the socket during rapid UI redraws.
func isMPVRunning() bool {
	mpvCheckMu.Lock()
	defer mpvCheckMu.Unlock()
	if time.Since(lastMPVCheck) < 250*time.Millisecond {
		return lastMPVState
	}
	sock := mpvSocketPath()
	conn, err := net.DialTimeout("unix", sock, 100*time.Millisecond)
	lastMPVCheck = time.Now()
	if err != nil {
		lastMPVState = false
		return false
	}
	_ = conn.Close()
	lastMPVState = true
	return true
}

// enqueueToMPV sends one or more URLs to the running mpv instance via IPC.
func enqueueToMPV(urls ...string) error {
	if len(urls) == 0 {
		return nil
	}
	sock := mpvSocketPath()
	conn, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
	if err != nil {
		mpvCheckMu.Lock()
		lastMPVState = false
		lastMPVCheck = time.Now()
		mpvCheckMu.Unlock()
		return fmt.Errorf("mpv ipc connect: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	enc := json.NewEncoder(conn)
	reader := bufio.NewReader(conn)

	for i, u := range urls {
		reqID := i + 1
		payload := map[string]interface{}{
			"command":    []string{"loadfile", u, "append-play"},
			"request_id": reqID,
		}
		if err := enc.Encode(payload); err != nil {
			return fmt.Errorf("mpv ipc send: %w", err)
		}
		// Read until we get the response matching our request_id,
		// skipping any asynchronous event notifications from mpv / scripts.
		for {
			respLine, err := reader.ReadBytes('\n')
			if err != nil {
				return fmt.Errorf("mpv ipc response: %w", err)
			}
			var resp struct {
				RequestID int    `json:"request_id"`
				Error     string `json:"error"`
				Event     string `json:"event"`
			}
			if err := json.Unmarshal(respLine, &resp); err != nil {
				continue
			}
			if resp.Event != "" {
				// skip asynchronous mpv / script events
				continue
			}
			if resp.RequestID == reqID || resp.RequestID == 0 {
				if resp.Error != "" && resp.Error != "success" {
					return fmt.Errorf("mpv error: %s", resp.Error)
				}
				break
			}
		}
	}
	mpvCheckMu.Lock()
	lastMPVState = true
	lastMPVCheck = time.Now()
	mpvCheckMu.Unlock()
	return nil
}

// playInMPV plays the given URLs. If an mpv instance is already running with an
// active IPC socket, the videos are automatically enqueued into it and enqueued=true
// is returned. Otherwise, a new detached mpv process is spawned and enqueued=false
// is returned.
func playInMPV(urls ...string) (bool, error) {
	if len(urls) == 0 {
		return false, nil
	}

	if isMPVRunning() {
		if err := enqueueToMPV(urls...); err == nil {
			return true, nil
		}
	}

	sock := mpvSocketPath()
	_ = os.Remove(sock)

	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return false, fmt.Errorf("mpv: %w", err)
	}
	defer devnull.Close()

	args := append([]string{"--input-ipc-server=" + sock}, urls...)
	cmd := exec.Command("mpv", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Detach mpv from the TUI's streams: stdin would otherwise receive the
	// keystrokes the TUI is listening for, and mpv's own output would print
	// into the alternate screen and desync the cell-anchored image layout.
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("mpv: %w", err)
	}
	// Reap the detached process in the background when it terminates so it doesn't linger as a zombie.
	go func() {
		_ = cmd.Wait()
		mpvCheckMu.Lock()
		lastMPVState = false
		lastMPVCheck = time.Now()
		mpvCheckMu.Unlock()
	}()
	resetMPVRunningCache()
	return false, nil
}

// sendMPVCommand sends a command to mpv over IPC and waits for confirmation.
func sendMPVCommand(args ...interface{}) error {
	if !isMPVRunning() {
		return nil
	}
	sock := mpvSocketPath()
	conn, err := net.DialTimeout("unix", sock, 150*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))

	enc := json.NewEncoder(conn)
	reader := bufio.NewReader(conn)
	reqID := 9998
	payload := map[string]interface{}{
		"command":    args,
		"request_id": reqID,
	}
	if err := enc.Encode(payload); err != nil {
		return err
	}

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}
		var resp struct {
			RequestID int    `json:"request_id"`
			Error     string `json:"error"`
			Event     string `json:"event"`
		}
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		if resp.Event != "" {
			continue
		}
		if resp.RequestID == reqID || resp.RequestID == 0 {
			if resp.Error != "" && resp.Error != "success" {
				return fmt.Errorf("mpv error: %s", resp.Error)
			}
			return nil
		}
	}
}

func removeMPVPlaylistItem(index int) {
	_ = sendMPVCommand("playlist-remove", index)
}

func moveMPVPlaylistItem(from, to int) {
	_ = sendMPVCommand("playlist-move", from, to)
}

func clearMPVPlaylist() {
	_ = sendMPVCommand("playlist-clear")
}

func playMPVPlaylistIndex(index int) {
	_ = sendMPVCommand("playlist-play-index", index)
}

// getMPVPlayingInfo queries mpv for the 0-based index of the currently playing playlist entry
// and the active file path / URL.
func getMPVPlayingInfo() (pos int, currentPath string) {
	pos = -1
	currentPath = ""
	if !isMPVRunning() {
		return -1, ""
	}
	sock := mpvSocketPath()
	conn, err := net.DialTimeout("unix", sock, 150*time.Millisecond)
	if err != nil {
		return -1, ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))

	enc := json.NewEncoder(conn)
	reader := bufio.NewReader(conn)

	_ = enc.Encode(map[string]interface{}{
		"command":    []string{"get_property", "playlist-pos"},
		"request_id": 1,
	})
	_ = enc.Encode(map[string]interface{}{
		"command":    []string{"get_property", "path"},
		"request_id": 2,
	})

	gotPos := false
	gotPath := false

	for !gotPos || !gotPath {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}
		var resp struct {
			RequestID int         `json:"request_id"`
			Data      interface{} `json:"data"`
			Event     string      `json:"event"`
		}
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		if resp.Event != "" {
			continue
		}
		if resp.RequestID == 1 {
			switch v := resp.Data.(type) {
			case float64:
				pos = int(v)
			case int:
				pos = v
			}
			gotPos = true
		} else if resp.RequestID == 2 {
			if s, ok := resp.Data.(string); ok {
				currentPath = s
			}
			gotPath = true
		}
	}
	return pos, currentPath
}

// getMPVPlaylistPos queries mpv for the 0-based index of the currently playing playlist entry.
// Returns -1 if mpv is not running or position is unavailable.
func getMPVPlaylistPos() int {
	pos, _ := getMPVPlayingInfo()
	return pos
}
