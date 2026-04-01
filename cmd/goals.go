package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var goalStatus string

var goalsCmd = &cobra.Command{
	Use:   "goals",
	Short: "Manage Companion goals",
}

var goalsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List goals",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		companionID := viper.GetString("default_companion_id")
		if companionID == "" {
			return fmt.Errorf("no default companion set (use: weside companions select <name>)")
		}

		path := fmt.Sprintf("/companions/%s/memories/goals", companionID)
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing goals: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		// API returns {active: [...], paused: [...], completed: [...]}
		headers := []string{"STATUS", "ORDER", "TITLE", "CONTENT"}
		var rows [][]string

		for _, status := range []string{"active", "paused", "completed"} {
			goals, _ := result[status].([]any)
			for _, item := range goals {
				g, _ := item.(map[string]any)
				title := truncate(fmt.Sprintf("%v", g["title"]), 30)
				content := truncate(fmt.Sprintf("%v", g["content"]), 40)
				order := fmt.Sprintf("%v", g["order"])
				rows = append(rows, []string{status, order, title, content})
			}
		}

		ui.PrintTable(headers, rows)
		return nil
	},
}

var goalsUpdateCmd = &cobra.Command{
	Use:   "update <title>",
	Short: "Update a goal's status",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if goalStatus == "" {
			return fmt.Errorf("--status is required (active, paused, completed)")
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		companionID := viper.GetString("default_companion_id")
		if companionID == "" {
			return fmt.Errorf("no default companion set (use: weside companions select <name>)")
		}

		// Find goal by title across all status groups
		path := fmt.Sprintf("/companions/%s/memories/goals", companionID)
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing goals: %w", err)
		}

		var groupID string
		for _, status := range []string{"active", "paused", "completed"} {
			goals, _ := result[status].([]any)
			for _, item := range goals {
				g, _ := item.(map[string]any)
				if fmt.Sprintf("%v", g["title"]) == args[0] {
					groupID = fmt.Sprintf("%v", g["memory_group_id"])
					break
				}
			}
			if groupID != "" {
				break
			}
		}

		if groupID == "" {
			return fmt.Errorf("goal %q not found", args[0])
		}

		body := map[string]string{"status": goalStatus}
		updatePath := fmt.Sprintf("/companions/%s/memories/%s/goal-status", companionID, groupID)
		if err := client.Patch(context.Background(), updatePath, body, nil); err != nil {
			return fmt.Errorf("updating goal: %w", err)
		}

		ui.PrintSuccess("Goal %q updated to %s", args[0], goalStatus)
		return nil
	},
}

func init() {
	goalsUpdateCmd.Flags().StringVar(&goalStatus, "status", "", "new status (active, paused, completed)")
	goalsCmd.AddCommand(goalsListCmd)
	goalsCmd.AddCommand(goalsUpdateCmd)
	// Note: goal creation is handled by the Companion during conversations,
	// not via REST API. Use the MCP or chat to create goals.
	rootCmd.AddCommand(goalsCmd)
}
