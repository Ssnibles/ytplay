package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	"github.com/blacktop/go-termimg"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- palette (matches "vague" theme) ------------------------------------

var (
	accent  = lipgloss.Color("6e94b2")
	red     = lipgloss.Color("d8647e")
	fg      = lipgloss.Color("cdcdcd")
	fgMid   = lipgloss.Color("878787")
	fgDim   = lipgloss.Color("606079")
	bgRound = lipgloss.Color("252530")
)

const searchResults = 25

// searchDebounce is how long the prompt waits after the last keystroke before
// firing a type-ahead search.
const searchDebounce = 400 * time.Millisecond

// pageStep is how many rows PgUp/PgDn move the selection.
const pageStep = 10

// detailLines is how many rows the preview pane reserves under the thumbnail
// for channel/video stats (subscribers, channel views, views · posted).
const detailLines = 3

// detailLookahead is how many videos past the selection get their stats
// prefetched. Stats are expensive (a full per-video extract), so fetching the
// next few means scrolling into a neighbour usually finds them cached.
const detailLookahead = 3

const maxDetails = 512
const maxThumbs = 64

// historyCap bounds how many searches are recalled/persisted.
const historyCap = 100

// historyFilePath returns where past queries are stored; overridable in tests.
var historyFilePath = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ytplay", "history")
}

func loadHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if q := strings.TrimSpace(sc.Text()); q != "" {
			out = append(out, q)
		}
	}
	return out
}

func saveHistory(path string, queries []string) error {
	if path == "" || len(queries) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, q := range queries {
		fmt.Fprintln(w, q)
	}
	return w.Flush()
}

// historyPrev steps toward older queries, remembering the typed text so
// Ctrl+N can return to it.
func (m model) historyPrev() model {
	if len(m.history) == 0 {
		return m
	}
	if m.histIdx == -1 {
		m.pending = m.input.Value()
		m.histIdx = len(m.history) - 1
	} else if m.histIdx > 0 {
		m.histIdx--
	}
	m.input.SetValue(m.history[m.histIdx])
	m.input.CursorEnd()
	return m
}

// historyNext steps toward newer queries and back to the typed text.
func (m model) historyNext() model {
	if m.histIdx == -1 {
		return m
	}
	if m.histIdx < len(m.history)-1 {
		m.histIdx++
		m.input.SetValue(m.history[m.histIdx])
	} else {
		m.histIdx = -1
		m.input.SetValue(m.pending)
	}
	m.input.CursorEnd()
	return m
}

// rememberQuery records a completed search in the persisted history.
func (m model) rememberQuery(q string) model {
	if q == "" {
		return m
	}
	for _, x := range m.history {
		if x == q {
			return m
		}
	}
	m.history = append(m.history, q)
	if len(m.history) > historyCap {
		m.history = m.history[len(m.history)-historyCap:]
	}
	_ = saveHistory(historyFilePath(), m.history)
	return m
}

// ---- video ---------------------------------------------------------------

type video struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Channel   string   `json:"channel"`
	Uploader  string   `json:"uploader"`
	Duration  *float64 `json:"duration"`
	ChannelID string   `json:"channel_id"`
}

func (v video) watchURL() string {
	if strings.HasPrefix(v.URL, "http") {
		return v.URL
	}
	return "https://www.youtube.com/watch?v=" + v.ID
}

func (v video) channel() string {
	if v.Channel != "" {
		return v.Channel
	}
	return v.Uploader
}

