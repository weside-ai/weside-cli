package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

func TestReadAPICmdBody(t *testing.T) {
	// inline
	if got, err := readAPICmdBody(`{"a":1}`); err != nil || string(got) != `{"a":1}` {
		t.Errorf("inline: got %q err=%v", got, err)
	}
	// @file
	f, err := os.CreateTemp("", "api-body-*.json")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	_, _ = f.WriteString(`{"b":2}`)

	if got, err := readAPICmdBody("@" + f.Name()); err != nil || string(got) != `{"b":2}` {
		t.Errorf("@file: got %q err=%v", got, err)
	}
	// stdin "-"
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })
	stdinReader = bytes.NewBufferString(`{"c":3}`)
	if got, err := readAPICmdBody("-"); err != nil || string(got) != `{"c":3}` {
		t.Errorf("stdin: got %q err=%v", got, err)
	}
}

func TestAPICmdGetDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/x" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	resp, err := client.DoRaw(context.Background(), http.MethodGet, "/x", nil)
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"ok":true`) {
		t.Errorf("body = %q", body)
	}
}

func TestAPICmdPostBodyRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	resp, err := client.DoRaw(context.Background(), http.MethodPost, "/x", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"hello":"world"`) {
		t.Errorf("echo body = %q", body)
	}
}
