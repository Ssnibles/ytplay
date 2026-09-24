package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

func (m model) titleLine() string {
	return titleStyle.Render(" ytplay ")
}

func (m model) viewPrompt() string {
	content := []string{
		promptTitleStyle.Render("Search YouTube"),
		"",
		m.input.View(),
	}
	if m.errMsg != "" {
		content = append(content, "", errorStyle.Render(m.errMsg))
	}

	cancelHint := "Ctrl+C / Esc to quit"
	if len(m.navStack) > 0 {
		cancelHint = "Esc to cancel / go back · Ctrl+C to quit"
	}
	hints := []string{
		greyHintStyle.Render("Type a query and press enter to search"),
		greyHintStyle.Render(cancelHint),
	}
	if len(m.history) > 0 {
		hints = append(hints,
			greyHintStyle.Render(
				fmt.Sprintf("Ctrl+P / Ctrl+N to recall past searches (%d)", len(m.history))),
		)
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center,
			m.titleLine(),
			promptBoxStyle.Render(strings.Join(content, "\n")),
			strings.Join(hints, "\n")))
}

func (m model) viewSearching() string {
	msg := searchingPrefixStyle.Render("Searching for ") +
		searchingQueryStyle.Render("“"+m.query+"”")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, m.titleLine(), m.spin.View(), " ", msg))
}

func (m model) viewQueue() string {
	l := computeLayout(m.width, m.height)

	// The queue page reuses the exact results layout: header, bar, the two
	// content panes (list + preview), a hint footer, and status/errors.
	header := headerStyle.Render(
		lipgloss.JoinHorizontal(lipgloss.Left,
			m.titleLine(),
			dimStyle.Render(fmt.Sprintf(" Queue · %d videos", len(m.queue))),
		),
	)

	bar := "Up next"
	hint := ""
	if len(m.queue) > 0 {
		hint = "j/k move · K/J reorder · x remove · Enter play · p play all · c copy · o open · / search · Esc back"
	}
	empty := ""
	if len(m.queue) == 0 && m.errMsg == "" {
		empty = "queue is empty — press a on a result to add videos"
	}

	return m.pageFrame(header, bar, hint, m.contentPanes(l, m.queue, m.queueCursor), empty)
}

// actionHints summarises the results-screen key bindings for the footer.
func (m model) actionHints() string {
	if m.focusPane == previewPane {
		return "j/k or ↑/↓ scroll desc · h/← list · Enter channel · o open · / search · Esc back"
	}
	return "Enter play · a queue · l/→ details · Tab focus · c copy · o open · / search"
}

func (m model) viewResults() string {
	l := computeLayout(m.width, m.height)

	header := headerStyle.Render(
		lipgloss.JoinHorizontal(lipgloss.Left,
			m.titleLine(),
			dimStyle.Render(fmt.Sprintf(" %d results", len(m.filtered))),
			accentStyle.Render(fmt.Sprintf(" · %d queued", len(m.queue))),
		),
	)

	// The input row is replaced by a plain read-only line here: typing is
	// ignored on the results screen (/ returns to the editable prompt).
	bar := "Search: " + m.query
	if m.fetchingMore {
		bar += "  ·  loading more…"
	}

	hint := ""
	if len(m.filtered) > 0 {
		hint = m.actionHints()
	}
	empty := ""
	if len(m.filtered) == 0 && m.errMsg == "" {
		empty = "Nothing to show — press / to search"
	}

	return m.pageFrame(header, bar, hint, m.contentPanes(l, m.filtered, m.cursor), empty)
}

