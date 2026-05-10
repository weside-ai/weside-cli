package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/auth"
	"github.com/weside-ai/weside-cli/internal/config"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(_ *cobra.Command, _ []string) error {
		settings := viper.AllSettings()

		if IsJSON() {
			ui.PrintJSON(settings)
			return nil
		}

		if len(settings) == 0 {
			fmt.Println("No configuration set.")
			fmt.Println("Config file: ~/.weside/config.yaml")
			return nil
		}

		headers := []string{"KEY", "VALUE"}
		var rows [][]string
		for k, v := range settings {
			rows = append(rows, []string{k, fmt.Sprintf("%v", v)})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		if err := config.PersistDottedKey(key, value); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		viper.Set(key, value)
		ui.PrintSuccess("Config %s = %s", key, value)
		return nil
	},
}

var configRefreshAuthCmd = &cobra.Command{
	Use:   "refresh-auth",
	Short: "Re-fetch auth-config from the backend's /.well-known/weside-auth endpoint",
	Long: `Forces a live fetch of the backend's auth-config (supabase_url, supabase_anon_key,
callback_port, mcp_url) and updates the cached values in ~/.weside/config.yaml.

Use this after the backend rotates its Supabase anon-key. The CLI will pick up
the new values on the next 'weside auth login' without needing a CLI release.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg, err := auth.Fetch(ctx, GetAPIURL())
		if err != nil {
			return fmt.Errorf("refresh-auth: %w", err)
		}
		if err := auth.SaveCachedAuth(cfg); err != nil {
			return fmt.Errorf("saving refreshed auth-config: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(cfg)
			return nil
		}
		fmt.Printf("auth-config refreshed (fetched_at=%s)\n", cfg.FetchedAt)
		fmt.Printf("  supabase_url:      %s\n", cfg.SupabaseURL)
		fmt.Printf("  supabase_anon_key: %s\n", truncateForDisplay(cfg.SupabaseAnonKey))
		fmt.Printf("  mcp_url:           %s\n", cfg.MCPURL)
		fmt.Printf("  callback_port:     %d\n", cfg.CallbackPort)
		return nil
	},
}

// truncateForDisplay shortens a credential to a head/tail preview so the user
// can verify it changed after a rotation without leaking the full secret to
// scrollback. Returns the original string if it's already short enough.
func truncateForDisplay(s string) string {
	const head, tail = 6, 4
	if len(s) <= head+tail+3 {
		return s
	}
	return s[:head] + "…" + s[len(s)-tail:]
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configRefreshAuthCmd)
	rootCmd.AddCommand(configCmd)
}
