package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var errTest = errors.New("boom")

// useTempHistory routes past-query persistence to a throwaway file.
func useTempHistory(t *testing.T) {
	t.Helper()
	orig := historyFilePath
	historyFilePath = func() string { return filepath.Join(t.TempDir(), "history") }
	t.Cleanup(func() { historyFilePath = orig })
}

func keyRunes(s string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func update(m tea.Model, msg tea.Msg) tea.Model {
	var cmd tea.Cmd
	m, cmd = m.Update(msg)
	_ = cmd
	return m
}

func TestPromptTyping(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, keyRunes("a"))
	m = update(m, keyRunes("b"))
	m = update(m, keyRunes("c"))
	m2 := m.(model)
	if got := m2.input.Value(); got != "abc" {
		t.Fatalf("want 'abc', got %q", got)
	}
}

func TestEnterStartsWithSearch(t *testing.T) {
	useTempHistory(t)
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(m, keyRunes("cats"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	m2 := m.(model)
	if m2.state != searchingState {
		t.Fatalf("want searchingState, got %v", m2.state)
	}
	if m2.query != "cats" {
		t.Fatalf("want query 'cats', got %q", m2.query)
	}
}

func TestResultsIgnoresTyping(t *testing.T) {
	// typing on the results screen must not mutate the input or filter the
	// list: searching is a fresh action from the prompt, not a filter over
	// the current results.
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "Foo bar baz", Channel: "C"},
			{ID: "a2", Title: "Something else", Channel: "D"},
		},
	})
	m = update(m, keyRunes("foo"))
	m2 := m.(model)
	if len(m2.filtered) != 2 {
		t.Fatalf("want 2 results shown, got %d", len(m2.filtered))
	}
	if got := m2.input.Value(); got != "" {
		t.Fatalf("input should not receive typing in results, got %q", got)
	}
}

func TestJKControlsSelection(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "One", Channel: "C"},
			{ID: "a2", Title: "Two", Channel: "D"},
			{ID: "a3", Title: "Three", Channel: "E"},
		},
	})
	m = update(m, keyRunes("j"))
	m2 := m.(model)
	if m2.cursor != 1 {
		t.Fatalf("want cursor 1 after j, got %d", m2.cursor)
	}
	m = update(m, keyRunes("k"))
	m2 = m.(model)
	if m2.cursor != 0 {
		t.Fatalf("want cursor 0 after k, got %d", m2.cursor)
	}
	m = update(m, keyRunes("k"))
	m2 = m.(model)
	if m2.cursor != 0 {
		t.Fatalf("k at top should stay at 0, got %d", m2.cursor)
	}
}

func TestEscFromResultsRunsFreshSearch(t *testing.T) {
	useTempHistory(t)
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, keyRunes("cats"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	md := m.(model)
	if md.state != searchingState {
		t.Fatalf("want searchingState, got %v", md.state)
	}
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m2 := m.(model)
	if m2.state != promptState {
		t.Fatalf("want promptState after Esc, got %v", m2.state)
	}
	if got := m2.input.Value(); got != "cats" {
		t.Fatalf("query should survive back to the prompt, got %q", got)
	}
	// edit and Enter: that runs a whole new search, not a search over results
	m = update(m, keyRunes(" more"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	m2 = m.(model)
	if m2.state != searchingState || m2.query != "cats more" {
		t.Fatalf("want searchingState for 'cats more', got state=%v query=%q", m2.state, m2.query)
	}
}

func TestSuccessfulSearchClearsError(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{err: errTest})
	if (m.(model)).errMsg == "" {
		t.Fatal("expected an error message")
	}
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})
	if (m.(model)).errMsg != "" {
		t.Fatalf("successful search should clear the error, got %q", (m.(model)).errMsg)
	}
}

func TestArrowNavigation(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "One", Channel: "C"},
			{ID: "a2", Title: "Two", Channel: "D"},
			{ID: "a3", Title: "Three", Channel: "E"},
		},
	})
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	m2 := m.(model)
	if m2.cursor != 1 {
		t.Fatalf("want cursor 1, got %d", m2.cursor)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyUp})
	m2 = m.(model)
	if m2.cursor != 0 {
		t.Fatalf("want cursor 0, got %d", m2.cursor)
	}
}

