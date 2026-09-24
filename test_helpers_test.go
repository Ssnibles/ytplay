package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var errTest = errors.New("boom")

// useTempHistory routes past-query persistence to a throwaway file.
func useTempHistory(t *testing.T) {
	t.Helper()
	orig := historyFilePath
	historyFilePath = func() string { return filepath.Join(t.TempDir(), "history") }
	t.Cleanup(func() { historyFilePath = orig })
}

func keyRunes(s string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func update(m tea.Model, msg tea.Msg) tea.Model {
	var cmd tea.Cmd
	m, cmd = m.Update(msg)
	_ = cmd
	return m
}

func i64p(n int64) *int64 { return &n }

func ids(vs []video) string {
	b := make([]byte, 0, len(vs))
	for _, v := range vs {
		b = append(b, v.ID...)
	}
	return string(b)
}

func kittyImageNum(t *testing.T, s string) string {
	t.Helper()
	start := strings.Index(s, "\x1b_G")
	if start < 0 {
		t.Fatal("no kitty transmit in block")
	}
	chunk := s[start+len("\x1b_G"):]
	for _, p := range strings.Split(chunk, ",") {
		if v, ok := strings.CutPrefix(p, "i="); ok {
			return v
		}
	}
	t.Fatalf("no i= in transmit chunk: %q", chunk[:min(60, len(chunk))])
	return ""
}
