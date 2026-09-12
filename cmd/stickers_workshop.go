package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// Sticker Workshop verbs (WA-2218). The Workshop is one screen per companion
// with sixteen emoji slots; these verbs are the CLI oracle for its story's
// `## Verification` block, which asserts the Gefallen a slot costs and the
// state a slot is in. Without them the only oracle is raw `weside api --v2`,
// and a verification that has to hand-assemble its own requests is one nobody
// re-runs.
//
// Two things shape this file.
//
// The emoji is a PATH SEGMENT, so every verb percent-encodes it exactly once.
// `url.PathEscape` is the encoder — `url.QueryEscape` would turn a space into
// `+` and is wrong for a path, and an emoji handed through unescaped produces
// a request line the router will not match.
//
// A slot's price belongs in the output. `generate` and `reroll` each cost one
// Gefallen and `reroll` is a FRESH charge, never a free retry — the story's
// AC5. The verbs therefore print what was spent rather than a bare "ok", so a
// reading of `weside me usage --json` before and after has something to be
// compared against.

const stickerWorkshopBasePath = "/stickers/workshop"

var stickerWorkshopCmd = &cobra.Command{
	Use:   "workshop",
	Short: "The Sticker Workshop: slots, generation, keep/discard, style",
}

// slotPath builds `/stickers/workshop/{companion}/slots/{emoji}` with the
// emoji escaped for a path segment. Kept as one function so no verb can
// forget the escape: a missed one is a 404 that reads like a missing route.
func slotPath(companionID, emoji, suffix string) string {
	path := fmt.Sprintf("%s/%s/slots/%s", stickerWorkshopBasePath, companionID, url.PathEscape(emoji))
	if suffix != "" {
		path += "/" + suffix
	}
	return path
}

// --- slots ---------------------------------------------------------------

