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

	// 4. Merge preserves base order and appends new items; it must not
	// re-prioritise channels on a follow-up page (that would move rows out
	// from under the reader). Prioritisation happens once, on the first page.
	merged := mergeResults([]video{v1, v2}, []video{c1})
	if len(merged) != 3 || merged[0].ID != "v1" || merged[2].ID != "UC111" {
		t.Fatalf("merge should preserve base order and append extras: %v", merged)
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
	received := make(chan []string, 16)

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
					if err := json.Unmarshal(line, &msg); err != nil {
						continue
					}
					cmdStrings := make([]string, len(msg.Command))
					for idx, arg := range msg.Command {
						cmdStrings[idx] = fmt.Sprint(arg)
					}
					if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" {
						// Queries are not interesting to record; answer playlist-pos.
						data := "null"
						if cmdStrings[1] == "playlist-pos" {
							data = "0"
						}
						_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":%s,"error":"success"}`+"\n", msg.RequestID, data)))
						continue
					}
					received <- cmdStrings
					// Emit an async event before the reply to exercise event skipping.
					_, _ = c.Write([]byte(`{"event":"tracks-changed"}` + "\n"))
					_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"error":"success"}`+"\n", msg.RequestID)))
				}
			}(conn)
		}
	}()

	cleanup := func() {
		_ = l.Close()
		_ = os.Remove(sockPath)
		mpv.reset()
	}
	return received, cleanup
}

// assertCommand reads the next recorded mpv command and checks it.
func assertCommand(t *testing.T, recv chan []string, want ...string) {
	t.Helper()
	select {
	case got := <-recv:
		if len(got) != len(want) {
			t.Fatalf("command = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("command = %v, want %v", got, want)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for command %v", want)
	}
}

func TestPlayerCommands(t *testing.T) {
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "test-mpv.sock")
	useTestSocket(t, sock)

	if mpv.Running() {
		t.Fatal("Running should be false with no listener")
	}

	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	if !mpv.Running() {
		t.Fatal("Running should be true when the mock is listening")
	}

	testURL := "https://www.youtube.com/watch?v=test1234"
	if err := mpv.Enqueue(testURL); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	assertCommand(t, recv, "loadfile", testURL, "append-play")

	urlA := "https://www.youtube.com/watch?v=vidA"
	urlB := "https://www.youtube.com/watch?v=vidB"
	if err := mpv.Enqueue(urlA, urlB); err != nil {
		t.Fatalf("Enqueue multiple failed: %v", err)
	}
	assertCommand(t, recv, "loadfile", urlA, "append-play")
	assertCommand(t, recv, "loadfile", urlB, "append-play")

	// Play replaces the playlist, then clears pause.
	if err := mpv.Play(testURL); err != nil {
		t.Fatalf("Play failed: %v", err)
	}
	assertCommand(t, recv, "loadfile", testURL)
	assertCommand(t, recv, "set", "pause", "no")
}