func (v video) duration() string {
	if v.Duration == nil || *v.Duration <= 0 {
		return "?:??"
	}
	d := int(math.Round(*v.Duration))
	h := d / 3600
	m := (d % 3600) / 60
	s := d % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// videoDetail carries channel/video stats fetched on demand for the selected
// video — flat search results don't include any of these. Fields are pointers
// because YouTube omits them for some videos/channels (hidden subscriber
// counts, unlisted views, …).
type videoDetail struct {
	subs       *int64 // channel subscriber count
	chViews    *int64 // total views across all the channel's videos
	views      *int64 // views on this video
	likes      *int64 // likes on this video
	uploaded   string // upload date, YYYYMMDD
	channelURL string // canonical channel URL
	channelID  string // channel id (fallback when URL is missing)
}

func (d videoDetail) date() string {
	t, err := time.Parse("20060102", d.uploaded)
	if err != nil {
		return ""
	}
	return t.Format("Jan 2, 2006")
}

// channelLink resolves the canonical channel URL for a video, preferring the
// full URL the extractor reported and falling back to the channel id.
func (d videoDetail) channelLink() string {
	if d.channelURL != "" {
		return d.channelURL
	}
	if d.channelID != "" {
		return "https://www.youtube.com/channel/" + d.channelID
	}
	return ""
}

// lines renders the stats as up to detailLines rows, combining views and post
// date on one line. Unknown fields are skipped.
func (d videoDetail) lines(width int) []string {
	var out []string
	if d.subs != nil {
		out = append(out, hint(formatCount(*d.subs)+" subscribers", width))
	}
	if d.chViews != nil {
		out = append(out, hint(formatCount(*d.chViews)+" total channel views", width))
	}
	var detail []string
	if d.views != nil {
		detail = append(detail, formatCount(*d.views)+" views")
	}
	if d.likes != nil {
		detail = append(detail, formatCount(*d.likes)+" likes")
	}
	if d.date() != "" {
		detail = append(detail, "posted "+d.date())
	}
	if len(detail) > 0 {
		out = append(out, hint(strings.Join(detail, " · "), width))
	}
	return out[:min(len(out), detailLines)]
}

// formatCount renders a view/subscriber count compactly: 999, 1.2k, 45k, 1.5M.
func formatCount(n int64) string {
	var div int64
	var suffix string
	switch {
	case n >= 1_000_000_000:
		div, suffix = 1_000_000_000, "B"
	case n >= 1_000_000:
		div, suffix = 1_000_000, "M"
	case n >= 1_000:
		div, suffix = 1_000, "k"
	default:
		return strconv.FormatInt(n, 10)
	}
	s := fmt.Sprintf("%.1f", float64(n)/float64(div))
	return strings.TrimSuffix(s, ".0") + suffix
}

// ---- messages -------------------------------------------------------------

// searchMsg delivers one yt-dlp search batch. A "more" batch refetches the
// query with a larger limit and is merged onto the results already shown, so
// scrolling can page deeper into the channel the way a web search does.
// gen tags the search generation so stale results (superseded by a newer
// fired search) can be dropped; live marks a type-ahead (non-Enter) search.
type searchMsg struct {
	videos []video
	err    error
	limit  int
	more   bool
	gen    int
	live   bool
}

type debounceMsg struct {
	gen   int
	query string
}

type thumbMsg struct {
	id   string
	art  string
	cols int
	rows int
	err  error
}

type detailMsg struct {
	id  string
	det videoDetail
	err error
}

type copyMsg struct {
	url string
	err error
}

type openMsg struct {
	url string
	err error
}

// ---- commands -------------------------------------------------------------

// searchCmd fetches results via yt-dlp's ytsearchN: syntax. N is the total
// count asked for: "more" batches re-ask with a bigger N (yt-dlp pages
// internally) and the results are deduplicated by ID on merge.
func searchCmd(query string, limit int, more bool, gen int, live bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "yt-dlp",
			"--flat-playlist", "--skip-download", "--no-warnings", "--no-playlist",
			"-J", fmt.Sprintf("ytsearch%d:%s", limit, query))
		out, err := cmd.Output()
		if err != nil {
			return searchMsg{err: fmt.Errorf("yt-dlp: %w", err), limit: limit, more: more, gen: gen, live: live}
		}

		var res struct {
			Entries []video `json:"entries"`
		}
		if err := json.Unmarshal(out, &res); err != nil {
			return searchMsg{err: fmt.Errorf("parse: %w", err), limit: limit, more: more, gen: gen, live: live}
		}

		videos := make([]video, 0, len(res.Entries))
		seen := make(map[string]bool, len(res.Entries))
		for _, v := range res.Entries {
			if v.ID == "" || v.Title == "" || seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			videos = append(videos, v)
		}
		if len(videos) == 0 {
			// a follow-up page hitting the end comes back empty — that's not
			// an error, it just means there is nothing more to load.
			if more {
				return searchMsg{limit: limit, more: more, gen: gen, live: live}
			}
			return searchMsg{err: fmt.Errorf("no results"), limit: limit, more: more, gen: gen, live: live}
		}
		return searchMsg{videos: videos, limit: limit, more: more, gen: gen, live: live}
	}
}

