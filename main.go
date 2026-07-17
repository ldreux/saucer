package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"saucer/internal/app"
)

func main() {
	themeName := flag.String("theme", "", "color theme: ocean, sunset, matrix, banana, bubblegum, vaporwave, toxic (default: last used, or ocean)")
	noFooter := flag.Bool("no-footer", false, "start with the bottom bar hidden")
	flag.Parse()

	state := app.LoadState()

	name := strings.ToLower(state.Theme)
	if *themeName != "" {
		// An explicit --theme flag always wins over saved state.
		name = strings.ToLower(*themeName)
	}
	if name == "" {
		name = "ocean"
	}
	if !app.IsValidTheme(name) {
		fmt.Fprintf(os.Stderr, "unknown theme %q, using ocean\n", name)
		name = "ocean"
	}

	m := app.New(name, !*noFooter)
	m.ApplyPersistedState(state)
	m.SyncSharedSession()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