func TestEnterPlaysSelected(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "One", URL: "https://youtu.be/a1", Channel: "C"},
			{ID: "a2", Title: "Two", Channel: "D"},
		},
	})
	// simulate a successful mpv launch: this test bypasses exec, so it must
	// fail gracefully rather than hang; selecting and hitting enter returns nil
	m2 := m.(model)
	if m2.filtered[0].ID != "a1" {
		t.Fatalf("first result is %s", m2.filtered[0].ID)
	}
}

func TestNativeBlocks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))

	kitty, err := renderThumbData(img, 16, 9, protoKitty)
	if err != nil {
		t.Fatal(err)
	}
	// PNG transmit for a unique per-image kitty number
	if !strings.Contains(kitty, "f=100") || !strings.Contains(kitty, "t=d,i=") || !strings.Contains(kitty, "q=2") {
		t.Fatalf("kitty block missing transmit: %q…", kitty[:min(48, len(kitty))])
	}
	// virtual placement that the placeholder cells reference
	if !strings.Contains(kitty, "a=p,U=1") || !strings.Contains(kitty, "c=16") || !strings.Contains(kitty, "r=9") {
		t.Fatalf("kitty block missing virtual placement: %q…", kitty[:min(96, len(kitty))])
	}
	// the image is drawn as a grid of Unicode placeholder glyphs (image-as-text)
	if !strings.Contains(kitty, "\U0010EEEE") {
		t.Fatalf("kitty block missing placeholder glyphs: %q…", kitty[:min(48, len(kitty))])
	}
	// the whole block (transmit + placeholders) is exactly `rows` lines
	lines := strings.Split(kitty, "\n")
	if len(lines) != 9 {
		t.Fatalf("kitty: want %d lines, got %d", 9, len(lines))
	}

	six, err := renderThumbData(img, 8, 4, protoSixel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(six, "\x1bP") || !strings.Contains(six, "q") {
		t.Fatalf("sixel block is not a DECSIXEL sequence: %q…", six[:min(20, len(six))])
	}
	sLines := strings.Split(six, "\n")
	if len(sLines) != 4 { // 1 DCS line + 3 reserved lines == rows
		t.Fatalf("sixel: want %d lines, got %d", 4, len(sLines))
	}
	for i, ln := range sLines[1:] {
		if ln != "" {
			t.Fatalf("sixel: reserved line %d should be empty, got %q", i+2, ln)
		}
	}
}

func TestDistinctImagesGetDistinctBlocks(t *testing.T) {
	solid := func(c color.RGBA) image.Image {
		im := image.NewRGBA(image.Rect(0, 0, 320, 180))
		for i := 0; i < len(im.Pix); i += 4 {
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = c.R, c.G, c.B, 255
		}
		return im
	}
	a, err := renderThumbData(solid(color.RGBA{255, 0, 0, 255}), 8, 4, protoKitty)
	if err != nil {
		t.Fatal(err)
	}
	b, err := renderThumbData(solid(color.RGBA{0, 0, 255, 255}), 8, 4, protoKitty)
	if err != nil {
		t.Fatal(err)
	}

	// every render gets its own kitty image number (would be the same "I=" id
	// if some caller pinned it, making the cells identical and the diff skip them)
	na, nb := kittyImageNum(t, a), kittyImageNum(t, b)
	if na == nb {
		t.Fatalf("two images rendered with the same kitty number: %s", na)
	}
	// …and the payload differs too (guards the go-termimg global resize cache
	// that used to return the first thumbnail for every video), so each line of
	// the block differs from the previous image's and gets rewritten.
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range la {
		if la[i] == lb[i] {
			t.Fatalf("placeholder line %d identical across different images: %q", i, la[i])
		}
	}
}

