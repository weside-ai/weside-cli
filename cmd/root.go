// Package cmd implements all CLI commands for the weside CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	Use:   "weside",
	Short: "CLI for the weside.ai AI Companion Platform",
	Long: `weside is a command-line interface for interacting with your AI Companions
on the weside.ai platform.

Chat with your Companion, manage memories and goals, configure providers,
and more — all from your terminal.`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
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
