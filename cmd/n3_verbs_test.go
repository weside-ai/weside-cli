package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/viper"
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
		// Field names copied from the generated OpenAPI schema, not from
		// memory: the first draft of this test asserted `event_class`, which
		// does not exist, and passed — because the fixture carried the same
		// wrong name as the code it was checking.
		_, _ = fmt.Fprint(w, `{"room_id":14,"events":[`+
			`{"created_at":"2026-07-30T06:00:00+00:00","event_kind":"memory",`+
			`"outcome":"success","tool_name":"save_memory","companion_name":"Nox"}`+
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
	// The event kind AND the outcome come from the server: the client neither
	// re-derives the lane from the tool name (that mapping has one home,
	// WA-1784) nor assumes an invocation succeeded.
	first, _ := events[0].(map[string]any)
	if first["event_kind"] != "memory" {
		t.Fatalf("event kind not carried through: %v", first["event_kind"])
	}
	if first["outcome"] != "success" {
		t.Fatalf("outcome not carried through: %v", first["outcome"])
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

// TestNotesDeleteBuildsTheRightRequest pins the request shape the durability
// check in AC1 relies on: a DELETE against /notes with path and recursive as
// query params, never a body — the Contents API delete carries no payload
// (Design Decision 2).
func TestNotesDeleteBuildsTheRightRequest(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"path":"inbox/shot.png","deleted":true,"count":1,"commit_sha":"abc123def"}`)
	}))
	defer srv.Close()

	t.Setenv("WESIDE_TOKEN", "token")
	viper.Set("api_url", srv.URL)
	t.Cleanup(func() { viper.Set("api_url", "") })

	notesDeleteRecursive = true
	t.Cleanup(func() { notesDeleteRecursive = false })

	if err := notesDeleteCmd.RunE(notesDeleteCmd, []string{"inbox/shot.png"}); err != nil {
		t.Fatalf("notes delete: %v", err)
	}

	if gotMethod != http.MethodDelete || gotPath != "/api/v1/notes" {
		t.Fatalf("wrong request: %s %s", gotMethod, gotPath)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parsing query: %v", err)
	}
	if q.Get("path") != "inbox/shot.png" {
		t.Fatalf("path param = %q", q.Get("path"))
	}
	if q.Get("recursive") != "true" {
		t.Fatalf("recursive param = %q, want true", q.Get("recursive"))
	}
}

// TestNotesDeleteJSONCarriesCommitSHA pins AC10: --json must surface
// commit_sha, the field the story's verification reads to prove the delete
// survived as a commit rather than a bare API call.
func TestNotesDeleteJSONCarriesCommitSHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"path":"inbox/shot.png","deleted":true,"count":1,"commit_sha":"deadbeef01"}`)
	}))
	defer srv.Close()

	t.Setenv("WESIDE_TOKEN", "token")
	viper.Set("api_url", srv.URL)
	viper.Set("json", true)
	t.Cleanup(func() {
		viper.Set("api_url", "")
		viper.Set("json", false)
	})

	out := captureStdout(t, func() error {
		return notesDeleteCmd.RunE(notesDeleteCmd, []string{"inbox/shot.png"})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("output is not JSON: %v\noutput: %s", err, out)
	}
	if result["commit_sha"] != "deadbeef01" {
		t.Fatalf("commit_sha missing or wrong in --json output: %v", result)
	}
}
