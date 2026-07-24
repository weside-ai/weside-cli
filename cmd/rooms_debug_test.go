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
)

func TestParseIntCSV(t *testing.T) {
	tests := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{"1,2,3", []int{1, 2, 3}, false},
		{" 42 ", []int{42}, false},
		{"", nil, true},
		{"1,x,3", nil, true},
	}
	for _, tt := range tests {
		got, err := parseIntCSV(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseIntCSV(%q) err=%v wantErr=%v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && !equalInts(got, tt.want) {
			t.Errorf("parseIntCSV(%q) = %v want %v", tt.in, got, tt.want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParticipantID(t *testing.T) {
	tests := []struct {
		name string
		p    map[string]any
		want string
	}{
		{"user", map[string]any{"kind": "user", "user_id": float64(7)}, "7"},
		{"companion", map[string]any{"kind": "companion", "companion_id": float64(101)}, "101"},
		{"external", map[string]any{"kind": "external", "external_id": float64(3)}, "3"},
		{"none", map[string]any{"kind": "user"}, "-"},
	}
	for _, tt := range tests {
		if got := participantID(tt.p); got != tt.want {
			t.Errorf("%s: participantID = %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestPrettyJSON(t *testing.T) {
	if prettyJSON(nil) != "" {
		t.Error("nil should render empty")
	}
	if got := prettyJSON(map[string]any{"a": 1}); got != `{"a":1}` {
		t.Errorf("obj = %q", got)
	}
}

func captureRoomsDebug(t *testing.T, fn func() error) string {
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
		t.Fatalf("error: %v", runErr)
	}
	return string(out)
}

func TestRoomsTracePrintsToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"room_id":1,"messages":[`+
			`{"message_id":"m1","role":"assistant","created_at":"2026-07-24T10:00:00+00:00",`+
			`"trace_items":[{"type":"tool_call","tool_name":"search","tool_args":{"q":"hi"},"tool_output":"ok"},`+
			`{"type":"reminder","category":"follow_up","message":"ping"}]}]}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureRoomsDebug(t, func() error {
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/1/trace?limit=50", &result); err != nil {
			return err
		}
		messages, _ := result["messages"].([]any)
		for _, item := range messages {
			m, _ := item.(map[string]any)
			fmt.Printf("[%s] %s\n", roleLabel(fmt.Sprintf("%v", m["role"])), m["created_at"])
			printTraceItems(m["trace_items"])
		}
		return nil
	})

	if !strings.Contains(out, "[Companion]") {
		t.Errorf("expected Companion label, got %q", out)
	}
	if !strings.Contains(out, "tool_call search") {
		t.Errorf("expected tool_call line, got %q", out)
	}
	if !strings.Contains(out, "reminder [follow_up]: ping") {
		t.Errorf("expected reminder line, got %q", out)
	}
}

func TestRoomsDmCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/dm/101") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":42,"kind":"dm","title":"Nox"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out := captureRoomsDebug(t, func() error {
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/dm/101", nil, &result); err != nil {
			return err
		}
		fmt.Printf("DM room with companion 101 (ID: %v).\n", result["id"])
		return nil
	})

	if !strings.Contains(out, "ID: 42") {
		t.Errorf("expected room id 42, got %q", out)
	}
}

func TestRoomsGroupCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/rooms/group") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"companion_ids":[1,2]`) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id":9,"kind":"group"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	ids, err := parseIntCSV("1,2")
	if err != nil {
		t.Fatalf("parseIntCSV: %v", err)
	}
	body := map[string]any{"companion_ids": ids}
	var result map[string]any
	if err := client.Post(context.Background(), "/rooms/group", body, &result); err != nil {
		t.Fatalf("post: %v", err)
	}
	if fmt.Sprintf("%v", result["id"]) != "9" {
		t.Errorf("expected group room id 9, got %v", result["id"])
	}
}

func TestRoomsCancelRequiresConfirm(t *testing.T) {
	// No server: the --confirm gate must fail before any request.
	if err := roomsCancelCmd.RunE(roomsCancelCmd, []string{"1"}); err == nil {
		t.Error("expected --confirm gate to block cancel without --confirm")
	}
	if err := roomsUndoCmd.RunE(roomsUndoCmd, []string{"1"}); err == nil {
		t.Error("expected --confirm gate to block undo without --confirm")
	}
	if err := roomsContextBreakCmd.RunE(roomsContextBreakCmd, []string{"1"}); err == nil {
		t.Error("expected --confirm gate to block context-break without --confirm")
	}
}
