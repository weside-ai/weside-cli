package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		viper.Set(key, value)

		dir, err := config.EnsureConfigDir()
		if err != nil {
			return err
		}
		if err := viper.WriteConfigAs(dir + "/config.yaml"); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		ui.PrintSuccess("Config %s = %s", key, value)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
