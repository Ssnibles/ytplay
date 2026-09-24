package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Init() tea.Cmd {
	// The input is focused on the model the update loop runs (set in
	// initialModel). Init runs on a copy, so focusing here would be lost.
	if m.state == searchingState {
		return searchCmd(m.query, searchResults, false)
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// The input is only interactive on the prompt screen. On the results
	// screen typing is deliberately ignored: searching is a fresh action
	// (started from the prompt), not a filter over the current results.
	if m.state == promptState {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		var cmd tea.Cmd
		m, cmd = m.handleWindowSize(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD:
			return m, tea.Quit
		}

		var cmd tea.Cmd
		m, cmd = m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case searchMsg:
		var cmd tea.Cmd
		m, cmd = m.handleSearchMsg(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case thumbMsg:
		m = m.handleThumbMsg(msg)

	case detailMsg:
		m = m.handleDetailMsg(msg)

	case copyMsg:
		m = m.handleCopyMsg(msg)

	case openMsg:
		m = m.handleOpenMsg(msg)

	case channelMsg:
		var cmd tea.Cmd
		m, cmd = m.handleChannelMsg(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if m.state == searchingState || (m.state == channelState && m.channelLoading) {
		cmds = append(cmds, m.spin.Tick)
	}

	return m, tea.Batch(cmds...)
}

func (m model) handleWindowSize(msg tea.WindowSizeMsg) (model, tea.Cmd) {
	m.width, m.height = msg.Width, msg.Height
	// Native image blocks are anchored to a cell position and painted at a
	// pixel size, so any size change invalidates them (the cache key
	// includes the cell size too).
	m.thumbs = make(map[string]string)
	if m.state == channelState && len(m.channelVideos) > 0 {
		return m, m.loadSelectionFor(m.channelVideos, m.channelCursor)
	}
	if m.state == queueState && len(m.queue) > 0 {
		return m, m.loadSelectionFor(m.queue, m.queueCursor)
	}
	if len(m.filtered) > 0 {
		return m, m.loadSelection()
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch m.state {
	case promptState:
		return m.handlePromptKey(msg)
	case searchingState:
		return m.handleSearchingKey(msg)
	case queueState:
		return m.handleQueueKey(msg)
	case resultsState:
		return m.handleResultsKey(msg)
	case channelState:
		return m.handleChannelKey(msg)
	default:
		return m, nil
	}
}

func (m model) handlePromptKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if prev, ok := m.popNav(); ok {
			var cmd tea.Cmd
			if prev.state == resultsState && len(prev.filtered) > 0 {
				cmd = prev.loadSelection()
			} else if prev.state == channelState && len(prev.channelVideos) > 0 {
				cmd = prev.loadSelectionFor(prev.channelVideos, prev.channelCursor)
			} else if prev.state == queueState && len(prev.queue) > 0 {
				cmd = prev.loadSelectionFor(prev.queue, prev.queueCursor)
			}
			return prev, cmd
		}
		return m, tea.Quit
	case tea.KeyCtrlP:
		return m.historyPrev(), nil
	case tea.KeyCtrlN:
		return m.historyNext(), nil
	case tea.KeyEnter:
		q := strings.TrimSpace(m.input.Value())
		if q == "" {
			return m, nil
		}
		m = m.pushNav()
		m.query = q
		m = m.rememberQuery(q)
		m.state = searchingState
		return m, searchCmd(q, searchResults, false)
	default:
		m.histIdx = -1
		m.pending = ""
		return m, nil
	}
}

func (m model) handleSearchingKey(msg tea.KeyMsg) (model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		if prev, ok := m.popNav(); ok {
			return prev, nil
		}
		m.state = promptState
		m.input.Focus()
		return m, nil
	}
	return m, nil
}

func (m model) handleQueueKey(msg tea.KeyMsg) (model, tea.Cmd) {
	if len(m.queue) == 0 {
		m.queueCursor = 0
	}
	switch msg.Type {
	case tea.KeyEsc:
		if prev, ok := m.popNav(); ok {
			var cmd tea.Cmd
			if prev.state == resultsState && len(prev.filtered) > 0 {
				cmd = prev.loadSelection()
			} else if prev.state == channelState && len(prev.channelVideos) > 0 {
				cmd = prev.loadSelectionFor(prev.channelVideos, prev.channelCursor)
			}
			return prev, cmd
		}
		m.state = resultsState
		m.errMsg = ""
		m.status = ""
		return m, nil
	case tea.KeyEnter:
		m = m.playFromQueue()
		return m, m.loadSelectionFor(m.queue, m.queueCursor)
	}

	var cmds []tea.Cmd
	var down, up bool
	switch msg.Type {
	case tea.KeyDown:
		down = true
	case tea.KeyUp:
		up = true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "/":
			m = m.pushNav()
			m.state = promptState
			m.input.Focus()
			m.errMsg = ""
			m.status = ""
			return m, nil
		case "j":
			down = true
		case "k":
			up = true
		case "J":
			m = m.moveQueueDown()
		case "K":
			m = m.moveQueueUp()
		case "x", "d":
			m = m.removeQueueAt(m.queueCursor)
			cmds = append(cmds, m.loadSelectionFor(m.queue, m.queueCursor))
		case "p":
			m = m.playQueue()
		case "c":
			if len(m.queue) > 0 {
				cmds = append(cmds, copyURLCmd(m.queue[m.queueCursor].watchURL()))
			}
		case "o":
			if len(m.queue) > 0 {
				cmds = append(cmds, m.openVideo(m.queue[m.queueCursor]))
			}
		}
	}
	if down && m.queueCursor < len(m.queue)-1 {
		m.queueCursor++
		cmds = append(cmds, m.loadSelectionFor(m.queue, m.queueCursor))
	}
	if up && m.queueCursor > 0 {
		m.queueCursor--
		cmds = append(cmds, m.loadSelectionFor(m.queue, m.queueCursor))
	}
	return m, tea.Batch(cmds...)
}

func (m model) handleResultsKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if prev, ok := m.popNav(); ok {
			var cmd tea.Cmd
			if prev.state == resultsState && len(prev.filtered) > 0 {
				cmd = prev.loadSelection()
			} else if prev.state == channelState && len(prev.channelVideos) > 0 {
				cmd = prev.loadSelectionFor(prev.channelVideos, prev.channelCursor)
			}
			return prev, cmd
		}
		m.state = promptState
		m.input.Focus()
		m.errMsg = ""
		m.status = ""
		return m, nil

	case tea.KeyTab:
		if m.focusPane == listPane {
			m.focusPane = previewPane
		} else {
			m.focusPane = listPane
		}
		return m, nil

	case tea.KeyShiftTab:
		if m.focusPane == previewPane {
			m.focusPane = listPane
		} else {
			m.focusPane = previewPane
		}
		return m, nil

	case tea.KeyEnter:
		// Launch mpv or open channel if focused on channel/channel item.
		if len(m.filtered) == 0 {
			return m, nil
		}
		v := m.filtered[m.cursor]
		if m.focusPane == previewPane || v.isChannel() {
			return m.openChannelView(v)
		}
		if err := playInMPV(v.watchURL()); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		return m, nil
	}

	var cmds []tea.Cmd
	// Selection movement (arrows and vim j/k).
	var down, up bool
	switch msg.Type {
	case tea.KeyDown:
		down = true
	case tea.KeyUp:
		up = true
	case tea.KeyPgUp:
		if m.cursor > pageStep {
			m.cursor -= pageStep
		} else {
			m.cursor = 0
		}
		cmds = append(cmds, m.loadSelection())
	case tea.KeyPgDown:
		m.cursor += pageStep
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		cmds = append(cmds, m.loadSelection())
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "/":
			m = m.pushNav()
			m.state = promptState
			m.input.Focus()
			m.errMsg = ""
			m.status = ""
			return m, nil
		case "j":
			down = true
		case "k":
			up = true
		case "c":
			if len(m.filtered) > 0 {
				cmds = append(cmds, copyURLCmd(m.filtered[m.cursor].watchURL()))
			}
		case "o":
			if len(m.filtered) > 0 {
				cmds = append(cmds, m.openVideo(m.filtered[m.cursor]))
			}
		case "C":
			if len(m.filtered) > 0 {
				return m.openChannelView(m.filtered[m.cursor])
			}
		case "a":
			if len(m.filtered) > 0 {
				m.queue = append(m.queue, m.filtered[m.cursor])
				m.errMsg = ""
				m.status = fmt.Sprintf("queued · %d in queue", len(m.queue))
			}
		case "p":
			m = m.playQueue()
		case "q":
			m = m.pushNav()
			m.state = queueState
			m.errMsg = ""
			m.status = ""
			cmds = append(cmds, m.loadSelectionFor(m.queue, m.queueCursor))
			return m, tea.Batch(cmds...)
		}
	}
	if down && m.cursor < len(m.filtered)-1 {
		m.cursor++
		if m.focusPane == previewPane {
			m.focusPane = listPane
		}
		cmds = append(cmds, m.loadSelection())
	}
	if up && m.cursor > 0 {
		m.cursor--
		if m.focusPane == previewPane {
			m.focusPane = listPane
		}
		cmds = append(cmds, m.loadSelection())
	}

	// Paging deeper into the search: once the selection nears the
	// bottom of what has been fetched, ask for the next page.
	if m.shouldLoadMore() {
		m.fetchingMore = true
		cmds = append(cmds, searchCmd(m.query, m.fetched+searchResults, true))
	}

	return m, tea.Batch(cmds...)
}

