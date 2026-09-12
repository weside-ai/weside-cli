package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

// These drive the real `runStickerWorkshop*` functions against an httptest
// server. The load-bearing one is TestWorkshopEscapesTheEmojiExactlyOnce: the
// emoji is a path segment, and an unescaped one produces a request line the
// router never matches — a 404 that reads like a missing route rather than
// like a missing escape.

func workshopClient(t *testing.T, handler http.HandlerFunc) (*api.Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := api.NewClient(server.URL, "token")
	return client, server.Close
}

func TestWorkshopEscapesTheEmojiExactlyOnce(t *testing.T) {
	var gotPath, gotRaw string
	client, done := workshopClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotRaw = r.URL.EscapedPath()
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"slot":    map[string]any{"emoji": "😂", "shortcode": "slot-02", "state": "preview"},
			"sticker": map[string]any{"id": 7},
		})
	})
	defer done()

	_, err := captureStdoutStickers(t, func() error {
		return runStickerWorkshopGenerate(context.Background(), client, "4", "😂", "generate", 1, false)
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// The wire form carries the escape...
	if !strings.Contains(gotRaw, "%F0%9F%98%82") {
		t.Fatalf("emoji not percent-encoded on the wire: %q", gotRaw)
	}
	// ...and it decodes back to exactly one emoji, not a double-escaped one.
	if !strings.HasSuffix(gotPath, "/stickers/workshop/4/slots/😂/generate") {
		t.Fatalf("decoded path is wrong (double escape?): %q", gotPath)
	}
}

func TestWorkshopGenerateSendsTheAttemptAndCoupleFlag(t *testing.T) {
	var body map[string]any
	client, done := workshopClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"slot": map[string]any{"emoji": "🤗", "shortcode": "couple-01", "state": "preview"},
		})
	})
	defer done()

	if _, err := captureStdoutStickers(t, func() error {
		return runStickerWorkshopGenerate(context.Background(), client, "4", "🤗", "generate", 3, true)
	}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got := body["attempt"]; got != float64(3) {
		t.Fatalf("attempt not forwarded: %v", got)
	}
	if got := body["couple"]; got != true {
		t.Fatalf("couple not forwarded: %v", got)
	}
}

// A reroll must reach its OWN path. Posting it to /generate would make it a
// replay of the previous attempt — refused, not charged — and the story's AC5
// says a reroll is a fresh charge.
func TestWorkshopRerollPostsToTheRerollPath(t *testing.T) {
	var gotPath string
	client, done := workshopClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"slot": map[string]any{"emoji": "⭐", "shortcode": "slot-16", "state": "preview"},
		})
	})
	defer done()

	if _, err := captureStdoutStickers(t, func() error {
		return runStickerWorkshopGenerate(context.Background(), client, "4", "⭐", "reroll", 2, false)
	}); err != nil {
		t.Fatalf("reroll: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/reroll") {
		t.Fatalf("reroll went to %q", gotPath)
	}
}

func TestWorkshopDiscardCarriesTheCoupleQuery(t *testing.T) {
	var gotQuery string
	client, done := workshopClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	})
	defer done()

	if _, err := captureStdoutStickers(t, func() error {
		return runStickerWorkshopDiscard(context.Background(), client, "4", "🥂", true)
	}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if gotQuery != "couple=true" {
		t.Fatalf("couple query missing: %q", gotQuery)
	}
}

// The Workshop table is the story's oracle for AC4 and AC5, so `state` has to
// reach the output. A verb that printed only the emoji would pass every call
// and prove nothing about whether a preview was kept.
func TestWorkshopSlotsRendersTheStateColumn(t *testing.T) {
	client, done := workshopClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"companion_id":     4,
			"couple_available": false,
			"slots": []any{
				map[string]any{"emoji": "😀", "shortcode": "slot-01", "state": "kept", "sticker_id": 3},
				map[string]any{"emoji": "😂", "shortcode": "slot-02", "state": "empty", "sticker_id": nil},
			},
		})
	})
	defer done()

	out, err := captureStdoutStickers(t, func() error {
		return runStickerWorkshopSlots(context.Background(), client, "4")
	})
	if err != nil {
		t.Fatalf("slots: %v", err)
	}
	for _, want := range []string{"slot-01", "kept", "slot-02", "empty"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "set your own picture first") {
		t.Fatalf("the absent-couple line is missing:\n%s", out)
	}
}

func TestWorkshopStyleRefusesAnEmptyChange(t *testing.T) {
	client, done := workshopClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("no request should be made when nothing was passed")
	})
	defer done()

	err := runStickerWorkshopStyle(context.Background(), client, "4", "", "", false)
	if err == nil || !strings.Contains(err.Error(), "nothing to set") {
		t.Fatalf("expected a refusal, got %v", err)
	}
}
