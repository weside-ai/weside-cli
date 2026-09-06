package ui

import (
	"html"
	"os"
	"regexp"

	"charm.land/glamour/v2"
)

var numericEntity = regexp.MustCompile(`&#(?:\d+|[xX][0-9a-fA-F]+);?`)

// Glamour decodes numeric entities after parsing Markdown. Neutralize only
// entities that produce terminal controls: decoding all entities before parsing
// would turn ordinary text such as &#42; into Markdown syntax.
func safeMarkdownEntities(text string) string {
	return numericEntity.ReplaceAllStringFunc(text, func(entity string) string {
		decoded := html.UnescapeString(entity)
		if safe := SafeText(decoded); safe != decoded {
			return safe
		}
		return entity
	})
}

// RenderMarkdown renders markdown text for terminal display.
// Falls back to plain text if rendering fails or output is not a TTY.
func RenderMarkdown(text string) string {
	text = SafeText(text)
	// Skip rendering if not a TTY or NO_COLOR is set
	if !isTTY() || os.Getenv("NO_COLOR") != "" {
		return text
	}

	rendered, err := glamour.Render(safeMarkdownEntities(text), "dark")
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
