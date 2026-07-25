package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	promptLimit         int
	identityFile        string
	identityBaseVer     int
	toolsIntegrationIDs string
)

// readBodyFromFileOrStdin reads text content from --file (or "-" for stdin).
func readBodyFromFileOrStdin(file string) (string, error) {
	if file == "-" {
		data, err := io.ReadAll(stdinReader)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("reading file %s: %w", file, err)
	}
	return string(data), nil
}

// --- companions resume (wake) ---------------------------------------------

var companionResumeCmd = &cobra.Command{
	Use:   "resume <id|name>",
	Short: "Wake a suspended companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		companionID, err := resolveCompanionID(client, args[0])
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/companions/"+companionID+"/resume", nil, &result); err != nil {
			return fmt.Errorf("resuming companion: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Companion %v resumed.", result["name"])
		return nil
	},
}

// --- system-prompt versioning ----------------------------------------------

var companionPromptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Inspect and restore companion system-prompt versions",
}

var promptsVersionsCmd = &cobra.Command{
	Use:   "versions <companion>",
	Short: "List system-prompt versions",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/companions/%s/system-prompt/versions?limit=%d", companionID, promptLimit)
		var result []any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing prompt versions: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		headers := []string{"VERSION", "CREATED", "BY", "CHARS", "REASON"}
		var rows [][]string
		for _, item := range result {
			v, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", v["version"]),
				fmt.Sprintf("%v", v["created_at"]),
				fmt.Sprintf("%v", v["created_by"]),
				fmt.Sprintf("%v", v["char_count"]),
				truncate(fmt.Sprintf("%v", v["update_reason"]), 40),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var promptsShowCmd = &cobra.Command{
	Use:   "show <companion> <version>",
	Short: "Show a specific system-prompt version",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+companionID+"/system-prompt/versions/"+args[1], &result); err != nil {
			return fmt.Errorf("reading prompt version: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		fmt.Printf("Version: %v  (%v)\n", result["version"], result["updated_at"])
		if r, ok := result["update_reason"]; ok && r != nil {
			fmt.Printf("Reason:  %v\n", r)
		}
		fmt.Println()
		fmt.Printf("%v\n", result["content"])
		return nil
	},
}

var promptsRestoreCmd = &cobra.Command{
	Use:   "restore <companion> <version>",
	Short: "Restore a previous system-prompt version",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		version, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid version %q: %w", args[1], err)
		}
		body := map[string]any{"version": version}
		var result map[string]any
		if err := client.Post(context.Background(), "/companions/"+companionID+"/system-prompt/restore", body, &result); err != nil {
			return fmt.Errorf("restoring prompt: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Restored system-prompt to version %s.", args[1])
		return nil
	},
}

// --- identity-prompt -------------------------------------------------------

var companionIdentityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Read or update a companion's identity prompt",
}

var identityShowCmd = &cobra.Command{
	Use:   "show <companion>",
	Short: "Show the active identity prompt",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+companionID+"/identity-prompt", &result); err != nil {
			return fmt.Errorf("reading identity prompt: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		fmt.Printf("Version: %v  (by %v, %v)\n", result["version"], result["created_by"], result["updated_at"])
		fmt.Println()
		fmt.Printf("%v\n", result["body"])
		return nil
	},
}

var identitySetCmd = &cobra.Command{
	Use:   "set <companion> [--file F | -]",
	Short: "Set the identity prompt (from --file or stdin)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if identityFile == "" {
			return fmt.Errorf("--file is required (use - for stdin)")
		}
		content, err := readBodyFromFileOrStdin(identityFile)
		if err != nil {
			return err
		}
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"new_identity": content}
		if identityBaseVer > 0 {
			body["base_version"] = identityBaseVer
		}
		var result map[string]any
		if err := client.Put(context.Background(), "/companions/"+companionID+"/identity-prompt", body, &result); err != nil {
			return fmt.Errorf("setting identity prompt: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Identity prompt updated (version %v).", result["version"])
		return nil
	},
}

// --- companion tools -------------------------------------------------------

var companionToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List or set a companion's enabled integrations",
}

var toolsListCmd = &cobra.Command{
	Use:   "list <companion>",
	Short: "List tools and integrations available to a companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+companionID+"/tools", &result); err != nil {
			return fmt.Errorf("listing tools: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		integrations, _ := result["integrations"].([]any)
		headers := []string{"INT ID", "TOOLKIT", "ENABLED", "STATUS"}
		var rows [][]string
		for _, item := range integrations {
			it, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", it["integration_id"]),
				truncate(fmt.Sprintf("%v", it["toolkit_name"]), 30),
				fmt.Sprintf("%v", it["enabled"]),
				fmt.Sprintf("%v", it["status"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var toolsSetCmd = &cobra.Command{
	Use:   "set <companion> --integration-ids 1,3",
	Short: "Set which integrations are enabled (sync semantics)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ids, err := parseIntCSV(toolsIntegrationIDs)
		if err != nil {
			return err
		}
		companionID, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"enabled_integration_ids": ids}
		var result map[string]any
		if err := client.Put(context.Background(), "/companions/"+companionID+"/tools", body, &result); err != nil {
			return fmt.Errorf("setting tools: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Enabled integrations set to %v.", ids)
		return nil
	},
}

func init() {
	companionPromptsCmd.PersistentFlags().IntVar(&promptLimit, "limit", 20, "max results")
	identitySetCmd.Flags().StringVar(&identityFile, "file", "", "read new identity from file (use - for stdin)")
	identitySetCmd.Flags().IntVar(&identityBaseVer, "base-version", 0, "optional base version for the update")
	toolsSetCmd.Flags().StringVar(&toolsIntegrationIDs, "integration-ids", "", "comma-separated integration ids to enable (sync)")

	companionsCmd.AddCommand(companionResumeCmd)
	companionsCmd.AddCommand(companionPromptsCmd)
	companionPromptsCmd.AddCommand(promptsVersionsCmd)
	companionPromptsCmd.AddCommand(promptsShowCmd)
	companionPromptsCmd.AddCommand(promptsRestoreCmd)
	companionsCmd.AddCommand(companionIdentityCmd)
	companionIdentityCmd.AddCommand(identityShowCmd)
	companionIdentityCmd.AddCommand(identitySetCmd)
	companionsCmd.AddCommand(companionToolsCmd)
	companionToolsCmd.AddCommand(toolsListCmd)
	companionToolsCmd.AddCommand(toolsSetCmd)
}
