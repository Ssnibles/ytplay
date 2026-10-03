package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "ytplay — waiting for terminal…"
	}
	switch m.state {
	case promptState:
		return m.viewPrompt()
	case searchingState:
		return m.viewSearching()
	case queueState:
		return m.viewQueue()
	case channelState:
		return m.viewChannel()
	default:
		return m.viewResults()
	}
}

// ---- chrome -----------------------------------------------------------------

// barPart is one styled run of text inside a full-width chrome bar.
type barPart struct {
	text  string
	style lipgloss.Style
}

// renderBar lays out a one-line bar: left segments, a fill, then right
// segments. Every segment carries the bar background so the bar reads as a
// single strip, and the whole line is clamped to exactly width columns so it can
// never wrap.
func renderBar(width int, left, right []barPart) string {
	if width <= 0 {
		return ""
	}
	render := func(parts []barPart) string {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.style.Background(bgBar).Render(p.text))
		}
		return b.String()
	}
	l, r := render(left), render(right)
	lw, rw := lipgloss.Width(l), lipgloss.Width(r)
	maxL := width - rw - 1
	if maxL < 0 {
		r = ansi.Truncate(r, max(width-2, 1), "…")
		rw = lipgloss.Width(r)
		maxL = width - rw - 1
	}
	if maxL < 0 {
		maxL = 0
	}
	if lw > maxL {
		l = ansi.Truncate(l, maxL, "…")
		lw = lipgloss.Width(l)
	}
	gap := width - lw - rw
	if gap < 0 {
		gap = 0
	}
	return l + barBgStyle.Render(strings.Repeat(" ", gap)) + r
}

// pageFrame assembles a page: header bar, the pinned content block, and the
// status bar. The content is forced to exactly l.midH lines so the total is
// always the window height; a frame that scrolled would desync the
// cell-anchored thumbnail.
func (m model) pageFrame(l layout, mid string, ctxLeft, ctxRight []barPart) string {
	header := renderBar(m.width, ctxLeft, ctxRight)
	bottom := renderBar(m.width, m.footerHints(), m.statusParts())

	lines := strings.Split(mid, "\n")
	if len(lines) > l.midH {
		lines = lines[:l.midH]
	}
	for len(lines) < l.midH {
		lines = append(lines, "")
	}
	return header + "\n" + strings.Join(lines, "\n") + "\n" + bottom
}

func (m model) statusParts() []barPart {
	if m.errMsg != "" {
		return []barPart{{"✖ " + m.errMsg, barErrorStyle}}
	}
	if m.status != "" {
		return []barPart{{"✓ " + m.status, barStatusStyle}}
	}
	return nil
}

type hintPair struct{ key, label string }

// hintPairs is the contextual key reference shown in the status bar.
func (m model) hintPairs() []hintPair {
	if m.focusPane == previewPane {
		scroll := ""
		if v, ok := m.currentVideo(); ok {
			if maxS := m.maxPreviewScroll(v); maxS > 0 {
				scroll = fmt.Sprintf(" [%d/%d]", m.descScroll, maxS)
			}
		}
		return []hintPair{
			{"j/k", "scroll" + scroll}, {"h", "list"}, {"Enter", "channel"},
			{"o", "video"}, {"/", "search"}, {"Esc", "back"},
		}
	}
	if m.state == queueState {
		return []hintPair{
			{"j/k", "move"}, {"K/J", "reorder"}, {"x", "remove"}, {"X", "clear"},
			{"Enter", "play"}, {"p", "play all"}, {"space", "pause"}, {"Esc", "back"},
		}
	}
	return []hintPair{
		{"Enter", "play"}, {"a", "queue"}, {"q", "queue"}, {"space", "pause"},
		{"n/b", "skip"}, {"Tab", "details"}, {"c", "copy"}, {"/", "search"},
	}
}

// actionHints is the plain-text form of hintPairs, kept for tests and for the
// odd place that only needs a string.
func (m model) actionHints() string {
	pairs := m.hintPairs()
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, p.key+" "+p.label)
	}
	return strings.Join(parts, " · ")
}

func (m model) footerHints() []barPart {
	pairs := m.hintPairs()
	parts := make([]barPart, 0, len(pairs)*3)
	for i, p := range pairs {
		if i > 0 {
			parts = append(parts, barPart{"  ", barSepStyle})
		}
		parts = append(parts, barPart{p.key, barHintKeyStyle}, barPart{" " + p.label, barHintStyle})
	}
	return parts
}

// ---- prompt / searching -----------------------------------------------------

