package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

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

func TestChannelPreviewShowsFullWrappedDescription(t *testing.T) {
	desc := "Linus Tech Tips is a passionate team of experts in consumer technology and video production.\n\nSponsorship Inquiries: partnerships@linusmediagroup.com"
	m := model{
		state:    resultsState,
		width:    80,
		height:   24,
		proto:    protoAnsi,
		thumbs:   make(map[string]string),
		filtered: []video{{ID: "UC123", Title: "Linus Tech Tips", IEKey: "YoutubeTab"}},
		details: map[string]videoDetail{"UC123": {
			subs:        i64p(16900000),
			description: desc,
		}},
	}
	l := computeLayout(80, 24)
	out := m.viewPreview(l, m.filtered, 0)
	for _, want := range []string{"16.9M subscribers", "Linus Tech Tips is a passionate", "partnerships@linusmediagroup.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, out)
		}
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
	// Both pages go through the same two-pane renderer, so the preview for the
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
	// Cache the same empty-art for the thumbnail so both render "no thumbnail".
	key := thumbKey("a1", l.cols, l.rows)
	qr.thumbs[key] = ""
	rr.thumbs[key] = ""
	if qr.contentPanes(l, qr.queue, 0) != rr.contentPanes(l, rr.filtered, 0) {
		t.Fatalf("queue and results panes render differently:\nqueue:\n%s\nresults:\n%s",
			qr.contentPanes(l, qr.queue, 0), rr.contentPanes(l, rr.filtered, 0))
	}
}

func TestSelectionIndicatorBorders(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(oldProfile)

	v := []video{{ID: "a1", Title: "T", Channel: "C"}}
	l := computeLayout(120, 40)
	m := model{
		state:    resultsState,
		width:    120,
		height:   40,
		proto:    protoAnsi,
		thumbs:   make(map[string]string),
		filtered: v,
		cursor:   0,
	}

	mList := m
	mList.focusPane = listPane

	mPrev := m
	mPrev.focusPane = previewPane

	// Both list and preview outputs must differ when focused vs unfocused
	// because of the active border style vs default border style.
	listWhenListActive := mList.viewList(l, v, 0)
	listWhenPrevActive := mPrev.viewList(l, v, 0)
	if listWhenListActive == listWhenPrevActive {
		t.Fatalf("viewList output should differ when active vs inactive")
	}

	prevWhenListActive := mList.viewPreview(l, v, 0)
	prevWhenPrevActive := mPrev.viewPreview(l, v, 0)
	if prevWhenListActive == prevWhenPrevActive {
		t.Fatalf("viewPreview output should differ when active vs inactive")
	}
}

func TestPreviewDescriptionScrolls(t *testing.T) {
	var descLines []string
	for i := 1; i <= 25; i++ {
		descLines = append(descLines, fmt.Sprintf("Description line %02d", i))
	}
	desc := strings.Join(descLines, "\n")

	v := []video{{ID: "UC123", Title: "Linus Tech Tips", IEKey: "YoutubeTab"}}
	l := computeLayout(80, 24)
	m := model{
		state:    resultsState,
		width:    80,
		height:   24,
		proto:    protoAnsi,
		thumbs:   make(map[string]string),
		filtered: v,
		cursor:   0,
		details: map[string]videoDetail{"UC123": {
			subs:        i64p(1000),
			description: desc,
		}},
	}

	m0 := m
	m0.descScroll = 0
	out0 := m0.viewPreview(l, v, 0)
	if !strings.Contains(out0, "Description line 01") {
		t.Fatalf("expected out0 to contain 'Description line 01', got:\n%s", out0)
	}

	mScrolled := m
	mScrolled.descScroll = 8
	outScrolled := mScrolled.viewPreview(l, v, 0)
	if strings.Contains(outScrolled, "Description line 01") {
		t.Fatalf("expected outScrolled to have scrolled past 'Description line 01', got:\n%s", outScrolled)
	}
	if !strings.Contains(outScrolled, "Description line 10") {
		t.Fatalf("expected outScrolled to show later lines like 'Description line 10', got:\n%s", outScrolled)
	}
}