var stickerWorkshopSlotsCmd = &cobra.Command{
	Use:   "slots <companion-id>",
	Short: "Show a companion's Workshop: every slot and its state",
	Long: `Show the sixteen emoji slots of a companion's Workshop, plus the
couple slots when your own picture is set.

Example:
  weside stickers workshop slots 4 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerWorkshopSlots(cmd.Context(), client, args[0])
	},
}

func runStickerWorkshopSlots(ctx context.Context, client *api.Client, companionID string) error {
	var result map[string]any
	path := stickerWorkshopBasePath + "/" + companionID
	if err := client.Get(ctx, path, &result); err != nil {
		return fmt.Errorf("reading the workshop: %w", err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}

	printSlotTable("YOUR SET", result["slots"])
	if available, _ := result["couple_available"].(bool); available {
		printSlotTable("TOGETHER", result["couple_slots"])
	} else {
		fmt.Println("\nTogether: not offered — set your own picture first.")
	}
	return nil
}

// printSlotTable renders one slot group. `state` is the column the story's
// AC4 and AC5 are read from: `empty`, `preview` (generated, not yet kept) and
// `kept` (exportable, sendable).
func printSlotTable(title string, slots any) {
	items, _ := slots.([]any)
	if len(items) == 0 {
		return
	}
	fmt.Printf("\n%s\n", title)
	headers := []string{"EMOJI", "SHORTCODE", "STATE", "STICKER"}
	var rows [][]string
	for _, item := range items {
		s, _ := item.(map[string]any)
		sticker := "—"
		if s["sticker_id"] != nil {
			sticker = fmt.Sprintf("%v", s["sticker_id"])
		}
		rows = append(rows, []string{
			fmt.Sprintf("%v", s["emoji"]),
			fmt.Sprintf("%v", s["shortcode"]),
			fmt.Sprintf("%v", s["state"]),
			sticker,
		})
	}
	ui.PrintTable(headers, rows)
}

// --- generate / reroll ----------------------------------------------------

var (
	stickerWorkshopAttempt int
	stickerWorkshopCouple  bool
)

var stickerWorkshopGenerateCmd = &cobra.Command{
	Use:   "generate <companion-id> <emoji>",
	Short: "Let the companion paint one slot (1 Gefallen)",
	Long: `Generate the sticker for one emoji slot. Costs one Gefallen, which
is spent before the image is asked for and given back if it never arrives.

--attempt is the idempotency seed: the same attempt on the same slot is a
replay and is refused, never charged twice. Raise it to ask again.

Example:
  weside stickers workshop generate 4 😂 --attempt 1`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerWorkshopGenerate(
			cmd.Context(), client, args[0], args[1], "generate",
			stickerWorkshopAttempt, stickerWorkshopCouple,
		)
	},
}

var stickerWorkshopRerollCmd = &cobra.Command{
	Use:   "reroll <companion-id> <emoji>",
	Short: "Ask again for one slot — a fresh Gefallen, never a free retry",
	Long: `Discard what a slot holds and ask for a new image. This is a FRESH
charge of one Gefallen (WA-2218 AC5), not a retry of the previous one.

Example:
  weside stickers workshop reroll 4 😂 --attempt 2`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerWorkshopGenerate(
			cmd.Context(), client, args[0], args[1], "reroll",
			stickerWorkshopAttempt, stickerWorkshopCouple,
		)
	},
}

func runStickerWorkshopGenerate(
	ctx context.Context, client *api.Client,
	companionID, emoji, verb string, attempt int, couple bool,
) error {
	if attempt < 1 {
		attempt = 1
	}
	body := map[string]any{"attempt": attempt, "couple": couple}

	var result map[string]any
	if err := client.Post(ctx, slotPath(companionID, emoji, verb), body, &result); err != nil {
		return fmt.Errorf("%s %s: %w", verb, emoji, err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}
	printSlotResult(result, "1 Gefallen spent")
	return nil
}

// --- keep / discard -------------------------------------------------------

var stickerWorkshopKeepCmd = &cobra.Command{
	Use:   "keep <companion-id> <emoji>",
	Short: "Keep the preview in a slot — only kept stickers export and send",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerWorkshopKeep(cmd.Context(), client, args[0], args[1], stickerWorkshopCouple)
	},
}

func runStickerWorkshopKeep(
	ctx context.Context, client *api.Client, companionID, emoji string, couple bool,
) error {
	path := slotPath(companionID, emoji, "keep") + coupleQuery(couple)
	var result map[string]any
	if err := client.Post(ctx, path, nil, &result); err != nil {
		return fmt.Errorf("keeping %s: %w", emoji, err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}
	printSlotResult(result, "kept — now sendable and exportable")
	return nil
}

var stickerWorkshopDiscardCmd = &cobra.Command{
	Use:   "discard <companion-id> <emoji>",
	Short: "Throw away what a slot holds; the Gefallen is not given back",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerWorkshopDiscard(
			cmd.Context(), client, args[0], args[1], stickerWorkshopCouple,
		)
	},
}

func runStickerWorkshopDiscard(
	ctx context.Context, client *api.Client, companionID, emoji string, couple bool,
) error {
	path := slotPath(companionID, emoji, "") + coupleQuery(couple)
	if err := client.Delete(ctx, path, nil); err != nil {
		return fmt.Errorf("discarding %s: %w", emoji, err)
	}
	if IsJSON() {
		ui.PrintJSON(map[string]any{"discarded": emoji})
		return nil
	}
	fmt.Printf("Slot %s is empty again.\n", emoji)
	return nil
}

// coupleQuery is a function rather than an inline ternary because three verbs
// need the same encoding and a `?couple=true` typed twice is a bug waiting for
// its third caller.
func coupleQuery(couple bool) string {
	if !couple {
		return ""
	}
	return "?couple=true"
}

// --- style ----------------------------------------------------------------

var (
	stickerWorkshopStyleAnchor string
	stickerWorkshopStyleFile   string
)

var stickerWorkshopStyleCmd = &cobra.Command{
	Use:   "style <companion-id>",
	Short: "Set the style anchor: the words, and optionally a reference picture",
	Long: `The style anchor is what makes a set look like a set. It is the
companion's, not the platform's — the platform contributes only the sticker
shape (thick white outline, transparent background, single subject).

Example:
  weside stickers workshop style 4 \
    --anchor "cute kitten style of a snow leopard in deep snow" \
    --file ./reference.png`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerWorkshopStyle(
			cmd.Context(), client, args[0],
			stickerWorkshopStyleAnchor, stickerWorkshopStyleFile, stickerWorkshopCouple,
		)
	},
}

func runStickerWorkshopStyle(
	ctx context.Context, client *api.Client,
	companionID, anchor, file string, couple bool,
) error {
	if anchor == "" && file == "" {
		return fmt.Errorf("nothing to set: pass --anchor, --file, or both")
	}
	path := fmt.Sprintf("%s/%s/style", stickerWorkshopBasePath, companionID)
	fields := map[string]string{"couple": fmt.Sprintf("%t", couple)}
	if anchor != "" {
		fields["style_anchor"] = anchor
	}

	var result map[string]any
	var err error
	if file != "" {
		var data []byte
		data, err = os.ReadFile(file) //nolint:gosec // a path the operator typed
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}
		err = client.PostMultipart(ctx, path, filepath.Base(file), data, fields, &result)
	} else {
		err = client.PostMultipart(ctx, path, "", nil, fields, &result)
	}
	if err != nil {
		return fmt.Errorf("setting the style: %w", err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}
	fmt.Printf("Style set for pack %v (%v).\n", result["id"], result["slug"])
	if a, ok := result["style_anchor"].(string); ok && a != "" {
		fmt.Printf("  anchor: %s\n", truncate(a, 72))
	}
	return nil
}

// --- shared rendering -----------------------------------------------------

// printSlotResult renders the answer of generate/reroll/keep, which all return
// the same `{pack, slot, sticker}` shape. The `note` is what the caller did,
// said in the user's terms — the API says `201`, the user wants to know what
// it cost.
func printSlotResult(result map[string]any, note string) {
	slot, _ := result["slot"].(map[string]any)
	sticker, _ := result["sticker"].(map[string]any)
	parts := []string{}
	if slot != nil {
		parts = append(parts, fmt.Sprintf("slot %v (%v)", slot["emoji"], slot["shortcode"]))
		parts = append(parts, fmt.Sprintf("state %v", slot["state"]))
	}
	if sticker != nil {
		parts = append(parts, fmt.Sprintf("sticker %v", sticker["id"]))
	}
	fmt.Printf("%s — %s\n", strings.Join(parts, " · "), note)
}

func init() {
	stickerWorkshopGenerateCmd.Flags().IntVar(&stickerWorkshopAttempt, "attempt", 1,
		"Idempotency seed; the same attempt on the same slot is a refused replay")
	stickerWorkshopRerollCmd.Flags().IntVar(&stickerWorkshopAttempt, "attempt", 1,
		"Idempotency seed; raise it to ask again")
	stickerWorkshopStyleCmd.Flags().StringVar(&stickerWorkshopStyleAnchor, "anchor", "",
		"The style anchor text")
	stickerWorkshopStyleCmd.Flags().StringVar(&stickerWorkshopStyleFile, "file", "",
		"A reference picture for the style")
	for _, c := range []*cobra.Command{
		stickerWorkshopGenerateCmd, stickerWorkshopRerollCmd,
		stickerWorkshopKeepCmd, stickerWorkshopDiscardCmd, stickerWorkshopStyleCmd,
	} {
		c.Flags().BoolVar(&stickerWorkshopCouple, "couple", false,
			"Address the couple pack rather than the solo one")
	}

	stickerWorkshopCmd.AddCommand(stickerWorkshopSlotsCmd)
	stickerWorkshopCmd.AddCommand(stickerWorkshopGenerateCmd)
	stickerWorkshopCmd.AddCommand(stickerWorkshopRerollCmd)
	stickerWorkshopCmd.AddCommand(stickerWorkshopKeepCmd)
	stickerWorkshopCmd.AddCommand(stickerWorkshopDiscardCmd)
	stickerWorkshopCmd.AddCommand(stickerWorkshopStyleCmd)
	stickersCmd.AddCommand(stickerWorkshopCmd)
}