func (m model) openChannelView(v video) (model, tea.Cmd) {
	chURL := m.resolveChannelURL(v)
	chTitle := v.channel()
	if chTitle == "" {
		chTitle = v.Title
	}
	if chURL == "" && chTitle != "" {
		chURL = chTitle
	}
	if chURL == "" {
		m.errMsg = "no channel available"
		return m, nil
	}

	m = m.pushNav()
	m.state = channelState
	m.channelTitle = chTitle
	m.channelURL = chURL
	m.channelVideos = nil
	m.channelCursor = 0
	m.channelFetched = searchResults
	m.channelLoading = true
	m.channelFetchingMore = false
	m.focusPane = listPane
	m.errMsg = ""
	m.status = "Loading channel " + chTitle + "…"

	return m, channelCmd(chTitle, chURL, searchResults, false)
}

func (m model) handleChannelKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if prev, ok := m.popNav(); ok {
			var cmd tea.Cmd
			if prev.state == resultsState && len(prev.filtered) > 0 {
				cmd = prev.loadSelection()
			} else if prev.state == channelState && len(prev.channelVideos) > 0 {
				cmd = prev.loadSelectionFor(prev.channelVideos, prev.channelCursor)
			}
			return prev, cmd
		}
		m.state = promptState
		m.input.Focus()
		m.errMsg = ""
		m.status = ""
		return m, nil

	case tea.KeyEnter:
		if len(m.channelVideos) == 0 {
			return m, nil
		}
		v := m.channelVideos[m.channelCursor]
		if err := playInMPV(v.watchURL()); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		return m, nil
	}

	var cmds []tea.Cmd
	var down, up bool
	switch msg.Type {
	case tea.KeyDown:
		down = true
	case tea.KeyUp:
		up = true
	case tea.KeyPgUp:
		if m.channelCursor > pageStep {
			m.channelCursor -= pageStep
		} else {
			m.channelCursor = 0
		}
		cmds = append(cmds, m.loadSelectionFor(m.channelVideos, m.channelCursor))
	case tea.KeyPgDown:
		m.channelCursor += pageStep
		if m.channelCursor >= len(m.channelVideos) {
			m.channelCursor = len(m.channelVideos) - 1
		}
		cmds = append(cmds, m.loadSelectionFor(m.channelVideos, m.channelCursor))
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "/":
			m = m.pushNav()
			m.state = promptState
			m.input.Focus()
			m.errMsg = ""
			m.status = ""
			return m, nil
		case "j":
			down = true
		case "k":
			up = true
		case "c":
			if len(m.channelVideos) > 0 {
				cmds = append(cmds, copyURLCmd(m.channelVideos[m.channelCursor].watchURL()))
			}
		case "o":
			if len(m.channelVideos) > 0 {
				cmds = append(cmds, m.openVideo(m.channelVideos[m.channelCursor]))
			}
		case "a":
			if len(m.channelVideos) > 0 {
				m.queue = append(m.queue, m.channelVideos[m.channelCursor])
				m.errMsg = ""
				m.status = fmt.Sprintf("queued · %d in queue", len(m.queue))
			}
		case "p":
			m = m.playQueue()
		case "q":
			m = m.pushNav()
			m.state = queueState
			m.errMsg = ""
			m.status = ""
			cmds = append(cmds, m.loadSelectionFor(m.queue, m.queueCursor))
			return m, tea.Batch(cmds...)
		}
	}

	if down && m.channelCursor < len(m.channelVideos)-1 {
		m.channelCursor++
		cmds = append(cmds, m.loadSelectionFor(m.channelVideos, m.channelCursor))
	}
	if up && m.channelCursor > 0 {
		m.channelCursor--
		cmds = append(cmds, m.loadSelectionFor(m.channelVideos, m.channelCursor))
	}

	if m.shouldLoadMoreChannel() {
		m.channelFetchingMore = true
		cmds = append(cmds, channelCmd(m.channelTitle, m.channelURL, m.channelFetched+searchResults, true))
	}

	return m, tea.Batch(cmds...)
}

