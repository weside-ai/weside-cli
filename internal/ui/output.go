// Package ui provides output formatting for the CLI.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	sepStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
)

// PrintJSON outputs data as formatted JSON to stdout.
func PrintJSON(data any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

// PrintError outputs an error message to stderr.
func PrintError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, errorStyle.Render("Error: "+msg))
}

// PrintSuccess outputs a success message to stdout.
func PrintSuccess(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(successStyle.Render(msg))
}

// PrintTable outputs a styled table with headers and rows.
func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println(sepStyle.Render("(no results)"))
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
	fmt.Println(headerStyle.Render(strings.Join(headerParts, "  ")))

	// Print separator
	var sepParts []string
	for _, w := range widths {
		sepParts = append(sepParts, strings.Repeat("─", w))
	}
	fmt.Println(sepStyle.Render(strings.Join(sepParts, "──")))

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
