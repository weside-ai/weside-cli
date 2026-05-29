package ui

import (
	"os"

	"charm.land/glamour/v2"
)

// RenderMarkdown renders markdown text for terminal display.
// Falls back to plain text if rendering fails or output is not a TTY.
func RenderMarkdown(text string) string {
	// Skip rendering if not a TTY or NO_COLOR is set
	if !isTTY() || os.Getenv("NO_COLOR") != "" {
		return text
	}

	rendered, err := glamour.Render(text, "dark")
	if err != nil {
		return text
	}
	return rendered
}

func isTTY() bool {
	return isTTYFile(os.Stdout)
}

func isTTYFile(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
