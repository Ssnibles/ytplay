package main

import (
	"strings"
	"testing"
)

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

func TestVideoMethods(t *testing.T) {
	f := 125.0
	fLong := 3665.0
	zero := 0.0

	v1 := video{ID: "xyz123", Duration: &f}
	if got := v1.watchURL(); got != "https://www.youtube.com/watch?v=xyz123" {
		t.Errorf("watchURL = %q", got)
	}
	if got := v1.duration(); got != "2:05" {
		t.Errorf("duration = %q, want 2:05", got)
	}

	v2 := video{ID: "xyz", URL: "https://youtu.be/custom", Duration: &fLong}
	if got := v2.watchURL(); got != "https://youtu.be/custom" {
		t.Errorf("watchURL custom = %q", got)
	}
	if got := v2.duration(); got != "1:01:05" {
		t.Errorf("duration = %q, want 1:01:05", got)
	}

	v3 := video{Duration: &zero}
	if got := v3.duration(); got != "?:??" {
		t.Errorf("zero duration = %q, want ?:??", got)
	}

	vNil := video{Duration: nil}
	if got := vNil.duration(); got != "?:??" {
		t.Errorf("nil duration = %q, want ?:??", got)
	}

	vChan := video{Channel: "MyChannel", Uploader: "MyUploader"}
	if got := vChan.channel(); got != "MyChannel" {
		t.Errorf("channel = %q, want MyChannel", got)
	}
	vUp := video{Uploader: "MyUploader"}
	if got := vUp.channel(); got != "MyUploader" {
		t.Errorf("channel fallback = %q, want MyUploader", got)
	}
}

func TestThumbURL(t *testing.T) {
	// Videos always use mqdefault.jpg for reliable JPEG decoding even if Thumbnails is populated
	vid := video{
		ID: "vid123",
		Thumbnails: []thumbInfo{
			{URL: "https://i.ytimg.com/vi/vid123/hq720.jpg?sqp=..."},
		},
	}
	if got := vid.thumbURL(); got != "https://i.ytimg.com/vi/vid123/mqdefault.jpg" {
		t.Fatalf("video thumbURL = %q, want mqdefault JPEG url", got)
	}

	// Channels use their avatar thumbnail
	ch := video{
		ID:         "UC123",
		IEKey:      "YoutubeTab",
		Thumbnails: []thumbInfo{{URL: "//yt3.ggpht.com/avatar.jpg"}},
	}
	if got := ch.thumbURL(); got != "https://yt3.ggpht.com/avatar.jpg" {
		t.Fatalf("channel thumbURL = %q, want normalized https avatar url", got)
	}
}

func TestChannelDetailLines(t *testing.T) {
	d := videoDetail{
		subs:        i64p(21300000),
		description: "MKBHD: Quality Tech Videos | YouTuber | Geek | Consumer Electronics | Tech Head | Internet Personality!\n\nbusiness@MKBHD.com\n\nNYC",
	}
	// With width 30 and maxLines 8, the multi-line description wraps across multiple lines
	// and preserves paragraph separation.
	lines := d.lines(30, 8)
	if len(lines) < 5 {
		t.Fatalf("expected description to wrap to at least 5 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "21.3M subscribers") {
		t.Errorf("line 0 = %q, want subscriber count", lines[0])
	}
	// Line 1 should be the first wrapped chunk, not truncated with ellipsis
	if strings.Contains(lines[1], "…") {
		t.Errorf("line 1 should wrap over to the next line rather than being cut off with ellipsis: %q", lines[1])
	}
	foundEmail := false
	for _, ln := range lines {
		if strings.Contains(ln, "business@MKBHD.com") {
			foundEmail = true
			break
		}
	}
	if !foundEmail {
		t.Errorf("expected multi-line description to include later paragraphs (business@MKBHD.com): %v", lines)
	}
}

func TestPreviewThumbDims(t *testing.T) {
	l := layout{cols: 40, rows: 20}
	vChannel := video{ID: "ch1", IEKey: "YoutubeTab"}
	cols, rows := previewThumbDims(vChannel, l)
	if rows != 6 || cols != 12 {
		t.Fatalf("channel thumb dims: got %dx%d, want 12x6", cols, rows)
	}

	vVideo := video{ID: "vid1"}
	cols, rows = previewThumbDims(vVideo, l)
	if rows != 20 || cols != 40 {
		t.Fatalf("video thumb dims: got %dx%d, want 40x20", cols, rows)
	}
}

func TestWrapText(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog"
	wrapped := wrapText(text, 15)
	if len(wrapped) != 3 {
		t.Fatalf("expected 3 wrapped lines, got %d: %v", len(wrapped), wrapped)
	}
	for i, ln := range wrapped {
		if len([]rune(ln)) > 15 {
			t.Errorf("line %d length %d > 15: %q", i, len([]rune(ln)), ln)
		}
	}
	if wrapped[0] != "The quick brown" || wrapped[1] != "fox jumps over" || wrapped[2] != "the lazy dog" {
		t.Errorf("unexpected wrap result: %v", wrapped)
	}

	// Test multi-line and paragraph handling
	multi := "First paragraph here.\n\nSecond paragraph here."
	wMulti := wrapText(multi, 40)
	if len(wMulti) != 3 || wMulti[0] != "First paragraph here." || wMulti[1] != "" || wMulti[2] != "Second paragraph here." {
		t.Fatalf("multi-line paragraph wrapping failed: got %v", wMulti)
	}
}