func (m model) shouldLoadMoreChannel() bool {
	if m.channelLoading || m.channelFetchingMore || len(m.channelVideos) == 0 {
		return false
	}
	l := computeLayout(m.width, m.height)
	return m.channelCursor >= len(m.channelVideos)-l.avail
}

func (m model) handleChannelMsg(msg channelMsg) (model, tea.Cmd) {
	m.channelLoading = false
	m.channelFetchingMore = false
	if msg.err != nil {
		if !msg.more {
			m.errMsg = msg.err.Error()
			m.status = ""
		}
		return m, nil
	}
	if msg.channelTitle != "" {
		m.channelTitle = msg.channelTitle
	}
	if msg.more {
		m.channelVideos = mergeResults(m.channelVideos, msg.videos)
		m.channelFetched = msg.limit
	} else {
		m.channelVideos = msg.videos
		m.channelFetched = msg.limit
		m.channelCursor = 0
	}
	m.errMsg = ""
	m.status = ""
	if len(m.channelVideos) > 0 {
		return m, m.loadSelectionFor(m.channelVideos, m.channelCursor)
	}
	return m, nil
}

func (m model) handleSearchMsg(msg searchMsg) (model, tea.Cmd) {
	m.fetchingMore = false
	if msg.err != nil {
		m.errMsg = msg.err.Error()
		if !msg.more {
			m.filtered = nil
			m.state = resultsState
		}
		return m, nil
	}
	m.errMsg = ""
	m.status = ""
	if msg.more {
		m.filtered = mergeResults(m.filtered, msg.videos)
	} else {
		m.filtered = msg.videos
		m.state = resultsState
		m.cursor = 0
	}
	m.fetched = msg.limit
	return m, m.loadSelection()
}

