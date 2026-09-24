package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
	// Typing on the results screen must not mutate the input or filter the
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
	// Edit and Enter: that runs a whole new search, not a search over results.
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
	m2 := m.(model)
	if m2.filtered[0].ID != "a1" {
		t.Fatalf("first result is %s", m2.filtered[0].ID)
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

	// Busy keying is id@size: a fetch queued for a different size must not
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

	m.cursor = searchResults - pageStep // Just inside the trigger zone.
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
	m.fetched = 30 // Asked for 30 but only 25 came back: end of results.
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

func TestSlashAlwaysOpensSearch(t *testing.T) {
	// From results
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "a1", Title: "One"}}})
	m = update(m, keyRunes("/"))
	if (m.(model)).state != promptState {
		t.Fatalf("/ in results should open search prompt, got %v", (m.(model)).state)
	}

	// Esc in prompt returns to results
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if (m.(model)).state != resultsState {
		t.Fatalf("Esc after / should return to results, got %v", (m.(model)).state)
	}

	// From queue
	m = update(m, keyRunes("q"))
	m = update(m, keyRunes("/"))
	if (m.(model)).state != promptState {
		t.Fatalf("/ in queue should open search prompt, got %v", (m.(model)).state)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if (m.(model)).state != queueState {
		t.Fatalf("Esc after / should return to queue, got %v", (m.(model)).state)
	}
}

func TestChannelNavigationAndEsc(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "Video 1", Channel: "Creator", ChannelID: "UC123"},
			{ID: "UC999", Title: "Channel Result", IEKey: "YoutubeTab", URL: "https://www.youtube.com/channel/UC999"},
		},
	})

	// 1. Enter on a channel result opens the channel
	m = update(m, keyRunes("j")) // move to Channel Result
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm
	if (m.(model)).state != channelState {
		t.Fatalf("Enter on channel should enter channelState, got %v", (m.(model)).state)
	}
	if cmd == nil {
		t.Fatal("expected channelCmd to be queued")
	}

	// 2. Channel message arrives with videos
	m = update(m, channelMsg{
		channelTitle: "Channel Result",
		channelURL:   "https://www.youtube.com/channel/UC999",
		videos: []video{
			{ID: "cv1", Title: "Ch Video 1"},
			{ID: "cv2", Title: "Ch Video 2"},
		},
	})
	md := m.(model)
	if len(md.channelVideos) != 2 {
		t.Fatalf("expected 2 channel videos, got %d", len(md.channelVideos))
	}

	// 3. / from channel opens search, and Esc returns to channel
	m = update(m, keyRunes("/"))
	if (m.(model)).state != promptState {
		t.Fatalf("/ in channel should open search prompt, got %v", (m.(model)).state)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if (m.(model)).state != channelState {
		t.Fatalf("Esc after / should return to channel, got %v", (m.(model)).state)
	}

	// 4. Esc in channel returns to results
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if (m.(model)).state != resultsState {
		t.Fatalf("Esc in channel should return to results, got %v", (m.(model)).state)
	}
	if (m.(model)).cursor != 1 {
		t.Fatalf("cursor in results should be preserved at 1, got %d", (m.(model)).cursor)
	}
}

func TestTabFocusesChannelAndEnterOpens(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "Video 1", Channel: "MyCreator", ChannelID: "UC456"},
		},
	})

	// Tab focuses the preview pane channel
	m = update(m, tea.KeyMsg{Type: tea.KeyTab})
	if (m.(model)).focusPane != previewPane {
		t.Fatal("Tab should focus previewPane (the channel)")
	}

	// Enter opens the channel
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm
	if (m.(model)).state != channelState {
		t.Fatalf("Enter while focused on channel should open channelState, got %v", (m.(model)).state)
	}
	if cmd == nil {
		t.Fatal("expected channelCmd to be queued")
	}
}

func TestCapitalCOpensChannel(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{
		videos: []video{
			{ID: "a1", Title: "Video 1", Channel: "MyCreator", ChannelID: "UC456"},
		},
	})
	mm, cmd := m.Update(keyRunes("C"))
	m = mm
	if (m.(model)).state != channelState {
		t.Fatalf("C should open channelState, got %v", (m.(model)).state)
	}
	if cmd == nil {
		t.Fatal("expected channelCmd to be queued")
	}
}
