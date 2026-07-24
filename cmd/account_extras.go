package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	byokTestProvider string
	byokTestKey      string
	byokTestModel    string
	byokDiscoverProv string
	byokDiscoverKey  string
)

// provider byok-test — POST /data-residency/byok/test
var providerByokTestCmd = &cobra.Command{
	Use:   "byok-test --provider P --model M [--key K]",
	Short: "Probe a BYOK provider key with a 1-token inference test",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if byokTestProvider == "" || byokTestModel == "" {
			return fmt.Errorf("--provider and --model are required")
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"provider": byokTestProvider, "model_name": byokTestModel}
		if cmd.Flags().Changed("key") {
			body["api_key"] = byokTestKey
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/data-residency/byok/test", body, &result); err != nil {
			return fmt.Errorf("BYOK test: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		if ok, _ := result["ok"].(bool); ok {
			ui.PrintSuccess("BYOK ok (latency %v ms, %v models).", result["latency_ms"], result["models_available_count"])
		} else {
			fmt.Printf("BYOK failed: %v (%v)\n", result["error_code"], result["error_detail"])
		}
		return nil
	},
}

// provider byok-discover — POST /data-residency/byok/discover-models
var providerByokDiscoverCmd = &cobra.Command{
	Use:   "byok-discover --provider P [--key K]",
	Short: "List models available from a BYOK provider",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if byokDiscoverProv == "" {
			return fmt.Errorf("--provider is required")
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"provider": byokDiscoverProv}
		if cmd.Flags().Changed("key") {
			body["api_key"] = byokDiscoverKey
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/data-residency/byok/discover-models", body, &result); err != nil {
			return fmt.Errorf("discovering models: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		models, _ := result["models"].([]any)
		headers := []string{"ID", "NAME", "CHAT", "DEFAULT"}
		var rows [][]string
		for _, item := range models {
			m, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", m["id"]),
				truncate(fmt.Sprintf("%v", m["display_name"]), 40),
				fmt.Sprintf("%v", m["recommended_for_chat"]),
				fmt.Sprintf("%v", m["is_default_pick"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

// config system — read public system-config flags
var configSystemCmd = &cobra.Command{
	Use:   "system [key]",
	Short: "List public system config, or show one key",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if len(args) == 1 {
			var result map[string]any
			if err := client.Get(context.Background(), "/system-config/"+args[0], &result); err != nil {
				return fmt.Errorf("reading system config: %w", err)
			}
			if IsJSON() {
				ui.PrintJSON(result)
				return nil
			}
			fmt.Printf("%v\n", result["value"])
			if desc, _ := result["description"].(string); desc != "" {
				fmt.Printf("(%s)\n", desc)
			}
			return nil
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/system-config", &result); err != nil {
			return fmt.Errorf("listing system config: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		configs, _ := result["configs"].([]any)
		headers := []string{"KEY", "VALUE", "DESCRIPTION"}
		var rows [][]string
		for _, item := range configs {
			c, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", c["key"]),
				truncate(fmt.Sprintf("%v", c["value"]), 30),
				truncate(fmt.Sprintf("%v", c["description"]), 40),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

func init() {
	providerByokTestCmd.Flags().StringVar(&byokTestProvider, "provider", "", "BYOK provider slug")
	providerByokTestCmd.Flags().StringVar(&byokTestKey, "key", "", "API key (omit to reuse stored key)")
	providerByokTestCmd.Flags().StringVar(&byokTestModel, "model", "", "model name to probe")
	providerByokDiscoverCmd.Flags().StringVar(&byokDiscoverProv, "provider", "", "BYOK provider slug")
	providerByokDiscoverCmd.Flags().StringVar(&byokDiscoverKey, "key", "", "API key (omit to reuse stored key)")

	providerCmd.AddCommand(providerByokTestCmd)
	providerCmd.AddCommand(providerByokDiscoverCmd)
	configCmd.AddCommand(configSystemCmd)
}
