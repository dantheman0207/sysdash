package main

import (
	"flag"
	"fmt"
	"log"

	tea "charm.land/bubbletea/v2"

	"github.com/daniel/sysdash/internal/app"
	"github.com/daniel/sysdash/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sysdash %s (%s) built %s\n", version.Version, version.Commit, version.Date)
		return
	}

	p := tea.NewProgram(app.New())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
