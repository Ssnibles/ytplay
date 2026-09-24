package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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

func TestMergeResultsEdgeCases(t *testing.T) {
	// Empty base returns extra
	extra := []video{{ID: "v1"}, {ID: "v2"}}
	if got := mergeResults(nil, extra); len(got) != 2 || got[0].ID != "v1" {
		t.Fatalf("merge with nil base failed: %v", got)
	}

	// Empty extra returns base
	base := []video{{ID: "v1"}}
	if got := mergeResults(base, nil); len(got) != 1 || got[0].ID != "v1" {
		t.Fatalf("merge with nil extra failed: %v", got)
	}

	// All duplicates in extra
	got := mergeResults(base, []video{{ID: "v1"}})
	if len(got) != 1 {
		t.Fatalf("expected 1 result after duplicate merge, got %d", len(got))
	}
}

func TestChannelVideosURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"UCuAXFkgsw1L7xaCfnd5JJOw", "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/videos"},
		{"@veritasium", "https://www.youtube.com/@veritasium/videos"},
		{"https://www.youtube.com/channel/UCabc", "https://www.youtube.com/channel/UCabc/videos"},
		{"https://www.youtube.com/channel/UCabc/videos", "https://www.youtube.com/channel/UCabc/videos"},
		{"https://www.youtube.com/@mkbhd", "https://www.youtube.com/@mkbhd/videos"},
		{"https://www.youtube.com/@mkbhd/videos", "https://www.youtube.com/@mkbhd/videos"},
	}
	for _, c := range cases {
		if got := channelVideosURL(c.in); got != c.want {
			t.Errorf("channelVideosURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
