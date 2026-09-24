package main

import (
	"testing"
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
