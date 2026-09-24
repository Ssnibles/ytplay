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

// playQueue starts mpv with every queued video, playing them in sequence, and
// clears the queue. The list survives on launch error so it can be retried.
func (m model) playQueue() model {
	if len(m.queue) == 0 {
		m.status = ""
		m.errMsg = "queue is empty — press a to add videos"
		return m
	}
	urls := queueURLs(m.queue)
	if err := playInMPV(urls...); err != nil {
		m.errMsg = err.Error()
		return m
	}
	m.errMsg = ""
	m.status = fmt.Sprintf("playing %d queued videos", len(urls))
	m.queue = nil
	m.queueCursor = 0
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
	m.status = ""
	m.errMsg = ""
	return m
}

// removeQueueAt deletes the queued video at i, keeping the cursor sensible.
func (m model) removeQueueAt(i int) model {
	if i < 0 || i >= len(m.queue) {
		return m
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

// playFromQueue plays only the selected queued video (unlike playQueue, which
// plays the whole list), removes it from the queue and keeps the rest in their
// current order. The item is removed even if launching mpv fails, so the queue
// always reflects "what's left to watch".
func (m model) playFromQueue() model {
	if len(m.queue) == 0 {
		return m
	}
	v := m.queue[m.queueCursor]
	m = m.removeQueueAt(m.queueCursor)
	m.status = fmt.Sprintf("playing %q", v.Title)
	if err := playInMPV(v.watchURL()); err != nil {
		m.status = ""
		m.errMsg = err.Error()
	}
	return m
}