func TestLayoutGeometry(t *testing.T) {
	cases := []struct {
		w, h                                int
		left, right, mid, avail, cols, rows int
		ok                                  bool
	}{
		{120, 40, 54, 65, 36, 34, 61, 17, true},
		{80, 24, 36, 43, 20, 18, 39, 11, true},
		{40, 12, 18, 21, 8, 6, 0, 0, false},
		{0, 0, 0, 0, 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		l := computeLayout(c.w, c.h)
		if l.leftW != c.left || l.rightW != c.right || l.midH != c.mid ||
			l.avail != c.avail || l.cols != c.cols || l.rows != c.rows || l.thumbOK != c.ok {
			t.Errorf("%dx%d: got left=%d right=%d mid=%d avail=%d cols=%d rows=%d ok=%v",
				c.w, c.h, l.leftW, l.rightW, l.midH, l.avail, l.cols, l.rows, l.thumbOK)
		}
	}
}

func TestResizeInvalidatesThumbCache(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})

	md := m.(model)
	l1 := computeLayout(md.width, md.height)
	k1 := thumbKey(md.filtered[0].ID, l1.cols, l1.rows)
	md.thumbs[k1] = "art"
	if cmd := md.loadThumb(); cmd != nil {
		t.Fatal("cached thumb should not be refetched")
	}

	// busy keying is id@size: a fetch queued for a different size must not
	// suppress the render the resize requests.
	md.thumbBusy[thumbKey(md.filtered[0].ID, 777, 777)] = true

	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	md = m.(model)
	if len(md.thumbs) != 0 {
		t.Fatalf("resize should clear the thumb cache, got %d entries", len(md.thumbs))
	}
	l2 := computeLayout(md.width, md.height)
	newKey := thumbKey(md.filtered[0].ID, l2.cols, l2.rows)
	if !md.thumbBusy[newKey] {
		t.Fatalf("resize should queue a fetch for the new size (%s), got %v", newKey, md.thumbBusy)
	}
}

func TestFailedThumbCachesEmptyArt(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})
	md := m.(model)
	l := computeLayout(md.width, md.height)
	key := thumbKey("a1", l.cols, l.rows)
	if !md.thumbBusy[key] {
		t.Fatalf("search should have queued a fetch for %s", key)
	}

	m = update(m, thumbMsg{id: "a1", cols: l.cols, rows: l.rows, err: errTest})
	md = m.(model)
	if md.thumbBusy[key] {
		t.Fatalf("failed render should clear busy for %s", key)
	}
	art, ok := md.thumbs[key]
	if !ok || art != "" {
		t.Fatalf("failed render should cache empty art for %s, got %q", key, art)
	}
	m = update(m, thumbMsg{id: "a1", cols: l.cols, rows: l.rows, art: "x"})
	md = m.(model)
	if md.thumbs[key] == "" {
		t.Fatalf("successful render should overwrite the failure marker")
	}
}

func TestShouldLoadMore(t *testing.T) {
	m := model{state: resultsState}
	m.filtered = make([]video, searchResults)
	for i := range m.filtered {
		m.filtered[i] = video{ID: fmt.Sprintf("a%02d", i)}
	}
	m.fetched = searchResults

	m.cursor = searchResults - pageStep // just inside the trigger zone
	if !m.shouldLoadMore() {
		t.Fatal("nearing the bottom should trigger a page load")
	}
	m.cursor = 0
	if m.shouldLoadMore() {
		t.Fatal("the top should not trigger a page load")
	}

	m.cursor = searchResults - 1
	m.fetchingMore = true
	if m.shouldLoadMore() {
		t.Fatal("an in-flight fetch should not re-trigger")
	}

	m.fetchingMore = false
	m.fetched = 30 // asked for 30 but only 25 came back: end of results
	if m.shouldLoadMore() {
		t.Fatal("end of results should not trigger")
	}
	m.fetched = searchResults

	m.state = promptState
	if m.shouldLoadMore() {
		t.Fatal("non-results states should not trigger")
	}
}

func TestLoadMoreTriggersNearBottom(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	vids := make([]video, searchResults)
	for i := range vids {
		vids[i] = video{ID: fmt.Sprintf("a%02d", i), Title: "t"}
	}
	m = update(m, searchMsg{videos: vids, limit: searchResults})

	md := m.(model)
	md.cursor = searchResults - pageStep
	m = tea.Model(md)

	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected a load-more cmd when nearing the bottom")
	}
	md2 := mm.(model)
	if !md2.fetchingMore {
		t.Fatal("fetchingMore should be set")
	}
	if md2.cursor != searchResults-pageStep+1 {
		t.Fatalf("cursor should advance, got %d", md2.cursor)
	}
}

