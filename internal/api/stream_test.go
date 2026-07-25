package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

func TestPostStreamSendsRawBody(t *testing.T) {
	var gotContent, gotAuth, gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading body: %v", err)
		}
		gotContent = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"path":"temp/seed.txt","size_bytes":9}`))
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	var result map[string]any
	err := client.PostStream(
		context.Background(),
		"/files/upload-to-temp?name=seed.txt",
		strings.NewReader("some text"),
		&result,
	)
	if err != nil {
		t.Fatalf("PostStream() error = %v", err)
	}

	// The upload endpoints stream the request body straight to storage, so the
	// body must be the file and NOTHING else — a multipart envelope would land
	// inside the stored file.
	if gotContent != "some text" {
		t.Errorf("body = %q, want exactly the file content", gotContent)
	}
	if strings.Contains(gotContent, "Content-Disposition") {
		t.Error("body carries multipart headers — it would be stored inside the file")
	}
	if gotContentType != "application/octet-stream" {
		t.Errorf("content-type = %q, want application/octet-stream", gotContentType)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("auth header = %q, want bearer token", gotAuth)
	}
	if result["path"] != "temp/seed.txt" {
		t.Errorf("decoded path = %v, want temp/seed.txt", result["path"])
	}
}

func TestPostStreamSurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte(`{"detail":"file too large"}`))
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	err := client.PostStream(
		context.Background(),
		"/files/upload-to-temp?name=big.bin",
		strings.NewReader("x"),
		nil,
	)
	if err == nil {
		t.Fatal("PostStream() error = nil, want an API error")
	}
	if !strings.Contains(err.Error(), "file too large") {
		t.Errorf("error = %q, want it to carry the server detail", err.Error())
	}
}