// mergeResults appends the videos in extra that aren't already in base,
// keeping base's order (and YouTube's order within extra).
func mergeResults(base, extra []video) []video {
	if len(base) == 0 {
		return extra
	}
	seen := make(map[string]bool, len(base)+len(extra))
	for _, v := range base {
		seen[v.ID] = true
	}
	merged := make([]video, len(base), len(base)+len(extra))
	copy(merged, base)
	for _, v := range extra {
		if !seen[v.ID] {
			seen[v.ID] = true
			merged = append(merged, v)
		}
	}
	return merged
}

// detailCmd extracts channel/video stats for one video. A full (non-flat)
// extraction is needed, so this runs for the selected video only.
func detailCmd(v video) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, "yt-dlp",
			"--no-playlist", "--skip-download", "--no-warnings",
			"-J", v.watchURL()).Output()
		if err != nil {
			return detailMsg{v.ID, videoDetail{}, err}
		}

		var d struct {
			Subs       *int64 `json:"channel_follower_count"`
			ChViews    *int64 `json:"channel_view_count"`
			Views      *int64 `json:"view_count"`
			Likes      *int64 `json:"like_count"`
			Date       string `json:"upload_date"`
			ChannelURL string `json:"channel_url"`
			ChannelID  string `json:"channel_id"`
		}
		if err := json.Unmarshal(out, &d); err != nil {
			return detailMsg{v.ID, videoDetail{}, err}
		}
		return detailMsg{v.ID, videoDetail{
			subs: d.Subs, chViews: d.ChViews, views: d.Views, likes: d.Likes,
			uploaded: d.Date, channelURL: d.ChannelURL, channelID: d.ChannelID,
		}, nil}
	}
}

// ---- image rendering --------------------------------------------------------

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

func thumbCmd(v video, cols, rows int, proto imgProto) tea.Cmd {
	return func() tea.Msg {
		art, err := renderThumb(v, cols, rows, proto)
		if err != nil {
			return thumbMsg{v.ID, "", cols, rows, err}
		}
		return thumbMsg{v.ID, art, cols, rows, nil}
	}
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
		Scale(termimg.ScaleFill).
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
	url := fmt.Sprintf("https://i.ytimg.com/vi/%s/mqdefault.jpg", v.ID)
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

// copyURLCmd writes url to the system clipboard (yank) and reports back so the
// TUI can confirm without blocking on the clipboard service.
func copyURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(url); err != nil {
			return copyMsg{url, err}
		}
		return copyMsg{url, nil}
	}
}

// openChannelCmd opens a URL in the system browser (via xdg-open) off the TUI.
func openChannelCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return openMsg{"", fmt.Errorf("no channel available")}
		}
		devnull, err := os.Open(os.DevNull)
		if err != nil {
			return openMsg{url, err}
		}
		defer devnull.Close()
		cmd := exec.Command("xdg-open", url)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
		if err := cmd.Run(); err != nil {
			return openMsg{url, err}
		}
		return openMsg{url, nil}
	}
}

// openChannel returns the browser command for the selected video, using the
// detail lookup for its channel URL when the flat search entry lacks one.
func (m model) openChannel(v video) tea.Cmd {
	if v.ChannelID != "" {
		return openChannelCmd("https://www.youtube.com/channel/" + v.ChannelID)
	}
	if d, ok := m.details[v.ID]; ok {
		if url := d.channelLink(); url != "" {
			return openChannelCmd(url)
		}
	}
	return openChannelCmd("")
}

// ---- model -----------------------------------------------------------------

type state int

const (
	promptState state = iota
	searchingState
	resultsState
)

