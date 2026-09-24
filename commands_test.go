package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLoadMoreMergesWithoutDuplicates(t *testing.T) {
	base := make([]video, searchResults)
	for i := range base {
		base[i] = video{ID: fmt.Sprintf("a%02d", i), Title: fmt.Sprintf("t%02d", i)}
	}
	extra := make([]video, 15)
	for i := range extra {
		extra[i] = video{ID: fmt.Sprintf("a%02d", i+searchResults-2), Title: fmt.Sprintf("t%02d", i+searchResults-2)}
	}

	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: base, limit: searchResults})
	m = update(m, searchMsg{videos: extra, limit: searchResults + 25, more: true})

	md := m.(model)
	want := searchResults + (len(extra) - 2) // extra overlaps the last two of base
	if len(md.filtered) != want {
		t.Fatalf("want %d unique results, got %d", want, len(md.filtered))
	}
	if md.fetched != searchResults+25 {
		t.Fatalf("fetched = %d, want %d", md.fetched, searchResults+25)
	}
	if md.fetchingMore {
		t.Fatal("a completed fetch should clear the fetching flag")
	}
	seen := make(map[string]bool, len(md.filtered))
	for i, v := range md.filtered {
		if seen[v.ID] {
			t.Fatalf("duplicate id %s at %d", v.ID, i)
		}
		seen[v.ID] = true
	}
	if md.filtered[0].ID != "a00" || md.filtered[len(md.filtered)-1].ID != "a37" {
		t.Fatalf("bad merge order: first=%s last=%s", md.filtered[0].ID, md.filtered[len(md.filtered)-1].ID)
	}
}

func TestMergeResultsEdgeCases(t *testing.T) {
	// Empty base returns extra
	extra := []video{{ID: "v1"}, {ID: "v2"}}
	if got := mergeResults(nil, extra); len(got) != 2 || got[0].ID != "v1" {
		t.Fatalf("merge with nil base failed: %v", got)
	}

	// Empty extra returns base
	base := []video{{ID: "v1"}}
	if got := mergeResults(base, nil); len(got) != 1 || got[0].ID != "v1" {
		t.Fatalf("merge with nil extra failed: %v", got)
	}

	// All duplicates in extra
	got := mergeResults(base, []video{{ID: "v1"}})
	if len(got) != 1 {
		t.Fatalf("expected 1 result after duplicate merge, got %d", len(got))
	}
}

func TestPrioritizeChannels(t *testing.T) {
	v1 := video{ID: "v1", Title: "Video 1"}
	v2 := video{ID: "v2", Title: "Video 2"}
	c1 := video{ID: "UC111", Title: "Channel 1", IEKey: "YoutubeTab"}
	c2 := video{ID: "UC222", Title: "Channel 2", URL: "https://www.youtube.com/@c2"}

	// 1. Channel at end moves to front
	input := []video{v1, v2, c1}
	got := prioritizeChannels(input)
	if len(got) != 3 || got[0].ID != "UC111" || got[1].ID != "v1" || got[2].ID != "v2" {
		t.Fatalf("channel not prioritized first: %v", got)
	}

	// 2. Multiple channels stay in relative order and come before videos
	input = []video{v1, c1, v2, c2}
	got = prioritizeChannels(input)
	if len(got) != 4 || got[0].ID != "UC111" || got[1].ID != "UC222" || got[2].ID != "v1" || got[3].ID != "v2" {
		t.Fatalf("multiple channels not prioritized properly: %v", got)
	}

	// 3. No channels leaves slice unchanged
	input = []video{v1, v2}
	got = prioritizeChannels(input)
	if len(got) != 2 || got[0].ID != "v1" || got[1].ID != "v2" {
		t.Fatalf("slice without channels modified: %v", got)
	}

	// 4. Merge results pulls channel from extra to the very front
	merged := mergeResults([]video{v1, v2}, []video{c1})
	if len(merged) != 3 || merged[0].ID != "UC111" {
		t.Fatalf("merge did not prioritize channel: %v", merged)
	}

	// 5. Playlists (even with IEKey: YoutubeTab) are not prioritized as channels
	p1 := video{ID: "PL123", Title: "Playlist 1", IEKey: "YoutubeTab", URL: "https://www.youtube.com/playlist?list=PL123"}
	input = []video{v1, p1, c1}
	got = prioritizeChannels(input)
	if len(got) != 3 || got[0].ID != "UC111" || got[1].ID != "v1" || got[2].ID != "PL123" {
		t.Fatalf("playlist should not be prioritized as channel: %v", got)
	}
}

func TestChannelVideosURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"UCuAXFkgsw1L7xaCfnd5JJOw", "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/videos"},
		{"@veritasium", "https://www.youtube.com/@veritasium/videos"},
		{"https://www.youtube.com/channel/UCabc", "https://www.youtube.com/channel/UCabc/videos"},
		{"https://www.youtube.com/channel/UCabc/videos", "https://www.youtube.com/channel/UCabc/videos"},
		{"https://www.youtube.com/@mkbhd", "https://www.youtube.com/@mkbhd/videos"},
		{"https://www.youtube.com/@mkbhd/videos", "https://www.youtube.com/@mkbhd/videos"},
	}
	for _, c := range cases {
		if got := channelVideosURL(c.in); got != c.want {
			t.Errorf("channelVideosURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func startMockMPVServer(t *testing.T, sockPath string) (chan []string, func()) {
	t.Helper()
	_ = os.Remove(sockPath)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on socket %s: %v", sockPath, err)
	}
	received := make(chan []string, 10)

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				scanner := bufio.NewScanner(c)
				for scanner.Scan() {
					line := scanner.Bytes()
					var msg struct {
						Command   []interface{} `json:"command"`
						RequestID int           `json:"request_id"`
					}
					if err := json.Unmarshal(line, &msg); err == nil {
						cmdStrings := make([]string, len(msg.Command))
						for idx, arg := range msg.Command {
							cmdStrings[idx] = fmt.Sprint(arg)
						}
						received <- cmdStrings
						if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "playlist-pos" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":0,"error":"success"}`+"\n", msg.RequestID)))
						} else if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "path" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":"","error":"success"}`+"\n", msg.RequestID)))
						} else {
							// Simulate realistic mpv emitting asynchronous event before or between command responses
							_, _ = c.Write([]byte(`{"event":"tracks-changed"}` + "\n"))
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"error":"success"}`+"\n", msg.RequestID)))
						}
					}
				}
			}(conn)
		}
	}()

	cleanup := func() {
		_ = l.Close()
		_ = os.Remove(sockPath)
		resetMPVRunningCache()
	}
	return received, cleanup
}

func TestMPVIPC(t *testing.T) {
	resetMPVRunningCache()
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "test-mpv.sock")
	customMPVSocket = sock
	defer func() {
		customMPVSocket = isolatedTestSocket
		resetMPVRunningCache()
	}()

	// 1. When MPV is not running
	if isMPVRunning() {
		t.Fatal("isMPVRunning should return false when no mpv instance is listening")
	}

	// 2. Start mock MPV server
	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()

	// Wait briefly for server to bind
	time.Sleep(20 * time.Millisecond)
	resetMPVRunningCache()

	if !isMPVRunning() {
		t.Fatal("isMPVRunning should return true when mock mpv is listening")
	}

	// 3. Test enqueueToMPV with single URL
	testURL := "https://www.youtube.com/watch?v=test1234"
	if err := enqueueToMPV(testURL); err != nil {
		t.Fatalf("enqueueToMPV failed: %v", err)
	}

	select {
	case cmd := <-recv:
		if len(cmd) != 3 || cmd[0] != "loadfile" || cmd[1] != testURL || cmd[2] != "append-play" {
			t.Fatalf("unexpected mpv command received: %v", cmd)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for mpv command")
	}

	// 4. Test enqueueToMPV with multiple URLs (sequential queue handoff)
	urlA := "https://www.youtube.com/watch?v=vidA"
	urlB := "https://www.youtube.com/watch?v=vidB"
	if err := enqueueToMPV(urlA, urlB); err != nil {
		t.Fatalf("enqueueToMPV with multiple URLs failed: %v", err)
	}
	for _, expected := range []string{urlA, urlB} {
		select {
		case cmd := <-recv:
			if len(cmd) != 3 || cmd[0] != "loadfile" || cmd[1] != expected || cmd[2] != "append-play" {
				t.Fatalf("unexpected mpv command for multiple urls: %v, want url %s", cmd, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for mpv command for %s", expected)
		}
	}

	// 5. Test playInMPV auto-enqueues into running MPV
	enqueued, err := playInMPV(testURL)
	if err != nil {
		t.Fatalf("playInMPV failed: %v", err)
	}
	if !enqueued {
		t.Fatal("playInMPV should return enqueued=true when mpv is running")
	}

	select {
	case cmd := <-recv:
		if len(cmd) != 3 || cmd[0] != "loadfile" || cmd[1] != testURL || cmd[2] != "append-play" {
			t.Fatalf("unexpected mpv command received from playInMPV: %v", cmd)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for mpv command from playInMPV")
	}
}