func TestPreviewKittyThumbnailScrollsCleanly(t *testing.T) {
	v := []video{{
		ID:      "vid1",
		Title:   "My Video",
		Channel: "My Channel",
	}}
	l := computeLayout(80, 24)
	cols, rows := previewThumbDims(v[0], l)
	key := thumbKey(v[0].ID, cols, rows)

	// Mock Kitty thumbnail with 3 rows of placeholders and a setup sequence
	kittyArt := "\x1b_Gsetup_seq\x1b\\thumb_row0\nthumb_row1\nthumb_row2"

	var descLines []string
	for i := 1; i <= 20; i++ {
		descLines = append(descLines, fmt.Sprintf("Paragraph line %d", i))
	}
	desc := strings.Join(descLines, "\n")

	m := model{
		width:    80,
		height:   24,
		proto:    protoKitty,
		thumbs:   map[string]string{key: kittyArt},
		filtered: v,
		cursor:   0,
		details: map[string]videoDetail{"vid1": {
			subs:        i64p(500),
			description: desc,
		}},
	}

	// At scroll = 0: setup sequence is present with thumb_row0
	out0 := m.viewPreview(l, v, 0)
	if !strings.Contains(out0, "\x1b_Gsetup_seq\x1b\\thumb_row0") {
		t.Fatalf("scroll 0 should have setup sequence prepended to thumb_row0, got:\n%s", out0)
	}

	// At scroll = 4: Title, Channel, "", and thumb_row0 are scrolled off.
	// First visible thumbnail row is thumb_row1, which must receive the setup sequence.
	m4 := m
	m4.descScroll = 4
	out4 := m4.viewPreview(l, v, 0)
	if strings.Contains(out4, "thumb_row0") {
		t.Fatalf("scroll 4 should have scrolled past thumb_row0, got:\n%s", out4)
	}
	if !strings.Contains(out4, "\x1b_Gsetup_seq\x1b\\thumb_row1") {
		t.Fatalf("scroll 4 should have setup sequence prepended to thumb_row1, got:\n%s", out4)
	}

	// At scroll = 10: Thumbnail is scrolled completely off.
	// Only description lines should appear, without setup sequence.
	m10 := m
	m10.descScroll = 10
	out10 := m10.viewPreview(l, v, 0)
	if strings.Contains(out10, "\x1b_Gsetup_seq\x1b\\") {
		t.Fatalf("scroll 10 has no thumbnail visible, should not contain setup sequence, got:\n%s", out10)
	}
	if !strings.Contains(out10, "Paragraph line 3") {
		t.Fatalf("scroll 10 should show later paragraph lines, got:\n%s", out10)
	}
}

func TestSmallTerminalCanScrollFullDescription(t *testing.T) {
	v := []video{{
		ID:      "vid1",
		Title:   "Compact Test",
		Channel: "Short Channel",
	}}
	// Very small terminal: 60 columns wide, 14 lines high
	l := computeLayout(60, 14)
	desc := "Line 01\nLine 02\nLine 03\nLine 04\nLine 05\nLine 06\nLine 07\nLine 08\nLine 09\nLine 10"

	m := model{
		width:    60,
		height:   14,
		proto:    protoAnsi,
		thumbs:   make(map[string]string),
		filtered: v,
		cursor:   0,
		details: map[string]videoDetail{"vid1": {
			description: desc,
		}},
	}

	// Usable height inside the box is 14 - 4 - 2 = 8 lines
	h := l.midH - 2
	if h != 8 {
		t.Fatalf("expected usable box height 8, got %d", h)
	}

	// At scroll = 0, title & channel take the top rows
	out0 := m.viewPreview(l, v, 0)
	if !strings.Contains(out0, "Compact Test") {
		t.Fatalf("out0 should show title, got:\n%s", out0)
	}

	// Scroll down past the header to read the description with all 8 available lines
	mScrolled := m
	mScrolled.descScroll = 6
	outScrolled := mScrolled.viewPreview(l, v, 0)
	if strings.Contains(outScrolled, "Compact Test") {
		t.Fatalf("outScrolled should have scrolled past title, got:\n%s", outScrolled)
	}
	// Check that multiple description lines now occupy the full height
	if !strings.Contains(outScrolled, "Line 04") || !strings.Contains(outScrolled, "Line 08") {
		t.Fatalf("outScrolled should show description lines 04-08 occupying full pane, got:\n%s", outScrolled)
	}
}

