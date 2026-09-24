package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/blacktop/go-termimg"
)

// imgProto is the terminal image protocol selected at startup.
type imgProto int

const (
	protoAnsi  imgProto = iota // half-block pixel art (portable fallback)
	protoKitty                 // kitty graphics protocol (kitty, ghostty, wezterm…)
	protoSixel                 // DECSIXEL (foot, mlterm, xterm…)
)

func (p imgProto) String() string {
	switch p {
	case protoKitty:
		return "kitty"
	case protoSixel:
		return "sixel"
	default:
		return "halfblocks"
	}
}

// protocol maps an imgProto to the go-termimg protocol it renders with.
func (p imgProto) protocol() termimg.Protocol {
	switch p {
	case protoKitty:
		return termimg.Kitty
	case protoSixel:
		return termimg.Sixel
	default:
		return termimg.Halfblocks
	}
}

// detectImageProtocol picks the best native image protocol via go-termimg.
// The library probes the terminal (queries that read stdin), so this must run
// before bubbletea starts: bubbletea owns stdin from then on. Its results are
// cached for the whole run. go-termimg skips probing entirely for terminals it
// recognizes from the environment (kitty, ghostty, wezterm, rio, …) and when
// it detects no graphical support, so unknown terminals are only ever queried
// once, up front.
func detectImageProtocol() imgProto {
	_ = termimg.QueryTerminalFeatures()
	switch termimg.DetectProtocol() {
	case termimg.Kitty:
		return protoKitty
	case termimg.Sixel:
		return protoSixel
	default:
		return protoAnsi
	}
}

// thumbKey uniquely identifies a rendered block; native images are also tied
// to the terminal size they were painted at.
func thumbKey(id string, cols, rows int) string {
	return fmt.Sprintf("%s@%dx%d", id, cols, rows)
}

// renderThumb fetches a thumbnail and renders it as a single block suitable to
// drop into the preview pane:
//   - kitty: PNG transmit + Unicode placeholder grid (image-as-text) — the
//     block is ordinary 'rows'-line text that repaints, scrolls and diffs
//     with the pane, and kitty auto-drops the previous image with its cells;
//   - sixel: DECSIXEL graphics block (pixels painted below the cursor);
//   - ansi: half-block pixel art that is recomputed on every render.
func renderThumb(v video, cols, rows int, proto imgProto) (string, error) {
	data, err := fetchThumb(v)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return renderThumbData(img, cols, rows, proto)
}

// renderThumbData encodes img for the given protocol.
//
//   - kitty: PNG transmit + virtual placement (U=1) followed by a grid of
//     Unicode placeholder cells (image-as-text — the approach the kitty spec
//     recommends for TUIs and that image.nvim/ratatui-image etc. use). Each
//     render gets its own image number which is encoded in the cells, so a new
//     thumbnail never matches the previous one line-for-line, the renderer
//     rewrites the whole block, and kitty drops the old image with its cells.
//     The block is `rows` lines of ordinary width-1 text; bubbletea's line diff
//     skips it while the selection is unchanged.
//   - sixel: DECSIXEL paints `rows` cells down from the current cursor without
//     moving it, so the block ends with (rows-1) bare newlines — enough line
//     positions for the image, but no characters written into its cells (spaces
//     would overwrite sixel pixels).
//   - ansi: half-block pixel art, returned as-is.
func renderThumbData(img image.Image, cols, rows int, proto imgProto) (string, error) {
	// go-termimg's ResizeImage keeps a global LRU cache keyed by (target size,
	// path, source size). Every YouTube thumbnail is the same size and path, so
	// all videos would collide on one cache entry and render as the first image
	// ever resized — bust it each call (we cache the finished blocks ourselves
	// in m.thumbs instead).
	termimg.ClearResizeCache()
	ti := termimg.New(img).
		Protocol(proto.protocol()).
		Width(cols).
		Height(rows).
		Scale(termimg.ScaleFit).
		PNG(true)
	if proto == protoKitty {
		ti = ti.UseUnicode(true)
	}
	out, err := ti.Render()
	if err != nil {
		return "", err
	}
	if proto == protoSixel {
		return out + strings.Repeat("\n", rows-1), nil
	}
	return out, nil
}

// httpClient is shared by all thumbnail fetches. A hung connection must not
// wedge the TUI, so every request gets an overall deadline.
var httpClient = &http.Client{Timeout: 15 * time.Second}

func fetchThumb(v video) ([]byte, error) {
	url := v.thumbURL()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ytplay")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}
