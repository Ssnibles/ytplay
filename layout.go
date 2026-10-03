package main

const (
	searchResults = 25

	// pageStep is how many rows PgUp/PgDn move the selection.
	pageStep = 10

	// previewChrome is how many lines the preview reserves for things other
	// than the image: title, metadata, gaps, stats and a line of slack. The
	// thumbnail is sized so all of that still fits under it.
	previewChrome = 7

	// detailLookahead is how many videos past the selection get their stats
	// prefetched. Stats are expensive (a full per-video extract), so fetching the
	// next few means scrolling into a neighbour usually finds them cached.
	detailLookahead = 3

	maxDetails = 512
	maxThumbs  = 64
)

// layout holds the page geometry, derived from the window size once per View so
// the pane arithmetic lives in one place.
//
// The frame is three rows: a one-line header bar, the content area, and a
// one-line status bar. The content area is a list column, a gap, a vertical
// rule, a gap, and the preview column. Every block is pinned to the exact
// content height so the frame fills the window and never scrolls: a frame that
// scrolls would drag the cell-anchored thumbnail out of place.
type layout struct {
	leftW   int  // list column width (columns)
	rightW  int  // preview column width (columns)
	midH    int  // pinned height of the content area (lines)
	avail   int  // usable list rows
	cols    int  // thumbnail cells wide (0 = not renderable)
	rows    int  // thumbnail cells tall (0 = not renderable)
	thumbOK bool // >0 cells fit the preview body
}

// computeLayout derives the page geometry from the window size. leftW is a
// bounded 42% sidebar; the preview takes the rest minus the two one-column gaps
// and the vertical rule.
func computeLayout(winW, winH int) layout {
	l := layout{}
	if winW <= 0 || winH <= 0 {
		return l
	}
	l.midH = winH - 2
	if l.midH < 1 {
		l.midH = 1
	}

	l.leftW = winW * 42 / 100
	if l.leftW > 64 {
		l.leftW = 64
	}
	if l.leftW < 24 {
		l.leftW = 24
	}
	l.rightW = winW - l.leftW - 3 // list + " " + "│" + " "
	if l.rightW < 20 {
		l.rightW = 20
		l.leftW = winW - l.rightW - 3
	}
	if l.leftW < 12 {
		l.leftW = 12
		l.rightW = winW - l.leftW - 3
	}
	if l.rightW < 0 {
		l.rightW = 0
	}
	if l.leftW < 0 {
		l.leftW = 0
	}

	l.avail = l.midH
	l.cols, l.rows, l.thumbOK = thumbDims(l.rightW, l.midH)
	return l
}

// thumbDims returns the cell size for a thumbnail that fits the preview body.
// The image takes the width it can while keeping a 16:9 aspect (a terminal cell
// is roughly 1:2, so 16:9 occupies cols/rows = 32/9).
func thumbDims(rightW, midH int) (cols, rows int, ok bool) {
	availCols := rightW
	availRows := midH - previewChrome
	if availRows < 4 || availCols < 16 {
		return 0, 0, false
	}
	rows = availRows
	cols = rows * 32 / 9
	if cols > availCols {
		cols = availCols
		rows = cols * 9 / 32
	}
	return cols, rows, true
}
