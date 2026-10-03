package main

import "fmt"

// queueURLs resolves the playable watch URLs for a slice of videos.
func queueURLs(queue []video) []string {
	urls := make([]string, len(queue))
	for i, v := range queue {
		urls[i] = v.watchURL()
	}
	return urls
}

// stageVideo appends v to the queue. If the queue is playing, the video is also
// appended to mpv so it plays in turn; otherwise it is only staged.
func (m model) stageVideo(v video) model {
	m.queue = append(m.queue, v)
	if m.queueActive {
		_ = mpv.Enqueue(v.watchURL())
	}
	m.errMsg = ""
	m.status = fmt.Sprintf("queued · %d in queue", len(m.queue))
	return m
}

// playStandalone plays v immediately and leaves the staged queue untouched.
func (m model) playStandalone(v video) model {
	if err := mpv.Play(v.watchURL()); err != nil {
		m.status = ""
		m.errMsg = err.Error()
		return m
	}
	m.errMsg = ""
	m.nowPlaying = v
	m.queueActive = false
	m.status = fmt.Sprintf("playing %q in mpv", v.Title)
	return m
}

// startQueue loads the queue from queueOffset onward into mpv. Only the suffix
// is loaded, so the selected entry is the first thing mpv plays — jumping to an
// index inside a yt-dlp-resolved playlist is unreliable. The earlier entries
// stay in the queue, and queueOffset maps mpv's position back onto it.
func (m model) startQueue() model {
	if m.queueOffset < 0 {
		m.queueOffset = 0
	}
	if m.queueOffset >= len(m.queue) {
		m.queueOffset = len(m.queue) - 1
	}
	suffix := m.queue[m.queueOffset:]
	if err := mpv.Play(queueURLs(suffix)...); err != nil {
		m.status = ""
		m.errMsg = err.Error()
		return m
	}
	m.queueActive = true
	m.nowPlaying = m.queue[m.queueOffset]
	m.errMsg = ""
	if rest := len(suffix) - 1; rest > 0 {
		m.status = fmt.Sprintf("playing %q + %d more in mpv", m.nowPlaying.Title, rest)
	} else {
		m.status = fmt.Sprintf("playing %q", m.nowPlaying.Title)
	}
	return m
}

// syncPlayer tracks which queue entry mpv is playing. The queue is a playlist:
// playback never removes entries, so the user keeps the list they built.
func (m model) syncPlayer() model {
	if !mpv.Running() || !m.queueActive || len(m.queue) == 0 {
		return m
	}
	pos := mpv.PlaylistPos()
	if pos < 0 {
		return m
	}
	idx := m.queueOffset + pos
	if idx >= len(m.queue) {
		// Played past the end of the queue.
		m.queueActive = false
		return m
	}
	m.nowPlaying = m.queue[idx]
	return m
}

// playQueue plays the whole staged queue from the top.
func (m model) playQueue() model {
	if len(m.queue) == 0 {
		m.status = ""
		m.errMsg = "queue is empty — press a to add videos"
		return m
	}
	m.queueOffset = 0
	m.queueCursor = 0
	return m.startQueue()
}

// moveQueueUp swaps the selected queued video with the one above it.
func (m model) moveQueueUp() model {
	if m.queueCursor <= 0 {
		return m
	}
	i := m.queueCursor
	j := i - 1
	if !m.canReorder(i, j) {
		m.status = "can't reorder across the playback start"
		return m
	}
	m.queue[i], m.queue[j] = m.queue[j], m.queue[i]
	m.queueCursor = j
	if m.queueActive && j >= m.queueOffset {
		_ = mpv.Move(i-m.queueOffset, j-m.queueOffset)
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
	j := i + 1
	if !m.canReorder(i, j) {
		m.status = "can't reorder across the playback start"
		return m
	}
	m.queue[i], m.queue[j] = m.queue[j], m.queue[i]
	m.queueCursor = j
	if m.queueActive && i >= m.queueOffset {
		_ = mpv.Move(i-m.queueOffset, j-m.queueOffset)
	}
	m.status = ""
	m.errMsg = ""
	return m
}

// canReorder reports whether swapping entries i and j is possible while the
// queue is playing. Moving across queueOffset would change which entries mpv
// knows about, so it is rejected rather than desyncing the two lists.
func (m model) canReorder(i, j int) bool {
	if !m.queueActive {
		return true
	}
	return (i >= m.queueOffset) == (j >= m.queueOffset)
}

// removeQueueAt deletes the queued video at i, keeping the cursor sensible.
func (m model) removeQueueAt(i int) model {
	if i < 0 || i >= len(m.queue) {
		return m
	}
	if m.queueActive {
		switch {
		case i >= m.queueOffset:
			_ = mpv.RemoveAt(i - m.queueOffset)
		case m.queueOffset > 0:
			m.queueOffset--
		}
	}
	m.queue = append(m.queue[:i], m.queue[i+1:]...)
	if m.queueOffset > len(m.queue) {
		m.queueOffset = len(m.queue)
	}
	if m.queueActive && m.queueOffset >= len(m.queue) {
		m.queueActive = false
	}
	if len(m.queue) == 0 {
		m.queueCursor = 0
	} else if m.queueCursor >= len(m.queue) {
		m.queueCursor = len(m.queue) - 1
	}
	m.status = ""
	m.errMsg = ""
	return m
}

// clearQueue empties the staged queue. If the queue is playing, mpv keeps the
// current item but drops everything after it, so playback is not interrupted.
func (m model) clearQueue() model {
	if len(m.queue) == 0 && !m.queueActive {
		return m
	}
	if m.queueActive {
		_ = mpv.Clear()
	}
	m.queue = nil
	m.queueCursor = 0
	m.queueActive = false
	m.queueOffset = 0
	m.status = "cleared queue"
	m.errMsg = ""
	return m
}

// playFromQueue plays the queue starting at the selection, keeping the whole
// queue intact so the user can still see and re-play the earlier entries.
func (m model) playFromQueue() model {
	if len(m.queue) == 0 {
		return m
	}
	m.queueOffset = m.queueCursor
	return m.startQueue()
}
