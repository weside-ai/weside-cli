package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	memoryType string
	// memCompanion is the REST verbs' --companion (a name or id, resolved client-side).
	memCompanion string
	// memorySaveCompanion is `memories save --companion` (a name: the MCP pin resolves names).
	memorySaveCompanion string
	memListAutoload     bool
	memListPage         int
	memListLimit        int
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
		companionID, err := resolveCompanion(memCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
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
			if s, ok := m["similarity"]; ok && s != nil {
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
	Long: `List a companion's memories, one page at a time.

--autoload=true lists only the pinned memories (always loaded into the
companion's context), --autoload=false only the unpinned ones; without the
flag both. --page/--limit page through the list (limit 1-100, default 20).`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		companionID, err := resolveCompanion(memCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		path, err := memoriesListPath(cmd, companionID)
		if err != nil {
			return err
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
		headers := []string{"ID", "MEMORY_ID", "PINNED", "TYPE", "TITLE", "CONTENT"}
		var rows [][]string
		for _, item := range memories {
			m, _ := item.(map[string]any)
			rows = append(rows, memoryListRow(m))
		}

		ui.PrintTable(headers, rows)
		ui.Printf("\n%d memory/ies on page %v of %v (total %v)\n",
			len(memories), result["page"], result["total_pages"], result["total"])
		return nil
	},
}

// addMemoriesListFilterFlags registers the filter and paging flags of
// `memories list` (shared with its test, which needs fresh Changed state).
func addMemoriesListFilterFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&memoryType, "type", "", "filter by type (fact, preference, experience, reflection)")
	cmd.Flags().BoolVar(&memListAutoload, "autoload", false, "true: only pinned (autoload) memories, false: only unpinned; unset: all")
	cmd.Flags().IntVar(&memListPage, "page", 1, "page number (from 1)")
	cmd.Flags().IntVar(&memListLimit, "limit", 20, "memories per page (1-100)")
}

// memoriesListPath builds the list request from the flags. --autoload is
// sent only when the user set it, so "unset" stays "no filter" rather than
// collapsing into autoload=false.
func memoriesListPath(cmd *cobra.Command, companionID string) (string, error) {
	q := url.Values{}
	if memoryType != "" {
		q.Set("memory_type", memoryType)
	}
	if cmd.Flags().Changed("autoload") {
		q.Set("autoload", strconv.FormatBool(memListAutoload))
	}
	if cmd.Flags().Changed("page") {
		if memListPage < 1 {
			return "", fmt.Errorf("--page must be at least 1")
		}
		q.Set("page", strconv.Itoa(memListPage))
	}
	if cmd.Flags().Changed("limit") {
		if memListLimit < 1 || memListLimit > 100 {
			return "", fmt.Errorf("--limit must be between 1 and 100")
		}
		q.Set("limit", strconv.Itoa(memListLimit))
	}
	path := fmt.Sprintf("/companions/%s/memories", companionID)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return path, nil
}

// memoryListRow renders one memory: the row id, the stable memory_id that
// `memories get/update` take, and the pin (autoload) flag.
func memoryListRow(m map[string]any) []string {
	pinned := "no"
	if b, _ := m["autoload"].(bool); b {
		pinned = "yes"
	}
	return []string{
		fmt.Sprintf("%v", m["id"]),
		fmt.Sprintf("%v", m["memory_id"]),
		pinned,
		fmt.Sprintf("%v", m["type"]),
		truncate(fmt.Sprintf("%v", m["title"]), 30),
		truncate(fmt.Sprintf("%v", m["content"]), 50),
	}
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

		client, err := newPinnedMCPClient(memorySaveCompanion)
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
				ui.Println(c.Text)
			}
			return nil
		}

		ui.PrintSuccess("Memory %q saved", args[0])
		return nil
	},
}

func init() {
	memoriesSearchCmd.Flags().StringVar(&memCompanion, "companion", "", "companion id or name (default: selected companion)")
	memoriesListCmd.Flags().StringVar(&memCompanion, "companion", "", "companion id or name (default: selected companion)")
	addMemoriesListFilterFlags(memoriesListCmd)

	memoriesSaveCmd.Flags().StringVar(&memorySaveContent, "content", "", "memory content (required)")
	memoriesSaveCmd.Flags().StringVar(&memorySaveType, "type", "fact", "memory type (fact, preference, experience, reflection)")
	memoriesSaveCmd.Flags().StringVar(&memorySaveTags, "tags", "", "comma-separated tags")
	memoriesSaveCmd.Flags().StringVar(&memorySaveCompanion, "companion", "", companionPinFlagUsage)

	memoriesCmd.AddCommand(memoriesSearchCmd)
	memoriesCmd.AddCommand(memoriesListCmd)
	memoriesCmd.AddCommand(memoriesSaveCmd)
	rootCmd.AddCommand(memoriesCmd)
}