type model struct {
	input        textinput.Model
	spin         spinner.Model
	state        state
	query        string
	filtered     []video
	cursor       int
	width        int
	height       int
	proto        imgProto
	thumbs       map[string]string
	thumbBusy    map[string]bool
	details      map[string]videoDetail
	detailBusy   map[string]bool
	fetched      int      // how many results have been asked for so far
	fetchingMore bool     // a follow-up page is in flight
	history      []string // past queries, oldest first
	histIdx      int      // -1 = editing fresh text, else index into history
	pending      string   // typed text saved when entering history navigation
	searchGen    int      // increments per fired search; stale results are dropped
	lastFired    string   // query of the most recently fired search
	liveBusy     bool     // a type-ahead search is in flight
	queue        []video  // videos staged for sequential playback
	status       string
	errMsg       string
}

func initialModel(args []string) model {
	ti := textinput.New()
	// No in-field placeholder: when the value is empty, textinput renders the
	// cursor as the placeholder's first rune (a stray glyph). The hint line in
	// viewPrompt covers the "what do I type here" case instead.
	ti.Placeholder = ""
	ti.CharLimit = 120
	ti.Prompt = "❯ "
	ti.TextStyle = lipgloss.NewStyle().Foreground(fg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(fgDim)

	// Focus must be set on the model that the update loop actually uses.
	// Init() runs on a copy (it only returns a Cmd), so focusing there
	// would be lost.
	ti.Focus()

	m := model{
		input:      ti,
		spin:       spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent))),
		state:      promptState,
		proto:      protoAnsi,
		thumbs:     make(map[string]string),
		thumbBusy:  make(map[string]bool),
		details:    make(map[string]videoDetail),
		detailBusy: make(map[string]bool),
		history:    loadHistory(historyFilePath()),
		histIdx:    -1,
	}
	if len(args) > 0 {
		m.query = strings.Join(args, " ")
		m.input.SetValue(m.query)
		m.state = searchingState
	}
	return m
}

// ---- layout -----------------------------------------------------------------

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
// pinned to winH-6 lines so header(1) + input(1) + panes + two footer lines
// can never exceed the window: a taller frame would scroll the terminal and
// desync the cell-anchored image. The preview body (borders out) holds title +
// meta + spacer (three lines), the image, and detailLines of channel/video
// stats, so the image rows are capped at midH-6-detailLines.
func computeLayout(winW, winH int) layout {
	l := layout{}
	if winW <= 0 || winH <= 0 {
		return l
	}
	l.leftW = winW * 45 / 100
	l.rightW = winW - l.leftW - 1
	l.midH = winH - 6
	l.avail = l.midH - 2 // list rows: pane lines minus its two borders
	l.cols, l.rows, l.thumbOK = thumbDims(l.rightW, l.midH)
	return l
}

// liveSearchState prepares a debounced type-ahead search for whatever the user
// typed, returning the new generation (only advanced when a tick is armed).
// Every keystroke bumps the generation; only the newest surviving tick fires,
// and it re-checks the text before doing so.
func (m model) liveSearchState() (int, tea.Cmd) {
	if m.state != promptState {
		return m.searchGen, nil
	}
	q := strings.TrimSpace(m.input.Value())
	if q == "" || q == m.lastFired {
		return m.searchGen, nil
	}
	gen := m.searchGen + 1
	return gen, tea.Tick(searchDebounce, func(_ time.Time) tea.Msg {
		return debounceMsg{gen: gen, query: q}
	})
}

// ---- Update ---------------------------------------------------------------

