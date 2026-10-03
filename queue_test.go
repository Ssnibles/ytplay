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
	m = m.clearQueue()
	if len(m.queue) != 0 {
		t.Fatalf("clearQueue on empty queue should remain empty: %v", m.queue)
	}
}

// startMockMPVPos serves playlist-pos = pos and records every other command.
func startMockMPVPos(t *testing.T, sockPath string, pos int) chan []string {
	t.Helper()
	_ = os.Remove(sockPath)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on socket %s: %v", sockPath, err)
	}
	t.Cleanup(func() {
		_ = l.Close()
		_ = os.Remove(sockPath)
		mpv.reset()
	})

	received := make(chan []string, 16)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					var msg struct {
						Command   []interface{} `json:"command"`
						RequestID int           `json:"request_id"`
					}
					if json.Unmarshal(sc.Bytes(), &msg) != nil {
						continue
					}
					cmd := make([]string, len(msg.Command))
					for i, a := range msg.Command {
						cmd[i] = fmt.Sprint(a)
					}
					if len(cmd) >= 2 && cmd[0] == "get_property" && cmd[1] == "playlist-pos" {
						_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"data":%d,"error":"success"}`+"\n", msg.RequestID, pos)))
						continue
					}
					received <- cmd
					_, _ = c.Write([]byte(fmt.Sprintf(`{"request_id":%d,"error":"success"}`+"\n", msg.RequestID)))
				}
			}(conn)
		}
	}()
	return received
}

func TestStageVideoOnlyStagesWhenIdle(t *testing.T) {
	useTestSocket(t, filepath.Join(t.TempDir(), "absent.sock"))
	m := model{}
	m = m.stageVideo(video{ID: "a"})
	if len(m.queue) != 1 || m.queueActive {
		t.Fatalf("staging while idle must only add to the queue: queue=%v active=%v", ids(m.queue), m.queueActive)
	}
	if m.status != "queued · 1 in queue" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestStageVideoEnqueuesWhenQueuePlaying(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{queueActive: true, queue: []video{{ID: "v1", URL: "https://youtu.be/v1"}}}
	m = m.stageVideo(video{ID: "v2", URL: "https://youtu.be/v2"})
	if len(m.queue) != 2 {
		t.Fatalf("expected 2 queued, got %v", ids(m.queue))
	}
	assertCommand(t, recv, "loadfile", "https://youtu.be/v2", "append-play")
}

func TestPlayStandaloneLeavesQueue(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{queue: []video{{ID: "q1", URL: "https://youtu.be/q1"}}}
	m = m.playStandalone(video{ID: "v1", URL: "https://youtu.be/v1"})
	if m.nowPlaying.ID != "v1" {
		t.Fatalf("nowPlaying = %q, want v1", m.nowPlaying.ID)
	}
	if m.queueActive {
		t.Fatal("standalone playback must not activate the queue")
	}
	if ids(m.queue) != "q1" {
		t.Fatalf("staged queue must be untouched, got %v", ids(m.queue))
	}
	assertCommand(t, recv, "loadfile", "https://youtu.be/v1")
	assertCommand(t, recv, "set", "pause", "no")
}

func TestPlayQueueActivatesAndPlaysInOrder(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{queue: []video{
		{ID: "v1", URL: "https://youtu.be/v1"},
		{ID: "v2", URL: "https://youtu.be/v2"},
	}}
	m = m.playQueue()
	if !m.queueActive || m.nowPlaying.ID != "v1" || m.queueCursor != 0 {
		t.Fatalf("playQueue state wrong: active=%v now=%q cursor=%d", m.queueActive, m.nowPlaying.ID, m.queueCursor)
	}
	assertCommand(t, recv, "loadfile", "https://youtu.be/v1")
	assertCommand(t, recv, "loadfile", "https://youtu.be/v2", "append")
	assertCommand(t, recv, "set", "pause", "no")
}

func TestPlayFromQueueKeepsQueue(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{
		queue: []video{
			{ID: "v0", URL: "https://youtu.be/v0"},
			{ID: "v1", URL: "https://youtu.be/v1"},
			{ID: "v2", URL: "https://youtu.be/v2"},
		},
		queueCursor: 1,
	}
	m = m.playFromQueue()
	if ids(m.queue) != "v0v1v2" || m.queueCursor != 1 {
		t.Fatalf("playing from the queue must not drop earlier entries: queue=%v cursor=%d", ids(m.queue), m.queueCursor)
	}
	if !m.queueActive || m.nowPlaying.ID != "v1" || m.queueOffset != 1 {
		t.Fatalf("playFromQueue state wrong: active=%v now=%q offset=%d", m.queueActive, m.nowPlaying.ID, m.queueOffset)
	}
	// Only the suffix is loaded, so the selected entry plays first with no jump.
	assertCommand(t, recv, "loadfile", "https://youtu.be/v1")
	assertCommand(t, recv, "loadfile", "https://youtu.be/v2", "append")
	assertCommand(t, recv, "set", "pause", "no")
}

func TestSyncPlayerMapsOffsetOntoQueue(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	startMockMPVPos(t, sock, 1)
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{
		queueActive: true,
		queueOffset: 2,
		queue: []video{
			{ID: "v0"}, {ID: "v1"}, {ID: "v2"}, {ID: "v3"}, {ID: "v4"},
		},
	}
	m = m.syncPlayer()
	if m.nowPlaying.ID != "v3" {
		t.Fatalf("offset+pos should map to queue index 3, got %q", m.nowPlaying.ID)
	}
}

func TestSyncPlayerTracksCurrentEntry(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	recv := startMockMPVPos(t, sock, 2)
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{
		queueActive: true,
		queue: []video{
			{ID: "v0", URL: "https://youtu.be/v0"},
			{ID: "v1", URL: "https://youtu.be/v1"},
			{ID: "v2", URL: "https://youtu.be/v2"},
			{ID: "v3", URL: "https://youtu.be/v3"},
		},
	}
	m = m.syncPlayer()

	if ids(m.queue) != "v0v1v2v3" {
		t.Fatalf("the queue must not be consumed by playback, got %v", ids(m.queue))
	}
	if m.nowPlaying.ID != "v2" {
		t.Fatalf("nowPlaying should track mpv's position, got %q", m.nowPlaying.ID)
	}
	if !m.queueActive {
		t.Fatal("queue should still be active")
	}
	select {
	case cmd := <-recv:
		t.Fatalf("sync must not touch the playlist, got %v", cmd)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSyncPlayerLeavesStagedQueueAlone(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	recv := startMockMPVPos(t, sock, 5)
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{
		queueActive: false, // a standalone video is playing
		queue: []video{
			{ID: "v0", URL: "https://youtu.be/v0"},
			{ID: "v1", URL: "https://youtu.be/v1"},
		},
	}
	m = m.syncPlayer()
	if ids(m.queue) != "v0v1" {
		t.Fatalf("a stale position must not touch the staged queue, got %v", ids(m.queue))
	}
	select {
	case cmd := <-recv:
		t.Fatalf("no mpv commands were expected, got %v", cmd)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSyncPlayerKeepsQueueWhenIdle(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test-mpv.sock")
	useTestSocket(t, sock)
	startMockMPVPos(t, sock, -1)
	time.Sleep(20 * time.Millisecond)
	mpv.reset()

	m := model{queueActive: true, queue: []video{{ID: "v0", URL: "https://youtu.be/v0"}}}
	m = m.syncPlayer()
	if ids(m.queue) != "v0" {
		t.Fatalf("an idle player must not change the queue, got %v", ids(m.queue))
	}
	if !m.queueActive {
		t.Fatal("a transient -1 position must not deactivate the queue")
	}
}
