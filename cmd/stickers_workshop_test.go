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

func workshopClient(t *testing.T, handler http.HandlerFunc) (client *api.Client, done func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	return api.NewClient(server.URL, "token"), server.Close
}

// An emoji is NOT the discriminating input for the escape: Go encodes a path
// segment on its way out, so a raw emoji and an escaped one produce the same
// request line. Measured — an arm that removed url.PathEscape entirely stayed
// green against an emoji. What the escape actually buys is refusing a segment
// that would otherwise change the ROUTE, so that is what this asserts.
func TestWorkshopEscapesASlashInTheSlotArgument(t *testing.T) {
	var gotPath string
	client, done := workshopClient(t, func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath, not Path: Path DECODES %2F back to a slash, so reading
		// it would show the same string whether or not we escaped.
		gotPath = r.URL.EscapedPath()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"slot": map[string]any{"emoji": "x", "shortcode": "slot-01", "state": "preview"},
		})
	})
	defer done()

	// A slot argument carrying a slash must stay ONE segment. Unescaped it
	// would address /slots/../style — a different route on the same host.
	if _, err := captureStdoutStickers(t, func() error {
		return runStickerWorkshopGenerate(context.Background(), client, "4", "a/b", "generate", 1, false)
	}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/slots/a%2Fb/generate") {
		t.Fatalf("the slash was not kept inside one segment: %q", gotPath)
	}
}

// And the ordinary case still reaches the right route with the emoji intact.
func TestWorkshopSendsTheEmojiAsOneSegment(t *testing.T) {
	var gotPath string
	client, done := workshopClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"slot":    map[string]any{"emoji": "\U0001F602", "shortcode": "slot-02", "state": "preview"},
			"sticker": map[string]any{"id": 7},
		})
	})
	defer done()

	if _, err := captureStdoutStickers(t, func() error {
		return runStickerWorkshopGenerate(context.Background(), client, "4", "\U0001F602", "generate", 1, false)
	}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/stickers/workshop/4/slots/\U0001F602/generate") {
		t.Fatalf("decoded path is wrong: %q", gotPath)
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
	client, done := workshopClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("no request should be made when nothing was passed")
	})
	defer done()

	err := runStickerWorkshopStyle(context.Background(), client, "4", "", "", false)
	if err == nil || !strings.Contains(err.Error(), "nothing to set") {
		t.Fatalf("expected a refusal, got %v", err)
	}
}
