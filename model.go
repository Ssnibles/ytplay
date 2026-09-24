package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	promptState state = iota
	searchingState
	resultsState
	queueState
	channelState
)

type pane int

const (
	listPane pane = iota
	previewPane
)

type navPage struct {
	state          state
	query          string
	filtered       []video
	cursor         int
	fetched        int
	focusPane      pane
	descScroll     int
	channelTitle   string
	channelURL     string
	channelVideos  []video
	channelCursor  int
	channelFetched int
}

type model struct {
	input               textinput.Model
	spin                spinner.Model
	state               state
	query               string
	filtered            []video
	cursor              int
	width               int
	height              int
	proto               imgProto
	thumbs              map[string]string
	thumbBusy           map[string]bool
	details             map[string]videoDetail
	detailBusy          map[string]bool
	fetched             int      // how many results have been asked for so far
	fetchingMore        bool     // a follow-up page is in flight
	history             []string // past queries, oldest first
	histIdx             int      // -1 = editing fresh text, else index into history
	pending             string   // typed text saved when entering history navigation
	queue               []video  // videos staged for sequential playback
	queueCursor         int      // selected row on the queue page
	channelTitle        string   // active channel name
	channelURL          string   // active channel videos endpoint or URL
	channelVideos       []video  // videos for active channel
	channelCursor       int      // selected row on channel page
	channelFetched      int      // how many channel results asked for so far
	channelFetchingMore bool     // a channel follow-up page is in flight
	channelLoading      bool     // initial channel fetch in flight
	focusPane           pane     // listPane vs previewPane focus
	descScroll          int      // scroll offset in preview description lines
	navStack            []navPage
	status              string
	errMsg              string
	mpvTicking          bool
}

func (m model) currentNavPage() navPage {
	return navPage{
		state:          m.state,
		query:          m.query,
		filtered:       m.filtered,
		cursor:         m.cursor,
		fetched:        m.fetched,
		focusPane:      m.focusPane,
		descScroll:     m.descScroll,
		channelTitle:   m.channelTitle,
		channelURL:     m.channelURL,
		channelVideos:  m.channelVideos,
		channelCursor:  m.channelCursor,
		channelFetched: m.channelFetched,
	}
}

func (m model) pushNav() model {
	m.navStack = append(m.navStack, m.currentNavPage())
	return m
}

func (m model) popNav() (model, bool) {
	if len(m.navStack) == 0 {
		return m, false
	}
	p := m.navStack[len(m.navStack)-1]
	m.navStack = m.navStack[:len(m.navStack)-1]

	m.state = p.state
	m.query = p.query
	m.filtered = p.filtered
	m.cursor = p.cursor
	m.fetched = p.fetched
	m.focusPane = p.focusPane
	m.descScroll = p.descScroll
	m.channelTitle = p.channelTitle
	m.channelURL = p.channelURL
	m.channelVideos = p.channelVideos
	m.channelCursor = p.channelCursor
	m.channelFetched = p.channelFetched
	m.channelLoading = false
	m.channelFetchingMore = false
	m.fetchingMore = false
	m.errMsg = ""
	m.status = ""
	return m, true
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

func (m model) findVideo(id string) (video, bool) {
	for _, v := range m.filtered {
		if v.ID == id {
			return v, true
		}
	}
	for _, v := range m.channelVideos {
		if v.ID == id {
			return v, true
		}
	}
	for _, v := range m.queue {
		if v.ID == id {
			return v, true
		}
	}
	return video{}, false
}

func (m model) isQueued(id string) bool {
	for _, v := range m.queue {
		if v.ID == id {
			return true
		}
	}
	return false
}

func (m model) removeQueueByID(id string) model {
	for i, v := range m.queue {
		if v.ID == id {
			return m.removeQueueAt(i)
		}
	}
	return m
}

type mpvTickMsg struct{}

func mpvTickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return mpvTickMsg{}
	})
}

func (m model) startMPVTick() (model, tea.Cmd) {
	if m.mpvTicking || !isMPVRunning() {
		return m, nil
	}
	m.mpvTicking = true
	return m, mpvTickCmd()
}

