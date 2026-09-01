package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
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
	out := captureStdout(t, func() error { return sendChat(context.Background(), client, 1, "hi", abortBound{}) })

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
	out := captureStdout(t, func() error { return sendChat(context.Background(), client, 1, "hi", abortBound{}) })

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
	out := captureStdout(t, func() error { return sendChat(context.Background(), client, 1, "hi", abortBound{}) })

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
	out := captureStdout(t, func() error { return sendChat(context.Background(), client, 1, "hi", abortBound{}) })

	if strings.Contains(out, "echo me") {
		t.Errorf("user echo must not be printed as the reply, got %q", out)
	}
	if !strings.Contains(out, "real answer") {
		t.Errorf("expected the companion answer, got %q", out)
	}
}

// --------------------------------------------------------------------------
// WA-2125 AC6: --abort-after, the verb that lets a verifier cancel a stream
// --------------------------------------------------------------------------

// hangingChatServer writes `sse`, then holds the connection open with no
// further events — a provider that is still generating. chatTestServer returns
// instead, which closes the body, and the scan loop would then end on EOF
// before any duration bound could fire: the test would measure the wrong
// thing and pass for the wrong reason.
func hangingChatServer(t *testing.T, sse string) *httptest.Server {
	t.Helper()
	return hangingChatServerWithCancel(t, sse, nil, true)
}

// cancelCall is what the server saw on /turns/cancel — WA-2140's whole subject.
type cancelCall struct {
	body map[string]any
}

