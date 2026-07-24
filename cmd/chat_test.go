package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

// chatTestServer mocks the two v2 endpoints sendChat touches: the room SSE
// subscription (GET /rooms/{id}/events) and the message send (POST
// /rooms/{id}/messages). sse is the full SSE body written once the events
// path is hit.
func chatTestServer(t *testing.T, sse string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			_, _ = fmt.Fprint(w, sse)
			if flusher != nil {
				flusher.Flush()
			}
		case strings.HasSuffix(r.URL.Path, "/messages") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"u1","role":"user","content":[{"type":"text","text":"hi"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
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
		t.Fatalf("sendChat error: %v", runErr)
	}
	return string(out)
}

// TestExtractCompleteText asserts the v2 room_message_complete form is parsed
// from event["message"]["content"][].text (no longer assistant_message).
func TestExtractCompleteText(t *testing.T) {
	tests := []struct {
		name  string
		event map[string]any
		want  string
	}{
		{
			name: "single text block",
			event: map[string]any{
				"message": map[string]any{
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
				"message": map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "foo "},
						map[string]any{"type": "text", "text": "bar"},
					},
				},
			},
			want: "foo bar",
		},
		{"no message", map[string]any{}, ""},
		{
			name:  "message without content",
			event: map[string]any{"message": map[string]any{}},
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

// TestSendChat_Streaming prints room_message_delta frames live and must NOT
// additionally print the room_message_complete fallback (would duplicate text).
func TestSendChat_Streaming(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"delta\":\"Hello \"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"delta\":\"world\"}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Hello world\"}]}}\n\n"
	srv := chatTestServer(t, sse)
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdout(t, func() error { return sendChat(client, 1, "hi") })

	if strings.Count(out, "Hello world") != 1 {
		t.Errorf("expected 'Hello world' exactly once, got %q", out)
	}
}

// TestSendChat_CompleteFallback covers a room that emits no deltas (no
// stream_deltas capability) — the full reply is printed once from the
// complete frame, even in stream mode.
func TestSendChat_CompleteFallback(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Solo answer\"}]}}\n\n"
	srv := chatTestServer(t, sse)
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdout(t, func() error { return sendChat(client, 1, "hi") })

	if !strings.Contains(out, "Solo answer") {
		t.Errorf("expected fallback text 'Solo answer' in output, got %q", out)
	}
}

// TestSendChat_NonStream ignores deltas and prints the rendered complete text.
func TestSendChat_NonStream(t *testing.T) {
	prev := chatStream
	chatStream = false
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"delta\":\"ignored\"}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Full reply\"}]}}\n\n"
	srv := chatTestServer(t, sse)
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdout(t, func() error { return sendChat(client, 1, "hi") })

	if strings.Contains(out, "ignored") {
		t.Errorf("deltas must not print in non-stream mode, got %q", out)
	}
	if !strings.Contains(out, "Full reply") {
		t.Errorf("expected rendered complete text, got %q", out)
	}
}

// TestSendChat_IgnoresUserMessageEcho ensures a user-role complete frame (the
// server echoing our own message) is not mistaken for the companion's reply.
func TestSendChat_IgnoresUserMessageEcho(t *testing.T) {
	prev := chatStream
	chatStream = false
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"echo me\"}]}}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"real answer\"}]}}\n\n"
	srv := chatTestServer(t, sse)
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdout(t, func() error { return sendChat(client, 1, "hi") })

	if strings.Contains(out, "echo me") {
		t.Errorf("user echo must not be printed as the reply, got %q", out)
	}
	if !strings.Contains(out, "real answer") {
		t.Errorf("expected the companion answer, got %q", out)
	}
}
