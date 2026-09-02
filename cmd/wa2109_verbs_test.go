package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// The five CLI verbs WA-2109's verification round needs before the backend
// endpoints exist (weside-core Phase 5/6/9). Contracts come from the story's
// Verification block, not from a live backend — see WORKER-REPORT.md for the
// request/response shapes this pins down.

func withTestAPI(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("WESIDE_TOKEN", "token")
	viper.Set("api_url", srv.URL)
	t.Cleanup(func() { viper.Set("api_url", "") })
}

// --- notes recent -----------------------------------------------------------

func TestNotesRecentDefaultRequest(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"title":"Packliste update","author":"Nox","committed_at":"2026-09-02T09:14:00Z","path":"notes/packliste.md","is_new":true}]}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	if err := notesRecentCmd.RunE(notesRecentCmd, nil); err != nil {
		t.Fatalf("notes recent: %v", err)
	}
	if gotPath != "/api/v1/notes/recent" {
		t.Fatalf("wrong path: %q", gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("expected no query params without --since-last-look, got %q", gotQuery)
	}
}

func TestNotesRecentSinceLastLookSetsQueryParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	notesRecentSinceLastLook = true
	t.Cleanup(func() { notesRecentSinceLastLook = false })

	if err := notesRecentCmd.RunE(notesRecentCmd, nil); err != nil {
		t.Fatalf("notes recent: %v", err)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parsing query: %v", err)
	}
	if q.Get("since_last_look") != "true" {
		t.Fatalf("since_last_look param = %q, want true", q.Get("since_last_look"))
	}
}

// TestNotesRecentJSONCarriesIsNew pins the field the verification round
// reads directly: is_new flips from true to false after POST
// /notes/last-look, and the CLI must pass that bit through untouched.
func TestNotesRecentJSONCarriesIsNew(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"title":"Packliste update","author":"Nox","committed_at":"2026-09-02T09:14:00Z","path":"notes/packliste.md","is_new":false}]}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)
	viper.Set("json", true)
	t.Cleanup(func() { viper.Set("json", false) })

	out := captureStdout(t, func() error { return notesRecentCmd.RunE(notesRecentCmd, nil) })

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("output is not JSON: %v\noutput: %s", err, out)
	}
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if isNew, ok := first["is_new"].(bool); !ok || isNew {
		t.Fatalf("is_new not carried through as false: %v", first["is_new"])
	}
}

// --- notes working-set -------------------------------------------------------

func TestNotesWorkingSetAddSendsPathAsBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"path":"vertrag-final.pdf","kind":"file","added_by":"user","added_at":"2026-09-02T09:00:00Z"}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	if err := notesWorkingSetAddCmd.RunE(notesWorkingSetAddCmd, []string{"vertrag-final.pdf"}); err != nil {
		t.Fatalf("working-set add: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/notes/working-set" {
		t.Fatalf("wrong request: %s %s", gotMethod, gotPath)
	}
	if gotBody["path"] != "vertrag-final.pdf" {
		t.Fatalf("body path = %v", gotBody["path"])
	}
}

// TestNotesWorkingSetAddFourthSurfacesMemberNames pins AC8: a fourth add is
// refused with 409, and the CLI must surface the server's detail text (which
// names the three current members) rather than replacing it with a generic
// failure message.
func TestNotesWorkingSetAddFourthSurfacesMemberNames(t *testing.T) {
	const detail = "Working set already holds 3 items: a.md, b.png, c.pdf"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprintf(w, `{"detail":%q}`, detail)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	err := notesWorkingSetAddCmd.RunE(notesWorkingSetAddCmd, []string{"d.txt"})
	if err == nil {
		t.Fatal("expected an error on the fourth add")
	}
	if !strings.Contains(err.Error(), detail) {
		t.Fatalf("error dropped the member names: %v", err)
	}
}

func TestNotesWorkingSetRemoveUsesQueryParam(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	if err := notesWorkingSetRemoveCmd.RunE(notesWorkingSetRemoveCmd, []string{"a.md"}); err != nil {
		t.Fatalf("working-set remove: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/notes/working-set" {
		t.Fatalf("wrong request: %s %s", gotMethod, gotPath)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parsing query: %v", err)
	}
	if q.Get("path") != "a.md" {
		t.Fatalf("path param = %q", q.Get("path"))
	}
}

func TestNotesWorkingSetListRendersJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/notes/working-set" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[{"path":"a.md","kind":"note","added_by":"user","added_at":"2026-09-02T09:00:00Z"}]}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)
	viper.Set("json", true)
	t.Cleanup(func() { viper.Set("json", false) })

	out := captureStdout(t, func() error { return notesWorkingSetListCmd.RunE(notesWorkingSetListCmd, nil) })

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("output is not JSON: %v\noutput: %s", err, out)
	}
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
}

// --- files keep ---------------------------------------------------------------

func TestFilesKeepPostsPath(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"path":"temp/regal-entwurf.png","kept":true,"expires_at":null}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	if err := filesKeepCmd.RunE(filesKeepCmd, []string{"temp/regal-entwurf.png"}); err != nil {
		t.Fatalf("files keep: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/files/keep" {
		t.Fatalf("wrong request: %s %s", gotMethod, gotPath)
	}
	if gotBody["path"] != "temp/regal-entwurf.png" {
		t.Fatalf("body path = %v", gotBody["path"])
	}
}

// --- files tree --recursive --sort recent -------------------------------------

func TestFilesTreeRecursiveAndSortQueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"entries":[{"type":"file","name":"a.png","size_bytes":10,"children_count":0,"expires_at":"2026-10-02T00:00:00Z"}],"total_count":1}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	filesRecursive = true
	filesSort = "recent"
	t.Cleanup(func() {
		filesRecursive = false
		filesSort = ""
	})

	if err := filesTreeCmd.RunE(filesTreeCmd, nil); err != nil {
		t.Fatalf("files tree: %v", err)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parsing query: %v", err)
	}
	if q.Get("recursive") != "true" {
		t.Fatalf("recursive param = %q, want true", q.Get("recursive"))
	}
	if q.Get("sort") != "recent" {
		t.Fatalf("sort param = %q, want recent", q.Get("sort"))
	}
}

