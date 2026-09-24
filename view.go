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
		return "Enter view channel · Tab back to list · o open video · / search · Esc back"
	}
	return "Enter play · a queue · q view queue · Tab channel · c copy link · o open · / search · Esc back"
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
	if len(videos) == 0 {
		return lipgloss.NewStyle().Width(l.leftW - 2).Height(l.midH - 2).Render("")
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

	return paneBoxStyle.Width(l.leftW - 2).Height(l.midH - 2).Render(strings.Join(lines, "\n"))
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

func (m model) viewPreview(l layout, videos []video, cursor int) string {
	if len(videos) == 0 {
		return lipgloss.NewStyle().Width(l.rightW).Height(l.midH - 2).Render("")
	}
	v := videos[cursor]

	// usable body width: the box is Width(rightW-2) with 1 column of inner
	// padding each side.
	w := l.rightW - 4

	var body strings.Builder
	title := truncate(v.Title, w)
	if dur := v.duration(); dur != "?:??" && !v.isChannel() {
		title = truncate(v.Title, w-len(dur)-3) + " • " + dur
	}
	body.WriteString(previewTitleStyle.Render(title))
	body.WriteString("\n")

	chName := v.channel()
	if v.isChannel() {
		chName = "Channel: " + chName
	}
	if m.focusPane == previewPane {
		body.WriteString(previewChannelActiveStyle.Render(truncate("▶ "+chName+" (Enter: view channel)", w)))
	} else {
		body.WriteString(previewChannelStyle.Render(truncate(chName, w)))
	}
	body.WriteString("\n\n")

	cols, rows := previewThumbDims(v, l)
	key := thumbKey(v.ID, cols, rows)
	if !l.thumbOK {
		body.WriteString(hint("terminal too small for a thumbnail", w))
	} else if art, ok := m.thumbs[key]; ok {
		switch {
		case art == "":
			body.WriteString(hint("no thumbnail available", w))
		case m.proto == protoAnsi:
			// half-block art is plain text: it is recomputed by the renderer
			// each frame and padded to the pane width.
			artLines := strings.Split(art, "\n")
			for i, ln := range artLines {
				artLines[i] = simplePad(ln, w)
			}
			body.WriteString(strings.Join(artLines, "\n"))
		default:
			// native image: the block (transmit + virtual placement + a grid of
			// Unicode placeholder cells, `rows` lines) is cached by id@size; the
			// per-image kitty number is encoded in the cells themselves, so
			// switching videos rewrites them all and repaints the new image.
			body.WriteString(art)
		}
	} else {
		body.WriteString(hint("loading thumbnail…", w))
	}
	if m.thumbBusy[key] {
		body.WriteString("\n" + hint("fetching thumbnail…", w))
	}

	// channel & video stats below the thumbnail
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
		linesInBody := strings.Count(body.String(), "\n") + 1
		availDetail := (l.midH - 2) - linesInBody - 1
		if availDetail > 0 {
			if lines := d.lines(w, availDetail); len(lines) > 0 {
				body.WriteString("\n")
				body.WriteString(strings.Join(lines, "\n"))
			}
		}
	} else if m.detailBusy[v.ID] {
		body.WriteString("\n" + hint("loading details…", w))
	}

	return paneBoxStyle.Width(l.rightW - 2).Height(l.midH - 2).Render(body.String())
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
