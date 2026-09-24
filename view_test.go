package main

import (
	"strings"
	"testing"
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