func (m model) viewChannel() string {
	l := computeLayout(m.width, m.height)

	header := headerStyle.Render(
		lipgloss.JoinHorizontal(lipgloss.Left,
			m.titleLine(),
			dimStyle.Render(fmt.Sprintf(" Channel · %s", m.channelTitle)),
			dimStyle.Render(fmt.Sprintf(" · %d videos", len(m.channelVideos))),
			accentStyle.Render(fmt.Sprintf(" · %d queued", len(m.queue))),
		),
	)

	bar := "Channel: " + m.channelTitle
	if m.channelFetchingMore {
		bar += "  ·  loading more…"
	}

	hint := ""
	if len(m.channelVideos) > 0 {
		hint = m.actionHints()
	}
	empty := ""
	if len(m.channelVideos) == 0 && m.errMsg == "" {
		if m.channelLoading {
			empty = "Loading channel videos…"
		} else {
			empty = "Nothing to show — press Esc to go back"
		}
	}

	return m.pageFrame(header, bar, hint, m.contentPanes(l, m.channelVideos, m.channelCursor), empty)
}

// pageFrame assembles the results-style layout shared by the results and queue
// pages: header, read-only bar, the two content panes, and a two-line footer
// zone (a hint on the first row, then error/status/empty on the second) that is
// always rendered — blank when unused — so the page fills the window exactly.
func (m model) pageFrame(header, bar, hint, mid, empty string) string {
	var out strings.Builder
	out.WriteString(header)
	out.WriteString("\n")
	out.WriteString(barStyle.Render(bar))
	out.WriteString("\n")
	out.WriteString(mid)

	// footer row one: hints (blank when there is nothing to hint about)
	if hint != "" {
		out.WriteString("\n" + footerHintStyle.Render(hint))
	} else {
		out.WriteString("\n")
	}
	// footer row two: error, else status, else the empty-state message
	switch {
	case m.errMsg != "":
		out.WriteString("\n" + errorStyle.Render(m.errMsg))
	case m.status != "":
		out.WriteString("\n" + statusStyle.Render(m.status))
	case empty != "":
		out.WriteString("\n" + emptyStyle.Render(empty))
	default:
		out.WriteString("\n")
	}

	// Belt and suspenders: the panes are height-pinned so the frame always
	// fits the window, but clamp anyway — if a render ever spilled past the
	// bottom the terminal would scroll under the cell-attached image and
	// desync the whole layout.
	lines := strings.Split(out.String(), "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

// contentPanes renders the shared two-pane list/preview layout for videos.
func (m model) contentPanes(l layout, videos []video, cursor int) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, m.viewList(l, videos, cursor), " ", m.viewPreview(l, videos, cursor))
}