// hangingChatServerWithCancel adds the cancel endpoint hangingChatServer hides.
// `cancelled` is the answer the endpoint gives: false models "no turn was
// running", the case that must make the CLI exit non-zero rather than print a
// reassuring line. Calls land in *calls under the mutex, because the timer
// path issues the cancel from its own goroutine.
func hangingChatServerWithCancel(
	t *testing.T, sse string, calls *[]cancelCall, cancelled bool,
) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/turns/cancel") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			if calls != nil {
				*calls = append(*calls, cancelCall{body: body})
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"cancelled":%t}`, cancelled)
		case strings.HasSuffix(r.URL.Path, "/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			_, _ = fmt.Fprint(w, sse)
			if flusher != nil {
				flusher.Flush()
			}
			select {
			case <-release:
			case <-r.Context().Done():
			}
		case strings.HasSuffix(r.URL.Path, "/messages") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"u1","role":"user","content":[{"type":"text","text":"hi"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	return srv
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestParseAbortAfter(t *testing.T) {
	tests := []struct {
		raw        string
		wantDeltas int
		wantAfter  time.Duration
		wantErr    bool
	}{
		{raw: "", wantDeltas: 0, wantAfter: 0},
		{raw: "3", wantDeltas: 3},
		{raw: " 12 ", wantDeltas: 12},
		{raw: "2s", wantAfter: 2 * time.Second},
		{raw: "1500ms", wantAfter: 1500 * time.Millisecond},
		{raw: "0", wantErr: true},
		{raw: "-2", wantErr: true},
		{raw: "0s", wantErr: true},
		{raw: "soon", wantErr: true},
	}
	for _, tc := range tests {
		got, err := parseAbortAfter(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseAbortAfter(%q): expected an error, got %+v", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAbortAfter(%q): unexpected error %v", tc.raw, err)
			continue
		}
		if got.deltas != tc.wantDeltas || got.after != tc.wantAfter {
			t.Errorf("parseAbortAfter(%q) = %+v, want deltas=%d after=%s",
				tc.raw, got, tc.wantDeltas, tc.wantAfter)
		}
	}
}

// A bare integer must NOT be read as seconds. On a slow provider that would
// silently turn every count-based verification into a time-based one.
func TestParseAbortAfter_BareIntegerIsAChunkCount(t *testing.T) {
	got, err := parseAbortAfter("3")
	if err != nil {
		t.Fatalf("parseAbortAfter: %v", err)
	}
	if got.after != 0 {
		t.Errorf("bare \"3\" set a duration of %s — it must be a chunk count", got.after)
	}
}

// TestSendChat_AbortAfterChunks stops at the bound and returns nil: an
// abandoned turn is the requested outcome, not a failure. The SSE body carries
// more deltas AND a room_message_complete after the bound, so a run that
// ignored --abort-after would print the full reply and be caught here.
func TestSendChat_AbortAfterChunks(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"srv-77\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-77\",\"delta\":\"one \"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-77\",\"delta\":\"two \"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-77\",\"delta\":\"three \"}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"server_message_id\":\"srv-77\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"one two three four\"}]}}\n\n"
	srv := hangingChatServer(t, sse)

	client := api.NewClient(srv.URL, "token")
	var out string
	stderr := captureStderr(t, func() {
		out = captureStdout(t, func() error {
			return sendChat(context.Background(), client, 1, "hi", abortBound{deltas: 2})
		})
	})

	if !strings.Contains(out, "one ") || !strings.Contains(out, "two ") {
		t.Errorf("expected the first two deltas on stdout, got %q", out)
	}
	if strings.Contains(out, "three") {
		t.Errorf("--abort-after 2 read a third delta: %q", out)
	}
	if strings.Contains(out, "one two three four") {
		t.Errorf("the turn completed — the abort never happened: %q", out)
	}
	if !strings.Contains(stderr, "server_message_id=srv-77") {
		t.Errorf("the abandoned turn's id must be reported for the usage_ledger lookup, got %q", stderr)
	}
	if !strings.Contains(stderr, "room_id=1") {
		t.Errorf("the abandoned room must be reported, got %q", stderr)
	}
}

// TestSendChat_AbortAfterDuration covers the wall-clock bound against a server
// that never completes the turn. Without the timer this would block until the
// test binary's own deadline.
func TestSendChat_AbortAfterDuration(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"srv-88\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-88\",\"delta\":\"slow \"}\n\n"
	srv := hangingChatServer(t, sse)

	client := api.NewClient(srv.URL, "token")
	start := time.Now()
	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() error {
			return sendChat(context.Background(), client, 1, "hi", abortBound{after: 150 * time.Millisecond})
		})
	})
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("the duration bound did not fire: took %s", elapsed)
	}
	if !strings.Contains(stderr, "server_message_id=srv-88") {
		t.Errorf("the abandoned turn's id must be reported, got %q", stderr)
	}
}

// TestSendChat_NoAbortBoundStillCompletes is the control: the same helper
// server, no bound, and the turn runs to its completion frame. Without it, the
// two tests above would also pass if --abort-after simply broke every stream.
func TestSendChat_NoAbortBoundStillCompletes(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"srv-66\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-66\",\"delta\":\"one \"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-66\",\"delta\":\"two\"}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"server_message_id\":\"srv-66\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"one two\"}]}}\n\n"
	srv := hangingChatServer(t, sse)

	client := api.NewClient(srv.URL, "token")
	out := captureStdout(t, func() error {
		return sendChat(context.Background(), client, 1, "hi", abortBound{})
	})

	if !strings.Contains(out, "one two") {
		t.Errorf("expected the completed reply, got %q", out)
	}
}

// TestSendChat_AbortAfterJSON emits a machine-readable document on stdout so a
// verification script can read the turn id without scraping stderr.
func TestSendChat_AbortAfterJSON(t *testing.T) {
	prevStream := chatStream
	prevJSON := viper.GetBool("json")
	chatStream = false
	viper.Set("json", true)
	t.Cleanup(func() {
		chatStream = prevStream
		viper.Set("json", prevJSON)
	})

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"srv-99\"}\n\n"
	srv := hangingChatServer(t, sse)

	client := api.NewClient(srv.URL, "token")
	out := captureStdout(t, func() error {
		return sendChat(context.Background(), client, 1, "hi", abortBound{after: 100 * time.Millisecond})
	})

	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("--json output is not one JSON document (%v): %q", err, out)
	}
	if doc["aborted"] != true {
		t.Errorf("expected aborted=true, got %v", doc["aborted"])
	}
	if doc["server_message_id"] != "srv-99" {
		t.Errorf("expected server_message_id srv-99, got %v", doc["server_message_id"])
	}
}

// --------------------------------------------------------------------------
// WA-2140: the abort must cancel the TURN, not just the listener
// --------------------------------------------------------------------------

// TestSendChat_AbortCancelsTheTurn is the story's reason to exist. Before
// WA-2140 the abort tore down the SSE subscription and nothing else, while the
// turn ran on in a server-side background task — so the ledger row a verifier
// then read was the row an uncancelled turn writes anyway. Asserting that the
// cancel endpoint was called with THIS turn's id is what separates the two.
func TestSendChat_AbortCancelsTheTurn(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"srv-2140\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-2140\",\"delta\":\"one \"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-2140\",\"delta\":\"two \"}\n\n"
	var calls []cancelCall
	srv := hangingChatServerWithCancel(t, sse, &calls, true)

	client := api.NewClient(srv.URL, "token")
	var err error
	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() error {
			err = sendChat(context.Background(), client, 1, "hi", abortBound{deltas: 1})
			return err
		})
	})

	if err != nil {
		t.Fatalf("a successful cancel is the requested outcome, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected exactly one cancel request, got %d", len(calls))
	}
	if got := calls[0].body["server_message_id"]; got != "srv-2140" {
		t.Errorf("the cancel must name the turn it is stopping, got %v", got)
	}
}

// TestSendChat_CancelledFalseFails pins AC2. A `cancelled: false` answer means
// nothing was stopped, so any ledger row read afterwards proves nothing —
// exiting 0 here would rebuild the false green the story removes.
func TestSendChat_CancelledFalseFails(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"srv-noop\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-noop\",\"delta\":\"one \"}\n\n"
	srv := hangingChatServerWithCancel(t, sse, nil, false)

	// captureStdout fails the test on a returned error, which is the expected
	// outcome here — so the call runs without it and stdout is left alone.
	client := api.NewClient(srv.URL, "token")
	var err error
	_ = captureStderr(t, func() {
		err = sendChat(context.Background(), client, 1, "hi", abortBound{deltas: 1})
	})

	if err == nil {
		t.Fatal("a cancel that cancelled nothing must fail the run, got nil")
	}
	if !strings.Contains(err.Error(), "proves nothing") {
		t.Errorf("the error must say why the run is worthless, got %q", err.Error())
	}
}

// TestSendChat_DurationCancelsWithoutTurnID covers AC3: the deadline fires
// before room_message_start, so no id is known. The cancel still goes out,
// without server_message_id — a turn that produced no token yet is exactly the
// zero-usage case WA-2125's subcase 2 is about, and skipping it would leave the
// verb blind where it is needed most.
func TestSendChat_DurationCancelsWithoutTurnID(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n"
	var calls []cancelCall
	srv := hangingChatServerWithCancel(t, sse, &calls, true)

	client := api.NewClient(srv.URL, "token")
	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() error {
			return sendChat(context.Background(), client, 1, "hi", abortBound{after: 150 * time.Millisecond})
		})
	})

	if len(calls) != 1 {
		t.Fatalf("the deadline must cancel even with no turn id yet, got %d call(s)", len(calls))
	}
	if _, present := calls[0].body["server_message_id"]; present {
		t.Errorf("with no id known the field must be omitted, got %v", calls[0].body)
	}
}

// TestSendChat_NoBoundSendsNoCancel is AC6's pin. This story edits the shared
// send path, so the ordinary chat must be shown untouched — otherwise every
// `weside chat` would start stopping its own turns.
func TestSendChat_NoBoundSendsNoCancel(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"srv-ok\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"srv-ok\",\"delta\":\"done\"}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"server_message_id\":\"srv-ok\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"done\"}]}}\n\n"
	var calls []cancelCall
	srv := hangingChatServerWithCancel(t, sse, &calls, true)

	client := api.NewClient(srv.URL, "token")
	var err error
	_ = captureStdout(t, func() error {
		err = sendChat(context.Background(), client, 1, "hi", abortBound{})
		return err
	})

	if err != nil {
		t.Fatalf("the ordinary path must still complete, got %v", err)
	}
	if len(calls) != 0 {
		t.Errorf("a chat without --abort-after must cancel nothing, got %d call(s)", len(calls))
	}
}

// TestSendChat_IgnoresTheUserMessageEcho pins the discriminator that makes the
// whole verb work. The POST path publishes its own room_message_start carrying
// the persisted user message (a UUID), and the companion's turn publishes a
// second one without it, under the id the cancel endpoint registered. Adopting
// the first cost both halves: every delta of the real turn was filtered out as
// "not our turn", and the cancel named an id the server could not match — which
// is exactly what production showed before this fix.
func TestSendChat_IgnoresTheUserMessageEcho(t *testing.T) {
	prev := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = prev })

	sse := "data: {\"type\":\"connected\"}\n\n" +
		// The echo: same shape, but it carries user_message and a UUID.
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"3f2a-uuid-echo\"," +
		"\"user_message\":{\"id\":\"3f2a-uuid-echo\",\"role\":\"user\"}}\n\n" +
		"data: {\"type\":\"room_message_complete\",\"server_message_id\":\"3f2a-uuid-echo\"," +
		"\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}}\n\n" +
		// The real turn: no user_message, the registered token id.
		"data: {\"type\":\"room_message_start\",\"server_message_id\":\"tok-real-turn\"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"tok-real-turn\",\"delta\":\"one \"}\n\n" +
		"data: {\"type\":\"room_message_delta\",\"server_message_id\":\"tok-real-turn\",\"delta\":\"two \"}\n\n"
	var calls []cancelCall
	srv := hangingChatServerWithCancel(t, sse, &calls, true)

	// Its own deadline, because the failure mode is a HANG, not a wrong value:
	// adopting the echo filters every delta of the real turn, the delta bound
	// never fires, and the hanging server holds the connection open forever.
	// Without this the negative control blocks the whole suite instead of
	// reporting the defect — measured.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)

	client := api.NewClient(srv.URL, "token")
	var out string
	_ = captureStderr(t, func() {
		out = captureStdout(t, func() error {
			err := sendChat(ctx, client, 1, "hi", abortBound{deltas: 1})
			if err != nil && ctx.Err() != nil {
				t.Errorf("the run never reached its delta bound — the echo was adopted " +
					"and the real turn's deltas were filtered out")
				return nil
			}
			return err
		})
	})

	if !strings.Contains(out, "one ") {
		t.Errorf("the real turn's deltas were filtered out — the echo was adopted: %q", out)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one cancel, got %d", len(calls))
	}
	if got := calls[0].body["server_message_id"]; got != "tok-real-turn" {
		t.Errorf("the cancel must name the TURN, not the user-message echo, got %v", got)
	}
}