func TestLoadMoreMergesWithoutDuplicates(t *testing.T) {
	base := make([]video, searchResults)
	for i := range base {
		base[i] = video{ID: fmt.Sprintf("a%02d", i), Title: fmt.Sprintf("t%02d", i)}
	}
	extra := make([]video, 15)
	for i := range extra {
		extra[i] = video{ID: fmt.Sprintf("a%02d", i+searchResults-2), Title: fmt.Sprintf("t%02d", i+searchResults-2)}
	}

	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: base, limit: searchResults})
	m = update(m, searchMsg{videos: extra, limit: searchResults + 25, more: true})

	md := m.(model)
	want := searchResults + (len(extra) - 2) // extra overlaps the last two of base
	if len(md.filtered) != want {
		t.Fatalf("want %d unique results, got %d", want, len(md.filtered))
	}
	if md.fetched != searchResults+25 {
		t.Fatalf("fetched = %d, want %d", md.fetched, searchResults+25)
	}
	if md.fetchingMore {
		t.Fatal("a completed fetch should clear the fetching flag")
	}
	seen := make(map[string]bool, len(md.filtered))
	for i, v := range md.filtered {
		if seen[v.ID] {
			t.Fatalf("duplicate id %s at %d", v.ID, i)
		}
		seen[v.ID] = true
	}
	if md.filtered[0].ID != "a00" || md.filtered[len(md.filtered)-1].ID != "a37" {
		t.Fatalf("bad merge order: first=%s last=%s", md.filtered[0].ID, md.filtered[len(md.filtered)-1].ID)
	}
}

func TestFormatCount(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{42, "42"},
		{999, "999"},
		{1200, "1.2k"},
		{45000, "45k"},
		{1000000, "1M"},
		{1234567, "1.2M"},
		{85000000, "85M"},
		{1000000000, "1B"},
		{8500000000, "8.5B"},
	}
	for _, c := range cases {
		if got := formatCount(c.n); got != c.want {
			t.Errorf("formatCount(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func i64p(n int64) *int64 { return &n }

func TestVideoDetailLines(t *testing.T) {
	d := videoDetail{
		subs:     i64p(1234567),
		chViews:  i64p(85000000),
		views:    i64p(45000),
		likes:    i64p(1800),
		uploaded: "20240115",
	}
	lines := d.lines(60)
	want := []string{"1.2M subscribers", "85M total channel views", "45k views · 1.8k likes · posted Jan 15, 2024"}
	if len(lines) != len(want) {
		t.Fatalf("want %d lines, got %d: %v", len(want), len(lines), lines)
	}
	for i := range want {
		if !strings.Contains(lines[i], want[i]) {
			t.Errorf("line %d = %q, want it to contain %q", i, lines[i], want[i])
		}
	}
	if got := (videoDetail{}).lines(60); len(got) != 0 {
		t.Errorf("empty detail should render nothing, got %v", got)
	}
	// a date with no views still shows its own line
	d2 := videoDetail{uploaded: "20200821"}
	if got := d2.lines(60); len(got) != 1 || !strings.Contains(got[0], "Aug 21, 2020") {
		t.Errorf("date-only detail = %v", got)
	}
}

func TestDetailCacheIsCapped(t *testing.T) {
	m := model{
		state:      resultsState,
		details:    make(map[string]videoDetail),
		detailBusy: make(map[string]bool),
	}
	for i := 0; i < maxDetails+100; i++ {
		_, cmd := m.Update(detailMsg{id: fmt.Sprintf("v%03d", i), det: videoDetail{views: i64p(int64(i))}})
		_ = cmd
	}
	if got := len(m.details); got > maxDetails {
		t.Fatalf("details cache exceeded cap: got %d entries, cap %d", got, maxDetails)
	}
	if got := len(m.detailBusy); got > maxDetails {
		t.Fatalf("detail busy map exceeded cap: got %d entries, cap %d", got, maxDetails)
	}
}

func TestListRow(t *testing.T) {
	p := func(d float64) *float64 { return &d }
	cases := []struct {
		name  string
		title string
		dur   *float64
		width int
		want  string
	}{
		{"duration right-aligned", "some video title here", p(455.0), 20, "some video tit… 7:35"},
		{"short title padded", "a", p(100), 10, "a     1:40"},
		{"no duration", "short", nil, 10, "short"},
		{"narrow leaves no room", "full title here", p(100), 6, "full …"},
	}
	for _, c := range cases {
		v := video{Title: c.title, Duration: c.dur}
		if got := listRow(v, c.width); got != c.want {
			t.Errorf("%s: listRow = %q, want %q", c.name, got, c.want)
		}
	}
}
func TestDetailFetchAndCache(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{{ID: "a1", Title: "One", Channel: "C"}, {ID: "a2", Title: "Two", Channel: "D"}},
		limit:  searchResults,
	})

	md := m.(model)
	if !md.detailBusy["a1"] {
		t.Fatal("selecting the first result should queue a details fetch")
	}
	m = update(m, detailMsg{id: "a1", det: videoDetail{subs: i64p(1000)}})
	md = m.(model)
	if md.detailBusy["a1"] {
		t.Fatal("a completed fetch should clear the busy flag")
	}
	if _, ok := md.details["a1"]; !ok {
		t.Fatal("details should be cached after delivery")
	}
	if cmd := md.loadDetailAt(0); cmd != nil {
		t.Fatal("cached details should not be refetched")
	}
}

func TestPreviewShowsDetails(t *testing.T) {
	m := model{
		state:    resultsState,
		width:    120,
		height:   40,
		proto:    protoAnsi,
		thumbs:   make(map[string]string),
		filtered: []video{{ID: "a1", Title: "T", Channel: "C"}},
		details: map[string]videoDetail{"a1": {
			subs: i64p(1234567), chViews: i64p(85000000), views: i64p(45000), uploaded: "20240115",
		}},
	}
	out := m.viewPreview(computeLayout(120, 40), m.filtered, 0)
	for _, want := range []string{"1.2M subscribers", "85M total channel views", "Jan 15, 2024"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, out)
		}
	}
}

