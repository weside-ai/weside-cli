package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

type seenRequest struct {
	method, path string
	body         map[string]any
}

func stageAppsServer(t *testing.T, seen *[]seenRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := seenRequest{method: r.Method, path: r.URL.Path}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			if err := json.Unmarshal(b, &req.body); err != nil {
				t.Errorf("%s %s: body is not a JSON object: %q", r.Method, r.URL.Path, b)
			}
		}
		*seen = append(*seen, req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[],"key":"k","revision":1,"capabilities":["app_data_read"],"active":true}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Drives the real helpers the commands call, so the paths and bodies asserted
// are the production ones. Pinned: owner vs room member route per --room, that
// an absent --key / --expected-revision sends NO key (a null key would be a
// different request), and the share capability set per --view-only.
func TestStageAppVerbsUseTheV2Routes(t *testing.T) {
	var seen []seenRequest
	srv := stageAppsServer(t, &seen)
	c := api.NewClient(srv.URL, "token")
	ctx := context.Background()
	rev := 4

	steps := []func() error{
		func() error { _, err := readAppData(ctx, c, "812", 0, ""); return err },
		func() error { _, err := readAppData(ctx, c, "812", 42, "scores"); return err },
		func() error {
			_, err := writeAppData(ctx, c, "812", 0, "scores", json.RawMessage(`{"foxy":3}`), nil)
			return err
		},
		func() error {
			_, err := writeAppData(ctx, c, "812", 42, "scores", json.RawMessage(`null`), &rev)
			return err
		},
		func() error {
			_, err := callAppTool(ctx, c, "812", 0, "companion_event", json.RawMessage(`{"name":"won"}`))
			return err
		},
		func() error { _, err := shareApp(ctx, c, "812", 42, false); return err },
		func() error { _, err := shareApp(ctx, c, "812", 42, true); return err },
		func() error { _, err := unshareApp(ctx, c, "812", 42); return err },
		func() error { _, err := listRoomApps(ctx, c, 42); return err },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	want := []struct{ method, path, body string }{
		{"POST", "/stage/artifacts/812/tools/app_data_read", `{}`},
		{"POST", "/rooms/42/apps/812/tools/app_data_read", `{"key":"scores"}`},
		{"POST", "/stage/artifacts/812/tools/app_data_write", `{"key":"scores","value":{"foxy":3}}`},
		{"POST", "/rooms/42/apps/812/tools/app_data_write", `{"expected_revision":4,"key":"scores","value":null}`},
		{"POST", "/stage/artifacts/812/tools/companion_event", `{"name":"won"}`},
		{"PUT", "/stage/artifacts/812/shares/42", `{"capabilities":["app_data_read","app_data_write"]}`},
		{"PUT", "/stage/artifacts/812/shares/42", `{"capabilities":["app_data_read"]}`},
		{"DELETE", "/stage/artifacts/812/shares/42", ``},
		{"GET", "/rooms/42/apps", ``},
	}
	if len(seen) != len(want) {
		t.Fatalf("got %d requests, want %d: %+v", len(seen), len(want), seen)
	}
	for i, w := range want {
		got := seen[i]
		if got.method != w.method || got.path != w.path {
			t.Errorf("request %d: got %s %s, want %s %s", i, got.method, got.path, w.method, w.path)
		}
		gotBody := ""
		if got.body != nil {
			b, _ := json.Marshal(got.body)
			gotBody = string(b)
		}
		if gotBody != w.body {
			t.Errorf("request %d (%s): body %s, want %s", i, w.path, gotBody, w.body)
		}
	}
}

func TestAppToolPathEscapesSegments(t *testing.T) {
	if got := appToolPath("8/12", 0, "a b"); got != "/stage/artifacts/8%2F12/tools/a%20b" {
		t.Fatalf("unescaped path: %s", got)
	}
}

func TestParseJSONArgRejectsInvalidJSON(t *testing.T) {
	tests := []struct {
		name, spec string
		ok         bool
	}{
		{"object", `{"a":1}`, true},
		{"null deletes", `null`, true},
		{"number", `3`, true},
		{"bare word", `hello`, false},
		{"empty", ``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseJSONArg("--value", tt.spec)
			if (err == nil) != tt.ok {
				t.Fatalf("parseJSONArg(%q) err = %v, want ok=%v", tt.spec, err, tt.ok)
			}
		})
	}
}

func TestRequiredRoom(t *testing.T) {
	if _, err := requiredRoom(""); err == nil {
		t.Fatal("empty room must fail")
	}
	if _, err := requiredRoom("0"); err == nil {
		t.Fatal("room 0 must fail")
	}
	if n, err := requiredRoom("42"); err != nil || n != 42 {
		t.Fatalf("requiredRoom(42) = %d, %v", n, err)
	}
}

func TestStageAppVerbsAreRegistered(t *testing.T) {
	for _, path := range [][]string{
		{"stage", "data"},
		{"stage", "data", "set"},
		{"stage", "call"},
		{"stage", "share"},
		{"stage", "unshare"},
		{"rooms", "apps"},
	} {
		cmd, _, err := rootCmd.Find(path)
		if err != nil || cmd.Name() != path[len(path)-1] {
			t.Errorf("%v not registered (found %v, err %v)", path, cmd.Name(), err)
		}
	}
}
