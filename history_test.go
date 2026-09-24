package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHistoryRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := saveHistory(path, []string{"aaa", "bbb"}); err != nil {
		t.Fatal(err)
	}
	got := loadHistory(path)
	if len(got) != 2 || got[0] != "aaa" || got[1] != "bbb" {
		t.Fatalf("roundtrip = %v", got)
	}
}

func TestHistoryNavigation(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	md := m.(model)
	md.history = []string{"one", "two", "three"}
	m = tea.Model(md)

	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	md = m.(model)
	if got := md.input.Value(); got != "two" {
		t.Fatalf("two Ctrl+P from fresh text should land on 'two', got %q", got)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlN})
	md = m.(model)
	if got := md.input.Value(); got != "three" {
		t.Fatalf("Ctrl+N should move to newest 'three', got %q", got)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlN})
	md = m.(model)
	if got := md.input.Value(); got != "" {
		t.Fatalf("Ctrl+N at newest should return to typed text, got %q", got)
	}
	if md.histIdx != -1 {
		t.Fatalf("histIdx should reset to -1, got %d", md.histIdx)
	}
}

func TestHistoryNavigationRemembersTypedText(t *testing.T) {
	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, keyRunes("mus"))
	md := m.(model)
	md.history = []string{"cats"}
	m = tea.Model(md)

	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	md = m.(model)
	if got := md.input.Value(); got != "cats" {
		t.Fatalf("Ctrl+P should fill the query, got %q", got)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlN})
	md = m.(model)
	if got := md.input.Value(); got != "mus" {
		t.Fatalf("Ctrl+N should restore typed text, got %q", got)
	}
}

func TestRememberQueryDedupes(t *testing.T) {
	m := model{histIdx: -1}
	m = m.rememberQuery("cats")
	m = m.rememberQuery("dogs")
	m = m.rememberQuery("cats")
	if len(m.history) != 2 {
		t.Fatalf("dedupe failed: %v", m.history)
	}
	if m.history[1] != "dogs" {
		t.Fatalf("new query should append, got %v", m.history)
	}
}
