package main

import (
	"fmt"
	"testing"
)

func TestThumbEvictionProtectsCurrentSelection(t *testing.T) {
	m := model{
		state:     resultsState,
		width:     120,
		height:    40,
		proto:     protoAnsi,
		thumbs:    map[string]string{},
		thumbBusy: map[string]bool{},
		filtered:  []video{{ID: "cur", Title: "Current"}},
	}
	l := computeLayout(120, 40)
	curKey := thumbKey("cur", l.cols, l.rows)
	m.thumbs[curKey] = "art"
	for i := 0; len(m.thumbs) < maxThumbs; i++ {
		m.thumbs[fmt.Sprintf("other%02d@%dx%d", i, l.cols, l.rows)] = "x"
	}

	m = m.handleThumbMsg(thumbMsg{id: "new", cols: l.cols, rows: l.rows, art: "y"})
	if _, ok := m.thumbs[curKey]; !ok {
		t.Fatal("eviction must keep the currently visible thumbnail")
	}
	if _, ok := m.thumbs[thumbKey("new", l.cols, l.rows)]; !ok {
		t.Fatal("the newly rendered thumbnail should still be cached")
	}
}
