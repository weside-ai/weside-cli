package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/weside-ai/weside-cli/internal/api"
)

func TestParseIntCSV(t *testing.T) {
	tests := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{"1,2,3", []int{1, 2, 3}, false},
		{" 42 ", []int{42}, false},
		{"", nil, true},
		{"1,x,3", nil, true},
	}
	for _, tt := range tests {
		got, err := parseIntCSV(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseIntCSV(%q) err=%v wantErr=%v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && !equalInts(got, tt.want) {
			t.Errorf("parseIntCSV(%q) = %v want %v", tt.in, got, tt.want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParticipantID(t *testing.T) {
	tests := []struct {
		name string
		p    map[string]any
		want string
	}{
		{"user", map[string]any{"kind": "user", "user_id": float64(7)}, "7"},
		{"companion", map[string]any{"kind": "companion", "companion_id": float64(101)}, "101"},
		{"external", map[string]any{"kind": "external", "external_id": float64(3)}, "3"},
		{"none", map[string]any{"kind": "user"}, "-"},
	}
	for _, tt := range tests {
		if got := participantID(tt.p); got != tt.want {
			t.Errorf("%s: participantID = %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestPrettyJSON(t *testing.T) {
	if prettyJSON(nil) != "" {
		t.Error("nil should render empty")
	}
	if got := prettyJSON(map[string]any{"a": 1}); got != `{"a":1}` {
		t.Errorf("obj = %q", got)
	}
}

func captureRoomsDebug(t *testing.T, fn func() error) string {
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
		t.Fatalf("error: %v", runErr)
	}
	return string(out)
}

func TestRoomsTracePrintsToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"room_id":1,"messages":[`+
			`{"message_id":"m1","role":"assistant","created_at":"2026-07-24T10:00:00+00:00",`+
			`"trace_items":[{"type":"tool_call","tool_name":"search","tool_args":{"q":"hi"},"tool_output":"ok"},`+
			`{"type":"reminder","category":"follow_up","message":"ping"}]}]}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureRoomsDebug(t, func() error {
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/1/trace?limit=50", &result); err != nil {
			return err
		}
		messages, _ := result["messages"].([]any)
		for _, item := range messages {
			m, _ := item.(map[string]any)
			fmt.Printf("[%s] %s\n", roleLabel(fmt.Sprintf("%v", m["role"])), m["created_at"])
			printTraceItems(m["trace_items"], false)
		}
		return nil
	})

	if !strings.Contains(out, "[Companion]") {
		t.Errorf("expected Companion label, got %q", out)
	}
	if !strings.Contains(out, "tool_call search") {
		t.Errorf("expected tool_call line, got %q", out)
	}
	if !strings.Contains(out, "reminder [follow_up]: ping") {
		t.Errorf("expected reminder line, got %q", out)
	}
}

func TestRoomsDmCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/dm/101") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":42,"kind":"dm","title":"Nox"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureRoomsDebug(t, func() error {
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/dm/101", nil, &result); err != nil {
			return err
		}
		fmt.Printf("DM room with companion 101 (ID: %v).\n", result["id"])
		return nil
	})

	if !strings.Contains(out, "ID: 42") {
		t.Errorf("expected room id 42, got %q", out)
	}
}

func TestRoomsGroupCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/rooms/group") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"companion_ids":[1,2]`) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id":9,"kind":"group"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	ids, err := parseIntCSV("1,2")
	if err != nil {
		t.Fatalf("parseIntCSV: %v", err)
	}
	body := map[string]any{"companion_ids": ids}
	var result map[string]any
	if err := client.Post(context.Background(), "/rooms/group", body, &result); err != nil {
		t.Fatalf("post: %v", err)
	}
	if fmt.Sprintf("%v", result["id"]) != "9" {
		t.Errorf("expected group room id 9, got %v", result["id"])
	}
}

