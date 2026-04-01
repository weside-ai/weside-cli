// Package ui provides output formatting for the CLI.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// PrintJSON outputs data as formatted JSON to stdout.
func PrintJSON(data any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

// PrintError outputs an error message to stderr.
func PrintError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// PrintSuccess outputs a success message to stdout.
func PrintSuccess(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// PrintTable outputs a simple table with headers and rows.
func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println("(no results)")
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
	for i, h := range headers {
		fmt.Printf("%-*s  ", widths[i], h)
	}
	fmt.Println()

	// Print separator
	for i := range headers {
		fmt.Print(strings.Repeat("-", widths[i]) + "  ")
	}
	fmt.Println()

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
