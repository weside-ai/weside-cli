// Package ui provides output formatting for the CLI.
package ui_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/ui"
)

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestPrintJSON(t *testing.T) {
	data := map[string]string{"name": "test", "value": "123"}
	output := captureStdout(func() {
		ui.PrintJSON(data)
	})

	var result map[string]string
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("PrintJSON output is not valid JSON: %v", err)
	}
	if result["name"] != "test" {
		t.Errorf("name = %q, want %q", result["name"], "test")
	}
}

func TestPrintTable(t *testing.T) {
	headers := []string{"ID", "NAME"}
	rows := [][]string{
		{"1", "Alice"},
		{"2", "Bob"},
	}

	output := captureStdout(func() {
		ui.PrintTable(headers, rows)
	})

	if !strings.Contains(output, "ID") {
		t.Error("table output should contain header 'ID'")
	}
	if !strings.Contains(output, "Alice") {
		t.Error("table output should contain 'Alice'")
	}
	if !strings.Contains(output, "Bob") {
		t.Error("table output should contain 'Bob'")
	}
}

func TestPrintTableEmpty(t *testing.T) {
	output := captureStdout(func() {
		ui.PrintTable([]string{"ID"}, nil)
	})

	if !strings.Contains(output, "(no results)") {
		t.Error("empty table should show '(no results)'")
	}
}

func TestPrintSuccess(t *testing.T) {
	output := captureStdout(func() {
		ui.PrintSuccess("Done: %s", "ok")
	})

	if !strings.Contains(output, "Done: ok") {
		t.Errorf("PrintSuccess output = %q, want to contain 'Done: ok'", output)
	}
}
