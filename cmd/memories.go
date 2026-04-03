package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

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

		path := fmt.Sprintf("/companions/%s/memories/search?q=%s", companionID, url.QueryEscape(args[0]))
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

var (
	memorySaveContent string
	memorySaveType    string
	memorySaveTags    string
)

var memoriesSaveCmd = &cobra.Command{
	Use:   "save <title>",
	Short: "Save a new memory via MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if memorySaveContent == "" {
			return fmt.Errorf("--content is required")
		}

		client, err := newMCPClient()
		if err != nil {
			return err
		}

		arguments := map[string]any{
			"title":       args[0],
			"content":     memorySaveContent,
			"memory_type": memorySaveType,
		}
		if memorySaveTags != "" {
			arguments["tags"] = memorySaveTags
		}

		result, err := client.CallTool(context.Background(), "save_memory", arguments)
		if err != nil {
			return fmt.Errorf("saving memory: %w", err)
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

		ui.PrintSuccess("Memory %q saved", args[0])
		return nil
	},
}

func init() {
	memoriesListCmd.Flags().StringVar(&memoryType, "type", "", "filter by type (fact, preference, experience, reflection)")

	memoriesSaveCmd.Flags().StringVar(&memorySaveContent, "content", "", "memory content (required)")
	memoriesSaveCmd.Flags().StringVar(&memorySaveType, "type", "fact", "memory type (fact, preference, experience, reflection)")
	memoriesSaveCmd.Flags().StringVar(&memorySaveTags, "tags", "", "comma-separated tags")

	memoriesCmd.AddCommand(memoriesSearchCmd)
	memoriesCmd.AddCommand(memoriesListCmd)
	memoriesCmd.AddCommand(memoriesSaveCmd)
	rootCmd.AddCommand(memoriesCmd)
}
