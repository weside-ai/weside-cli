package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	memoryType    string
	memoryTitle   string
	memoryContent string
)

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

		path := fmt.Sprintf("/companions/%s/memories/search?query=%s", companionID, args[0])
		var result struct {
			Items []map[string]any `json:"items"`
		}
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("searching memories: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		headers := []string{"TYPE", "TITLE", "CONTENT", "SCORE"}
		var rows [][]string
		for _, m := range result.Items {
			mtype := fmt.Sprintf("%v", m["memory_type"])
			title := truncate(fmt.Sprintf("%v", m["title"]), 30)
			content := truncate(fmt.Sprintf("%v", m["content"]), 50)
			score := fmt.Sprintf("%.2f", m["similarity_score"])
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

		var result struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		}
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing memories: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		headers := []string{"ID", "TYPE", "TITLE", "CONTENT"}
		var rows [][]string
		for _, m := range result.Items {
			id := fmt.Sprintf("%v", m["id"])
			mtype := fmt.Sprintf("%v", m["memory_type"])
			title := truncate(fmt.Sprintf("%v", m["title"]), 30)
			content := truncate(fmt.Sprintf("%v", m["content"]), 50)
			rows = append(rows, []string{id, mtype, title, content})
		}

		ui.PrintTable(headers, rows)
		fmt.Printf("\n%d memory/ies\n", result.Total)
		return nil
	},
}

var memoriesSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save a new memory",
	RunE: func(_ *cobra.Command, _ []string) error {
		if memoryTitle == "" || memoryContent == "" {
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
			"title":       memoryTitle,
			"content":     memoryContent,
			"memory_type": memoryType,
		}
		if memoryType == "" {
			body["memory_type"] = "fact"
		}

		// Use MCP endpoint for memory creation (no REST endpoint exists)
		// For now, try direct API call
		path := fmt.Sprintf("/companions/%s/memories", companionID)
		var result map[string]any
		if err := client.Post(context.Background(), path, body, &result); err != nil {
			return fmt.Errorf("saving memory: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		ui.PrintSuccess("Memory saved: %s", memoryTitle)
		return nil
	},
}

func init() {
	memoriesListCmd.Flags().StringVar(&memoryType, "type", "", "filter by type (fact, preference, experience, reflection)")
	memoriesSaveCmd.Flags().StringVar(&memoryTitle, "title", "", "memory title")
	memoriesSaveCmd.Flags().StringVar(&memoryContent, "content", "", "memory content")
	memoriesSaveCmd.Flags().StringVar(&memoryType, "type", "fact", "memory type (fact, preference, experience, reflection)")
	memoriesCmd.AddCommand(memoriesSearchCmd)
	memoriesCmd.AddCommand(memoriesListCmd)
	memoriesCmd.AddCommand(memoriesSaveCmd)
	rootCmd.AddCommand(memoriesCmd)
}
