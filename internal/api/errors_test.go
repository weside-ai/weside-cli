package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

// A 401 names the server that was called: a dev api_url with a prod token is
// otherwise indistinguishable from an expired session (WA-2694).
func TestUnauthorizedNamesAPIURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	const token = "secret-token-value"
	client := api.NewClient(srv.URL+"/api/v1", token)
	err := client.Get(context.Background(), "/companions", nil)
	if err == nil {
		t.Fatal("want an error for a 401")
	}
	msg := err.Error()
	for _, want := range []string{"auth failed against " + srv.URL, "api_url", "weside auth login"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
	if strings.Contains(msg, "/api/v1") || strings.Contains(msg, token) {
		t.Errorf("error %q must name only scheme and host, never the path or token", msg)
	}
}

func TestConnectionFailureNamesAPIURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close() // nothing listens there any more

	err := api.NewClient(base+"/api/v1", "tok").Get(context.Background(), "/companions", nil)
	if err == nil {
		t.Fatal("want an error for a closed server")
	}
	if !strings.Contains(err.Error(), "cannot reach "+base) {
		t.Errorf("error %q does not name %s", err, base)
	}
}

func TestEndpoint(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://localhost:8000/api/v1", "http://localhost:8000"},
		{"https://user:pw@api.weside.ai/api/v2?x=1", "https://api.weside.ai"},
		{"not a url", "not a url"},
	}
	for _, tt := range tests {
		if got := api.Endpoint(tt.in); got != tt.want {
			t.Errorf("Endpoint(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
