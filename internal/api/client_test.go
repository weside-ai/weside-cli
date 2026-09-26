package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

func TestClientGet(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantErr    bool
	}{
		{
			name:       "successful GET",
			statusCode: http.StatusOK,
			response:   `{"id": 1, "name": "test"}`,
			wantErr:    false,
		},
		{
			name:       "404 error",
			statusCode: http.StatusNotFound,
			response:   `{"detail": "not found"}`,
			wantErr:    true,
		},
		{
			name:       "500 error",
			statusCode: http.StatusInternalServerError,
			response:   `{"detail": "internal error"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := api.NewClient(server.URL, "test-token")
			var result map[string]any
			err := client.Get(context.Background(), "/test", &result)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestClientSetsAuthHeader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "my-jwt-token")
	_ = client.Get(context.Background(), "/test", nil)

	want := "Bearer my-jwt-token"
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

func TestClientPost(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "token")
	body := map[string]string{"name": "test"}
	var result map[string]any
	err := client.Post(context.Background(), "/test", body, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["name"] != "test" {
		t.Errorf("request body name = %q, want %q", gotBody["name"], "test")
	}
}

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  api.Error
		want string
	}{
		{
			name: "with detail",
			err:  api.Error{StatusCode: 404, Detail: "not found"},
			want: "API error 404: not found",
		},
		{
			name: "with message",
			err:  api.Error{StatusCode: 500, Message: "server error"},
			want: "API error 500: server error",
		},
		{
			name: "with status only",
			err:  api.Error{StatusCode: 403, Status: "403 Forbidden"},
			want: "API error 403: 403 Forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorMessage_MFASessionConfirmationRequired(t *testing.T) {
	for _, slug := range []string{"mfa-session-confirmation-required", "mfa-session-binding-required"} {
		t.Run(slug, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"type":"https://api.weside.ai/errors/` + slug + `","title":"x","status":403,` +
					`"detail":"Confirm this sign-in once in the weside app to use it.",` +
					`"confirm_url":"https://mobile.weside.ai/oauth/confirm?session=s1","bind_url":"https://mobile.weside.ai/oauth/bind?session=s1"}`))
			}))
			defer srv.Close()

			err := api.NewClient(srv.URL, "tok").Get(context.Background(), "/auth/me", nil)
			if err == nil {
				t.Fatal("want an error")
			}
			msg := err.Error()
			if !strings.Contains(msg, "weside auth login") {
				t.Errorf("message %q does not tell the user to log in again", msg)
			}
			if strings.Contains(msg, "http") || strings.Contains(msg, "weside app") {
				t.Errorf("message %q carries the URL or the app instruction", msg)
			}
		})
	}
}

func TestErrorMessage_OtherForbiddenKeepsDetail(t *testing.T) {
	e := api.Error{StatusCode: 403, Type: "https://api.weside.ai/errors/mfa-required", Detail: "nope"}
	if got := e.Error(); got != "API error 403: nope" {
		t.Errorf("Error() = %q", got)
	}
}