func (m model) Init() tea.Cmd {
	// The input is focused on the model the update loop runs (set in
	// initialModel). Init runs on a copy, so focusing here would be lost.
	if m.state == searchingState {
		return searchCmd(m.query, searchResults, false, m.searchGen, false)
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
		m.width, m.height = msg.Width, msg.Height
		// native image blocks are anchored to a cell position and painted at a
		// pixel size, so any size change invalidates them (the cache key
		// includes the cell size too).
		m.thumbs = make(map[string]string)
		if len(m.filtered) > 0 {
			cmds = append(cmds, m.loadSelection())
		}

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD:
			return m, tea.Quit
		}

		switch m.state {
		case promptState:
			switch msg.Type {
			case tea.KeyEsc:
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
				m.query = q
				m = m.rememberQuery(q)
				m.searchGen++
				m.liveBusy = false
				m.state = searchingState
				return m, searchCmd(q, searchResults, false, m.searchGen, false)
			default:
				m.histIdx = -1
				m.pending = ""
				// Typing and editing reschedule the type-ahead search; arrow
				// keys also land here, but the debounce re-checks the query
				// text, so the extra tick is a harmless no-op.
				gen, c := m.liveSearchState()
				if c != nil {
					m.searchGen = gen
					cmds = append(cmds, c)
				}
			}

		case searchingState:
			if msg.Type == tea.KeyEsc {
				// a search is single-run; Esc bails out of it.
				return m, tea.Quit
			}

		case resultsState:
			switch msg.Type {
			case tea.KeyEsc:
				// back to the search bar for a fresh search
				m.state = promptState
				m.errMsg = ""
				m.status = ""
				m.liveBusy = false
				m.lastFired = ""
				return m, nil
			case tea.KeyEnter:
				// Launch mpv in the background (Cmd.Start, non-blocking) and keep
				// the interface open so more videos can be picked.
				if len(m.filtered) == 0 {
					return m, nil
				}
				v := m.filtered[m.cursor]
				if err := playInMPV(v.watchURL()); err != nil {
					m.errMsg = err.Error()
					return m, nil
				}
				return m, nil
			}

			// selection movement (arrows, tabs, and vim j/k)
			var down, up bool
			switch msg.Type {
			case tea.KeyDown, tea.KeyTab:
				down = true
			case tea.KeyUp, tea.KeyShiftTab:
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
						if c := m.openChannel(m.filtered[m.cursor]); c != nil {
							cmds = append(cmds, c)
						}
					}
				case "a":
					if len(m.filtered) > 0 {
						m.queue = append(m.queue, m.filtered[m.cursor])
						m.errMsg = ""
						m.status = fmt.Sprintf("queued · %d in queue", len(m.queue))
					}
				case "p":
					m = m.playQueue()
				}
			}
			if down && m.cursor < len(m.filtered)-1 {
				m.cursor++
				cmds = append(cmds, m.loadSelection())
			}
			if up && m.cursor > 0 {
				m.cursor--
				cmds = append(cmds, m.loadSelection())
			}

			// Paging deeper into the channel: once the selection nears the
			// bottom of what has been fetched, ask for the next page (unless a
			// fetch is already in flight). Only key presses trigger this, so
			// after a merge the user still decides when to scroll on.
			if m.shouldLoadMore() {
				m.fetchingMore = true
				cmds = append(cmds, searchCmd(m.query, m.fetched+searchResults, true, m.searchGen, false))
			}
		}

	case searchMsg:
		m.fetchingMore = false
		if msg.gen != m.searchGen {
			// superseded by a newer fired search; drop the stale batch.
			return m, nil
		}
		m.liveBusy = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			if !msg.more {
				m.filtered = nil
				if msg.live {
					m.state = promptState
				} else {
					m.state = resultsState
				}
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
		cmds = append(cmds, m.loadSelection())

	case debounceMsg:
		if msg.gen != m.searchGen || m.state != promptState {
			return m, nil
		}
		q := strings.TrimSpace(m.input.Value())
		if q == "" || q != msg.query {
			return m, nil
		}
		m.query = q
		m.lastFired = q
		m.liveBusy = true
		m.errMsg = ""
		return m, searchCmd(q, searchResults, false, m.searchGen, true)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case thumbMsg:
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

	case detailMsg:
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
			return m, nil
		}
		m.details[msg.id] = msg.det
	case copyMsg:
		if msg.err != nil {
			m.status = ""
			m.errMsg = "copy: " + msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		m.status = "copied " + msg.url

	case openMsg:
		if msg.err != nil {
			m.status = ""
			m.errMsg = "open: " + msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		m.status = "opened " + msg.url
	}

	if m.state == searchingState {
		cmds = append(cmds, m.spin.Tick)
	}

	return m, tea.Batch(cmds...)
}

