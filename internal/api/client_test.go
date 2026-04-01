package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestAPIErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  api.APIError
		want string
	}{
		{
			name: "with detail",
			err:  api.APIError{StatusCode: 404, Detail: "not found"},
			want: "API error 404: not found",
		},
		{
			name: "with message",
			err:  api.APIError{StatusCode: 500, Message: "server error"},
			want: "API error 500: server error",
		},
		{
			name: "with status only",
			err:  api.APIError{StatusCode: 403, Status: "403 Forbidden"},
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
