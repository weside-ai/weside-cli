package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

// The three verbs the N3 wave earned. Each one exists because the alternative
// was a shell dance somebody would copy out of a transcript twice — the
// standing rule in app-verification.md.

func TestN3VerbsAreRegistered(t *testing.T) {
	if cmd, _, err := roomsCmd.Find([]string{"activity"}); err != nil || cmd == nil || cmd.Name() != "activity" {
		t.Fatalf("rooms activity command missing: cmd=%v err=%v", cmd, err)
	}
	for _, name := range []string{"list", "delete"} {
		cmd, _, err := stageCmd.Find([]string{name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("stage %s command missing: cmd=%v err=%v", name, cmd, err)
		}
	}
	if searchCmd.Name() != "search" || !searchCmd.Runnable() {
		t.Fatalf("search command missing or not runnable")
	}
}

func TestRoomActivityReadsTheFeedEndpoint(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"room_id":14,"events":[`+
			`{"created_at":"2026-07-30T06:00:00+00:00","event_class":"memory",`+
			`"tool_name":"save_memory","companion_name":"Nox"}`+
			`],"next_cursor":"abc"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	var result map[string]any
	if err := client.Get(context.Background(), "/rooms/14/activity?limit=50", &result); err != nil {
		t.Fatalf("activity request: %v", err)
	}

	if gotPath != "/rooms/14/activity" {
		t.Fatalf("wrong path: %q", gotPath)
	}
	if gotQuery != "limit=50" {
		t.Fatalf("wrong query: %q", gotQuery)
	}
	events, _ := result["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	// The event class comes from the server, not from the client re-deriving it
	// from the tool name — that mapping has exactly one home (WA-1784).
	first, _ := events[0].(map[string]any)
	if first["event_class"] != "memory" {
		t.Fatalf("event class not carried through: %v", first["event_class"])
	}
}

func TestSearchKeepsTheGroupsApart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "vat" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{`+
			`"semantic":{"items":[{"title":"Danish VAT","snippet":"…"}],"total":1,"unavailable":false},`+
			`"notes":{"items":[],"total":0,"unavailable":true},`+
			`"files":{"items":[],"total":0,"unavailable":false}}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	var result map[string]any
	if err := client.Get(context.Background(), "/search?q=vat&limit=10", &result); err != nil {
		t.Fatalf("search request: %v", err)
	}

	// Three named blocks, never one merged list: the engines' ranks are not
	// comparable, so a single order would be arbitrary AND look authoritative.
	for _, key := range []string{"semantic", "notes", "files"} {
		if _, ok := result[key].(map[string]any); !ok {
			t.Fatalf("group %q missing from the response", key)
		}
	}
	if _, merged := result["items"]; merged {
		t.Fatal("a merged item list appeared — the groups must stay apart")
	}

	// One dead engine is a property of ITS block. A search that answers two of
	// three groups has done its job; DEV has no notes backend at all, so this
	// is the shape that surface is walked in.
	notes, _ := result["notes"].(map[string]any)
	if unavailable, _ := notes["unavailable"].(bool); !unavailable {
		t.Fatal("the notes block lost its unavailable flag")
	}
	semantic, _ := result["semantic"].(map[string]any)
	if unavailable, _ := semantic["unavailable"].(bool); unavailable {
		t.Fatal("a healthy engine must not be marked unavailable")
	}
}

func TestStageDeleteUsesTheArtifactResource(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"deleted":true}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	var result map[string]any
	if err := client.Delete(context.Background(), "/stage/artifacts/abc123", &result); err != nil {
		t.Fatalf("delete request: %v", err)
	}

	if gotMethod != http.MethodDelete || gotPath != "/stage/artifacts/abc123" {
		t.Fatalf("wrong request: %s %s", gotMethod, gotPath)
	}
}
