package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// historyCap bounds how many searches are recalled/persisted.
const historyCap = 100

// historyFilePath returns where past queries are stored; overridable in tests.
var historyFilePath = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ytplay", "history")
}

func loadHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if q := strings.TrimSpace(sc.Text()); q != "" {
			out = append(out, q)
		}
	}
	return out
}

func saveHistory(path string, queries []string) error {
	if path == "" || len(queries) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, q := range queries {
		fmt.Fprintln(w, q)
	}
	return w.Flush()
}

// historyPrev steps toward older queries, remembering the typed text so
// Ctrl+N can return to it.
func (m model) historyPrev() model {
	if len(m.history) == 0 {
		return m
	}
	if m.histIdx == -1 {
		m.pending = m.input.Value()
		m.histIdx = len(m.history) - 1
	} else if m.histIdx > 0 {
		m.histIdx--
	}
	m.input.SetValue(m.history[m.histIdx])
	m.input.CursorEnd()
	return m
}

// historyNext steps toward newer queries and back to the typed text.
func (m model) historyNext() model {
	if m.histIdx == -1 {
		return m
	}
	if m.histIdx < len(m.history)-1 {
		m.histIdx++
		m.input.SetValue(m.history[m.histIdx])
	} else {
		m.histIdx = -1
		m.input.SetValue(m.pending)
	}
	m.input.CursorEnd()
	return m
}

// rememberQuery records a completed search in the persisted history.
func (m model) rememberQuery(q string) model {
	if q == "" {
		return m
	}
	for _, x := range m.history {
		if x == q {
			return m
		}
	}
	m.history = append(m.history, q)
	if len(m.history) > historyCap {
		m.history = m.history[len(m.history)-historyCap:]
	}
	_ = saveHistory(historyFilePath(), m.history)
	return m
}
