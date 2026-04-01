package cmd

import (
	"context"
	"fmt"

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

		var result map[string]any
		if err := client.Get(context.Background(), "/data-residency", &result); err != nil {
			return fmt.Errorf("getting provider config: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		fmt.Printf("Provider: %v\n", result["provider"])
		fmt.Printf("Region:   %v\n", result["region"])
		fmt.Printf("Quality:  %v\n", result["quality"])
		if model, ok := result["model_name"]; ok && model != nil {
			fmt.Printf("Model:    %v\n", model)
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

		var result struct {
			Items []map[string]any `json:"items"`
		}
		if err := client.Get(context.Background(), "/data-residency/presets", &result); err != nil {
			return fmt.Errorf("listing presets: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		headers := []string{"ID", "NAME", "REGION", "QUALITY", "PROVIDER"}
		var rows [][]string
		for _, p := range result.Items {
			id := fmt.Sprintf("%v", p["id"])
			name := fmt.Sprintf("%v", p["name"])
			region := fmt.Sprintf("%v", p["region"])
			quality := fmt.Sprintf("%v", p["quality"])
			provider := fmt.Sprintf("%v", p["provider"])
			rows = append(rows, []string{id, name, region, quality, provider})
		}

		ui.PrintTable(headers, rows)
		return nil
	},
}

var providerSetCmd = &cobra.Command{
	Use:   "set <preset>",
	Short: "Set regional provider preset",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]string{"preset_id": args[0]}
		if err := client.Put(context.Background(), "/data-residency", body, nil); err != nil {
			return fmt.Errorf("setting provider: %w", err)
		}

		ui.PrintSuccess("Provider preset set to %s", args[0])
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

		body := map[string]string{
			"provider":  args[0],
			"api_key":   args[1],
			"preset_id": "BYOK",
		}
		if err := client.Put(context.Background(), "/data-residency", body, nil); err != nil {
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