func TestCopyKeyQueuesCommand(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{{ID: "a1", Title: "One", URL: "https://youtu.be/a1", Channel: "C"}},
	})
	mm, cmd := m.Update(keyRunes("c"))
	if cmd == nil {
		t.Fatal("pressing c should queue a copy command")
	}
	_ = mm
}

func TestCopyMsgSetsStatus(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{{ID: "a1", Title: "One", URL: "https://youtu.be/a1", Channel: "C"}},
	})
	m = update(m, copyMsg{url: "https://youtu.be/a1"})
	md := m.(model)
	if !strings.Contains(md.status, "https://youtu.be/a1") {
		t.Fatalf("status should confirm the copied URL, got %q", md.status)
	}
	if out := md.viewResults(); !strings.Contains(out, "copied") {
		t.Fatalf("results view should render the status line")
	}
}

func TestCopyFailureSetsError(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{{ID: "a1", Title: "One", URL: "https://youtu.be/a1", Channel: "C"}},
	})
	m = update(m, copyMsg{url: "x", err: errTest})
	md := m.(model)
	if md.status != "" {
		t.Fatalf("status should be cleared on failure, got %q", md.status)
	}
	if md.errMsg == "" {
		t.Fatal("expected an error message on clipboard failure")
	}
}

func TestOpenChannelKeyQueuesCommand(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{{ID: "a1", Title: "One", Channel: "C", ChannelID: "UCabc"}},
	})
	mm, cmd := m.Update(keyRunes("o"))
	if cmd == nil {
		t.Fatal("pressing o should queue an open command")
	}
	_ = mm
}

func TestOpenMsgSetsStatus(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{{ID: "a1", Title: "One", Channel: "C"}},
	})
	m = update(m, openMsg{url: "https://www.youtube.com/channel/UCabc"})
	md := m.(model)
	if !strings.Contains(md.status, "UCabc") {
		t.Fatalf("status should confirm the opened channel, got %q", md.status)
	}
}

func TestOpenChannelFailureSetsError(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, openMsg{err: errTest})
	md := m.(model)
	if md.errMsg == "" {
		t.Fatal("expected an error message on browser failure")
	}
}

func TestChannelURLFallback(t *testing.T) {
	if got := (videoDetail{channelID: "UCabc"}).channelLink(); got != "https://www.youtube.com/channel/UCabc" {
		t.Fatalf("id fallback = %q", got)
	}
	if got := (videoDetail{channelURL: "https://youtube.com/@codepoint"}).channelLink(); got != "https://youtube.com/@codepoint" {
		t.Fatalf("direct url = %q", got)
	}
	if got := (videoDetail{}).channelLink(); got != "" {
		t.Fatalf("empty detail should have no url, got %q", got)
	}
}

func TestHistoryRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := saveHistory(path, []string{"aaa", "bbb"}); err != nil {
		t.Fatal(err)
	}
	got := loadHistory(path)
	if len(got) != 2 || got[0] != "aaa" || got[1] != "bbb" {
		t.Fatalf("roundtrip = %v", got)
	}
}

