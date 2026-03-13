package main

import (
	"log"

	tea "charm.land/bubbletea/v2"

	"github.com/daniel/sysdash/internal/app"
)

func main() {
	p := tea.NewProgram(app.New())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
