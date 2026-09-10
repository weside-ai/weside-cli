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

// Sticker packs (WA-2216). These verbs wrap the /api/v2/stickers surface so
// the story's `## Verification` block has an oracle other than raw
// `weside api` calls — verification.md treats a missing verb as a bug in the
// CLI, not as a reason to verify by hand.
//
// The one rule that shapes this file: `publish` MUST surface the 409. An
// `uploaded` pack cannot leave `private` until its rights are confirmed, and
// the whole point of the verb is to make that refusal visible. A verb that
// swallowed the error — printing "published" over a body that says otherwise —
// would be worse than absent, because it would read as a passing check.
// `runStickerPublish` therefore returns the API error unchanged and prints
// nothing on failure, and `TestStickerPublishSurfacesThe409` is what keeps it
// that way.
//
// `export --telegram` and `send` call endpoints that Phase 4 of WA-2216 lands.
// They are written here, in the same wave, because the verbs are one surface
// and a half-built command group is a worse handover than a complete one that
// reports the backend's own 404 until the routes exist.

const stickersBasePath = "/stickers"

var stickersCmd = &cobra.Command{
	Use:   "stickers",
	Short: "Sticker packs: list, upload, publish, export and send",
}

var stickerPacksCmd = &cobra.Command{
	Use:   "packs",
	Short: "Work with sticker packs",
}

// --- list ---------------------------------------------------------------

var stickerPacksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your sticker packs plus every public one",
	Long: `List the sticker packs you can see: your own, plus packs other
people published.

Example:
  weside stickers packs list --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerPacksList(cmd.Context(), client, stickerListLimit, stickerListCursor)
	},
}

var (
	stickerListLimit  int
	stickerListCursor string
)

func runStickerPacksList(
	ctx context.Context, client *api.Client, limit int, cursor string,
) error {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	path := stickersBasePath + "/packs"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var result map[string]any
	if err := client.Get(ctx, path, &result); err != nil {
		return fmt.Errorf("listing sticker packs: %w", err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}

	items, _ := result["items"].([]any)
	headers := []string{"ID", "SLUG", "TITLE", "SOURCE", "VISIBILITY", "RIGHTS"}
	var rows [][]string
	for _, item := range items {
		p, _ := item.(map[string]any)
		rows = append(rows, []string{
			fmt.Sprintf("%v", p["id"]),
			truncate(fmt.Sprintf("%v", p["slug"]), 24),
			truncate(fmt.Sprintf("%v", p["title"]), 28),
			fmt.Sprintf("%v", p["source_kind"]),
			fmt.Sprintf("%v", p["visibility"]),
			rightsLabel(p["rights_confirmed_at"]),
		})
	}
	ui.PrintTable(headers, rows)
	if next, ok := result["next_cursor"].(string); ok && next != "" {
		fmt.Printf("\nMore packs: weside stickers packs list --cursor %s\n", next)
	}
	return nil
}

// rightsLabel says whether the rights claim exists, never when it was made.
// A timestamp in a list column is noise; whether the pack can be published is
// the fact the column is for.
func rightsLabel(value any) string {
	if value == nil {
		return "unconfirmed"
	}
	if s, ok := value.(string); ok && s == "" {
		return "unconfirmed"
	}
	return "confirmed"
}

// --- show ---------------------------------------------------------------

var stickerPacksShowCmd = &cobra.Command{
	Use:   "show <pack-id>",
	Short: "Show one sticker pack and its stickers",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerPacksShow(cmd.Context(), client, args[0])
	},
}

func runStickerPacksShow(ctx context.Context, client *api.Client, packID string) error {
	var result map[string]any
	path := stickersBasePath + "/packs/" + url.PathEscape(packID)
	if err := client.Get(ctx, path, &result); err != nil {
		return fmt.Errorf("reading sticker pack %s: %w", packID, err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}

	fmt.Printf("%v  (%v)\n", result["title"], result["slug"])
	fmt.Printf("source: %v   visibility: %v   rights: %s\n",
		result["source_kind"], result["visibility"], rightsLabel(result["rights_confirmed_at"]))
	if set, ok := result["telegram_set_name"].(string); ok && set != "" {
		fmt.Printf("telegram set: https://t.me/addstickers/%s\n", set)
	}
	fmt.Println()

	stickers, _ := result["stickers"].([]any)
	headers := []string{"SHORTCODE", "FORMAT", "EMOJI"}
	var rows [][]string
	for _, item := range stickers {
		s, _ := item.(map[string]any)
		emoji, _ := s["emoji"].([]any)
		var parts []string
		for _, e := range emoji {
			parts = append(parts, fmt.Sprintf("%v", e))
		}
		rows = append(rows, []string{
			fmt.Sprintf("%v", s["shortcode"]),
			fmt.Sprintf("%v", s["format"]),
			strings.Join(parts, " "),
		})
	}
	ui.PrintTable(headers, rows)
	return nil
}

// --- upload -------------------------------------------------------------

var stickerUploadConfirmRights bool

var stickerPacksUploadCmd = &cobra.Command{
	Use:   "upload <file.wsp>",
	Short: "Upload a .wsp sticker pack",
	Long: `Upload a .wsp sticker pack. It lands private and unconfirmed unless
