package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

func captureStdoutRooms(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("command error: %v", runErr)
	}
	return string(out)
}

func TestRoomsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rooms" || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"rooms":[`+
			`{"id":1,"kind":"dm","title":"Nox","last_message":{"snippet":"hello"},"updated_at":"2026-07-24T10:00:00+00:00"},`+
			`{"id":2,"kind":"group","auto_title":"Team","updated_at":"2026-07-24T11:00:00+00:00"}`+
			`],"total":2}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdoutRooms(t, func() error {
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms", &result); err != nil {
			return err
		}
		rooms, _ := result["rooms"].([]any)
		headers := []string{"ID", "KIND", "TITLE", "LAST MESSAGE", "UPDATED"}
		var rows [][]string
		for _, item := range rooms {
			r, _ := item.(map[string]any)
			id := fmt.Sprintf("%v", r["id"])
			kind := fmt.Sprintf("%v", r["kind"])
			title := fmt.Sprintf("%v", r["title"])
			if title == "<nil>" || title == "" {
				title = fmt.Sprintf("%v", r["auto_title"])
			}
			lastMsg := ""
			if lm, ok := r["last_message"].(map[string]any); ok {
				lastMsg = truncate(fmt.Sprintf("%v", lm["snippet"]), 40)
			}
			updated := fmt.Sprintf("%v", r["updated_at"])
			rows = append(rows, []string{id, kind, truncate(title, 30), lastMsg, updated})
		}
		ui.PrintTable(headers, rows)
		fmt.Printf("\n%v room(s)\n", result["total"])
		return nil
	})

	if !strings.Contains(out, "Nox") || !strings.Contains(out, "Team") {
		t.Errorf("expected both room titles in table, got %q", out)
	}
}

func TestRoomsShowRoleLabels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages") || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"messages":[`+
			`{"role":"user","content":[{"type":"text","text":"hi"}]},`+
			`{"role":"assistant","content":[{"type":"text","text":"hello back"}]}`+
			`],"next_cursor":null,"prev_cursor":null}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureStdoutRooms(t, func() error {
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/1/messages", &result); err != nil {
			return err
		}
		messages, _ := result["messages"].([]any)
		for _, item := range messages {
			msg, _ := item.(map[string]any)
			role := fmt.Sprintf("%v", msg["role"])
			prefix := roleLabel(role)
			if content, ok := msg["content"].([]any); ok {
				for _, block := range content {
					if b, ok := block.(map[string]any); ok {
						if text, ok := b["text"].(string); ok {
							fmt.Printf("[%s] %s\n\n", prefix, text)
						}
					}
				}
			}
		}
		return nil
	})

	if !strings.Contains(out, "[You] hi") {
		t.Errorf("expected user message labelled [You], got %q", out)
	}
	if !strings.Contains(out, "[Companion] hello back") {
		t.Errorf("expected assistant reply labelled [Companion], got %q", out)
	}
}
