package main

import (
	"fmt"
	"strings"
)

// queueURLs resolves the playable watch URLs for a staged queue.
func queueURLs(queue []video) []string {
	urls := make([]string, len(queue))
	for i, v := range queue {
		urls[i] = v.watchURL()
	}
	return urls
}

// syncQueueWithMPV drops videos that mpv has already played. It identifies the
// currently playing entry by URL and removes everything before it. It must not
// trust mpv's playlist position as an index into ytplay's queue: the two lists
// can diverge (a fresh session, a replaced playlist, an mpv that outlived a
// previous run), and using the raw position silently deleted queue entries.
func (m model) syncQueueWithMPV() model {
	if !isMPVRunning() || len(m.queue) == 0 {
		return m
	}
	_, currentPath := getMPVPlayingInfo()
	if currentPath == "" {
		return m
	}

	dropCount := 0
	found := false
	for i, v := range m.queue {
		if v.watchURL() == currentPath || v.URL == currentPath || (v.ID != "" && strings.Contains(currentPath, v.ID)) {
			dropCount = i
			found = true
			break
		}
	}
	if !found || dropCount <= 0 {
		return m
	}

	// Everything before the current entry is finished; mirror that in mpv too.
	for i := 0; i < dropCount; i++ {
		removeMPVPlaylistItem(0)
	}
	if dropCount > len(m.queue) {
		dropCount = len(m.queue)
	}
	m.queue = m.queue[dropCount:]
	m.queueCursor -= dropCount
	if m.queueCursor < 0 {
		m.queueCursor = 0
	}
	if m.queueCursor >= len(m.queue) && len(m.queue) > 0 {
		m.queueCursor = len(m.queue) - 1
	}
	return m
}

// playQueue starts mpv with every queued video or enqueues them into running mpv.
// The list is preserved in ytplay so the queue can be viewed and managed while playing.
func (m model) playQueue() model {
	if len(m.queue) == 0 {
		m.status = ""
		m.errMsg = "queue is empty — press a to add videos"
		return m
	}
	urls := queueURLs(m.queue)
	if err := playNowInMPV(urls...); err != nil {
		m.errMsg = err.Error()
		return m
	}
	m.errMsg = ""
	m.queueCursor = 0
	m.status = fmt.Sprintf("playing %d queued videos", len(urls))
	return m
}

// moveQueueUp swaps the selected queued video with the one above it.
func (m model) moveQueueUp() model {
	if m.queueCursor <= 0 {
		return m
	}
	i := m.queueCursor
	m.queue[i], m.queue[i-1] = m.queue[i-1], m.queue[i]
	m.queueCursor--
	if isMPVRunning() {
		moveMPVPlaylistItem(i, i-1)
	}
	m.status = ""
	m.errMsg = ""
	return m
}

// moveQueueDown swaps the selected queued video with the one below it.
func (m model) moveQueueDown() model {
	if m.queueCursor >= len(m.queue)-1 {
		return m
	}
	i := m.queueCursor
	m.queue[i], m.queue[i+1] = m.queue[i+1], m.queue[i]
	m.queueCursor++
	if isMPVRunning() {
		moveMPVPlaylistItem(i, i+1)
	}
	m.status = ""
	m.errMsg = ""
	return m
}

// removeQueueAt deletes the queued video at i, keeping the cursor sensible.
func (m model) removeQueueAt(i int) model {
	if i < 0 || i >= len(m.queue) {
		return m
	}
	if isMPVRunning() {
		removeMPVPlaylistItem(i)
	}
	m.queue = append(m.queue[:i], m.queue[i+1:]...)
	if len(m.queue) == 0 {
		m.queueCursor = 0
	} else if m.queueCursor >= len(m.queue) {
		m.queueCursor = len(m.queue) - 1
	}
	m.status = ""
	m.errMsg = ""
	return m
}

// clearQueue empties the queue.
func (m model) clearQueue() model {
	if len(m.queue) == 0 {
		return m
	}
	if isMPVRunning() {
		clearMPVPlaylist()
		if len(m.queue) > 1 {
			m.queue = m.queue[:1]
			m.queueCursor = 0
			m.status = "cleared upcoming queue"
			m.errMsg = ""
			return m
		}
	}
	m.queue = nil
	m.queueCursor = 0
	m.status = "cleared queue"
	m.errMsg = ""
	return m
}

// playFromQueue plays the selected queued video followed by the remainder of the
// queue so mpv autoplays sequentially. It trims the queue down to the selection.
func (m model) playFromQueue() model {
	if len(m.queue) == 0 {
		return m
	}
	v := m.queue[m.queueCursor]
	remaining := m.queue[m.queueCursor:]
	urls := queueURLs(remaining)
	if err := playNowInMPV(urls...); err != nil {
		m.status = ""
		m.errMsg = err.Error()
		return m
	}
	if m.queueCursor > 0 {
		m.queue = remaining
		m.queueCursor = 0
	}
	m.errMsg = ""
	if len(urls) > 1 {
		m.status = fmt.Sprintf("playing %q + %d more in mpv", v.Title, len(urls)-1)
	} else {
		m.status = fmt.Sprintf("playing %q", v.Title)
	}
	return m
}
