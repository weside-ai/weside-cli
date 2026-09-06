package ui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// SafeText makes terminal controls visible without interpreting remote text.
// Escape each control independently so separately streamed chunks cannot form
// an escape sequence. Keep Unicode, line feeds and tabs for ordinary prose.
// Call before adding our own terminal styling, never on machine-readable data.
func SafeText(text string) string {
	var safe strings.Builder
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			quoted := strconv.QuoteRune(r)
			safe.WriteString(quoted[1 : len(quoted)-1])
		} else {
			safe.WriteRune(r)
		}
	}
	return safe.String()
}

// Print displays unstyled human-readable text safely.
func Print(args ...any) { fmt.Print(SafeText(fmt.Sprint(args...))) }

// Println displays an unstyled human-readable line safely.
func Println(args ...any) { fmt.Print(SafeText(fmt.Sprintln(args...))) }

// Printf displays formatted human-readable text safely.
func Printf(format string, args ...any) { _, _ = Fprintf(os.Stdout, format, args...) }

// Fprintf is the safe human-readable counterpart of fmt.Fprintf.
func Fprintf(w io.Writer, format string, args ...any) (int, error) {
	return io.WriteString(w, SafeText(fmt.Sprintf(format, args...)))
}