// loadThumb issues a thumbnail render for the currently selected video if it
// hasn't been rendered at the current pane size. Busy tracking is keyed the
// same way as the render cache (id@size), so a resize in flight doesn't get
// suppressed by a smaller-size fetch that is still queued.
func (m model) loadThumb() tea.Cmd {
	if len(m.filtered) == 0 || m.state != resultsState {
		return nil
	}
	l := computeLayout(m.width, m.height)
	if !l.thumbOK {
		return nil
	}
	v := m.filtered[m.cursor]
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
	var cmds []tea.Cmd
	if c := m.loadThumb(); c != nil {
		cmds = append(cmds, c)
	}
	// Stats are the slow part of a selection (a full extract per video), so
	// fetch a small lookahead window rather than only the selected one: by the
	// time the user scrolls into a neighbour its stats are usually cached.
	// Each fetch is a no-op when cached or in flight, so this stays cheap.
	for i := 0; i <= detailLookahead && m.cursor+i < len(m.filtered); i++ {
		if c := m.loadDetailAt(m.cursor + i); c != nil {
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

// thumbKey uniquely identifies a rendered block; native images are also tied
// to the terminal size they were painted at.
func thumbKey(id string, cols, rows int) string {
	return fmt.Sprintf("%s@%dx%d", id, cols, rows)
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
	return m
}

func playInMPV(urls ...string) error {
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("mpv: %w", err)
	}
	defer devnull.Close()

	cmd := exec.Command("mpv", urls...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Detach mpv from the TUI's streams: stdin would otherwise receive the
	// keystrokes the TUI is listening for, and mpv's own output would print
	// into the alternate screen and desync the cell-anchored image layout.
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mpv: %w", err)
	}
	return nil
}

// ---- View ------------------------------------------------------------------

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "ytplay — waiting for terminal…"
	}
	switch m.state {
	case promptState:
		return m.viewPrompt()
	case searchingState:
		return m.viewSearching()
	default:
		return m.viewResults()
	}
}

func (m model) titleLine() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("000000")).
		Background(accent).
		Padding(0, 1).
		Bold(true)
	return style.Render(" ytplay ")
}

func (m model) viewPrompt() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(bgRound).
		Padding(1, 2).
		Width(46)

	content := []string{
		lipgloss.NewStyle().Bold(true).Foreground(accent).Render("Search YouTube"),
		"",
		m.input.View(),
		"",
		lipgloss.NewStyle().Foreground(fgDim).Render("Type a query and press enter to play in mpv"),
		lipgloss.NewStyle().Foreground(fgDim).Render("Ctrl+C / Esc to quit"),
	}
	if len(m.history) > 0 {
		content = append(content,
			lipgloss.NewStyle().Foreground(fgDim).Render(
				fmt.Sprintf("Ctrl+P / Ctrl+N to recall past searches (%d)", len(m.history))),
		)
	}
	if m.liveBusy {
		content = append(content, lipgloss.NewStyle().Foreground(accent).Render("searching…"))
	}
	if m.errMsg != "" {
		content = append(content, lipgloss.NewStyle().Foreground(red).Render(m.errMsg))
	}

	centered := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, m.titleLine(), box.Render(strings.Join(content, "\n"))))
	return centered
}

func (m model) viewSearching() string {
	msg := lipgloss.NewStyle().Foreground(fg).Render("Searching for ") +
		lipgloss.NewStyle().Bold(true).Foreground(fg).Render("“"+m.query+"”")
	centered := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, m.titleLine(), m.spin.View(), " ", msg))
	return centered
}

