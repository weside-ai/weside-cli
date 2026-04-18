package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/api"
)

// setupTestServer creates a test HTTP server with the given handler and
// returns a client pointing at it.
func setupTestServer(t *testing.T, handler http.HandlerFunc) *api.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return api.NewClient(srv.URL, "test-token")
}

// companionListResponse returns a typical companions list response body.
func companionListResponse(id, name string) string {
	return `{"companions":[{"id":` + id + `,"name":"` + name + `","personality":"test personality"}],"total":1}`
}

// companionDetailResponse returns a typical companion detail response body.
func companionDetailResponse(id, name string) string {
	return `{"id":` + id + `,"name":"` + name + `","personality":"test","system_prompt":"Hello from system","short_description":"A test companion","category":"general","tags":["a","b"],"is_published":true,"avatar_url":null,"banner_url":null}`
}

// --- resolveCompanionID ---

func TestResolveCompanionIDByNumber(t *testing.T) {
	// Numeric arg → return as-is, no API call needed
	calls := 0
	client := setupTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})

	id, err := resolveCompanionID(client, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "42" {
		t.Errorf("id = %q, want %q", id, "42")
	}
	if calls != 0 {
		t.Errorf("API was called %d times for numeric ID (expected 0)", calls)
	}
}

func TestResolveCompanionIDByName(t *testing.T) {
	client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/companions" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(companionListResponse("7", "Nox")))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	id, err := resolveCompanionID(client, "Nox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "7" {
		t.Errorf("id = %q, want %q", id, "7")
	}
}

func TestResolveCompanionIDNotFound(t *testing.T) {
	client := setupTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"companions":[],"total":0}`))
	})

	_, err := resolveCompanionID(client, "Unknown")
	if err == nil {
		t.Fatal("expected error for unknown companion, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

// --- readSystemPrompt ---

func TestReadSystemPromptInline(t *testing.T) {
	cmd := companionsUpdateCmd
	// Reset flags state
	cmd.ResetFlags()
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")

	if err := cmd.Flags().Set("system-prompt", "hello world"); err != nil {
		t.Fatal(err)
	}

	sp, has, err := readSystemPrompt(cmd, "hello world", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected has=true")
	}
	if sp != "hello world" {
		t.Errorf("sp = %q, want %q", sp, "hello world")
	}
}

func TestReadSystemPromptFromStdin(t *testing.T) {
	cmd := companionsUpdateCmd
	cmd.ResetFlags()
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")

	if err := cmd.Flags().Set("system-prompt", "-"); err != nil {
		t.Fatal(err)
	}

	orig := stdinReader
	stdinReader = strings.NewReader("piped content\n")
	defer func() { stdinReader = orig }()

	sp, has, err := readSystemPrompt(cmd, "-", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected has=true")
	}
	if sp != "piped content\n" {
		t.Errorf("sp = %q, want %q", sp, "piped content\n")
	}
}

func TestReadSystemPromptMutuallyExclusive(t *testing.T) {
	cmd := companionsUpdateCmd
	cmd.ResetFlags()
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")

	_ = cmd.Flags().Set("system-prompt", "inline")
	_ = cmd.Flags().Set("system-prompt-file", "somefile.md")

	_, _, err := readSystemPrompt(cmd, "inline", "somefile.md")
	if err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want 'mutually exclusive'", err.Error())
	}
}

func TestReadSystemPromptNoFlag(t *testing.T) {
	cmd := companionsUpdateCmd
	cmd.ResetFlags()
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")

	sp, has, err := readSystemPrompt(cmd, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected has=false when no flag set")
	}
	if sp != "" {
		t.Errorf("sp = %q, want empty", sp)
	}
}

// --- update command PATCH body ---

func TestUpdateSendsPatchBody(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/companions/5" && r.Method == http.MethodPatch {
			gotMethod = r.Method
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":5,"name":"Nox","personality":"updated"}`))
		}
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "tok")
	var result map[string]any
	body := map[string]any{"personality": "updated"}
	if err := client.Patch(context.Background(), "/companions/5", body, &result); err != nil {
		t.Fatalf("PATCH error: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotBody["personality"] != "updated" {
		t.Errorf("body personality = %v, want 'updated'", gotBody["personality"])
	}
}

