package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// preflight verifies the external programs ytplay shells out to are actually
// installed, so a missing dependency produces a clear message instead of a
// cryptic "exec: ... executable file not found in $PATH" mid-search.
func preflight() error {
	for _, bin := range []string{"yt-dlp", "mpv"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required program %q not found in PATH — install it and try again", bin)
		}
	}
	return nil
}

func main() {
	if err := preflight(); err != nil {
		fmt.Fprintln(os.Stderr, "ytplay:", err)
		os.Exit(1)
	}
	m := initialModel(os.Args[1:])
	m.proto = detectImageProtocol()
	// Resolve the light/dark choice for the adaptive palette now, while we can
	// still query the terminal: once bubbletea starts it owns stdin.
	_ = lipgloss.HasDarkBackground()
	if m.proto != protoAnsi {
		fmt.Fprintln(os.Stderr, "ytplay: native image protocol:", m.proto)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ytplay:", err)
		os.Exit(1)
	}
}