func (m model) viewList(l layout, videos []video, cursor int) string {
	boxStyle := paneBoxStyle
	if m.focusPane == listPane {
		boxStyle = paneBoxActiveStyle
	}
	if len(videos) == 0 {
		return boxStyle.Width(l.leftW - 2).Height(l.midH - 2).Render("")
	}

	// Keep the cursor on screen: once it passes the last visible row, the
	// window scrolls instead of letting the highlight vanish off the bottom.
	start := 0
	if cursor >= l.avail {
		start = cursor - l.avail + 1
	}
	end := start + l.avail
	if end > len(videos) {
		end = len(videos)
	}

	var lines []string
	for i := start; i < end; i++ {
		v := videos[i]
		marker := " "
		if i == cursor {
			marker = "▌"
		}
		line := marker + " " + listRow(v, l.leftW-6)
		if i == cursor {
			if m.focusPane == previewPane {
				line = listRowStyle.Render(line)
			} else {
				line = listActiveRowStyle.Render(line)
			}
		} else {
			line = listRowStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return boxStyle.Width(l.leftW - 2).Height(l.midH - 2).Render(strings.Join(lines, "\n"))
}

// hint renders a short dim grey line that fits the preview pane body.
func hint(s string, width int) string {
	return greyHintStyle.Render(truncate(s, width))
}

// previewThumbDims returns thumbnail cell dimensions for the given video.
// Channel avatars are square profile pictures, so they need far fewer rows
// than a 16:9 video thumbnail, leaving ample room for the channel description.
func previewThumbDims(v video, l layout) (cols, rows int) {
	if v.isChannel() {
		rows = l.rows
		if rows > 6 {
			rows = 6
		}
		return rows * 2, rows
	}
	return l.cols, l.rows
}

// previewContent builds the complete vertical lines of the preview pane for video v,
// along with any image control sequence (setupSeq) and the line range [thumbStart, thumbEnd)
// occupied by the thumbnail within the lines slice.
func (m model) previewContent(v video, l layout) (setupSeq string, thumbStart, thumbEnd int, lines []string) {
	w := l.rightW - 4
	if w <= 0 {
		return "", -1, -1, nil
	}

	title := truncate(v.Title, w)
	if dur := v.duration(); dur != "?:??" && !v.isChannel() {
		title = truncate(v.Title, w-len(dur)-3) + " • " + dur
	}
	lines = append(lines, previewTitleStyle.Render(title))

	chName := v.channel()
	if v.isChannel() {
		chName = "Channel: " + chName
	}
	if m.focusPane == previewPane {
		lines = append(lines, previewChannelActiveStyle.Render(truncate("▶ "+chName+" (Enter: view channel)", w)))
	} else {
		lines = append(lines, previewChannelStyle.Render(truncate(chName, w)))
	}
	lines = append(lines, "")

	thumbStart = -1
	thumbEnd = -1

	cols, rows := previewThumbDims(v, l)
	key := thumbKey(v.ID, cols, rows)
	if !l.thumbOK {
		lines = append(lines, hint("terminal too small for a thumbnail", w))
	} else if art, ok := m.thumbs[key]; ok {
		switch {
		case art == "":
			lines = append(lines, hint("no thumbnail available", w))
		case m.proto == protoAnsi:
			artLines := strings.Split(art, "\n")
			for _, ln := range artLines {
				lines = append(lines, simplePad(ln, w))
			}
		default:
			seq, rawLines := splitThumbArt(art)
			setupSeq = seq
			thumbStart = len(lines)
			lines = append(lines, rawLines...)
			thumbEnd = len(lines)
		}
	} else {
		lines = append(lines, hint("loading thumbnail…", w))
	}
	if m.thumbBusy[key] {
		lines = append(lines, hint("fetching thumbnail…", w))
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
		detailLines := d.lines(w)
		if len(detailLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, detailLines...)
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
	boxStyle := paneBoxStyle
	if m.focusPane == previewPane {
		boxStyle = paneBoxActiveStyle
	}
	if len(videos) == 0 {
		return boxStyle.Width(l.rightW - 2).Height(l.midH - 2).Render("")
	}
	v := videos[cursor]

	setupSeq, thumbStart, thumbEnd, lines := m.previewContent(v, l)

	h := l.midH - 2
	if h <= 0 {
		return boxStyle.Width(l.rightW - 2).Height(0).Render("")
	}

	total := len(lines)
	maxScroll := total - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.descScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	end := scroll + h
	if end > total {
		end = total
	}

	visible := make([]string, end-scroll)
	copy(visible, lines[scroll:end])

	if thumbStart >= 0 && thumbEnd > thumbStart && setupSeq != "" {
		if scroll < thumbEnd && end > thumbStart {
			if m.proto == protoKitty {
				firstVisibleThumb := max(scroll, thumbStart)
				visIdx := firstVisibleThumb - scroll
				visible[visIdx] = setupSeq + visible[visIdx]
			} else if m.proto == protoSixel {
				if scroll <= thumbStart {
					visIdx := thumbStart - scroll
					visible[visIdx] = setupSeq + visible[visIdx]
				}
			}
		}
	}

	return boxStyle.Width(l.rightW - 2).Height(l.midH - 2).Render(strings.Join(visible, "\n"))
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

// simplePad pads a line (which may contain trailing ANSI escapes) with spaces.
func simplePad(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return s + strings.Repeat(" ", n)
}