func TestUpdateTagsParsedFromCSV(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/companions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(companionListResponse("1", "Nox")))
		case "/api/v1/companions/1":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":1,"name":"Nox"}`))
		}
	}))
	defer srv.Close()

	t.Setenv("WESIDE_TOKEN", "test-token")
	viper.Set("api_url", srv.URL)
	defer viper.Set("api_url", "")

	companionsUpdateCmd.ResetFlags()
	companionsUpdateCmd.Flags().StringVar(&compUpdateName, "name", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdatePersonality, "personality", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateShortDescription, "short-description", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateCategory, "category", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateTags, "tags", "", "")
	companionsUpdateCmd.Flags().BoolVar(&compUpdatePublish, "publish", false, "")
	companionsUpdateCmd.Flags().BoolVar(&compUpdateUnpublish, "unpublish", false, "")

	if err := companionsUpdateCmd.Flags().Set("tags", "a, b, c"); err != nil {
		t.Fatal(err)
	}
	compUpdatePublish = false
	compUpdateUnpublish = false

	if err := companionsUpdateCmd.RunE(companionsUpdateCmd, []string{"Nox"}); err != nil {
		t.Fatalf("RunE error: %v", err)
	}

	rawTags, ok := gotBody["tags"].([]any)
	if !ok {
		t.Fatalf("tags in body is %T, want []any", gotBody["tags"])
	}
	if len(rawTags) != 3 {
		t.Errorf("tags len = %d, want 3", len(rawTags))
	}
	for i, want := range []string{"a", "b", "c"} {
		if rawTags[i] != want {
			t.Errorf("tags[%d] = %v, want %q", i, rawTags[i], want)
		}
	}
}

// TestUpdateNoFieldsProvidedError verifies the guard that rejects update calls
// with no flags set. Uses a numeric companion ID to skip the list API call and
// WESIDE_TOKEN env var to satisfy auth — no network call is made since body is
// empty before the PATCH would fire.
func TestUpdateNoFieldsProvidedError(t *testing.T) {
	t.Setenv("WESIDE_TOKEN", "test-token")

	// Reset flags so no Changed() state leaks from previous tests.
	companionsUpdateCmd.ResetFlags()
	companionsUpdateCmd.Flags().StringVar(&compUpdateName, "name", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdatePersonality, "personality", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateShortDescription, "short-description", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateCategory, "category", "", "")
	companionsUpdateCmd.Flags().StringVar(&compUpdateTags, "tags", "", "")
	companionsUpdateCmd.Flags().BoolVar(&compUpdatePublish, "publish", false, "")
	companionsUpdateCmd.Flags().BoolVar(&compUpdateUnpublish, "unpublish", false, "")
	compUpdatePublish = false
	compUpdateUnpublish = false

	err := companionsUpdateCmd.RunE(companionsUpdateCmd, []string{"1"})
	if err == nil {
		t.Fatal("expected 'no fields provided' error, got nil")
	}
	if !strings.Contains(err.Error(), "no fields") {
		t.Errorf("error = %q, want 'no fields provided'", err.Error())
	}
}

// TestReadSystemPromptFromFile verifies that --system-prompt-file reads the
// file and returns its content.
func TestReadSystemPromptFromFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/identity.md"
	content := "You are a helpful assistant with lots of personality."
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := companionsUpdateCmd
	cmd.ResetFlags()
	cmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	cmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")

	if err := cmd.Flags().Set("system-prompt-file", path); err != nil {
		t.Fatal(err)
	}

	sp, has, err := readSystemPrompt(cmd, "", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected has=true")
	}
	if sp != content {
		t.Errorf("sp = %q, want %q", sp, content)
	}
}

