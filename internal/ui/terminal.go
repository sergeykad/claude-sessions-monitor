package ui

import (
	"os"

	"golang.org/x/term"
)

const (
	defaultTerminalWidth  = 120
	defaultTerminalHeight = 40
)

// terminalSize returns the terminal's columns and rows. ok is false when
// stdout is not a terminal, which is the case whenever csm is piped or
// redirected.
func terminalSize() (cols, rows int, ok bool) {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0, 0, false
	}
	return cols, rows, true
}

// getTerminalWidth returns the current terminal width in columns.
// Falls back to defaultTerminalWidth if detection fails.
func getTerminalWidth() int {
	cols, _, ok := terminalSize()
	if !ok || cols == 0 {
		return defaultTerminalWidth
	}
	return cols
}

// getTerminalHeight returns the current terminal height in rows.
// Falls back to defaultTerminalHeight if detection fails.
func getTerminalHeight() int {
	_, rows, ok := terminalSize()
	if !ok || rows == 0 {
		return defaultTerminalHeight
	}
	return rows
}
