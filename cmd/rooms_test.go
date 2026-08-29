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

	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

func captureStdoutRooms(t *testing.T, fn func() error) string {
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
		t.Fatalf("command error: %v", runErr)
	}
	return string(out)
}

func TestFollowEventsParsesFrames(t *testing.T) {
	// One frame with an id, one without (the "reconnect" shape), a heartbeat
	// comment in between (counted, never printed), and a multi-line data
	// payload joined by \n before being parsed as JSON.
	body := "id: cur-1\n" +
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rooms/1/events" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	resp, err := client.Subscribe(context.Background(), "/rooms/1/events")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out bytes.Buffer
	summary, err := followEvents(context.Background(), resp.Body, &out)
	if err != nil {
		t.Fatalf("followEvents: %v", err)
	}
	if summary.Frames != 3 {
		t.Fatalf("expected 3 frames, got %d (out=%q)", summary.Frames, out.String())
	}
	if summary.Heartbeats != 1 {
		t.Fatalf("expected 1 heartbeat, got %d", summary.Heartbeats)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d: %q", len(lines), out.String())
	}
	if strings.Contains(out.String(), "heartbeat") {
		t.Errorf("heartbeat comment leaked into stdout: %q", out.String())
	}

	var first followFrame
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

	var second followFrame
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("unmarshal second line: %v", err)
	}
	if second.Event != "reconnect" || second.ID != "" {
		t.Errorf("second frame (no id) = %+v", second)
	}

	var third followFrame
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

func TestFollowEventsMalformedDataIsAnError(t *testing.T) {
	body := "event: connected\ndata: {not json}\n\n"
	var out bytes.Buffer
	_, err := followEvents(context.Background(), strings.NewReader(body), &out)
	if err == nil {
		t.Fatal("expected an error on malformed data, got nil")
	}
}

func TestFollowEventsCancelledContextIsNotAnError(t *testing.T) {
	// A cancelled context turns a read error into a clean stop (SIGINT path),
	// not a reported failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	summary, err := followEvents(ctx, &erroringReader{}, &out)
	if err != nil {
		t.Fatalf("expected no error once ctx is cancelled, got %v", err)
	}
	if summary.Frames != 0 {
		t.Errorf("expected no frames, got %d", summary.Frames)
	}
}

type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("connection reset")
}

func TestRoomsFollowRegistered(t *testing.T) {
	cmd, _, err := roomsCmd.Find([]string{"follow"})
	if err != nil || cmd == nil || cmd.Name() != "follow" {
		t.Fatalf("rooms follow missing: cmd=%v err=%v", cmd, err)
	}
}

func TestRoomsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rooms" || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"rooms":[`+
			`{"id":1,"kind":"dm","title":"Nox","muted":true,"last_message":{"snippet":"hello"},"updated_at":"2026-07-24T10:00:00+00:00"},`+
			`{"id":2,"kind":"group","auto_title":"Team","muted":false,"updated_at":"2026-07-24T11:00:00+00:00"}`+
			`],"total":2}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdoutRooms(t, func() error {
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms", &result); err != nil {
			return err
		}
		rooms, _ := result["rooms"].([]any)
		headers := []string{"ID", "KIND", "TITLE", "MUTED", "LAST MESSAGE", "UPDATED"}
		var rows [][]string
		for _, item := range rooms {
			r, _ := item.(map[string]any)
			id := fmt.Sprintf("%v", r["id"])
			kind := fmt.Sprintf("%v", r["kind"])
			title := fmt.Sprintf("%v", r["title"])
			if title == "<nil>" || title == "" {
				title = fmt.Sprintf("%v", r["auto_title"])
			}
			lastMsg := ""
			if lm, ok := r["last_message"].(map[string]any); ok {
				lastMsg = truncate(fmt.Sprintf("%v", lm["snippet"]), 40)
			}
			updated := fmt.Sprintf("%v", r["updated_at"])
			muted := ""
			if r["muted"] == true {
				muted = "yes"
			}
			rows = append(rows, []string{id, kind, truncate(title, 30), muted, lastMsg, updated})
		}
		ui.PrintTable(headers, rows)
		fmt.Printf("\n%v room(s)\n", result["total"])
		return nil
	})

	if !strings.Contains(out, "Nox") || !strings.Contains(out, "Team") {
		t.Errorf("expected both room titles in table, got %q", out)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("expected muted marker in table, got %q", out)
	}
}

func TestSetRoomMuteUsesResourceMethods(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rooms/42/mute" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":42,"muted":%t}`, r.Method == http.MethodPut)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	muted, err := setRoomMute(context.Background(), client, "42", true)
	if err != nil {
		t.Fatalf("mute: %v", err)
	}
	unmuted, err := setRoomMute(context.Background(), client, "42", false)
	if err != nil {
		t.Fatalf("unmute: %v", err)
	}

	if fmt.Sprintf("%v", muted["muted"]) != "true" || fmt.Sprintf("%v", unmuted["muted"]) != "false" {
		t.Fatalf("unexpected responses: muted=%v unmuted=%v", muted, unmuted)
	}
	if fmt.Sprint(methods) != "[PUT DELETE]" {
		t.Fatalf("expected PUT then DELETE, got %v", methods)
	}
}