func (m model) viewResults() string {
	l := computeLayout(m.width, m.height)

	header := lipgloss.NewStyle().Padding(0, 1).Render(
		lipgloss.JoinHorizontal(lipgloss.Left,
			m.titleLine(),
			lipgloss.NewStyle().Foreground(fgDim).Render(fmt.Sprintf(" %d results", len(m.filtered))),
			lipgloss.NewStyle().Foreground(accent).Render(fmt.Sprintf(" · %d queued", len(m.queue))),
		),
	)

	mid := lipgloss.JoinHorizontal(lipgloss.Top, m.viewList(l), " ", m.viewPreview(l))

	var out strings.Builder
	out.WriteString(header)
	out.WriteString("\n")
	// The input row is replaced by a plain read-only line here: typing is
	// ignored on the results screen (Esc returns to the editable prompt).
	bar := "Search: " + m.query
	if m.fetchingMore {
		bar += "  ·  loading more…"
	}
	out.WriteString(lipgloss.NewStyle().Padding(0, 1).Foreground(fgDim).Render(bar))
	out.WriteString("\n")
	out.WriteString(mid)

	if m.errMsg != "" {
		out.WriteString("\n" + lipgloss.NewStyle().Foreground(red).Render(m.errMsg))
	}
	if m.status != "" {
		out.WriteString("\n" + lipgloss.NewStyle().Padding(0, 1).Foreground(accent).Render(m.status))
	}
	if len(m.filtered) == 0 && m.errMsg == "" {
		out.WriteString("\n" + lipgloss.NewStyle().Foreground(fgMid).Render("Nothing to show — press Esc to search again"))
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

func (m model) viewList(l layout) string {
	if len(m.filtered) == 0 {
		return lipgloss.NewStyle().Width(l.leftW - 2).Height(l.midH).Render("")
	}

	// Keep the cursor on screen: once it passes the last visible row, the
	// window scrolls instead of letting the highlight vanish off the bottom.
	start := 0
	if m.cursor >= l.avail {
		start = m.cursor - l.avail + 1
	}
	end := start + l.avail
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	var lines []string
	for i := start; i < end; i++ {
		v := m.filtered[i]
		marker := " "
		if i == m.cursor {
			marker = "▌"
		}
		line := marker + " " + listRow(v, l.leftW-4)
		if i == m.cursor {
			line = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(line)
		} else {
			line = lipgloss.NewStyle().Foreground(fg).Render(line)
		}
		lines = append(lines, line)
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(bgRound).
		Width(l.leftW-2).
		Height(l.midH).
		Padding(0, 1)
	return style.Render(strings.Join(lines, "\n"))
}

// hint renders a short dim line that fits the preview pane body.
func hint(s string, width int) string {
	return lipgloss.NewStyle().Foreground(fgDim).Render(truncate(s, width))
}

func (m model) viewPreview(l layout) string {
	if len(m.filtered) == 0 {
		return lipgloss.NewStyle().Width(l.rightW).Height(l.midH).Render("")
	}
	v := m.filtered[m.cursor]

	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Bold(true).Foreground(accent).Render(truncate(v.Title, l.rightW-6)))
	body.WriteString("\n")
	meta := truncate(v.channel()+"  •  "+v.duration(), l.rightW-6)
	body.WriteString(lipgloss.NewStyle().Foreground(fgMid).Render(meta))
	body.WriteString("\n\n")

	key := thumbKey(v.ID, l.cols, l.rows)
	if !l.thumbOK {
		body.WriteString(hint("terminal too small for a thumbnail", l.rightW-6))
	} else if art, ok := m.thumbs[key]; ok {
		switch {
		case art == "":
			body.WriteString(hint("no thumbnail available", l.rightW-6))
		case m.proto == protoAnsi:
			// half-block art is plain text: it is recomputed by the renderer
			// each frame and padded to the pane width.
			artLines := strings.Split(art, "\n")
			for i, ln := range artLines {
				artLines[i] = simplePad(ln, l.rightW-4)
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
		body.WriteString(hint("loading thumbnail…", l.rightW-6))
	}
	if m.thumbBusy[key] {
		body.WriteString("\n" + hint("fetching thumbnail…", l.rightW-6))
	}

	// channel & video stats below the thumbnail
	if d, ok := m.details[v.ID]; ok {
		if lines := d.lines(l.rightW - 6); len(lines) > 0 {
			body.WriteString("\n")
			body.WriteString(strings.Join(lines, "\n"))
		}
	} else if m.detailBusy[v.ID] {
		body.WriteString("\n" + hint("loading details…", l.rightW-6))
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(bgRound).
		Width(l.rightW-2).
		Height(l.midH).
		Padding(0, 1)
	return style.Render(body.String())
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

// ---- main ------------------------------------------------------------------

func main() {
	m := initialModel(os.Args[1:])
	m.proto = detectImageProtocol()
	if m.proto != protoAnsi {
		fmt.Fprintln(os.Stderr, "ytplay: native image protocol:", m.proto)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ytplay:", err)
		os.Exit(1)
	}
}
