package cmd

import (
	"context"
	"encoding/json"
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

var (
	goalSaveContent  string
	goalSaveTags     string
	goalSaveDue      string
	goalSaveFollowUp string
)

var goalsSaveCmd = &cobra.Command{
	Use:   "save <title>",
	Short: "Create or update a goal via MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if goalSaveContent == "" {
			return fmt.Errorf("--content is required")
		}

		client, err := newMCPClient()
		if err != nil {
			return err
		}

		arguments := map[string]any{
			"title":   args[0],
			"content": goalSaveContent,
		}
		if goalSaveTags != "" {
			arguments["tags"] = goalSaveTags
		}
		if goalSaveDue != "" {
			arguments["due_date"] = goalSaveDue
		}
		if goalSaveFollowUp != "" {
			arguments["follow_up_date"] = goalSaveFollowUp
		}

		result, err := client.CallTool(context.Background(), "save_goal", arguments)
		if err != nil {
			return fmt.Errorf("saving goal: %w", err)
		}

		if IsJSON() {
			fmt.Println(string(result))
			return nil
		}

		// Parse MCP tool result content
		var callResult struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(result, &callResult); err == nil && len(callResult.Content) > 0 {
			for _, c := range callResult.Content {
				fmt.Println(c.Text)
			}
			return nil
		}

		ui.PrintSuccess("Goal %q saved", args[0])
		return nil
	},
}

func init() {
	goalsUpdateCmd.Flags().StringVar(&goalStatus, "status", "", "new status (active, paused, completed)")

	goalsSaveCmd.Flags().StringVar(&goalSaveContent, "content", "", "goal description and details (required)")
	goalsSaveCmd.Flags().StringVar(&goalSaveTags, "tags", "", "comma-separated tags")
	goalsSaveCmd.Flags().StringVar(&goalSaveDue, "due", "", "target date (YYYY-MM-DD)")
	goalsSaveCmd.Flags().StringVar(&goalSaveFollowUp, "follow-up", "", "follow-up date (YYYY-MM-DD)")

	goalsCmd.AddCommand(goalsListCmd)
	goalsCmd.AddCommand(goalsUpdateCmd)
	goalsCmd.AddCommand(goalsSaveCmd)
	rootCmd.AddCommand(goalsCmd)
}
