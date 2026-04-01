package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	goalTitle   string
	goalContent string
	goalStatus  string
)

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
		var result struct {
			Items []map[string]any `json:"items"`
		}
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing goals: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		headers := []string{"TITLE", "STATUS", "CONTENT"}
		var rows [][]string
		for _, g := range result.Items {
			title := fmt.Sprintf("%v", g["title"])
			status := fmt.Sprintf("%v", g["goal_status"])
			content := truncate(fmt.Sprintf("%v", g["content"]), 50)
			rows = append(rows, []string{title, status, content})
		}

		ui.PrintTable(headers, rows)
		return nil
	},
}

var goalsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new goal",
	RunE: func(_ *cobra.Command, _ []string) error {
		if goalTitle == "" || goalContent == "" {
			return fmt.Errorf("--title and --content are required")
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		companionID := viper.GetString("default_companion_id")
		if companionID == "" {
			return fmt.Errorf("no default companion set (use: weside companions select <name>)")
		}

		body := map[string]string{
			"title":       goalTitle,
			"content":     goalContent,
			"memory_type": "goal",
		}

		path := fmt.Sprintf("/companions/%s/memories", companionID)
		var result map[string]any
		if err := client.Post(context.Background(), path, body, &result); err != nil {
			return fmt.Errorf("creating goal: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		ui.PrintSuccess("Goal created: %s", goalTitle)
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

		// Find goal by title
		path := fmt.Sprintf("/companions/%s/memories/goals", companionID)
		var goals struct {
			Items []map[string]any `json:"items"`
		}
		if err := client.Get(context.Background(), path, &goals); err != nil {
			return fmt.Errorf("listing goals: %w", err)
		}

		var groupID string
		for _, g := range goals.Items {
			if fmt.Sprintf("%v", g["title"]) == args[0] {
				groupID = fmt.Sprintf("%v", g["group_id"])
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
	goalsCreateCmd.Flags().StringVar(&goalTitle, "title", "", "goal title")
	goalsCreateCmd.Flags().StringVar(&goalContent, "content", "", "goal description")
	goalsUpdateCmd.Flags().StringVar(&goalStatus, "status", "", "new status (active, paused, completed)")
	goalsCmd.AddCommand(goalsListCmd)
	goalsCmd.AddCommand(goalsCreateCmd)
	goalsCmd.AddCommand(goalsUpdateCmd)
	rootCmd.AddCommand(goalsCmd)
}
