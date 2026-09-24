package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	m := initialModel(os.Args[1:])
	m.proto = detectImageProtocol()
	if m.proto != protoAnsi {
		fmt.Fprintln(os.Stderr, "ytplay: native image protocol:", m.proto)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ytplay:", err)
		os.Exit(1)
	}
}