// TestFilesTreeJSONCarriesExpiresAt pins AC7: a temp/ row's expires_at must
// reach --json output untouched, since the retention predicate test binds
// this same value to temp_cleanup.py's deletion decision.
func TestFilesTreeJSONCarriesExpiresAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"entries":[{"type":"file","name":"a.png","size_bytes":10,"children_count":0,"expires_at":"2026-10-02T00:00:00Z"}],"total_count":1}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)
	viper.Set("json", true)
	t.Cleanup(func() { viper.Set("json", false) })

	out := captureStdout(t, func() error { return filesTreeCmd.RunE(filesTreeCmd, nil) })

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("output is not JSON: %v\noutput: %s", err, out)
	}
	entries, _ := result["entries"].([]any)
	first, _ := entries[0].(map[string]any)
	if first["expires_at"] != "2026-10-02T00:00:00Z" {
		t.Fatalf("expires_at not carried through: %v", first["expires_at"])
	}
}

// --- ablage search -------------------------------------------------------------

func TestAblageSearchBuildsRequest(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[`+
			`{"kind":"note","path":"notes/vertrag.md","title":"Vertrag"},`+
			`{"kind":"file","path":"documents/vertrag-final.pdf","title":"vertrag-final.pdf"}`+
			`]}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)

	if err := ablageSearchCmd.RunE(ablageSearchCmd, []string{"vertrag"}); err != nil {
		t.Fatalf("ablage search: %v", err)
	}
	if gotPath != "/api/v1/ablage/search" {
		t.Fatalf("wrong path: %q", gotPath)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parsing query: %v", err)
	}
	if q.Get("q") != "vertrag" {
		t.Fatalf("q param = %q, want vertrag", q.Get("q"))
	}
}

// TestAblageSearchJSONReturnsBothKinds pins AC9: the vault PDF and the plain
// file must both come back from ONE endpoint, not two separate lookups.
func TestAblageSearchJSONReturnsBothKinds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[`+
			`{"kind":"note","path":"notes/vertrag.md","title":"Vertrag"},`+
			`{"kind":"file","path":"documents/vertrag-final.pdf","title":"vertrag-final.pdf"}`+
			`]}`)
	}))
	defer srv.Close()
	withTestAPI(t, srv)
	viper.Set("json", true)
	t.Cleanup(func() { viper.Set("json", false) })

	out := captureStdout(t, func() error { return ablageSearchCmd.RunE(ablageSearchCmd, []string{"vertrag"}) })

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("output is not JSON: %v\noutput: %s", err, out)
	}
	items, _ := result["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected both the vault row and the plain file, got %d", len(items))
	}
	kinds := map[string]bool{}
	for _, item := range items {
		m, _ := item.(map[string]any)
		kinds[fmt.Sprintf("%v", m["kind"])] = true
	}
	if !kinds["note"] || !kinds["file"] {
		t.Fatalf("expected both note and file kinds, got %v", kinds)
	}
}