func (m model) handleThumbMsg(msg thumbMsg) model {
	key := thumbKey(msg.id, msg.cols, msg.rows)
	m.thumbBusy[key] = false
	if len(m.thumbs)+1 > maxThumbs {
		for k := range m.thumbs {
			delete(m.thumbs, k)
			delete(m.thumbBusy, k)
			break
		}
	}
	if msg.err != nil {
		// Empty art marks a failed render; viewPreview shows a friendly
		// "no thumbnail" hint instead of caching a multi-line string.
		m.thumbs[key] = ""
	} else {
		m.thumbs[key] = msg.art
	}
	return m
}

func (m model) handleDetailMsg(msg detailMsg) model {
	m.detailBusy[msg.id] = false
	if len(m.details)+1 > maxDetails {
		for id := range m.details {
			delete(m.details, id)
			delete(m.detailBusy, id)
			break
		}
	}
	if msg.err != nil {
		// Cache an empty detail so a video whose extractor fails isn't
		// re-fetched on every selection change.
		m.details[msg.id] = videoDetail{}
		return m
	}
	m.details[msg.id] = msg.det
	return m
}

func (m model) handleCopyMsg(msg copyMsg) model {
	if msg.err != nil {
		m.status = ""
		m.errMsg = "copy: " + msg.err.Error()
		return m
	}
	m.errMsg = ""
	m.status = "copied " + msg.url
	return m
}

