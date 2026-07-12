// Command akuaku is a terminal UI to monitor and launch AI agents.
//
// With no arguments it starts the monitor. This thin entrypoint only wires the
// program together; all behavior lives in testable internal packages.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akuaku-ai/akuaku/internal/tui"
)

func main() {
	program := tea.NewProgram(tui.New(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "akuaku:", err)
		os.Exit(1)
	}
}