func (m model) viewPrompt() string {
	boxW := 52
	if boxW > m.width-8 {
		boxW = m.width - 8
	}
	if boxW < 20 {
		boxW = max(m.width-4, 10)
	}
	m.input.Width = max(boxW-6, 8)

	wordmark := wordmarkStyle.Render("ytplay")
	subtitle := dimStyle.Render("search YouTube · play in mpv")
	box := promptBoxStyle.Width(boxW).Render(m.input.View())

	hint := func(key, label string) string {
		return promptKeyStyle.Render(key) + " " + promptHintStyle.Render(label)
	}
	hints := hint("Enter", "search") + "   " +
		hint("↑/↓", "history") + "   " +
		hint("Esc", "quit")
	if len(m.navStack) > 0 {
		hints = hint("Enter", "search") + "   " +
			hint("↑/↓", "history") + "   " +
			hint("Esc", "go back")
	}

	lines := []string{wordmark, subtitle, "", box, "", hints}
	if m.errMsg != "" {
		lines = append(lines, "", errorStyle.Render(ansi.Truncate(m.errMsg, max(m.width-4, 8), "…")))
	}
	if recent := m.recentSearches(5); len(recent) > 0 {
		w := 0
		for _, q := range recent {
			w = max(w, lipgloss.Width(q))
		}
		w = min(w, max(m.width-8, 8))
		lines = append(lines, "", padRight(dimStyle.Render("recent"), w))
		for _, q := range recent {
			lines = append(lines, padRight(midStyle.Render(ansi.Truncate(q, w, "…")), w))
		}
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
}

// recentSearches returns up to n past queries, newest first.
func (m model) recentSearches(n int) []string {
	if len(m.history) == 0 {
		return nil
	}
	out := make([]string, 0, n)
	for i := len(m.history) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, m.history[i])
	}
	return out
}

