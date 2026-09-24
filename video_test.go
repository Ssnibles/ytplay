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
		description: "Tech reviews and gadgets\nNYC",
	}
	lines := d.lines(60)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "21.3M subscribers") {
		t.Errorf("line 0 = %q, want subscriber count", lines[0])
	}
	if !strings.Contains(lines[1], "Tech reviews and gadgets") {
		t.Errorf("line 1 = %q, want description", lines[1])
	}
}