you pass --confirm-rights.

Pass --confirm-rights only if you actually hold the rights to the artwork:
it is the claim that lets the pack be published later.

Example:
  weside stickers packs upload nox-alltag.wsp
  weside stickers packs upload nox-alltag.wsp --confirm-rights --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerPacksUpload(cmd.Context(), client, args[0], stickerUploadConfirmRights)
	},
}

func runStickerPacksUpload(
	ctx context.Context, client *api.Client, path string, confirmRights bool,
) error {
	data, err := os.ReadFile(path) // #nosec G304 - a path the user typed
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	fields := map[string]string{"rights_confirmed": "false"}
	if confirmRights {
		fields["rights_confirmed"] = "true"
	}

	var result map[string]any
	if err := client.PostMultipart(
		ctx, stickersBasePath+"/packs", filepath.Base(path), data, fields, &result,
	); err != nil {
		return fmt.Errorf("uploading %s: %w", filepath.Base(path), err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}
	fmt.Printf("Uploaded pack %v (%v) — %v, rights %s\n",
		result["id"], result["slug"], result["visibility"],
		rightsLabel(result["rights_confirmed_at"]))
	return nil
}

// --- publish ------------------------------------------------------------

var stickerPublishConfirmRights bool

var stickerPacksPublishCmd = &cobra.Command{
	Use:   "publish <pack-id>",
	Short: "Make a pack public",
	Long: `Make a sticker pack public.

An uploaded pack whose rights are not confirmed CANNOT be published: the API
refuses with 409 sticker-rights-unconfirmed and this command reports that
refusal. Add --confirm-rights to claim the rights in the same call.

Example:
  weside stickers packs publish 12
  weside stickers packs publish 12 --confirm-rights`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerPacksPublish(cmd.Context(), client, args[0], stickerPublishConfirmRights)
	},
}