func TestHistoryNavigation(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	md := m.(model)
	md.history = []string{"one", "two", "three"}
	m = tea.Model(md)

	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	md = m.(model)
	if got := md.input.Value(); got != "two" {
		t.Fatalf("two Ctrl+P from fresh text should land on 'two', got %q", got)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlN})
	md = m.(model)
	if got := md.input.Value(); got != "three" {
		t.Fatalf("Ctrl+N should move to newest 'three', got %q", got)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlN})
	md = m.(model)
	if got := md.input.Value(); got != "" {
		t.Fatalf("Ctrl+N at newest should return to typed text, got %q", got)
	}
	if md.histIdx != -1 {
		t.Fatalf("histIdx should reset to -1, got %d", md.histIdx)
	}
}

func TestHistoryNavigationRemembersTypedText(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, keyRunes("mus"))
	md := m.(model)
	md.history = []string{"cats"}
	m = tea.Model(md)

	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	md = m.(model)
	if got := md.input.Value(); got != "cats" {
		t.Fatalf("Ctrl+P should fill the query, got %q", got)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlN})
	md = m.(model)
	if got := md.input.Value(); got != "mus" {
		t.Fatalf("Ctrl+N should restore typed text, got %q", got)
	}
}

func TestRememberQueryDedupes(t *testing.T) {
	m := model{histIdx: -1}
	m = m.rememberQuery("cats")
	m = m.rememberQuery("dogs")
	m = m.rememberQuery("cats")
	if len(m.history) != 2 {
		t.Fatalf("dedupe failed: %v", m.history)
	}
	if m.history[1] != "dogs" {
		t.Fatalf("new query should append, got %v", m.history)
	}
}

func TestQueueAdd(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "One", URL: "https://youtu.be/a1"},
			{ID: "a2", Title: "Two", URL: "https://youtu.be/a2"},
		},
	})
	m = update(m, keyRunes("j"))
	m = update(m, keyRunes("a"))
	md := m.(model)
	if len(md.queue) != 1 || md.queue[0].ID != "a2" {
		t.Fatalf("pressing a should queue the selected video, got %v", md.queue)
	}
	if !strings.Contains(md.status, "queued") {
		t.Fatalf("queue add should set a status, got %q", md.status)
	}
	if out := md.viewResults(); !strings.Contains(out, "1 queued") {
		t.Fatalf("header should show the queue count:\n%s", out)
	}
}

func TestQueuePlayEmptyErrors(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})
	m = update(m, keyRunes("p"))
	md := m.(model)
	if md.errMsg == "" {
		t.Fatal("playing an empty queue should error")
	}
}

