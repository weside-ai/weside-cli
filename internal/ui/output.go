// Package ui provides output formatting for the CLI.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	sepStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
)

// styled applies a lipgloss style only when the target stream is a color-capable
// TTY and NO_COLOR is unset. lipgloss v2 removed the global renderer that used to
// auto-strip ANSI for non-TTY output, so we gate styling ourselves to keep piped
// output clean (matches RenderMarkdown's behaviour).
func styled(style *lipgloss.Style, text string, f *os.File) string {
	if os.Getenv("NO_COLOR") != "" || !isTTYFile(f) {
		return text
	}
	return style.Render(text)
}

// PrintJSON outputs data as formatted JSON to stdout.
func PrintJSON(data any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

// PrintError outputs an error message to stderr.
func PrintError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, styled(&errorStyle, "Error: "+msg, os.Stderr))
}

// PrintSuccess outputs a success message to stdout.
func PrintSuccess(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(styled(&successStyle, msg, os.Stdout))
}

// PrintTable outputs a styled table with headers and rows.
func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println(styled(&sepStyle, "(no results)", os.Stdout))
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header
	var headerParts []string
	for i, h := range headers {
		headerParts = append(headerParts, fmt.Sprintf("%-*s", widths[i], h))
	}
	fmt.Println(styled(&headerStyle, strings.Join(headerParts, "  "), os.Stdout))

	// Print separator
	var sepParts []string
	for _, w := range widths {
		sepParts = append(sepParts, strings.Repeat("─", w))
	}
	fmt.Println(styled(&sepStyle, strings.Join(sepParts, "──"), os.Stdout))

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Printf("%-*s  ", widths[i], cell)
			}
		}
		fmt.Println()
	}
}
