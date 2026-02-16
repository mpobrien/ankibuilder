package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	wr, err := NewWordReference("en", "es")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	m := newModel(wr)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
