package main

import "fmt"

// queueURLs resolves the playable watch URLs for a staged queue.
func queueURLs(queue []video) []string {
	urls := make([]string, len(queue))
	for i, v := range queue {
		urls[i] = v.watchURL()
	}
	return urls
}

// stageVideo appends v to the queue. If the queue is the active playlist (it is
// being played), the video is also appended to mpv so it plays in turn;
// otherwise it is only staged.
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

// syncPlayer tracks which queue entry mpv is playing. The queue is a playlist:
// playing it never removes entries, so the user keeps the list they built. Only
// explicit removal (`x` / `X`) changes it. While the queue is the active
// playlist, mpv's position is the queue index, so no guessing is needed.
func (m model) syncPlayer() model {
	if !mpv.Running() || !m.queueActive || len(m.queue) == 0 {
		return m
	}
	pos := mpv.PlaylistPos()
	switch {
	case pos >= len(m.queue):
		// Played past the end of the queue.
		m.queueActive = false
	case pos >= 0:
		m.nowPlaying = m.queue[pos]
	}
	return m
}

// playQueue plays the whole staged queue from the top.
func (m model) playQueue() model {
	if len(m.queue) == 0 {
		m.status = ""
		m.errMsg = "queue is empty — press a to add videos"
		return m
	}
	if err := mpv.Play(queueURLs(m.queue)...); err != nil {
		m.errMsg = err.Error()
		return m
	}
	m.errMsg = ""
	m.queueActive = true
	m.queueCursor = 0
	m.nowPlaying = m.queue[0]
	m.status = fmt.Sprintf("playing %d queued videos", len(m.queue))
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
	if m.queueActive {
		_ = mpv.Move(i, i-1)
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
	if m.queueActive {
		_ = mpv.Move(i, i+1)
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
	if m.queueActive {
		_ = mpv.RemoveAt(i)
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
	start := m.queueCursor
	if err := mpv.PlayFrom(start, queueURLs(m.queue)...); err != nil {
		m.status = ""
		m.errMsg = err.Error()
		return m
	}
	m.queueActive = true
	m.nowPlaying = m.queue[start]
	m.errMsg = ""
	if remaining := len(m.queue) - start - 1; remaining > 0 {
		m.status = fmt.Sprintf("playing %q + %d more in mpv", m.nowPlaying.Title, remaining)
	} else {
		m.status = fmt.Sprintf("playing %q", m.nowPlaying.Title)
	}
	return m
}