// TestReadSystemPromptFromFileMissing verifies that a missing file path
// returns a clear error.
func TestReadSystemPromptFromFileMissing(t *testing.T) {
	cmd := companionsUpdateCmd
	cmd.ResetFlags()
	cmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
	cmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")

	_ = cmd.Flags().Set("system-prompt-file", "/nonexistent/path/identity.md")

	_, _, err := readSystemPrompt(cmd, "", "/nonexistent/path/identity.md")
	if err == nil {
		t.Fatal("expected file-not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "reading system prompt file") {
		t.Errorf("error = %q, want file read error", err.Error())
	}
}

// TestUpdateIsPublishedBodyContract verifies the sparse-PATCH behaviour for
// the three is_published states: --publish sets true, --unpublish sets false,
// neither flag = key absent from body.
func TestUpdateIsPublishedBodyContract(t *testing.T) {
	cases := []struct {
		name        string
		publish     bool
		unpublish   bool
		wantPresent bool
		wantValue   any
	}{
		{"--publish sets true", true, false, true, true},
		{"--unpublish sets false", false, true, true, false},
		{"neither: key absent", false, false, false, nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/companions":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(companionListResponse("9", "TestBot")))
				case "/api/v1/companions/9":
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"id":9,"name":"TestBot"}`))
				}
			}))
			defer srv.Close()

			t.Setenv("WESIDE_TOKEN", "test-token")
			viper.Set("api_url", srv.URL)
			defer viper.Set("api_url", "")

			compUpdatePublish = tc.publish
			compUpdateUnpublish = tc.unpublish
			// Provide at least one other field so body is not empty in the
			// "neither" case; personality is always present in the body when
			// the flag is Changed.
			compUpdatePersonality = "some personality"

			// Reset flags so Changed() reflects our manual assignments.
			companionsUpdateCmd.ResetFlags()
			companionsUpdateCmd.Flags().StringVar(&compUpdateName, "name", "", "")
			companionsUpdateCmd.Flags().StringVar(&compUpdatePersonality, "personality", "", "")
			companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "")
			companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "")
			companionsUpdateCmd.Flags().StringVar(&compUpdateShortDescription, "short-description", "", "")
			companionsUpdateCmd.Flags().StringVar(&compUpdateCategory, "category", "", "")
			companionsUpdateCmd.Flags().StringVar(&compUpdateTags, "tags", "", "")
			companionsUpdateCmd.Flags().BoolVar(&compUpdatePublish, "publish", false, "")
			companionsUpdateCmd.Flags().BoolVar(&compUpdateUnpublish, "unpublish", false, "")

			if err := companionsUpdateCmd.Flags().Set("personality", "some personality"); err != nil {
				t.Fatal(err)
			}
			if tc.publish {
				if err := companionsUpdateCmd.Flags().Set("publish", "true"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.unpublish {
				if err := companionsUpdateCmd.Flags().Set("unpublish", "true"); err != nil {
					t.Fatal(err)
				}
			}

			err := companionsUpdateCmd.RunE(companionsUpdateCmd, []string{"TestBot"})
			if err != nil {
				t.Fatalf("RunE error: %v", err)
			}

			_, present := gotBody["is_published"]
			if tc.wantPresent && !present {
				t.Errorf("is_published missing from body, want %v", tc.wantValue)
			}
			if !tc.wantPresent && present {
				t.Errorf("is_published present in body, want absent")
			}
			if present && gotBody["is_published"] != tc.wantValue {
				t.Errorf("is_published = %v, want %v", gotBody["is_published"], tc.wantValue)
			}
		})
	}
}

// --- delete command ---

func TestDeleteWithoutYesFlagErrors(t *testing.T) {
	compDeleteYes = false
	err := companionsDeleteCmd.RunE(companionsDeleteCmd, []string{"Nox"})
	if err == nil {
		t.Fatal("expected 'refusing to delete' error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to delete") {
		t.Errorf("error = %q, want 'refusing to delete'", err.Error())
	}
}

func TestDeleteWithYesSendsDeleteRequest(t *testing.T) {
	var deleteCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/companions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(companionListResponse("3", "TestBot")))
		case "/api/v1/companions/3":
			if r.Method == http.MethodDelete {
				deleteCalled = true
				w.WriteHeader(http.StatusNoContent)
			}
		}
	}))
	defer srv.Close()

	t.Setenv("WESIDE_TOKEN", "test-token")
	viper.Set("api_url", srv.URL)
	defer viper.Set("api_url", "")

	compDeleteYes = true
	if err := companionsDeleteCmd.RunE(companionsDeleteCmd, []string{"TestBot"}); err != nil {
		t.Fatalf("RunE error: %v", err)
	}
	if !deleteCalled {
		t.Error("expected DELETE to be called on /api/v1/companions/3")
	}
}

// --- show output includes new fields ---

func TestShowResponseContainsSystemPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/companions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(companionListResponse("2", "Nox")))
		case "/companions/2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(companionDetailResponse("2", "Nox")))
		}
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "tok")
	var companion map[string]any
	err := client.Get(context.Background(), "/companions/2", &companion)
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}

	if companion["system_prompt"] != "Hello from system" {
		t.Errorf("system_prompt = %v, want 'Hello from system'", companion["system_prompt"])
	}
	if companion["category"] != "general" {
		t.Errorf("category = %v, want 'general'", companion["category"])
	}
	if companion["is_published"] != true {
		t.Errorf("is_published = %v, want true", companion["is_published"])
	}
	tags, _ := companion["tags"].([]any)
	if len(tags) != 2 {
		t.Errorf("tags len = %d, want 2", len(tags))
	}
}

// --- JSON output flag ---

func TestJSONOutputFormatting(t *testing.T) {
	data := map[string]any{"deleted": true, "id": "3"}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
	output := buf.String()
	if !strings.Contains(output, `"deleted"`) {
		t.Errorf("JSON output missing 'deleted' key: %s", output)
	}
	if !strings.Contains(output, `"id"`) {
		t.Errorf("JSON output missing 'id' key: %s", output)
	}
}