// runStickerPacksPublish sends the visibility change and returns the API's
// error UNCHANGED.
//
// The 409 is the point. Do not wrap it into a success message, do not retry it,
// and do not print anything before the call returns: the verb's whole value in
// the WA-2216 verification block is that an unconfirmed pack makes it go red.
func runStickerPacksPublish(
	ctx context.Context, client *api.Client, packID string, confirmRights bool,
) error {
	body := map[string]any{"visibility": "public"}
	if confirmRights {
		body["rights_confirmed"] = true
	}
	var result map[string]any
	path := stickersBasePath + "/packs/" + url.PathEscape(packID)
	if err := client.Patch(ctx, path, body, &result); err != nil {
		return fmt.Errorf("publishing pack %s: %w", packID, err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}
	fmt.Printf("Pack %v is now %v\n", result["id"], result["visibility"])
	return nil
}

// --- export -------------------------------------------------------------

var stickerExportTelegram bool

var stickerPacksExportCmd = &cobra.Command{
	Use:   "export <pack-id>",
	Short: "Export a pack to a platform (currently Telegram only)",
	Long: `Export a sticker pack as a real sticker set on a platform.

--telegram creates the set on YOUR Telegram bot for YOUR Telegram account.
Telegram bots on weside are per user, so a pack cannot be exported without a
registered bot and a linked Telegram account — the API refuses with 409
telegram-bot-required and this command reports it.

Example:
  weside stickers packs export 12 --telegram`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !stickerExportTelegram {
			return fmt.Errorf("name a target: --telegram is the only one today")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerPacksExportTelegram(cmd.Context(), client, args[0])
	},
}

func runStickerPacksExportTelegram(
	ctx context.Context, client *api.Client, packID string,
) error {
	var result map[string]any
	path := stickersBasePath + "/packs/" + url.PathEscape(packID) + "/export/telegram"
	if err := client.Post(ctx, path, map[string]any{}, &result); err != nil {
		return fmt.Errorf("exporting pack %s to Telegram: %w", packID, err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}
	fmt.Printf("Set %v created.\nInstall it: %v\n", result["set_name"], result["link"])
	return nil
}

// --- send ---------------------------------------------------------------

var stickerSendBinding string

var stickerSendCmd = &cobra.Command{
	Use:   "send <pack-id> <shortcode>",
	Short: "Send one sticker into a bound channel",
	Long: `Send one sticker from a pack into a bound channel.

Example:
  weside stickers send 12 stampf --binding 4`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if stickerSendBinding == "" {
			return fmt.Errorf("--binding is required: name the channel binding to send into")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		return runStickerSend(cmd.Context(), client, args[0], args[1], stickerSendBinding)
	},
}

func runStickerSend(
	ctx context.Context, client *api.Client, packID, shortcode, bindingID string,
) error {
	var result map[string]any
	path := stickersBasePath + "/packs/" + url.PathEscape(packID) +
		"/stickers/" + url.PathEscape(shortcode) + "/send"
	body := map[string]any{"binding_id": bindingID}
	if err := client.Post(ctx, path, body, &result); err != nil {
		return fmt.Errorf("sending %s from pack %s: %w", shortcode, packID, err)
	}
	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}

	// Never render a bare `%v` of a field that might be absent: `fmt` prints a
	// missing key as "<nil>", so a response whose shape drifted would produce
	// "platform message id <nil>" — a success line carrying no id, which is
	// the same failure class as a publish that swallowed its 409. The id is
	// what proves the send reached the platform, so when it is missing the
	// line says so instead of printing punctuation.
	if id, ok := result["platform_message_id"]; ok && fmt.Sprintf("%v", id) != "" {
		fmt.Printf("Sent %s — platform message id %v\n", shortcode, id)
		return nil
	}
	fmt.Printf("Sent %s — but the response carried no platform message id, "+
		"so delivery is unconfirmed\n", shortcode)
	return nil
}

func init() {
	stickerPacksListCmd.Flags().IntVar(&stickerListLimit, "limit", 0, "Packs per page (1-100)")
	stickerPacksListCmd.Flags().StringVar(&stickerListCursor, "cursor", "",
		"Opaque cursor from a previous page's output")
	stickerPacksUploadCmd.Flags().BoolVar(&stickerUploadConfirmRights, "confirm-rights", false,
		"Declare that you hold the rights to this artwork")
	stickerPacksPublishCmd.Flags().BoolVar(&stickerPublishConfirmRights, "confirm-rights", false,
		"Confirm the rights in the same call")
	stickerPacksExportCmd.Flags().BoolVar(&stickerExportTelegram, "telegram", false,
		"Export as a Telegram sticker set on your own bot")
	stickerSendCmd.Flags().StringVar(&stickerSendBinding, "binding", "",
		"Channel binding id to send into")

	stickerPacksCmd.AddCommand(stickerPacksListCmd)
	stickerPacksCmd.AddCommand(stickerPacksShowCmd)
	stickerPacksCmd.AddCommand(stickerPacksUploadCmd)
	stickerPacksCmd.AddCommand(stickerPacksPublishCmd)
	stickerPacksCmd.AddCommand(stickerPacksExportCmd)
	stickersCmd.AddCommand(stickerPacksCmd)
	stickersCmd.AddCommand(stickerSendCmd)
	rootCmd.AddCommand(stickersCmd)
}
