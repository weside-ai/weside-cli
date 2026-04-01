package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var memoryType string

var memoriesCmd = &cobra.Command{
	Use:   "memories",
	Short: "Manage Companion memories",
}

var memoriesSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search memories semantically",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		companionID := viper.GetString("default_companion_id")
		if companionID == "" {
			return fmt.Errorf("no default companion set (use: weside companions select <name>)")
		}

		path := fmt.Sprintf("/companions/%s/memories/search?q=%s", companionID, args[0])
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("searching memories: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		memories, _ := result["memories"].([]any)
		headers := []string{"TYPE", "TITLE", "CONTENT", "SCORE"}
		var rows [][]string
		for _, item := range memories {
			m, _ := item.(map[string]any)
			mtype := fmt.Sprintf("%v", m["type"])
			title := truncate(fmt.Sprintf("%v", m["title"]), 30)
			content := truncate(fmt.Sprintf("%v", m["content"]), 50)
			score := "—"
			if s, ok := m["similarity_score"]; ok && s != nil {
				score = fmt.Sprintf("%.2f", s)
			}
			rows = append(rows, []string{mtype, title, content, score})
		}

		ui.PrintTable(headers, rows)
		return nil
	},
}

var memoriesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List memories",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		companionID := viper.GetString("default_companion_id")
		if companionID == "" {
			return fmt.Errorf("no default companion set (use: weside companions select <name>)")
		}

		path := fmt.Sprintf("/companions/%s/memories", companionID)
		if memoryType != "" {
			path += "?memory_type=" + memoryType
		}

		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing memories: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		memories, _ := result["memories"].([]any)
		headers := []string{"ID", "TYPE", "TITLE", "CONTENT"}
		var rows [][]string
		for _, item := range memories {
			m, _ := item.(map[string]any)
			id := fmt.Sprintf("%v", m["id"])
			mtype := fmt.Sprintf("%v", m["type"])
			title := truncate(fmt.Sprintf("%v", m["title"]), 30)
			content := truncate(fmt.Sprintf("%v", m["content"]), 50)
			rows = append(rows, []string{id, mtype, title, content})
		}

		ui.PrintTable(headers, rows)
		fmt.Printf("\n%d memory/ies\n", len(memories))
		return nil
	},
}

func init() {
	memoriesListCmd.Flags().StringVar(&memoryType, "type", "", "filter by type (fact, preference, experience, reflection)")
	memoriesCmd.AddCommand(memoriesSearchCmd)
	memoriesCmd.AddCommand(memoriesListCmd)
	// Note: memory creation is handled by the Companion during conversations,
	// not via REST API. Use the MCP or chat to create memories.
	rootCmd.AddCommand(memoriesCmd)
}
