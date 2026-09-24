package main

const (
	searchResults = 25

	// pageStep is how many rows PgUp/PgDn move the selection.
	pageStep = 10

	// detailLines is how many rows the preview pane reserves under the thumbnail
	// for channel/video stats (subscribers, channel views, views · posted).
	detailLines = 3

	// detailLookahead is how many videos past the selection get their stats
	// prefetched. Stats are expensive (a full per-video extract), so fetching the
	// next few means scrolling into a neighbour usually finds them cached.
	detailLookahead = 3

	maxDetails = 512
	maxThumbs  = 64
)

// layout holds the results-pane geometry, derived from the window size once
// per View so the pane arithmetic lives in one place.
type layout struct {
	leftW   int  // list pane width (columns)
	rightW  int  // preview pane width (columns)
	midH    int  // pinned height of both pane boxes (lines)
	avail   int  // usable list rows inside the list box (borders excluded)
	cols    int  // thumbnail cells wide (0 = not renderable)
	rows    int  // thumbnail cells tall (0 = not renderable)
	thumbOK bool // >0 cells fit the preview body
}

// computeLayout derives pane geometry from the window size. Both panes are
// pinned to midH = winH-4 lines so header(1) + input(1) + panes + a two-line
// text/footer zone fill the window exactly, top to bottom: a frame that scrolls
// would desync the cell-anchored image. Widths are content widths (lipgloss
// adds the two border columns), so the outer boxes sum to exactly the window
// width. The preview body holds title + meta + spacer (three lines), the image,
// and detailLines of channel/video stats, so the image rows are capped at
// midH-6-detailLines.
func computeLayout(winW, winH int) layout {
	l := layout{}
	if winW <= 0 || winH <= 0 {
		return l
	}
	l.leftW = winW * 45 / 100
	l.rightW = winW - l.leftW - 1
	l.midH = winH - 4
	l.avail = l.midH - 2 // list rows: pane lines minus its two borders
	l.cols, l.rows, l.thumbOK = thumbDims(l.rightW, l.midH)
	return l
}

// thumbDims returns the cell size for a thumbnail that fits the preview pane
// body: availCols is the pane width minus its borders and padding; availRows
// is the pinned midH body minus the three text lines (title, meta, spacer),
// the detailLines reserved beneath the image, and a line of slack. A terminal
// cell is roughly 1:2 (w:h), so a 16:9 area occupies cols/rows = (16/9)*(1/2)
// = 32/9 cells.
func thumbDims(rightW, midH int) (cols, rows int, ok bool) {
	availCols := rightW - 4 // 2 borders + 2 padding
	availRows := midH - 6 - detailLines
	if availRows < 4 || availCols < 16 {
		return 0, 0, false
	}
	rows = availRows
	cols = rows * 32 / 9 // 16:9 in cells (1:2 cell aspect)
	if cols > availCols {
		cols = availCols
		rows = cols * 9 / 32
	}
	return cols, rows, true
}
