package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	triggerEnabled  bool
	triggerOneShot  bool
	triggerGeofence string
)

var triggersCmd = &cobra.Command{
	Use:   "triggers",
	Short: "Inspect and manage companion triggers",
}

var triggersListCmd = &cobra.Command{
	Use:   "list <companion>",
	Short: "List a companion's triggers",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+companionID+"/triggers", &result); err != nil {
			return fmt.Errorf("listing triggers: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		triggers, _ := result["triggers"].([]any)
		headers := []string{"ID", "TYPE", "TAGS", "ENABLED", "NEXT"}
		var rows [][]string
		for _, item := range triggers {
			t, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", t["id"]),
				fmt.Sprintf("%v", t["trigger_type"]),
				joinAnySlice(t["attention_tags"]),
				fmt.Sprintf("%v", t["enabled"]),
				fmt.Sprintf("%v", t["next_trigger_at"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var triggersToggleCmd = &cobra.Command{
	Use:   "toggle <companion> <trigger_id>",
	Short: "Enable or disable a trigger",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if !cmd.Flags().Changed("enabled") {
			return fmt.Errorf("--enabled is required (true/false)")
		}
		body := map[string]any{"enabled": triggerEnabled}
		var result map[string]any
		if err := client.Patch(context.Background(), "/companions/"+companionID+"/triggers/"+args[1], body, &result); err != nil {
			return fmt.Errorf("toggling trigger: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Trigger %s enabled=%v.", args[1], triggerEnabled)
		return nil
	},
}

var triggersSetCmd = &cobra.Command{
	Use:   "set <companion> <trigger_id>",
	Short: "Update trigger settings (one-shot, geofence-event, enabled)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if cmd.Flags().Changed("enabled") {
			body["enabled"] = triggerEnabled
		}
		if cmd.Flags().Changed("one-shot") {
			body["one_shot"] = triggerOneShot
		}
		if cmd.Flags().Changed("geofence-event") {
			body["geofence_event"] = triggerGeofence
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to set (use --enabled/--one-shot/--geofence-event)")
		}

		var result map[string]any
		if err := client.Patch(context.Background(), "/companions/"+companionID+"/triggers/"+args[1]+"/settings", body, &result); err != nil {
			return fmt.Errorf("updating trigger: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Trigger %s updated.", args[1])
		return nil
	},
}

var triggersDeleteCmd = &cobra.Command{
	Use:   "delete <companion> <trigger_id>",
	Short: "Delete a trigger",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/companions/"+companionID+"/triggers/"+args[1], nil); err != nil {
			return fmt.Errorf("deleting trigger: %w", err)
		}

		ui.PrintSuccess("Trigger %s deleted.", args[1])
		return nil
	},
}

// joinAnySlice renders a []any of strings as a comma-joined string.
func joinAnySlice(v any) string {
	items, _ := v.([]any)
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, fmt.Sprintf("%v", it))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func init() {
	triggersToggleCmd.Flags().BoolVar(&triggerEnabled, "enabled", false, "enable or disable")
	triggersSetCmd.Flags().BoolVar(&triggerEnabled, "enabled", false, "enable or disable")
	triggersSetCmd.Flags().BoolVar(&triggerOneShot, "one-shot", false, "one-shot trigger")
	triggersSetCmd.Flags().StringVar(&triggerGeofence, "geofence-event", "", "enter|exit|both")

	triggersCmd.AddCommand(triggersListCmd)
	triggersCmd.AddCommand(triggersToggleCmd)
	triggersCmd.AddCommand(triggersSetCmd)
	triggersCmd.AddCommand(triggersDeleteCmd)
	rootCmd.AddCommand(triggersCmd)
}