// TestRoomsDestructiveCommandsRegisterConfirm pins what the RunE-level gate
// test cannot see: calling RunE directly bypasses cobra's flag parsing, so a
// command whose --confirm flag was never registered passes that test while
// being unusable ("unknown flag: --confirm"). Measured on the live endpoint
// while adding regenerate.
func TestRoomsDestructiveCommandsRegisterConfirm(t *testing.T) {
	for _, c := range []*cobra.Command{
		roomsCancelCmd, roomsUndoCmd, roomsContextBreakCmd, roomsRegenerateCmd,
	} {
		if c.Flags().Lookup("confirm") == nil {
			t.Errorf("%s: --confirm is not a registered flag", c.Name())
		}
	}
}

func TestRoomsCancelRequiresConfirm(t *testing.T) {
	// No server: the --confirm gate must fail before any request.
	//
	// Assert on WHICH error, not merely that one came back. Without a gate
	// these commands still fail — on `newAuthenticatedClientV2`, which has no
	// credentials here — so `err != nil` alone stays green with the gate
	// deleted. Measured while adding regenerate: the naive form passed under a
	// mutation that removed the gate entirely.
	cases := []struct {
		name string
		run  func() error
	}{
		{"cancel", func() error { return roomsCancelCmd.RunE(roomsCancelCmd, []string{"1"}) }},
		{"undo", func() error { return roomsUndoCmd.RunE(roomsUndoCmd, []string{"1"}) }},
		{"context-break", func() error { return roomsContextBreakCmd.RunE(roomsContextBreakCmd, []string{"1"}) }},
		{"regenerate", func() error { return roomsRegenerateCmd.RunE(roomsRegenerateCmd, []string{"1"}) }},
	}
	for _, tc := range cases {
		err := tc.run()
		if err == nil {
			t.Errorf("%s: expected the --confirm gate to block it", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), "--confirm") {
			t.Errorf("%s: expected the gate's own refusal naming --confirm, got %q", tc.name, err.Error())
		}
	}
}

// sseFixtureBody is shared across the streamRoomEvents tests: a frame with
// an id (the "connected" shape), a heartbeat comment in between (counted,
// never printed under --ndjson), a frame without an id (the "reconnect"
// shape), and a multi-line data: payload joined by \n before being parsed.
const sseFixtureBody = "id: cur-1\n" +
	"event: connected\n" +
	"data: {\"room_id\":1,\"device_id\":\"d1\"}\n" +
	"\n" +
	": heartbeat\n" +
	"event: reconnect\n" +
	"data: {\"type\":\"reconnect\"}\n" +
	"\n" +
	"id: cur-2\n" +
	"event: room_message_delta\n" +
	"data: {\"a\":1,\n" +
	"data: \"b\":2}\n" +
	"\n"

func TestStreamRoomEventsNDJSONParsesFrames(t *testing.T) {
	var out, stderr bytes.Buffer
	err := streamRoomEvents(context.Background(), strings.NewReader(sseFixtureBody), &out, &stderr, false, true)
	if err != nil {
		t.Fatalf("streamRoomEvents: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d: %q", len(lines), out.String())
	}
	if strings.Contains(out.String(), "heartbeat") {
		t.Errorf("heartbeat comment leaked into stdout: %q", out.String())
	}
	if !strings.Contains(stderr.String(), "frames=3 heartbeats=1") {
		t.Errorf("expected frames=3 heartbeats=1 summary on stderr, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "stream closed by server") {
		t.Errorf("expected a clean-close reason on stderr, got %q", stderr.String())
	}

	var first ndjsonFrame
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if first.Event != "connected" || first.ID != "cur-1" {
		t.Errorf("first frame = %+v", first)
	}
	data, _ := first.Data.(map[string]any)
	if fmt.Sprintf("%v", data["device_id"]) != "d1" {
		t.Errorf("first frame data = %v", first.Data)
	}

	var second ndjsonFrame
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("unmarshal second line: %v", err)
	}
	if second.Event != "reconnect" || second.ID != "" {
		t.Errorf("second frame (no id) = %+v", second)
	}

	var third ndjsonFrame
	if err := json.Unmarshal([]byte(lines[2]), &third); err != nil {
		t.Fatalf("unmarshal third line: %v", err)
	}
	if third.ID != "cur-2" {
		t.Errorf("third frame id = %q", third.ID)
	}
	thirdData, _ := third.Data.(map[string]any)
	if fmt.Sprintf("%v", thirdData["a"]) != "1" || fmt.Sprintf("%v", thirdData["b"]) != "2" {
		t.Errorf("multi-line data payload not joined correctly: %v", third.Data)
	}
}

