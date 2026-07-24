package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	memEditCompanion  string
	memGetVersion     int
	memUpdTitle       string
	memUpdType        string
	memUpdTags        string
	memUpdImportance  int
	memUpdAutoload    bool
	memEditFile       string
	memEditTags       string
	memEditImportance int
)

// memoriesGetCmd — GET /companions/{cid}/memories/{memory_id}
var memoriesGetCmd = &cobra.Command{
	Use:   "get <memory_id>",
	Short: "Show a single memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(memEditCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		path := "/companions/" + companionID + "/memories/" + args[0]
		if cmd.Flags().Changed("version") {
			path += "?version=" + strconv.Itoa(memGetVersion)
		}
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("getting memory: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		fmt.Printf("ID:       %v  (group %v)\n", result["id"], result["memory_group_id"])
		fmt.Printf("Type:     %v\n", result["type"])
		fmt.Printf("Title:    %v\n", result["title"])
		fmt.Printf("Importance: %v  Version: %v\n", result["importance"], result["version"])
		fmt.Printf("Tags:     %v\n", result["tags"])
		fmt.Printf("Date:     %v\n", result["memory_date"])
		fmt.Println()
		fmt.Printf("%v\n", result["content"])
		return nil
	},
}

var memoriesDeleteCmd = &cobra.Command{
	Use:   "delete <memory_id>",
	Short: "Delete a memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(memEditCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/companions/"+companionID+"/memories/"+args[0], nil); err != nil {
			return fmt.Errorf("deleting memory: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(map[string]any{"deleted": true, "id": args[0]})
			return nil
		}
		ui.PrintSuccess("Memory %s deleted.", args[0])
		return nil
	},
}

// memoriesUpdateCmd — PATCH (metadata only)
var memoriesUpdateCmd = &cobra.Command{
	Use:   "update <memory_id>",
	Short: "Update memory metadata (title, type, tags, importance, autoload)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(memEditCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if cmd.Flags().Changed("title") {
			body["title"] = memUpdTitle
		}
		if cmd.Flags().Changed("type") {
			body["memory_type"] = memUpdType
		}
		if cmd.Flags().Changed("tags") {
			body["tags"] = csvSlice(memUpdTags)
		}
		if cmd.Flags().Changed("importance") {
			body["importance"] = memUpdImportance
		}
		if cmd.Flags().Changed("autoload") {
			body["autoload"] = memUpdAutoload
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to update (use --title/--type/--tags/--importance/--autoload)")
		}

		var result map[string]any
		if err := client.Patch(context.Background(), "/companions/"+companionID+"/memories/"+args[0], body, &result); err != nil {
			return fmt.Errorf("updating memory: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Memory %s updated.", args[0])
		return nil
	},
}

// memoriesEditCmd — PUT (content versioning)
var memoriesEditCmd = &cobra.Command{
	Use:   "edit <memory_id> --file F",
	Short: "Replace memory content (creates a new version)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if memEditFile == "" {
			return fmt.Errorf("--file is required (use - for stdin)")
		}
		content, err := readBodyFromFileOrStdin(memEditFile)
		if err != nil {
			return err
		}
		companionID, err := resolveCompanion(memEditCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"content": content}
		if cmd.Flags().Changed("tags") {
			body["tags"] = csvSlice(memEditTags)
		}
		if cmd.Flags().Changed("importance") {
			body["importance"] = memEditImportance
		}

		var result map[string]any
		if err := client.Put(context.Background(), "/companions/"+companionID+"/memories/"+args[0], body, &result); err != nil {
			return fmt.Errorf("editing memory: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Memory %s content updated (version %v).", args[0], result["version"])
		return nil
	},
}

// csvSlice splits a comma-separated string into a []string (or nil if empty).
func csvSlice(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func init() {
	for _, c := range []*cobra.Command{memoriesGetCmd, memoriesDeleteCmd, memoriesUpdateCmd, memoriesEditCmd} {
		c.Flags().StringVar(&memEditCompanion, "companion", "", "companion id or name (default: selected companion)")
	}
	memoriesGetCmd.Flags().IntVar(&memGetVersion, "version", 0, "specific version to read")
	memoriesUpdateCmd.Flags().StringVar(&memUpdTitle, "title", "", "new title")
	memoriesUpdateCmd.Flags().StringVar(&memUpdType, "type", "", "new memory type")
	memoriesUpdateCmd.Flags().StringVar(&memUpdTags, "tags", "", "comma-separated tags")
	memoriesUpdateCmd.Flags().IntVar(&memUpdImportance, "importance", 0, "importance 1-10")
	memoriesUpdateCmd.Flags().BoolVar(&memUpdAutoload, "autoload", false, "autoload into context")
	memoriesEditCmd.Flags().StringVar(&memEditFile, "file", "", "read new content from file (use - for stdin)")
	memoriesEditCmd.Flags().StringVar(&memEditTags, "tags", "", "comma-separated tags")
	memoriesEditCmd.Flags().IntVar(&memEditImportance, "importance", 0, "importance 1-10")

	memoriesCmd.AddCommand(memoriesGetCmd)
	memoriesCmd.AddCommand(memoriesDeleteCmd)
	memoriesCmd.AddCommand(memoriesUpdateCmd)
	memoriesCmd.AddCommand(memoriesEditCmd)
}