func TestRoomsMuteCommandsAreRegistered(t *testing.T) {
	for _, name := range []string{"mute", "unmute"} {
		cmd, _, err := roomsCmd.Find([]string{name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("rooms %s command missing: cmd=%v err=%v", name, cmd, err)
		}
	}
}

func TestRoomsShowRoleLabels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages") || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"messages":[`+
			`{"role":"user","content":[{"type":"text","text":"hi"}]},`+
			`{"role":"assistant","content":[{"type":"text","text":"hello back"}]}`+
			`],"next_cursor":null,"prev_cursor":null}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdoutRooms(t, func() error {
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/1/messages", &result); err != nil {
			return err
		}
		messages, _ := result["messages"].([]any)
		for _, item := range messages {
			msg, _ := item.(map[string]any)
			role := fmt.Sprintf("%v", msg["role"])
			prefix := roleLabel(role)
			if content, ok := msg["content"].([]any); ok {
				for _, block := range content {
					if b, ok := block.(map[string]any); ok {
						if text, ok := b["text"].(string); ok {
							fmt.Printf("[%s] %s\n\n", prefix, text)
						}
					}
				}
			}
		}
		return nil
	})

	if !strings.Contains(out, "[You] hi") {
		t.Errorf("expected user message labelled [You], got %q", out)
	}
	if !strings.Contains(out, "[Companion] hello back") {
		t.Errorf("expected assistant reply labelled [Companion], got %q", out)
	}
}

func TestRoomActivityQuery(t *testing.T) {
	t.Run("scope last_turn reaches the wire", func(t *testing.T) {
		q := roomActivityQuery(100, "", "last_turn")
		if got := q.Get("scope"); got != "last_turn" {
			t.Fatalf("scope = %q, want last_turn", got)
		}
		if got := q.Get("limit"); got != "100" {
			t.Fatalf("limit = %q, want 100", got)
		}
		if _, ok := q["cursor"]; ok {
			t.Fatal("cursor must be absent when unset")
		}
	})

	t.Run("the default scope is not sent", func(t *testing.T) {
		q := roomActivityQuery(50, "abc", "all")
		if _, ok := q["scope"]; ok {
			t.Fatal("scope=all is the server default and must not be sent")
		}
		if got := q.Get("cursor"); got != "abc" {
			t.Fatalf("cursor = %q, want abc", got)
		}
	})
}

// Drives the REAL helpers the commands call, so the assertion is about the
// production path construction and not about a path the test typed itself. The
// mistake worth catching is exactly that: an accept is code-scoped
// (/rooms/invites/<code>/accept), not room-scoped like its siblings, and a
// wrong path returns a 404 indistinguishable from the deliberate uniform 404
// for an invalid code — so it would survive a manual round unnoticed.
func TestInviteAcceptAndPreviewUseTheCodeScopedPaths(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":7,"title":"Verein","room_title":"Verein",`+
			`"inviter_display_name":"Foxy","human_count":2,"companion_count":1}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	accepted, err := acceptInvite(context.Background(), client, "abc123")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	preview, err := previewInvite(context.Background(), client, "abc123")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	want := "[POST /rooms/invites/abc123/accept GET /rooms/invites/abc123]"
	if fmt.Sprint(seen) != want {
		t.Fatalf("wrong invite paths: got %v want %v", seen, want)
	}
	if fmt.Sprintf("%v", accepted["id"]) != "7" {
		t.Fatalf("accept did not return the joined room: %v", accepted)
	}
	if fmt.Sprintf("%v", preview["human_count"]) != "2" {
		t.Fatalf("preview did not return the four public fields: %v", preview)
	}
}

func TestRoomsInviteVerbsAreRegistered(t *testing.T) {
	for _, name := range []string{"create", "list", "revoke", "accept", "preview"} {
		cmd, _, err := roomsInvitesCmd.Find([]string{name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("rooms invites %s missing: cmd=%v err=%v", name, cmd, err)
		}
	}
}
