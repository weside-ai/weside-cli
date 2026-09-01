package cmd

import (
	"context"
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
		q := roomActivityQuery(100, "last_turn")
		if got := q.Get("scope"); got != "last_turn" {
			t.Fatalf("scope = %q, want last_turn", got)
		}
		if got := q.Get("limit"); got != "100" {
			t.Fatalf("limit = %q, want 100", got)
		}
	})

	t.Run("the default scope is not sent", func(t *testing.T) {
		q := roomActivityQuery(50, "all")
		if _, ok := q["scope"]; ok {
			t.Fatal("scope=all is the server default and must not be sent")
		}
	})

	// WA-2145 removed the activity endpoint's cursor. The query builder must
	// not put one back: the endpoint ignores it, so a `cursor=` on the wire
	// would read as paging that silently does nothing — the shape this repo's
	// no-legacy-fallback rule exists to keep out. The message timeline's own
	// cursor (`rooms show --cursor`) is a different endpoint and stays.
	t.Run("no cursor reaches the wire, whatever the scope", func(t *testing.T) {
		for _, scope := range []string{"all", "last_turn"} {
			q := roomActivityQuery(100, scope)
			if _, ok := q["cursor"]; ok {
				t.Fatalf("scope %q: cursor must never be sent", scope)
			}
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
