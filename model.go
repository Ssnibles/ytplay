package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	promptState state = iota
	searchingState
	resultsState
	queueState
)

type model struct {
	input        textinput.Model
	spin         spinner.Model
	state        state
	query        string
	filtered     []video
	cursor       int
	width        int
	height       int
	proto        imgProto
	thumbs       map[string]string
	thumbBusy    map[string]bool
	details      map[string]videoDetail
	detailBusy   map[string]bool
	fetched      int      // how many results have been asked for so far
	fetchingMore bool     // a follow-up page is in flight
	history      []string // past queries, oldest first
	histIdx      int      // -1 = editing fresh text, else index into history
	pending      string   // typed text saved when entering history navigation
	queue        []video  // videos staged for sequential playback
	queueCursor  int      // selected row on the queue page
	status       string
	errMsg       string
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