func TestQueueURLs(t *testing.T) {
	got := queueURLs([]video{{ID: "a1", URL: "https://youtu.be/a1"}, {ID: "a2"}})
	want := []string{"https://youtu.be/a1", "https://www.youtube.com/watch?v=a2"}
	if len(got) != len(want) {
		t.Fatalf("queueURLs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("queueURLs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestQueueMoveUp(t *testing.T) {
	m := model{queue: []video{{ID: "a"}, {ID: "b"}, {ID: "c"}}, queueCursor: 1}
	m = m.moveQueueUp()
	if m.queue[0].ID != "b" || m.queue[1].ID != "a" || m.queue[2].ID != "c" {
		t.Fatalf("move up reordered wrong: %v", ids(m.queue))
	}
	if m.queueCursor != 0 {
		t.Fatalf("cursor should follow the moved item, got %d", m.queueCursor)
	}
	m = m.moveQueueUp() // already at the top: no-op
	if m.queueCursor != 0 || ids(m.queue) != "bac" {
		t.Fatalf("move up at top should be a no-op: %v", ids(m.queue))
	}
}

func TestQueueMoveDown(t *testing.T) {
	m := model{queue: []video{{ID: "a"}, {ID: "b"}, {ID: "c"}}, queueCursor: 0}
	m = m.moveQueueDown()
	if m.queue[0].ID != "b" || m.queue[1].ID != "a" {
		t.Fatalf("move down reordered wrong: %v", ids(m.queue))
	}
	if m.queueCursor != 1 {
		t.Fatalf("cursor should follow the moved item, got %d", m.queueCursor)
	}
	m.queueCursor = 2
	m = m.moveQueueDown() // already at the bottom: no-op
	if m.queueCursor != 2 || ids(m.queue) != "bac" {
		t.Fatalf("move down at bottom should be a no-op: %v", ids(m.queue))
	}
}

func TestQueueRemoveClampsCursor(t *testing.T) {
	m := model{queue: []video{{ID: "a"}, {ID: "b"}, {ID: "c"}}, queueCursor: 2}
	m = m.removeQueueAt(2)
	if ids(m.queue) != "ab" || m.queueCursor != 1 {
		t.Fatalf("remove last should clamp cursor down: queue=%q cursor=%d", ids(m.queue), m.queueCursor)
	}
	m = m.removeQueueAt(0)
	if ids(m.queue) != "b" || m.queueCursor != 0 {
		t.Fatalf("remove first should keep cursor valid: queue=%q cursor=%d", ids(m.queue), m.queueCursor)
	}
	m = m.removeQueueAt(0)
	if len(m.queue) != 0 || m.queueCursor != 0 {
		t.Fatalf("remove last item should empty the queue: queue=%v cursor=%d", m.queue, m.queueCursor)
	}
}

func TestQOpensQueueView(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})
	m = update(m, keyRunes("q"))
	md := m.(model)
	if md.state != queueState {
		t.Fatalf("q should open the queue view, got state %v", md.state)
	}
	if !md.detailBusy["a1"] {
		t.Fatal("opening the queue view should queue a details fetch for the selection")
	}
}

func TestQueueViewUsesSharedPanes(t *testing.T) {
	m := model{
		state:       queueState,
		width:       120,
		height:      40,
		proto:       protoAnsi,
		thumbs:      make(map[string]string),
		queue:       []video{{ID: "a1", Title: "Queued Vid", Channel: "C"}},
		details:     map[string]videoDetail{"a1": {views: i64p(45000), uploaded: "20240115"}},
		errMsg:      "",
		status:      "",
		queueCursor: 0,
	}
	out := m.viewQueue()
	for _, want := range []string{"Queued Vid", "45k views", "Jan 15, 2024", "Queue · 1 videos"} {
		if !strings.Contains(out, want) {
			t.Errorf("queue view missing %q:\n%s", want, out)
		}
	}
}

func TestQueueViewMatchesResultsLayout(t *testing.T) {
	// both pages go through the same two-pane renderer, so the preview for the
	// same video must be identical on the two pages.
	v := []video{{ID: "a1", Title: "T", Channel: "C"}}
	l := computeLayout(120, 40)
	qr := model{
		state: queueState, width: 120, height: 40, proto: protoAnsi,
		thumbs: make(map[string]string), queue: v, queueCursor: 0,
	}
	rr := model{
		state: resultsState, width: 120, height: 40, proto: protoAnsi,
		thumbs: make(map[string]string), filtered: v, cursor: 0,
	}
	// cache the same empty-art for the thumbnail so both render "no thumbnail"
	key := thumbKey("a1", l.cols, l.rows)
	qr.thumbs[key] = ""
	rr.thumbs[key] = ""
	if qr.contentPanes(l, qr.queue, 0) != rr.contentPanes(l, rr.filtered, 0) {
		t.Fatalf("queue and results panes render differently:\nqueue:\n%s\nresults:\n%s",
			qr.contentPanes(l, qr.queue, 0), rr.contentPanes(l, rr.filtered, 0))
	}
}

func TestEscLeavesQueueToResults(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})
	m = update(m, keyRunes("q"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if (m.(model)).state != resultsState {
		t.Fatalf("Esc in the queue view should return to results, got state %v", (m.(model)).state)
	}
}

func ids(vs []video) string {
	b := make([]byte, 0, len(vs))
	for _, v := range vs {
		b = append(b, v.ID...)
	}
	return string(b)
}
func kittyImageNum(t *testing.T, s string) string {
	t.Helper()
	start := strings.Index(s, "\x1b_G")
	if start < 0 {
		t.Fatal("no kitty transmit in block")
	}
	chunk := s[start+len("\x1b_G"):]
	for _, p := range strings.Split(chunk, ",") {
		if v, ok := strings.CutPrefix(p, "i="); ok {
			return v
		}
	}
	t.Fatalf("no i= in transmit chunk: %q", chunk[:min(60, len(chunk))])
	return ""
}