func (m model) viewSearching() string {
	q := ansi.Truncate(m.query, max(m.width-24, 12), "…")
	content := lipgloss.JoinVertical(lipgloss.Center,
		wordmarkStyle.Render("ytplay"),
		"",
		m.spin.View(),
		"",
		dimStyle.Render("Searching for ")+searchQueryStyle.Render("“"+q+"”"),
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// ---- results / channel / queue ---------------------------------------------

func (m model) viewResults() string {
	l := computeLayout(m.width, m.height)

	ctxLeft := []barPart{
		{"▍ ytplay", barBrandStyle},
		{"  ›  ", barSepStyle},
		{ansi.Truncate(m.query, max(l.leftW, 12), "…"), barCtxStyle},
	}
	ctxRight := m.pageStateParts(len(m.filtered), "result")

	var mid string
	if len(m.filtered) == 0 {
		mid = m.emptyContent(l, "No results", "press / to search")
	} else {
		mid = m.contentPanes(l, m.filtered, m.cursor)
	}
	return m.pageFrame(l, mid, ctxLeft, ctxRight)
}

func (m model) viewChannel() string {
	l := computeLayout(m.width, m.height)

	ctxLeft := []barPart{
		{"▍ ytplay", barBrandStyle},
		{"  ›  ", barSepStyle},
		{"Channel · " + truncate(m.channelTitle, max(l.leftW-6, 10)), barCtxStyle},
	}
	ctxRight := m.pageStateParts(len(m.channelVideos), "video")
	if m.channelFetchingMore {
		ctxRight = append(ctxRight, barPart{"  ·  ", barSepStyle}, barPart{"loading more…", barStateStyle})
	}

	var mid string
	if len(m.channelVideos) == 0 {
		if m.channelLoading {
			mid = m.emptyContent(l, "Loading channel…", m.channelTitle)
		} else {
			mid = m.emptyContent(l, "Nothing here", "press Esc to go back")
		}
	} else {
		mid = m.contentPanes(l, m.channelVideos, m.channelCursor)
	}
	return m.pageFrame(l, mid, ctxLeft, ctxRight)
}

func (m model) viewQueue() string {
	l := computeLayout(m.width, m.height)

	ctxLeft := []barPart{
		{"▍ ytplay", barBrandStyle},
		{"  ›  ", barSepStyle},
		{fmt.Sprintf("Queue · %s", plural(len(m.queue), "video")), barCtxStyle},
	}
	ctxRight := []barPart{}
	if isMPVRunning() {
		ctxRight = append(ctxRight, barPart{"● mpv active", barMpvStyle})
	}

	var mid string
	if len(m.queue) == 0 {
		mid = m.emptyContent(l, "Queue is empty", "press a on any video to add it")
	} else {
		mid = m.contentPanes(l, m.queue, m.queueCursor)
	}
	return m.pageFrame(l, mid, ctxLeft, ctxRight)
}

// pageStateParts builds the right-hand header segments shared by the list pages.
func (m model) pageStateParts(count int, noun string) []barPart {
	var parts []barPart
	if count > 0 {
		parts = append(parts, barPart{plural(count, noun), barStateStyle})
	}
	if len(m.queue) > 0 {
		parts = append(parts, barPart{"  ·  ", barSepStyle}, barPart{fmt.Sprintf("%d queued", len(m.queue)), barAccentStyle})
	}
	if isMPVRunning() {
		parts = append(parts, barPart{"  ·  ", barSepStyle}, barPart{"● mpv active", barMpvStyle})
	}
	return parts
}

// plural renders a count with a naively pluralised noun.
func plural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

// emptyContent centers a two-line empty state in the content area.
func (m model) emptyContent(l layout, title, sub string) string {
	maxW := max(m.width-4, 8)
	body := lipgloss.JoinVertical(lipgloss.Center,
		emptyTitleStyle.Render(ansi.Truncate(title, maxW, "…")),
		dimStyle.Render(ansi.Truncate(sub, maxW, "…")))
	return lipgloss.Place(m.width, l.midH, lipgloss.Center, lipgloss.Center, body)
}

// ---- panes ------------------------------------------------------------------

// contentPanes renders the shared two-column layout: list, gap, vertical rule,
// gap, preview.
func (m model) contentPanes(l layout, videos []video, cursor int) string {
	list := m.viewList(l, videos, cursor)
	prev := m.viewPreview(l, videos, cursor)
	sep := sepStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", l.midH), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", sep, " ", prev)
}

func (m model) viewList(l layout, videos []video, cursor int) string {
	if l.leftW <= 0 || l.midH <= 0 {
		return ""
	}
	if len(videos) == 0 {
		return blankBlock(l.leftW, l.midH)
	}
	focused := m.focusPane == listPane
	numW := max(len(strconv.Itoa(len(videos))), 2)

	playingID := ""
	if isMPVRunning() && len(m.queue) > 0 {
		playingID = m.queue[0].ID
	}

	start := 0
	if cursor >= l.avail {
		start = cursor - l.avail + 1
	}
	end := min(start+l.avail, len(videos))

	lines := make([]string, 0, l.midH)
	for i := start; i < end; i++ {
		v := videos[i]
		playing := playingID != "" && v.ID == playingID
		lines = append(lines, m.listLine(v, i, i == cursor, focused, l.leftW, numW, playing, m.isQueued(v.ID)))
	}
	for len(lines) < l.midH {
		lines = append(lines, strings.Repeat(" ", l.leftW))
	}
	return strings.Join(lines[:l.midH], "\n")
}

// listLine renders one row: a state marker, the row number, the title (with the
// duration right-aligned), all sized to exactly width.
func (m model) listLine(v video, i int, selected, focused bool, width, numW int, playing, queued bool) string {
	prefixW := numW + 3 // marker + space + index + space
	body := listRow(v, width-prefixW)
	idx := fmt.Sprintf("%*d", numW, i+1)

	if selected {
		markerStyle, titleStyle := rowSelMarker, rowSelTitle
		if !focused {
			markerStyle, titleStyle = rowSelMarkerDim, rowSelTitleDim
		}
		return markerStyle.Render("▌") +
			rowSelBg.Render(" ") +
			rowSelMeta.Render(idx) +
			rowSelBg.Render(" ") +
			titleStyle.Render(body)
	}

	marker, markerStyle := " ", lipgloss.NewStyle()
	switch {
	case playing:
		marker, markerStyle = "▶", rowMarkerPlaying
	case queued:
		marker, markerStyle = "•", rowMarkerQueued
	}
	return markerStyle.Render(marker) + " " + rowMeta.Render(idx) + " " + rowTitle.Render(body)
}

// previewThumbDims returns thumbnail cell dimensions for the given video.
// Channel avatars are square profile pictures, so they need far fewer rows than
// a 16:9 video thumbnail, leaving room for the channel description.
func previewThumbDims(v video, l layout) (cols, rows int) {
	if v.isChannel() {
		rows = min(l.rows, 6)
		return rows * 2, rows
	}
	return l.cols, l.rows
}

// previewContent builds the full vertical content of the preview pane for v,
// along with any image control sequence (setupSeq) and the [thumbStart, thumbEnd)
// range of lines occupied by the thumbnail.
func (m model) previewContent(v video, l layout) (setupSeq string, thumbStart, thumbEnd int, lines []string) {
	w := l.rightW
	if w <= 0 {
		return "", -1, -1, nil
	}
	focused := m.focusPane == previewPane

	titleStyle := previewTitleDim
	if focused {
		titleStyle = previewTitle
	}
	lines = append(lines, titleStyle.Render(ansi.Truncate(v.Title, w, "…")))

	metaBarStyle, metaStyle := previewMetaBarD, previewMetaDim
	if focused {
		metaBarStyle, metaStyle = previewMetaBar, previewMeta
	}
	meta := v.channel()
	if v.isChannel() {
		meta = "Channel · " + meta
	}
	if dur := v.duration(); dur != "?:??" && !v.isChannel() {
		meta += "  ·  " + dur
	}
	lines = append(lines, metaBarStyle.Render("▍")+" "+metaStyle.Render(ansi.Truncate(meta, max(w-2, 1), "…")))
	lines = append(lines, "")

	thumbStart, thumbEnd = -1, -1
	cols, rows := previewThumbDims(v, l)
	key := thumbKey(v.ID, cols, rows)
	if !l.thumbOK {
		lines = append(lines, hint("terminal too small for a thumbnail", w))
	} else if art, ok := m.thumbs[key]; !ok {
		lines = append(lines, hint("loading thumbnail…", w))
	} else if art == "" {
		lines = append(lines, hint("no thumbnail available", w))
	} else if m.proto == protoAnsi {
		lines = append(lines, strings.Split(art, "\n")...)
	} else {
		seq, raw := splitThumbArt(art)
		setupSeq = seq
		thumbStart = len(lines)
		lines = append(lines, raw...)
		thumbEnd = len(lines)
	}

	d, ok := m.details[v.ID]
	if !ok && v.isChannel() && (v.Followers != nil || v.Description != "") {
		d = videoDetail{
			subs:        v.Followers,
			channelURL:  v.channelTargetURL(),
			channelID:   v.ChannelID,
			description: v.Description,
		}
		ok = true
	}
	if ok {
		if dl := d.lines(w); len(dl) > 0 {
			lines = append(lines, "")
			lines = append(lines, dl...)
		}
	} else if m.detailBusy[v.ID] {
		lines = append(lines, "")
		lines = append(lines, hint("loading details…", w))
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return setupSeq, thumbStart, thumbEnd, lines
}

func (m model) viewPreview(l layout, videos []video, cursor int) string {
	if l.rightW <= 0 || l.midH <= 0 {
		return ""
	}
	if len(videos) == 0 {
		return blankBlock(l.rightW, l.midH)
	}
	v := videos[cursor]

	setupSeq, thumbStart, thumbEnd, lines := m.previewContent(v, l)
	h := l.midH
	total := len(lines)
	scroll := m.descScroll
	if maxScroll := total - h; scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	end := min(scroll+h, total)

	visible := make([]string, 0, h)
	visible = append(visible, lines[scroll:end]...)
	for len(visible) < h {
		visible = append(visible, "")
	}

	if thumbStart >= 0 && thumbEnd > thumbStart && setupSeq != "" && scroll < thumbEnd && end > thumbStart {
		if m.proto == protoKitty {
			first := max(scroll, thumbStart)
			visible[first-scroll] = setupSeq + visible[first-scroll]
		} else if m.proto == protoSixel && scroll <= thumbStart {
			visible[thumbStart-scroll] = setupSeq + visible[thumbStart-scroll]
		}
	}

	for i := range visible {
		visible[i] = padRight(visible[i], l.rightW)
	}
	return strings.Join(visible, "\n")
}

// ---- helpers ----------------------------------------------------------------

// hint renders a short dim line that fits width.
func hint(s string, width int) string {
	return previewHintStyle.Render(ansi.Truncate(s, width, "…"))
}

// listRow lays out a list entry with the duration right-aligned: the title is
// truncated to leave room for a timestamp, and the row is exactly width runes.
func listRow(v video, width int) string {
	if width <= 0 {
		return ""
	}
	dur := v.duration()
	if dur == "?:??" || width < len(dur)+3 {
		return truncate(v.Title, width)
	}
	title := truncate(v.Title, width-len(dur)-1)
	pad := width - len([]rune(title)) - len(dur)
	if pad < 0 {
		pad = 0
	}
	return title + strings.Repeat(" ", pad) + dur
}

// truncate clips s to n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// padRight pads s with spaces to at least w visible columns.
func padRight(s string, w int) string {
	if sw := lipgloss.Width(s); sw < w {
		return s + strings.Repeat(" ", w-sw)
	}
	return s
}

// blankBlock returns a w-column, h-line block of spaces.
func blankBlock(w, h int) string {
	if w < 0 {
		w = 0
	}
	if h < 1 {
		h = 1
	}
	line := strings.Repeat(" ", w)
	lines := make([]string, h)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
