package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	backgroundAll           bool
	backgroundCancelConfirm bool
)

var backgroundCmd = &cobra.Command{
	Use:   "background",
	Short: "What your companions do in the background, and cancelling it",
	Long: `The owner's background register: a companion's triggers, pending
reminders, scheduled skills, Dreaming, the mentor, goal reminders and
Listening — who created each, when it fires next and what you may do about
it. Times and kinds only, never content.`,
}

var backgroundListCmd = &cobra.Command{
	Use:   "list [companion]",
	Short: "List a companion's background work (--all: every companion)",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(_ *cobra.Command, args []string) error {
		if backgroundAll == (len(args) == 1) {
			return fmt.Errorf("name a companion, or pass --all for every companion — not both")
		}
		path := "/me/background"
		if !backgroundAll {
			companionID, err := resolveCompanion(args[0])
			if err != nil {
				return err
			}
			path = "/companions/" + companionID + "/background"
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("reading background work: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		printBackground(result)
		return nil
	},
}

var backgroundCancelCmd = &cobra.Command{
	Use:   "cancel <companion> <item-id>",
	Short: "Cancel one item: a reminder for good, a routine switched off",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		if !backgroundCancelConfirm {
			return fmt.Errorf("cancelling %s may remove it for good — pass --confirm to proceed", args[1])
		}
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		path := "/companions/" + companionID + "/background/" + url.PathEscape(args[1]) + "/cancel"
		if err := client.Post(context.Background(), path, nil, &result); err != nil {
			return fmt.Errorf("cancelling %s: %w", args[1], err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		if result["effect"] == "occurrence" {
			ui.PrintSuccess("Cancelled %s: it will not run.", args[1])
		} else {
			ui.PrintSuccess("Switched %s off; the usual switch turns it back on.", args[1])
		}
		return nil
	},
}

// printBackground renders the register as one table, in the server's order.
func printBackground(result map[string]any) {
	items, _ := result["items"].([]any)
	headers := []string{"ID", "KIND", "COMPANION", "STATUS", "NEXT", "BY", "OWNER CAN"}
	rows := make([][]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		next := "-"
		if nf, ok := item["next_fire"].(map[string]any); ok {
			basis := "-"
			if b, ok := nf["basis"].(string); ok && b != "" {
				basis = b
			}
			next = fmt.Sprintf("%s (%s)", clockTime(nf["at"]), basis)
		}
		by := "-"
		if cb, ok := item["created_by"].(map[string]any); ok {
			by = fmt.Sprintf("%v", cb["actor"])
		}
		ownerCan := "-"
		if actions, ok := item["allowed_actions"].(map[string]any); ok {
			if owner := joinAnySlice(actions["owner"]); owner != "-" {
				ownerCan = owner
			}
		}
		rows = append(rows, []string{
			fmt.Sprintf("%v", item["id"]),
			fmt.Sprintf("%v", item["kind"]),
			numberText(item["companion_id"]),
			fmt.Sprintf("%v", item["status"]),
			next,
			by,
			ownerCan,
		})
	}
	ui.PrintTable(headers, rows)
}

func init() {
	backgroundListCmd.Flags().BoolVar(&backgroundAll, "all", false, "every companion you own")
	backgroundCancelCmd.Flags().BoolVar(&backgroundCancelConfirm, "confirm", false, "confirm the cancel")
	backgroundCmd.AddCommand(backgroundListCmd)
	backgroundCmd.AddCommand(backgroundCancelCmd)
	rootCmd.AddCommand(backgroundCmd)
}
