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

// Drives the REAL helpers the commands call, so the assertion is about the
// production path construction and not about a path the test typed itself.
// The mistake worth catching is exactly a wrong path: every one of these
// answers the same uniform 404 as an invalid code, so it survives a manual
// round unnoticed. Pinned: the v1 prefix (`/invites`, not `/rooms/…`), the
// code-scoped accept and preview, and that app scope sends NO room_id key —
// `"room_id": 0` would make the server resolve room 0 and 404.
func TestInviteVerbsUseTheV1Paths(t *testing.T) {
	var seen []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":"K7QP3M2X","room_id":42,"expires_at":"2026-09-18T00:00:00Z",`+
			`"room_title":"Verein","inviter_display_name":"Foxy","human_count":2,"companion_count":1,`+
			`"room":{"id":42,"title":"Verein"},"outcome":"seated"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	ctx := context.Background()
	if _, err := mintInvite(ctx, client, 0); err != nil {
		t.Fatalf("mint app: %v", err)
	}
	if _, err := mintInvite(ctx, client, 42); err != nil {
		t.Fatalf("mint room: %v", err)
	}
	if _, err := rotateInvite(ctx, client, 42); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := showInvite(ctx, client, "K7QP3M2X"); err != nil {
		t.Fatalf("show: %v", err)
	}
	accepted, err := acceptInvite(ctx, client, "K7QP3M2X")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	want := "[POST /invites POST /invites POST /invites/rotate GET /invites/K7QP3M2X/preview POST /invites/K7QP3M2X/accept]"
	if fmt.Sprint(seen) != want {
		t.Fatalf("wrong invite paths:\n got %v\nwant %v", seen, want)
	}

	var appBody map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &appBody); err != nil {
		t.Fatalf("app-scope body is not JSON: %q", bodies[0])
	}
	if _, has := appBody["room_id"]; has {
		t.Fatalf("app scope must send NO room_id key, got %q", bodies[0])
	}
	var roomBody map[string]any
	_ = json.Unmarshal([]byte(bodies[1]), &roomBody)
	if fmt.Sprintf("%v", roomBody["room_id"]) != "42" {
		t.Fatalf("room scope must send room_id 42, got %q", bodies[1])
	}
	room, _ := accepted["room"].(map[string]any)
	if fmt.Sprintf("%v", room["id"]) != "42" {
		t.Fatalf("accept did not return the joined room: %v", accepted)
	}
}

func TestInviteVerbsAreRegistered(t *testing.T) {
	for _, name := range []string{"mint", "rotate", "show", "accept"} {
		cmd, _, err := inviteCmd.Find([]string{name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("invite %s missing: cmd=%v err=%v", name, cmd, err)
		}
	}
	// The v2 subtree these verbs replace must be gone — a leftover verb would
	// drive an endpoint Phase 3 of WA-2235 deletes and answer 404 forever.
	if cmd, _, _ := roomsCmd.Find([]string{"invites"}); cmd != nil && cmd.Name() == "invites" {
		t.Fatalf("rooms invites must not exist any more")
	}
}

func TestParseRoomFlag(t *testing.T) {
	if n, err := parseRoomFlag(""); err != nil || n != 0 {
		t.Fatalf("empty must be app scope: %d %v", n, err)
	}
	if n, err := parseRoomFlag("42"); err != nil || n != 42 {
		t.Fatalf("42 must parse: %d %v", n, err)
	}
	for _, bad := range []string{"0", "-1", "x"} {
		if _, err := parseRoomFlag(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
