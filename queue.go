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

// syncQueueWithMPV queries mpv to remove completed or skipped videos from the active queue.
func (m model) syncQueueWithMPV() model {
	if !isMPVRunning() || len(m.queue) == 0 {
		return m
	}
	pos, currentPath := getMPVPlayingInfo()

	dropCount := 0
	if currentPath != "" {
		for i, v := range m.queue {
			if v.watchURL() == currentPath || v.URL == currentPath || (v.ID != "" && strings.Contains(currentPath, v.ID)) {
				dropCount = i
				break
			}
		}
	}

	if dropCount == 0 && pos > 0 && pos <= len(m.queue) {
		dropCount = pos
	}

	if dropCount > 0 {
		toRemoveInMPV := dropCount
		if pos >= 0 && pos < toRemoveInMPV {
			toRemoveInMPV = pos
		}
		for i := 0; i < toRemoveInMPV; i++ {
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
		} else if len(m.queue) == 0 {
			m.queueCursor = 0
		}
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
	enqueued, err := playInMPV(urls...)
	if err != nil {
		m.errMsg = err.Error()
		return m
	}
	m.errMsg = ""
	if enqueued {
		m.status = fmt.Sprintf("enqueued %d queued videos into mpv", len(urls))
	} else {
		m.status = fmt.Sprintf("playing %d queued videos", len(urls))
	}
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
// queue so mpv autoplays sequentially. It preserves the launched videos in the queue.
func (m model) playFromQueue() model {
	if len(m.queue) == 0 {
		return m
	}
	v := m.queue[m.queueCursor]
	remaining := m.queue[m.queueCursor:]
	urls := queueURLs(remaining)
	enqueued, err := playInMPV(urls...)
	if err != nil {
		m.status = ""
		m.errMsg = err.Error()
		return m
	}
	// If starting fresh from an offset, slice queue to start from that video
	if !enqueued && m.queueCursor > 0 {
		m.queue = m.queue[m.queueCursor:]
		m.queueCursor = 0
	}
	m.errMsg = ""
	if enqueued {
		if len(urls) > 1 {
			m.status = fmt.Sprintf("enqueued %d videos into mpv", len(urls))
		} else {
			m.status = fmt.Sprintf("enqueued %q into mpv", v.Title)
		}
	} else {
		if len(urls) > 1 {
			m.status = fmt.Sprintf("playing %q + %d more in mpv", v.Title, len(urls)-1)
		} else {
			m.status = fmt.Sprintf("playing %q", v.Title)
		}
	}
	return m
}
