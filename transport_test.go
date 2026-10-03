package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMPVTransportKeys(t *testing.T) {
	mpv.reset()
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "test-mpv.sock")
	useTestSocket(t, sock)
	defer func() {
		mpv.reset()
	}()

	recv, cleanup := startMockMPVServer(t, sock)
	defer cleanup()
	mpv.reset()

	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "v1", Title: "Video"}}})

	// space toggles pause
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space should queue a transport command")
	}
	ctl, ok := cmd().(mpvControlMsg)
	if !ok || ctl.err != nil || ctl.label != "toggle pause" {
		t.Fatalf("space should toggle pause, got %#v", ctl)
	}
	select {
	case got := <-recv:
		if len(got) != 2 || got[0] != "cycle" || got[1] != "pause" {
			t.Fatalf("unexpected mpv command for space: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the pause command")
	}
	m = mm

	// n skips to the next entry
	mm, cmd = m.Update(keyRunes("n"))
	if cmd == nil {
		t.Fatal("n should queue a next command")
	}
	if _, ok := cmd().(mpvControlMsg); !ok {
		t.Fatal("n should report an mpvControlMsg")
	}
	select {
	case got := <-recv:
		if len(got) != 1 || got[0] != "playlist-next" {
			t.Fatalf("unexpected mpv command for n: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the next command")
	}
	m = mm

	// A control result sets a status line.
	m = update(m, mpvControlMsg{label: "toggle mute"})
	if !strings.Contains(m.(model).status, "toggle mute") {
		t.Fatalf("control msg should set a status, got %q", m.(model).status)
	}
}

func TestMPVTransportWithoutPlayerErrors(t *testing.T) {
	mpv.reset()
	useTestSocket(t, filepath.Join(t.TempDir(), "absent.sock"))
	defer func() {
		mpv.reset()
	}()

	m := tea.Model(initialModel(nil))
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, searchMsg{videos: []video{{ID: "v1", Title: "Video"}}})
	mm, cmd := m.Update(keyRunes("n"))
	if cmd == nil {
		t.Fatal("n should still queue a command when mpv is absent")
	}
	ctl, ok := cmd().(mpvControlMsg)
	if !ok || ctl.err == nil {
		t.Fatalf("expected an error control msg, got %#v", ctl)
	}
	m = update(mm, ctl)
	if (m.(model)).errMsg == "" {
		t.Fatal("a failed transport command should surface an error")
	}
}
