package cmd

import "testing"

// The v2 timeline names a message's server id `id` — measured against the
// running backend, not assumed. The first version of this test used
// `message_id`, a key that payload does not carry, so the CLI printed the
// literal "<nil>" while the test was green.
//
// The sentinel is a wire format the app and this CLI parse independently — a
// drift between the two regexes is invisible until a card fails to render, so
// the shapes that actually occur are pinned here.
func TestParseConfirmationSentinels(t *testing.T) {
	timeline := map[string]any{"messages": []any{
		map[string]any{
			"id": "m1",
			"content": []any{map[string]any{
				"text": `<action-confirmation id=18 weight=3 remaining=7 tool="GEMINI_GENERATE_IMAGE" />` +
					"\nDiese Aktion würde 3 Gefallen kosten.",
			}},
		},
		map[string]any{
			"id":      "m2",
			"content": []any{map[string]any{"text": "just a normal reply"}},
		},
		map[string]any{
			"id": "m3",
			"content": []any{map[string]any{
				"text": `<action-confirmation id=19 weight=10 remaining=unlimited tool="arena_run" />`,
			}},
		},
	}}

	// A payload with no id member must answer "", never "<nil>".
	idless := parseConfirmationSentinels(map[string]any{"messages": []any{
		map[string]any{"content": []any{map[string]any{
			"text": `<action-confirmation id=20 weight=1 remaining=4 tool="X" />`,
		}}},
	}})
	if len(idless) != 1 || idless[0].MessageID != "" {
		t.Errorf("an id-less message must leave MessageID empty: %+v", idless)
	}

	found := parseConfirmationSentinels(timeline)
	if len(found) != 2 {
		t.Fatalf("expected 2 asks, got %d", len(found))
	}
	if found[0].ID != 18 || found[0].Weight != 3 || found[0].Remaining != "7" {
		t.Errorf("first ask parsed wrong: %+v", found[0])
	}
	if found[0].Tool != "GEMINI_GENERATE_IMAGE" || found[0].MessageID != "m1" {
		t.Errorf("first ask lost tool/message: %+v", found[0])
	}
	// `unlimited` is not a number and must survive as the literal — a plan with
	// no monthly ceiling reports no finite remainder.
	if found[1].Remaining != "unlimited" || found[1].ID != 19 {
		t.Errorf("unlimited ask parsed wrong: %+v", found[1])
	}
}

func TestParseConfirmationSentinelsEmptyTimeline(t *testing.T) {
	if got := parseConfirmationSentinels(map[string]any{}); got != nil {
		t.Errorf("expected nil for an empty payload, got %+v", got)
	}
}
