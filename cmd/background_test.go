package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const backgroundBody = `{"generated_at":"2026-10-07T10:00:00Z","items":[` +
	`{"id":"reminder:41","source":"reminder","kind":"reminder","companion_id":7,"status":"pending",` +
	`"cadence":{"type":"once"},"next_fire":{"at":"2026-10-08T09:00:00+02:00","basis":"stored"},` +
	`"last_outcome":null,"created_by":{"actor":"companion","ref":"companion:7"},` +
	`"allowed_actions":{"owner":["cancel"],"companion":["cancel"]},"opaque":false,"parent_ref":null},` +
	`{"id":"ambient:7","source":"ambient","kind":"ambient","companion_id":7,"status":"disabled",` +
	`"cadence":{"type":"event"},"next_fire":null,"last_outcome":null,` +
	`"created_by":{"actor":"platform","ref":"job:attention_consumer"},` +
	`"allowed_actions":{"owner":[],"companion":[]},"opaque":false,"parent_ref":null}]}`

// TestBackgroundListReadsOneCompanionsRegister: the name resolves through the
// list, the register is read from /companions/{id}/background, one row per item.
func TestBackgroundListReadsOneCompanionsRegister(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/companions":
			_, _ = w.Write([]byte(companionListResponse("7", "Nox")))
		case "/api/v1/companions/7/background":
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(backgroundBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	humanOutput(t, srv.URL)
	backgroundAll = false

	out := captureStdout(t, func() error {
		return backgroundListCmd.RunE(backgroundListCmd, []string{"Nox"})
	})

	if gotPath != "/api/v1/companions/7/background" {
		t.Fatalf("path = %q", gotPath)
	}
	for _, want := range []string{"reminder:41", "ambient:7", "2026-10-08 09:00 +02:00 (stored)", "companion", "cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// TestBackgroundListAllReadsTheOwnersRegister: --all asks /me/background.
func TestBackgroundListAllReadsTheOwnersRegister(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(backgroundBody))
	}))
	defer srv.Close()
	humanOutput(t, srv.URL)
	backgroundAll = true
	t.Cleanup(func() { backgroundAll = false })

	captureStdout(t, func() error {
		return backgroundListCmd.RunE(backgroundListCmd, nil)
	})

	if gotPath != "/api/v1/me/background" {
		t.Fatalf("path = %q, want /api/v1/me/background", gotPath)
	}
}

// TestBackgroundListNeedsACompanionOrAll refuses before any request.
func TestBackgroundListNeedsACompanionOrAll(t *testing.T) {
	backgroundAll = false
	err := backgroundListCmd.RunE(backgroundListCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--all") {
		t.Fatalf("err = %v, want the companion-or---all refusal", err)
	}
}

// TestBackgroundCancelPostsAndSaysWhatHappened: an occurrence is gone for
// good, a routine is switched off.
func TestBackgroundCancelPostsAndSaysWhatHappened(t *testing.T) {
	for _, tc := range []struct {
		item, effect, want string
	}{
		{"reminder:41", "occurrence", "it will not run"},
		{"dreaming:7", "definition_off", "Switched dreaming:7 off"},
	} {
		var gotPath, gotMethod string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotMethod = r.URL.Path, r.Method
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"` + tc.item + `","effect":"` + tc.effect + `"}`))
		}))
		humanOutput(t, srv.URL)

		out := captureStdout(t, func() error {
			return backgroundCancelCmd.RunE(backgroundCancelCmd, []string{"7", tc.item})
		})
		srv.Close()

		if gotMethod != http.MethodPost || gotPath != "/api/v1/companions/7/background/"+tc.item+"/cancel" {
			t.Fatalf("request = %s %s", gotMethod, gotPath)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("output lacks %q:\n%s", tc.want, out)
		}
	}
}

// TestBackgroundCancelSurfacesARefusal: a 409 (running) is an error, wrapped.
func TestBackgroundCancelSurfacesARefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail":"This item is running right now."}`))
	}))
	defer srv.Close()
	humanOutput(t, srv.URL)

	err := backgroundCancelCmd.RunE(backgroundCancelCmd, []string{"7", "reminder:41"})

	if err == nil || !strings.Contains(err.Error(), "cancelling reminder:41") {
		t.Fatalf("err = %v, want a wrapped 409", err)
	}
}
