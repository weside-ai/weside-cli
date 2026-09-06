package ui_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"charm.land/glamour/v2"
	"github.com/weside-ai/weside-cli/internal/ui"
)

func TestTerminalControlsAtDisplaySinks(t *testing.T) {
	input := "Grüße 世界 👋\n\t" + "\x1b]52;c;clipboard\a\x1b[2J\r\b\x7f\u009b31m\u009dtitle\u009c"
	if got := ui.RenderMarkdown(input); strings.ContainsAny(got, "\x1b\u009b\u009d\u009c") {
		t.Fatalf("Markdown plain fallback preserved controls: %q", got)
	}
	for _, mode := range []string{"table", "markdown", "success"} {
		t.Run(mode, func(t *testing.T) {
			output := captureStdout(func() {
				switch mode {
				case "table":
					ui.PrintTable([]string{input}, [][]string{{input}})
				case "markdown":
					printText := ui.RenderMarkdown(input)
					ui.PrintSuccess("%s", printText)
				case "success":
					ui.PrintSuccess("%s", input)
				}
			})
			for _, r := range output {
				if unicode.IsControl(r) && r != '\n' && r != '\t' {
					t.Fatalf("active terminal control %U in %q", r, output)
				}
			}
			if !strings.Contains(output, "Grüße 世界 👋\n\t") {
				t.Fatalf("legitimate text changed: %q", output)
			}
		})
	}
	output := captureStdout(func() { ui.PrintJSON(map[string]string{"text": input}) })
	var got map[string]string
	if err := json.Unmarshal([]byte(output), &got); err != nil || got["text"] != input {
		t.Fatalf("JSON must retain original value: %q, %v", output, err)
	}
}

func TestMarkdownKeepsItsOwnStyling(t *testing.T) {
	// The renderer's existing TTY predicate recognizes character devices.
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stdout
	os.Stdout = f
	t.Cleanup(func() { os.Stdout = old })
	t.Setenv("NO_COLOR", "")
	sgr := regexp.MustCompile("\x1b\\[[0-9;:]*m")
	for _, input := range []string{
		"**Grüße 世界**\x1b]52;c;payload\a",
		"**Grüße 世界**&#x1b;]52;c;payload&#7;",
		"**Grüße 世界**&#27]52;c;payload&#0007;",
		"**Grüße 世界**&#13;hidden&#127;",
		"**Grüße 世界**&amp;#27; &#38;#27; &amp;amp;#27;",
		"**Grüße 世界**<b>&amp;#27;</b>",
		"**Grüße 世界** `&#x1b;[2J`",
	} {
		out := ui.RenderMarkdown(input)
		if !strings.Contains(out, "\x1b[") {
			t.Fatalf("legitimate glamour styling lost: %q", out)
		}
		for _, r := range sgr.ReplaceAllString(out, "") {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				t.Fatalf("regenerated control %U for %q: %q", r, input, out)
			}
		}
	}
	for _, input := range []string{"&#42;not emphasis&#42; &amp;", "**bold** &lt;literal&gt;", "[link](https://example.com)"} {
		want, err := glamour.Render(input, "dark")
		if err != nil {
			t.Fatal(err)
		}
		if got := ui.RenderMarkdown(input); got != want {
			t.Fatalf("legitimate Markdown changed for %q", input)
		}
	}
}
