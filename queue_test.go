package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestQueueURLs(t *testing.T) {
	got := queueURLs([]video{{ID: "a1", URL: "https://youtu.be/a1"}, {ID: "a2"}})
	want := []string{"https://youtu.be/a1", "https://www.youtube.com/watch?v=a2"}
	if len(got) != len(want) {
		t.Fatalf("queueURLs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("queueURLs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestQueueMoveUp(t *testing.T) {
	m := model{queue: []video{{ID: "a"}, {ID: "b"}, {ID: "c"}}, queueCursor: 1}
	m = m.moveQueueUp()
	if m.queue[0].ID != "b" || m.queue[1].ID != "a" || m.queue[2].ID != "c" {
		t.Fatalf("move up reordered wrong: %v", ids(m.queue))
	}
	if m.queueCursor != 0 {
		t.Fatalf("cursor should follow the moved item, got %d", m.queueCursor)
	}
	m = m.moveQueueUp() // already at the top: no-op
	if m.queueCursor != 0 || ids(m.queue) != "bac" {
		t.Fatalf("move up at top should be a no-op: %v", ids(m.queue))
	}
}

func TestQueueMoveDown(t *testing.T) {
	m := model{queue: []video{{ID: "a"}, {ID: "b"}, {ID: "c"}}, queueCursor: 0}
	m = m.moveQueueDown()
	if m.queue[0].ID != "b" || m.queue[1].ID != "a" {
		t.Fatalf("move down reordered wrong: %v", ids(m.queue))
	}
	if m.queueCursor != 1 {
		t.Fatalf("cursor should follow the moved item, got %d", m.queueCursor)
	}
	m.queueCursor = 2
	m = m.moveQueueDown() // already at the bottom: no-op
	if m.queueCursor != 2 || ids(m.queue) != "bac" {
		t.Fatalf("move down at bottom should be a no-op: %v", ids(m.queue))
	}
}

func TestQueueRemoveClampsCursor(t *testing.T) {
	m := model{queue: []video{{ID: "a"}, {ID: "b"}, {ID: "c"}}, queueCursor: 2}
	m = m.removeQueueAt(2)
	if ids(m.queue) != "ab" || m.queueCursor != 1 {
		t.Fatalf("remove last should clamp cursor down: queue=%q cursor=%d", ids(m.queue), m.queueCursor)
	}
	m = m.removeQueueAt(0)
	if ids(m.queue) != "b" || m.queueCursor != 0 {
		t.Fatalf("remove first should keep cursor valid: queue=%q cursor=%d", ids(m.queue), m.queueCursor)
	}
	m = m.removeQueueAt(0)
	if len(m.queue) != 0 || m.queueCursor != 0 {
		t.Fatalf("remove last item should empty the queue: queue=%v cursor=%d", m.queue, m.queueCursor)
	}
}

func TestClearQueue(t *testing.T) {
	m := model{queue: []video{{ID: "a"}, {ID: "b"}, {ID: "c"}}, queueCursor: 2}
	m = m.clearQueue()
	if len(m.queue) != 0 || m.queueCursor != 0 {
		t.Fatalf("clearQueue should empty the queue: %v, cursor: %d", m.queue, m.queueCursor)
	}
	if m.status != "cleared queue" {
		t.Fatalf("clearQueue should set status 'cleared queue', got %q", m.status)
	}
	// Calling clear on already empty queue should be a no-op
	m = m.clearQueue()
	if len(m.queue) != 0 {
		t.Fatalf("clearQueue on empty queue should remain empty: %v", m.queue)
	}
}

func TestPlayFromQueue(t *testing.T) {
	resetMPVRunningCache()
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "test-mpv.sock")
	customMPVSocket = sock
	defer func() {
		customMPVSocket = isolatedTestSocket
		resetMPVRunningCache()
	}()

	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()

	time.Sleep(20 * time.Millisecond)
	resetMPVRunningCache()

	m := model{
		queue: []video{
			{ID: "v0", Title: "Video 0", URL: "https://youtu.be/v0"},
			{ID: "v1", Title: "Video 1", URL: "https://youtu.be/v1"},
			{ID: "v2", Title: "Video 2", URL: "https://youtu.be/v2"},
		},
		queueCursor: 1, // cursor on v1
	}

	m = m.playFromQueue()

	// Should have enqueued v1 and v2 into mpv for sequential autoplay
	for _, wantURL := range []string{"https://youtu.be/v1", "https://youtu.be/v2"} {
		select {
		case cmd := <-recv:
			if len(cmd) != 3 || cmd[0] != "loadfile" || cmd[1] != wantURL || cmd[2] != "append-play" {
				t.Fatalf("unexpected mpv command: %v, want url %s", cmd, wantURL)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for mpv command for %s", wantURL)
		}
	}

	// All queued items are preserved in the queue interface
	if len(m.queue) != 3 || m.queue[0].ID != "v0" || m.queue[1].ID != "v1" || m.queue[2].ID != "v2" {
		t.Fatalf("queue should retain all items in interface, got: %v", m.queue)
	}
}

func TestSyncQueueWithMPVAdvances(t *testing.T) {
	resetMPVRunningCache()
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "test-mpv.sock")
	customMPVSocket = sock
	defer func() {
		customMPVSocket = isolatedTestSocket
		resetMPVRunningCache()
	}()

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("failed to listen on socket %s: %v", sock, err)
	}
	defer l.Close()

	removed := make(chan []string, 5)
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
						if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "playlist-pos" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":1,"error":"success"}`+"\n", msg.RequestID)))
						} else if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "path" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":"","error":"success"}`+"\n", msg.RequestID)))
						} else if len(cmdStrings) >= 2 && cmdStrings[0] == "playlist-remove" {
							removed <- cmdStrings
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"error":"success"}`+"\n", msg.RequestID)))
						}
					}
				}
			}(conn)
		}
	}()

	time.Sleep(20 * time.Millisecond)
	resetMPVRunningCache()

	m := model{
		queue: []video{
			{ID: "v0", Title: "Finished Video"},
			{ID: "v1", Title: "Now Playing Video"},
			{ID: "v2", Title: "Upcoming Video"},
		},
		queueCursor: 1,
	}

	m = m.syncQueueWithMPV()

	// Should send playlist-remove 0 to mpv
	select {
	case cmd := <-removed:
		if len(cmd) != 2 || cmd[0] != "playlist-remove" || cmd[1] != "0" {
			t.Fatalf("expected playlist-remove 0, got: %v", cmd)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for playlist-remove command")
	}

	// Queue should have dropped v0 and now start at v1
	if len(m.queue) != 2 || m.queue[0].ID != "v1" || m.queue[1].ID != "v2" {
		t.Fatalf("expected queue to advance to [v1, v2], got: %v", m.queue)
	}
	if m.queueCursor != 0 {
		t.Fatalf("expected cursor to adjust to 0, got: %d", m.queueCursor)
	}
}

func TestSyncQueueWithMPVSkipByPath(t *testing.T) {
	resetMPVRunningCache()
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "test-mpv.sock")
	customMPVSocket = sock
	defer func() {
		customMPVSocket = isolatedTestSocket
		resetMPVRunningCache()
	}()

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("failed to listen on socket %s: %v", sock, err)
	}
	defer l.Close()

	removed := make(chan []string, 5)
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
						if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "playlist-pos" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":1,"error":"success"}`+"\n", msg.RequestID)))
						} else if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "path" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":"https://www.youtube.com/watch?v=v1","error":"success"}`+"\n", msg.RequestID)))
						} else if len(cmdStrings) >= 2 && cmdStrings[0] == "playlist-remove" {
							removed <- cmdStrings
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"error":"success"}`+"\n", msg.RequestID)))
						}
					}
				}
			}(conn)
		}
	}()

	time.Sleep(20 * time.Millisecond)
	resetMPVRunningCache()

	m := model{
		queue: []video{
			{ID: "v0", Title: "Skipped Video", URL: "https://www.youtube.com/watch?v=v0"},
			{ID: "v1", Title: "Now Playing Video", URL: "https://www.youtube.com/watch?v=v1"},
			{ID: "v2", Title: "Upcoming Video", URL: "https://www.youtube.com/watch?v=v2"},
		},
		queueCursor: 1,
	}

	m = m.syncQueueWithMPV()

	// Should send playlist-remove 0 to mpv
	select {
	case cmd := <-removed:
		if len(cmd) != 2 || cmd[0] != "playlist-remove" || cmd[1] != "0" {
			t.Fatalf("expected playlist-remove 0, got: %v", cmd)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for playlist-remove command")
	}

	// Queue should have dropped skipped v0 and now start at v1
	if len(m.queue) != 2 || m.queue[0].ID != "v1" || m.queue[1].ID != "v2" {
		t.Fatalf("expected queue to drop skipped v0, got: %v", m.queue)
	}
	if m.queueCursor != 0 {
		t.Fatalf("expected cursor to adjust to 0, got: %d", m.queueCursor)
	}
}

func TestSyncQueueWithMPVSkipMultiple(t *testing.T) {
	resetMPVRunningCache()
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "test-mpv.sock")
	customMPVSocket = sock
	defer func() {
		customMPVSocket = isolatedTestSocket
		resetMPVRunningCache()
	}()

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("failed to listen on socket %s: %v", sock, err)
	}
	defer l.Close()

	removed := make(chan []string, 5)
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
						if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "playlist-pos" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":2,"error":"success"}`+"\n", msg.RequestID)))
						} else if len(cmdStrings) >= 2 && cmdStrings[0] == "get_property" && cmdStrings[1] == "path" {
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":"https://www.youtube.com/watch?v=v2","error":"success"}`+"\n", msg.RequestID)))
						} else if len(cmdStrings) >= 2 && cmdStrings[0] == "playlist-remove" {
							removed <- cmdStrings
							_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"error":"success"}`+"\n", msg.RequestID)))
						}
					}
				}
			}(conn)
		}
	}()

	time.Sleep(20 * time.Millisecond)
	resetMPVRunningCache()

	m := model{
		queue: []video{
			{ID: "v0", Title: "Skipped 1", URL: "https://www.youtube.com/watch?v=v0"},
			{ID: "v1", Title: "Skipped 2", URL: "https://www.youtube.com/watch?v=v1"},
			{ID: "v2", Title: "Playing 3", URL: "https://www.youtube.com/watch?v=v2"},
			{ID: "v3", Title: "Upcoming 4", URL: "https://www.youtube.com/watch?v=v3"},
		},
		queueCursor: 2,
	}

	m = m.syncQueueWithMPV()

	// Should send playlist-remove 0 twice to prune both skipped videos
	for i := 0; i < 2; i++ {
		select {
		case cmd := <-removed:
			if len(cmd) != 2 || cmd[0] != "playlist-remove" || cmd[1] != "0" {
				t.Fatalf("expected playlist-remove 0, got: %v", cmd)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for playlist-remove command")
		}
	}

	// Queue should have dropped v0 and v1 and now start at v2
	if len(m.queue) != 2 || m.queue[0].ID != "v2" || m.queue[1].ID != "v3" {
		t.Fatalf("expected queue to drop v0 and v1, got: %v", m.queue)
	}
	if m.queueCursor != 0 {
		t.Fatalf("expected cursor to adjust to 0, got: %d", m.queueCursor)
	}
}
