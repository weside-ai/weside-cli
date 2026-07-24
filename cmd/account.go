package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	usageMonth   string
	usageDaily   bool
	secretValue  string
	secretLabel  string
	secretInject string
	secretEnv    string
	secretFile   string
)

// --- me usage --------------------------------------------------------------

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "Account-level reads",
}

var meUsageCmd = &cobra.Command{
	Use:   "usage [--month YYYY-MM]",
	Short: "Show credit usage for a month",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		path := "/me/usage"
		if usageMonth != "" {
			path += "?month=" + usageMonth
		}
		if usageDaily {
			path = "/me/usage/daily"
			if usageMonth != "" {
				path += "?month=" + usageMonth
			}
		}
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("reading usage: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		if usageDaily {
			fmt.Printf("Month: %v\n\n", result["month"])
			days, _ := result["days"].([]any)
			headers := []string{"DATE", "CREDITS"}
			var rows [][]string
			for _, item := range days {
				d, _ := item.(map[string]any)
				rows = append(rows, []string{fmt.Sprintf("%v", d["date"]), fmt.Sprintf("%v", d["credits"])})
			}
			ui.PrintTable(headers, rows)
			return nil
		}

		fmt.Printf("Month: %v   Total credits: %v\n", result["month"], result["total_credits"])
		fmt.Printf("Actions: %v/%v (remaining %v, purchased %v)\n",
			result["actions_used_this_month"], result["actions_per_month"], result["actions_remaining"], result["purchased_actions"])
		if ps, ok := result["per_source"].(map[string]any); ok {
			fmt.Printf("By source: chat=%v voice=%v embeddings=%v tooling=%v\n",
				ps["chat"], ps["voice"], ps["embeddings"], ps["tooling"])
		}
		if comps, ok := result["per_companion"].([]any); ok {
			fmt.Println("\nBy companion:")
			for _, item := range comps {
				c, _ := item.(map[string]any)
				fmt.Printf("  %v (%v): %v\n", c["companion_name"], c["companion_id"], c["credits"])
			}
		}
		return nil
	},
}

// --- user-config (KV) ------------------------------------------------------

var userConfigCmd = &cobra.Command{
	Use:   "user-config",
	Short: "Read/write the user config key-value store",
}

var userConfigGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "List all config values, or show one key",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if len(args) == 1 {
			var result map[string]any
			if err := client.Get(context.Background(), "/user-config/"+args[0], &result); err != nil {
				return fmt.Errorf("reading config: %w", err)
			}
			if IsJSON() {
				ui.PrintJSON(result)
				return nil
			}
			fmt.Printf("%v\n", result["value"])
			return nil
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/user-config", &result); err != nil {
			return fmt.Errorf("listing config: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		configs, _ := result["configs"].([]any)
		headers := []string{"KEY", "VALUE", "SECRET"}
		var rows [][]string
		for _, item := range configs {
			c, _ := item.(map[string]any)
			secret := "no"
			if s, _ := c["is_secret"].(bool); s {
				secret = "yes"
			}
			rows = append(rows, []string{fmt.Sprintf("%v", c["key"]), truncate(fmt.Sprintf("%v", c["value"]), 50), secret})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var userConfigSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Create or update a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"key": args[0], "value": args[1]}
		var result map[string]any
		if err := client.Post(context.Background(), "/user-config", body, &result); err != nil {
			return fmt.Errorf("setting config: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Config %s set.", args[0])
		return nil
	},
}

var userConfigDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/user-config/"+args[0], nil); err != nil {
			return fmt.Errorf("deleting config: %w", err)
		}
		ui.PrintSuccess("Config %s deleted.", args[0])
		return nil
	},
}

// --- sandbox-secrets -------------------------------------------------------

var sandboxSecretsCmd = &cobra.Command{
	Use:   "sandbox-secrets",
	Short: "Manage the sandbox secret vault (values masked)",
}

var sandboxSecretsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sandbox secrets (masked)",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result []any
		if err := client.Get(context.Background(), "/sandbox-secrets", &result); err != nil {
			return fmt.Errorf("listing sandbox secrets: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		headers := []string{"SLUG", "LABEL", "INJECT", "VALUE"}
		var rows [][]string
		for _, item := range result {
			s, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", s["slug"]),
				truncate(fmt.Sprintf("%v", s["label"]), 25),
				fmt.Sprintf("%v", s["injection_type"]),
				fmt.Sprintf("%v", s["masked_value"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var sandboxSecretsPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List available secret presets and their config status",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result []any
		if err := client.Get(context.Background(), "/sandbox-secrets/presets", &result); err != nil {
			return fmt.Errorf("listing presets: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		headers := []string{"SLUG", "LABEL", "INJECT", "CONFIGURED"}
		var rows [][]string
		for _, item := range result {
			p, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", p["slug"]),
				truncate(fmt.Sprintf("%v", p["label"]), 30),
				fmt.Sprintf("%v", p["injection_type"]),
				fmt.Sprintf("%v", p["configured"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var sandboxSecretsPutCmd = &cobra.Command{
	Use:   "put <slug> --value V",
	Short: "Create or update a sandbox secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if secretValue == "" {
			return fmt.Errorf("--value is required")
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"slug": args[0], "value": secretValue}
		if cmd.Flags().Changed("label") {
			body["label"] = secretLabel
		}
		if cmd.Flags().Changed("injection-type") {
			body["injection_type"] = secretInject
		}
		if cmd.Flags().Changed("env-name") {
			body["env_name"] = secretEnv
		}
		if cmd.Flags().Changed("file-path") {
			body["file_path"] = secretFile
		}

		var result map[string]any
		if err := client.Put(context.Background(), "/sandbox-secrets", body, &result); err != nil {
			return fmt.Errorf("saving secret: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Secret %s saved.", args[0])
		return nil
	},
}

var sandboxSecretsDeleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Delete a sandbox secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/sandbox-secrets/"+args[0], nil); err != nil {
			return fmt.Errorf("deleting secret: %w", err)
		}
		ui.PrintSuccess("Secret %s deleted.", args[0])
		return nil
	},
}

func init() {
	meUsageCmd.Flags().StringVar(&usageMonth, "month", "", "YYYY-MM")
	meUsageCmd.Flags().BoolVar(&usageDaily, "daily", false, "daily breakdown")
	sandboxSecretsPutCmd.Flags().StringVar(&secretValue, "value", "", "secret value (required)")
	sandboxSecretsPutCmd.Flags().StringVar(&secretLabel, "label", "", "label (required for custom slugs)")
	sandboxSecretsPutCmd.Flags().StringVar(&secretInject, "injection-type", "", "env|file|git_credential")
	sandboxSecretsPutCmd.Flags().StringVar(&secretEnv, "env-name", "", "env var name (injection-type=env)")
	sandboxSecretsPutCmd.Flags().StringVar(&secretFile, "file-path", "", "file path (injection-type=file/git_credential)")

	meCmd.AddCommand(meUsageCmd)
	rootCmd.AddCommand(meCmd)

	userConfigCmd.AddCommand(userConfigGetCmd)
	userConfigCmd.AddCommand(userConfigSetCmd)
	userConfigCmd.AddCommand(userConfigDeleteCmd)
	rootCmd.AddCommand(userConfigCmd)

	sandboxSecretsCmd.AddCommand(sandboxSecretsListCmd)
	sandboxSecretsCmd.AddCommand(sandboxSecretsPresetsCmd)
	sandboxSecretsCmd.AddCommand(sandboxSecretsPutCmd)
	sandboxSecretsCmd.AddCommand(sandboxSecretsDeleteCmd)
	rootCmd.AddCommand(sandboxSecretsCmd)
}
