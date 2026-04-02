package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/auth"
	"github.com/weside-ai/weside-cli/internal/mcp"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Discover and execute Companion tools",
}

var toolsDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover available tools from the MCP server",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newMCPClient()
		if err != nil {
			return err
		}

		result, err := client.ListTools(context.Background())
		if err != nil {
			return fmt.Errorf("discovering tools: %w", err)
		}

		if IsJSON() {
			fmt.Println(string(result))
			return nil
		}

		// Parse tools list
		var toolsResult struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(result, &toolsResult); err != nil {
			// Fallback: print raw
			fmt.Println(string(result))
			return nil
		}

		headers := []string{"NAME", "DESCRIPTION"}
		var rows [][]string
		for _, t := range toolsResult.Tools {
			rows = append(rows, []string{t.Name, truncate(t.Description, 60)})
		}
		ui.PrintTable(headers, rows)
		fmt.Printf("\n%d tool(s)\n", len(toolsResult.Tools))
		return nil
	},
}

var toolsExecCmd = &cobra.Command{
	Use:   "exec <name> [json-args]",
	Short: "Execute a tool",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newMCPClient()
		if err != nil {
			return err
		}

		toolName := args[0]
		arguments := map[string]any{}
		if len(args) > 1 {
			if err := json.Unmarshal([]byte(args[1]), &arguments); err != nil {
				return fmt.Errorf("parsing arguments JSON: %w", err)
			}
		}

		result, err := client.CallTool(context.Background(), toolName, arguments)
		if err != nil {
			return fmt.Errorf("executing tool %q: %w", toolName, err)
		}

		// Parse result content
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

		// Fallback: print raw JSON
		fmt.Println(string(result))
		return nil
	},
}

func newMCPClient() (*mcp.Client, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, err
	}

	baseURL := GetAPIURL() + "/mcp/"
	return mcp.NewClient(baseURL, token), nil
}

func init() {
	toolsCmd.AddCommand(toolsDiscoverCmd)
	toolsCmd.AddCommand(toolsExecCmd)
	rootCmd.AddCommand(toolsCmd)
}