func (m model) handleOpenMsg(msg openMsg) model {
	if msg.err != nil {
		m.status = ""
		m.errMsg = "open: " + msg.err.Error()
		return m
	}
	m.errMsg = ""
	m.status = "opened " + msg.url
	return m
}

// loadThumb issues a thumbnail render for the currently selected result if it
// hasn't been rendered at the current pane size.
func (m model) loadThumb() tea.Cmd {
	if m.state != resultsState {
		return nil
	}
	return m.loadThumbFor(m.filtered, m.cursor)
}

// loadThumbFor issues a thumbnail render for the video at cursor in videos if
// it hasn't been rendered at the current pane size. Busy tracking is keyed the
// same way as the render cache (id@size), so a resize in flight doesn't get
// suppressed by a smaller-size fetch that is still queued.
func (m model) loadThumbFor(videos []video, cursor int) tea.Cmd {
	if len(videos) == 0 || cursor < 0 || cursor >= len(videos) {
		return nil
	}
	l := computeLayout(m.width, m.height)
	if !l.thumbOK {
		return nil
	}
	v := videos[cursor]
	key := thumbKey(v.ID, l.cols, l.rows)
	if _, cached := m.thumbs[key]; cached {
		return nil
	}
	if m.thumbBusy[key] {
		return nil
	}
	m.thumbBusy[key] = true
	return thumbCmd(v, l.cols, l.rows, m.proto)
}

// loadSelection issues whatever the selected video still needs fetched: its
// thumbnail (the two panes may paint it at a different size) and its channel
// stats. Both are no-ops when already cached or in flight.
func (m model) loadSelection() tea.Cmd {
	return m.loadSelectionFor(m.filtered, m.cursor)
}

// loadSelectionFor issues thumbnails and stats for the video at cursor in
// videos. Stats are the slow part of a selection (a full extract per video), so
// fetch a small lookahead window rather than only the selected one: by the time
// the user scrolls into a neighbour its stats are usually cached. Each fetch is
// a no-op when cached or in flight, so this stays cheap.
func (m model) loadSelectionFor(videos []video, cursor int) tea.Cmd {
	var cmds []tea.Cmd
	if c := m.loadThumbFor(videos, cursor); c != nil {
		cmds = append(cmds, c)
	}
	for i := 0; i <= detailLookahead && cursor+i < len(videos); i++ {
		if c := m.loadDetailFor(videos[cursor+i]); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

// loadDetailAt starts a stats fetch for the video at index i if one isn't
// already cached or in flight.
func (m model) loadDetailAt(i int) tea.Cmd {
	if len(m.filtered) == 0 || m.state != resultsState || i < 0 || i >= len(m.filtered) {
		return nil
	}
	return m.loadDetailFor(m.filtered[i])
}

// loadDetailFor starts a stats fetch for the given video if one isn't already
// cached or in flight.
func (m model) loadDetailFor(v video) tea.Cmd {
	if _, ok := m.details[v.ID]; ok {
		return nil
	}
	if m.detailBusy[v.ID] {
		return nil
	}
	m.detailBusy[v.ID] = true
	return detailCmd(v)
}

// shouldLoadMore reports whether the next results page should be fetched: the
// selection is within pageStep of the bottom of what has been fetched, and the
// previous page returned as many as asked (otherwise we've hit the end).
func (m model) shouldLoadMore() bool {
	if m.state != resultsState || m.fetchingMore || len(m.filtered) == 0 {
		return false
	}
	if m.cursor < len(m.filtered)-pageStep {
		return false
	}
	return m.fetched <= len(m.filtered)
}
