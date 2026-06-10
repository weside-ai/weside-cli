package cmd

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestExtractCompleteText(t *testing.T) {
	tests := []struct {
		name  string
		event map[string]any
		want  string
	}{
		{
			name: "single text block",
			event: map[string]any{
				"assistant_message": map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "hello"},
					},
				},
			},
			want: "hello",
		},
		{
			name: "multiple blocks concatenated",
			event: map[string]any{
				"assistant_message": map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "foo "},
						map[string]any{"type": "text", "text": "bar"},
					},
				},
			},
			want: "foo bar",
		},
		{"no assistant_message", map[string]any{}, ""},
		{
			name:  "assistant_message without content",
			event: map[string]any{"assistant_message": map[string]any{}},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractCompleteText(tt.event); got != tt.want {
				t.Errorf("extractCompleteText() = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakeRawClient feeds a canned SSE body into sendStreaming.
type fakeRawClient struct{ body string }

func (f fakeRawClient) DoRaw(_ context.Context, _, _ string, _ any) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("sendStreaming error: %v", runErr)
	}
	return string(out)
}

// TestSendStreaming_Deltas asserts the parser prints incremental message_delta
// frames and does NOT additionally print the message_complete fallback (which
// would duplicate the text). Regression guard for the SSE-format drift fix.
func TestSendStreaming_Deltas(t *testing.T) {
	body := "event: message_delta\n" +
		`data: {"type":"message_delta","delta":"Hello "}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":"world"}` + "\n\n" +
		"event: message_complete\n" +
		`data: {"type":"message_complete","assistant_message":{"content":[{"type":"text","text":"Hello world"}]}}` + "\n\n"

	out := captureStdout(t, func() error {
		return sendStreaming(fakeRawClient{body: body}, map[string]any{})
	})

	if strings.Count(out, "Hello world") != 1 {
		t.Errorf("expected 'Hello world' exactly once, got %q", out)
	}
}

// TestSendStreaming_CompleteFallback covers a backend that emits no deltas —
// the final text must still be printed once from message_complete.
func TestSendStreaming_CompleteFallback(t *testing.T) {
	body := "event: message_complete\n" +
		`data: {"type":"message_complete","assistant_message":{"content":[{"type":"text","text":"Solo answer"}]}}` + "\n\n"

	out := captureStdout(t, func() error {
		return sendStreaming(fakeRawClient{body: body}, map[string]any{})
	})

	if !strings.Contains(out, "Solo answer") {
		t.Errorf("expected fallback text 'Solo answer' in output, got %q", out)
	}
}
