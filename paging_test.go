package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFollowUpPageWithNoNewResultsExhausts(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	first := make([]video, searchResults)
	for i := range first {
		first[i] = video{ID: fmt.Sprintf("a%02d", i), Title: "t"}
	}
	m = update(m, searchMsg{videos: first, limit: searchResults})
	if (m.(model)).resultsExhausted {
		t.Fatal("a full first page should not be marked exhausted")
	}

	// A follow-up that brings nothing new must stop paging.
	m = update(m, searchMsg{videos: first, limit: 2 * searchResults, more: true})
	if !(m.(model)).resultsExhausted {
		t.Fatal("a follow-up with no new results should mark the search exhausted")
	}
	if (m.(model)).shouldLoadMore() {
		t.Fatal("an exhausted search should not request another page")
	}

	// A fresh search clears the exhausted flag.
	m = update(m, searchMsg{videos: first, limit: searchResults})
	if (m.(model)).resultsExhausted {
		t.Fatal("a new search should reset the exhausted flag")
	}
}
