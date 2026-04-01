package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Discover and execute Companion tools",
}

var toolsDiscoverCmd = &cobra.Command{
	Use:   "discover [category]",
	Short: "Discover available tool categories",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		// Tools are accessed via MCP HTTP transport
		path := "/mcp"

		// Build MCP tool call
		toolName := "discover_tools"
		toolArgs := map[string]any{}
		if len(args) > 0 {
			toolArgs["category"] = args[0]
		}

		body := map[string]any{
			"method": "tools/call",
			"params": map[string]any{
				"name":      toolName,
				"arguments": toolArgs,
			},
		}

		var result map[string]any
		if err := client.Post(context.Background(), path, body, &result); err != nil {
			// Fallback: try REST endpoint if MCP fails
			return fmt.Errorf("tool discovery not available via CLI yet (use MCP or web interface)")
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		// Display result
		if content, ok := result["result"].(string); ok {
			fmt.Println(content)
		} else {
			ui.PrintJSON(result)
		}
		return nil
	},
}

var toolsSchemaCmd = &cobra.Command{
	Use:   "schema <name>",
	Short: "Show tool input schema",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		// TODO(WA-698 Phase 8): MCP HTTP transport for tool schema
		fmt.Printf("Tool schema for %q not yet available via CLI (use MCP or web interface)\n", args[0])
		return nil
	},
}

var toolsExecCmd = &cobra.Command{
	Use:   "exec <name> [args...]",
	Short: "Execute a tool",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		// TODO(WA-698 Phase 8): MCP HTTP transport for tool execution
		fmt.Printf("Tool execution for %q not yet available via CLI (use MCP or web interface)\n", args[0])
		return nil
	},
}

func init() {
	toolsCmd.AddCommand(toolsDiscoverCmd)
	toolsCmd.AddCommand(toolsSchemaCmd)
	toolsCmd.AddCommand(toolsExecCmd)
	rootCmd.AddCommand(toolsCmd)
}