func TestStreamRoomEventsNDJSONMalformedDataIsAnError(t *testing.T) {
	body := "event: connected\ndata: {not json}\n\n"
	var out, stderr bytes.Buffer
	err := streamRoomEvents(context.Background(), strings.NewReader(body), &out, &stderr, false, true)
	if err == nil {
		t.Fatal("expected an error on malformed data, got nil")
	}
	if !strings.Contains(stderr.String(), "frames=0") {
		t.Errorf("expected the summary on stderr even on error, got %q", stderr.String())
	}
}

type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("connection reset")
}

type erroringWriter struct{}

func (erroringWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("broken pipe")
}

func TestStreamRoomEventsNDJSONCancelledContextIsNotAnError(t *testing.T) {
	// A cancelled context turns a read error into a clean stop (SIGINT path),
	// reported on stderr but not returned as an error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	err := streamRoomEvents(ctx, &erroringReader{}, &out, &stderr, false, true)
	if err != nil {
		t.Fatalf("expected no error once ctx is cancelled, got %v", err)
	}
	if !strings.Contains(stderr.String(), "interrupted") {
		t.Errorf("expected an interrupted reason on stderr, got %q", stderr.String())
	}
}

func TestStreamRoomEventsDefaultAndRawUnchanged(t *testing.T) {
	var defaultOut, rawOut, stderr bytes.Buffer

	if err := streamRoomEvents(context.Background(), strings.NewReader(sseFixtureBody), &defaultOut, &stderr, false, false); err != nil {
		t.Fatalf("default form: %v", err)
	}
	if !strings.Contains(defaultOut.String(), "id: cur-1\nevent: connected\n") {
		t.Errorf("default form output changed: %q", defaultOut.String())
	}
	if !strings.Contains(defaultOut.String(), ": heartbeat") {
		t.Errorf("default form must still surface the heartbeat comment: %q", defaultOut.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("default form must stay silent on stderr, got %q", stderr.String())
	}

	if err := streamRoomEvents(context.Background(), strings.NewReader(sseFixtureBody), &rawOut, &stderr, true, false); err != nil {
		t.Fatalf("raw form: %v", err)
	}
	if !strings.Contains(rawOut.String(), "connected cur-1 ") {
		t.Errorf("raw form output changed: %q", rawOut.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("raw form must stay silent on stderr, got %q", stderr.String())
	}
}

func TestStreamRoomEventsWriteErrorIsReturned(t *testing.T) {
	// A failed write to out comes back as err ("a break as a break"), for
	// frame writes and SSE comment lines alike.
	var stderr bytes.Buffer
	err := streamRoomEvents(context.Background(), strings.NewReader(sseFixtureBody), erroringWriter{}, &stderr, false, false)
	if err == nil {
		t.Fatal("expected the frame write error to be returned")
	}

	err = streamRoomEvents(context.Background(), strings.NewReader(": heartbeat\n\n"), erroringWriter{}, &stderr, false, false)
	if err == nil {
		t.Fatal("expected the comment-line write error to be returned")
	}
}

func TestRoomsEventsRawAndNDJSONAreMutuallyExclusive(t *testing.T) {
	eventsRaw, eventsNDJSON = true, true
	defer func() { eventsRaw, eventsNDJSON = false, false }()

	if err := roomsEventsCmd.RunE(roomsEventsCmd, []string{"1"}); err == nil {
		t.Error("expected --raw + --ndjson to be rejected before any request is made")
	}
}
