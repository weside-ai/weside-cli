// Package cmd implements all CLI commands for the weside CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	cfgFile         string
	jsonOut         bool
	verbose         bool
	apiURL          string
	noColor         bool
	supabaseURL     string
	supabaseAnonKey string
)

var rootCmd = &cobra.Command{
	Use:           "weside",
	SilenceErrors: true,
	// Usage is help for someone who typed the command wrong; it is noise above
	// a connection refused. cobra prints it for EVERY error unless silenced, so
	// a `weside api DELETE /rooms/9/participants/9 --v2` against a stopped
	// backend answered with the full flag table and the real error one line
	// below it — which reads like a rejected signature, and was misread as
	// exactly that (WA-2283 state file, point 3).
	//
	// The hook rather than a plain `SilenceUsage: true` is what splits the two
	// cases: cobra validates args and flags BEFORE running any PersistentPreRun
	// (`command.go`: ValidateArgs at 968, the PersistentPreRunE walk at 985), so
	// a wrong argument count still reaches the usage block, and only an error
	// raised from RunE onwards is silent. Pinned from both sides by
	// TestArgumentErrorStillPrintsUsage / TestRuntimeErrorPrintsNoUsageBlock.
	//
	// The one case that moves with it: ValidateRequiredFlags runs AFTER the hook
	// (1007), so a missing required flag now prints no usage either. `goals
	// reorder --ids` is the only required flag in the tree and says what it
	// needs in its own error.
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return nil
	},
	Short: "CLI for the weside.ai AI Companion Platform",
	Long: `weside is a command-line interface for interacting with your AI Companions
on the weside.ai platform.

Chat with your Companion, manage memories and goals, configure providers,
and more — all from your terminal.`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		ui.PrintError("%s", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.weside/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "output as JSON")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "API base URL (default: https://api.weside.ai)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	rootCmd.PersistentFlags().StringVar(&supabaseURL, "supabase-url", "", "Supabase URL override (escape hatch for self-hosted backends; otherwise discovered via /.well-known/weside-auth)")
	rootCmd.PersistentFlags().StringVar(&supabaseAnonKey, "supabase-anon-key", "", "Supabase anon-key override (paired with --supabase-url)")

	_ = viper.BindPFlag("json", rootCmd.PersistentFlags().Lookup("json"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("api_url", rootCmd.PersistentFlags().Lookup("api-url"))
	_ = viper.BindPFlag("no_color", rootCmd.PersistentFlags().Lookup("no-color"))
	_ = viper.BindPFlag("supabase_url", rootCmd.PersistentFlags().Lookup("supabase-url"))
	_ = viper.BindPFlag("supabase_anon_key", rootCmd.PersistentFlags().Lookup("supabase-anon-key"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error finding home directory:", err)
			os.Exit(1)
		}

		viper.AddConfigPath(home + "/.weside")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("WESIDE")
	viper.AutomaticEnv()

	if apiURL == "" {
		viper.SetDefault("api_url", "https://api.weside.ai")
	}

	// Respect NO_COLOR environment variable
	if os.Getenv("NO_COLOR") != "" {
		viper.Set("no_color", true)
	}

	_ = viper.ReadInConfig()
}

// GetAPIURL returns the configured API base URL.
func GetAPIURL() string {
	if url := viper.GetString("api_url"); url != "" {
		return url
	}
	return "https://api.weside.ai"
}

// IsJSON returns whether JSON output is enabled.
func IsJSON() bool {
	return viper.GetBool("json")
}

// IsVerbose returns whether verbose output is enabled.
func IsVerbose() bool {
	return viper.GetBool("verbose")
}
