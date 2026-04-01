package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage AI provider and data residency",
}

var providerShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current provider configuration",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		// Trailing slash required (307 redirect without it)
		var result map[string]any
		if err := client.Get(context.Background(), "/data-residency/", &result); err != nil {
			return fmt.Errorf("getting provider config: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		fmt.Printf("Type:   %v\n", result["type"])
		fmt.Printf("Preset: %v\n", result["preset_display_name"])
		fmt.Printf("Model:  %v\n", result["model_name"])
		if region := result["region"]; region != nil {
			fmt.Printf("Region: %v\n", region)
		}
		if result["has_api_key"] == true {
			fmt.Printf("BYOK:   yes\n")
		}
		return nil
	},
}

var providerPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List available regional presets",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/data-residency/presets", &result); err != nil {
			return fmt.Errorf("listing presets: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		// API returns {groups: [{region: "EUR", presets: [...]}]}
		groups, _ := result["groups"].([]any)
		for _, gItem := range groups {
			group, _ := gItem.(map[string]any)
			region := fmt.Sprintf("%v", group["region"])
			fmt.Printf("\n%s:\n", region)

			presets, _ := group["presets"].([]any)
			headers := []string{"ID", "TIER", "NAME", "DESCRIPTION"}
			var rows [][]string
			for _, pItem := range presets {
				p, _ := pItem.(map[string]any)
				id := fmt.Sprintf("%v", p["id"])
				tier := fmt.Sprintf("%v", p["tier"])
				name := fmt.Sprintf("%v", p["display_name"])
				desc := truncate(fmt.Sprintf("%v", p["description"]), 50)
				rows = append(rows, []string{id, tier, name, desc})
			}
			ui.PrintTable(headers, rows)
		}
		return nil
	},
}

var providerSetCmd = &cobra.Command{
	Use:   "set <preset_id>",
	Short: "Set regional provider preset (use numeric ID from 'presets')",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		presetID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("preset_id must be a number (use 'weside provider presets' to see IDs)")
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"preset_id": presetID}
		if err := client.Put(context.Background(), "/data-residency/", body, nil); err != nil {
			return fmt.Errorf("setting provider: %w", err)
		}

		ui.PrintSuccess("Provider preset set to %d", presetID)
		return nil
	},
}

var providerByokCmd = &cobra.Command{
	Use:   "byok <provider> <key>",
	Short: "Bring Your Own Key",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{
			"type":     "byok",
			"provider": args[0],
			"api_key":  args[1],
		}
		if err := client.Put(context.Background(), "/data-residency/", body, nil); err != nil {
			return fmt.Errorf("setting BYOK: %w", err)
		}

		ui.PrintSuccess("BYOK configured for %s", args[0])
		return nil
	},
}

func init() {
	providerCmd.AddCommand(providerShowCmd)
	providerCmd.AddCommand(providerPresetsCmd)
	providerCmd.AddCommand(providerSetCmd)
	providerCmd.AddCommand(providerByokCmd)
	rootCmd.AddCommand(providerCmd)
}
