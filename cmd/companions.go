package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/auth"
	"github.com/weside-ai/weside-cli/internal/config"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var companionsCmd = &cobra.Command{
	Use:   "companions",
	Short: "Manage your Companions",
}

var companionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all your Companions",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/companions", &result); err != nil {
			return fmt.Errorf("listing companions: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		companions, _ := result["companions"].([]any)
		total := result["total"]

		defaultComp := config.GetDefaultCompanion()

		headers := []string{"ID", "NAME", "PERSONALITY", "DEFAULT"}
		var rows [][]string
		for _, item := range companions {
			c, _ := item.(map[string]any)
			id := fmt.Sprintf("%v", c["id"])
			name := fmt.Sprintf("%v", c["name"])
			personality := truncate(fmt.Sprintf("%v", c["personality"]), 40)
			def := ""
			if name == defaultComp {
				def = "*"
			}
			rows = append(rows, []string{id, name, personality, def})
		}

		ui.PrintTable(headers, rows)
		ui.Printf("\n%v companion(s)\n", total)
		return nil
	},
}

var companionsShowCmd = &cobra.Command{
	Use:   "show <id|name>",
	Short: "Show Companion details",
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

		var companion map[string]any
		if err := client.Get(context.Background(), "/companions/"+companionID, &companion); err != nil {
			return fmt.Errorf("getting companion: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(companion)
			return nil
		}

		ui.Printf("ID:                %v\n", companion["id"])
		ui.Printf("Name:              %v\n", companion["name"])
		ui.Printf("Personality:       %v\n", companion["personality"])
		ui.Printf("Published:         %v\n", companion["is_published"])
		ui.Printf("Category:          %v\n", companion["category"])

		if tags, ok := companion["tags"].([]any); ok && len(tags) > 0 {
			tagStrs := make([]string, len(tags))
			for i, t := range tags {
				tagStrs[i] = fmt.Sprintf("%v", t)
			}
			ui.Printf("Tags:              %s\n", strings.Join(tagStrs, ", "))
		} else {
			ui.Printf("Tags:              \n")
		}

		ui.Printf("Short Description: %v\n", companion["short_description"])
		ui.Printf("Avatar:            %s\n", imageURL(companion["avatar"]))
		ui.Printf("Banner:            %s\n", imageURL(companion["banner"]))

		if created, ok := companion["created_at"]; ok {
			ui.Printf("Created:           %v\n", created)
		}
		if updated, ok := companion["updated_at"]; ok {
			ui.Printf("Updated:           %v\n", updated)
		}

		if sp, ok := companion["system_prompt"]; ok && sp != nil && fmt.Sprintf("%v", sp) != "" {
			ui.Printf("\nSystem Prompt:\n%s\n", fmt.Sprintf("%v", sp))
		}

		return nil
	},
}

var (
	compName        string
	compPersonality string
)

var companionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Companion",
	RunE: func(_ *cobra.Command, _ []string) error {
		if compName == "" {
			return fmt.Errorf("--name is required")
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]string{
			"name":        compName,
			"personality": compPersonality,
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/companions", body, &result); err != nil {
			return fmt.Errorf("creating companion: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		ui.PrintSuccess("Companion %q created (ID: %v)", result["name"], result["id"])
		return nil
	},
}

var companionsSelectCmd = &cobra.Command{
	Use:   "select <name>",
	Short: "Set the default Companion for chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]

		// Verify the companion exists
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var listResult map[string]any
		if err := client.Get(context.Background(), "/companions", &listResult); err != nil {
			return fmt.Errorf("listing companions: %w", err)
		}
		companions, _ := listResult["companions"].([]any)

		found := false
		var foundID string
		for _, item := range companions {
			c, _ := item.(map[string]any)
			if fmt.Sprintf("%v", c["name"]) == name {
				found = true
				foundID = fmt.Sprintf("%v", c["id"])
				break
			}
		}

		if !found {
			// Try by ID
			for _, item := range companions {
				c, _ := item.(map[string]any)
				if fmt.Sprintf("%v", c["id"]) == name {
					found = true
					foundID = name
					name = fmt.Sprintf("%v", c["name"])
					break
				}
			}
		}

		if !found {
			return fmt.Errorf("companion %q not found", name)
		}

		if err := config.SetDefaultCompanion(name, foundID); err != nil {
			return fmt.Errorf("saving default companion: %w", err)
		}

		ui.PrintSuccess("Default Companion set to %q (ID: %s)", name, foundID)
		return nil
	},
}

// Update flags
var (
	compUpdateName             string
	compUpdatePersonality      string
	compUpdateSystemPrompt     string
	compUpdateSystemPromptFile string
	compUpdateShortDescription string
	compUpdateCategory         string
	compUpdateTags             string
	compUpdatePublish          bool
	compUpdateUnpublish        bool
)

// readSystemPrompt reads the system prompt from --system-prompt, --system-prompt-file, or stdin (-).
func readSystemPrompt(cmd *cobra.Command, inline, file string) (prompt string, found bool, err error) {
	hasInline := cmd.Flags().Changed("system-prompt")
	hasFile := cmd.Flags().Changed("system-prompt-file")

	if hasInline && hasFile {
		return "", false, fmt.Errorf("--system-prompt and --system-prompt-file are mutually exclusive")
	}

	if hasInline {
		if inline == "-" {
			data, err := io.ReadAll(stdinReader)
			if err != nil {
				return "", false, fmt.Errorf("reading stdin: %w", err)
			}
			return string(data), true, nil
		}
		return inline, true, nil
	}

	if hasFile {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", false, fmt.Errorf("reading system prompt file: %w", err)
		}
		return string(data), true, nil
	}

	return "", false, nil
}

// stdinReader is the reader used for stdin input (injectable for testing).
var stdinReader io.Reader = os.Stdin

var companionsUpdateCmd = &cobra.Command{
	Use:   "update <id|name>",
	Short: "Update a Companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if compUpdatePublish && compUpdateUnpublish {
			return fmt.Errorf("--publish and --unpublish are mutually exclusive")
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		companionID, err := resolveCompanionID(client, args[0])
		if err != nil {
			return err
		}

		// Build PATCH body with only the fields that were explicitly set
		body := map[string]any{}

		if cmd.Flags().Changed("name") {
			body["name"] = compUpdateName
		}
		if cmd.Flags().Changed("personality") {
			body["personality"] = compUpdatePersonality
		}
		if cmd.Flags().Changed("short-description") {
			body["short_description"] = compUpdateShortDescription
		}
		if cmd.Flags().Changed("category") {
			body["category"] = compUpdateCategory
		}
		if cmd.Flags().Changed("tags") {
			if compUpdateTags == "" {
				body["tags"] = []string{}
			} else {
				parts := strings.Split(compUpdateTags, ",")
				tags := make([]string, 0, len(parts))
				for _, p := range parts {
					if t := strings.TrimSpace(p); t != "" {
						tags = append(tags, t)
					}
				}
				body["tags"] = tags
			}
		}
		if compUpdatePublish {
			body["is_published"] = true
		}
		if compUpdateUnpublish {
			body["is_published"] = false
		}

		sp, hasPrompt, err := readSystemPrompt(cmd, compUpdateSystemPrompt, compUpdateSystemPromptFile)
		if err != nil {
			return err
		}
		if hasPrompt {
			body["system_prompt"] = sp
		}

		if len(body) == 0 {
			return fmt.Errorf("no fields provided — specify at least one flag to update")
		}

		var result map[string]any
		if err := client.Patch(context.Background(), "/companions/"+companionID, body, &result); err != nil {
			return fmt.Errorf("updating companion: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		ui.PrintSuccess("Companion %q updated (ID: %v)", result["name"], result["id"])
		return nil
	},
}

// Delete flags
var compDeleteYes bool

var companionsDeleteCmd = &cobra.Command{
	Use:   "delete <id|name>",
	Short: "Delete a Companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !compDeleteYes {
			return fmt.Errorf(
				"refusing to delete without --yes flag. Use 'weside companions delete %s --yes' to confirm",
				args[0],
			)
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		companionID, err := resolveCompanionID(client, args[0])
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/companions/"+companionID, nil); err != nil {
			return fmt.Errorf("deleting companion: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(map[string]any{"deleted": true, "id": companionID})
			return nil
		}

		ui.PrintSuccess("Companion %q deleted", args[0])
		return nil
	},
}

func newAuthenticatedClient() (*api.Client, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, err
	}
	return api.NewClient(GetAPIURL()+"/api/v1", token), nil
}

// newAuthenticatedClientV2 builds a client against the /api/v2 surface — the
// rooms/chat v2 contract (WA-1548) lives there. v1 stays the default for the
// legacy read commands (companions, memories, goals, provider) which have not
// been migrated; chat + rooms use v2.
func newAuthenticatedClientV2() (*api.Client, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, err
	}
	return api.NewClient(GetAPIURL()+"/api/v2", token), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func resolveCompanionID(client *api.Client, nameOrID string) (string, error) {
	// If it's a number, assume ID
	if _, err := strconv.Atoi(nameOrID); err == nil {
		return nameOrID, nil
	}

	// Otherwise, look up by name
	var result map[string]any
	if err := client.Get(context.Background(), "/companions", &result); err != nil {
		return "", fmt.Errorf("listing companions: %w", err)
	}
	companions, _ := result["companions"].([]any)

	for _, item := range companions {
		c, _ := item.(map[string]any)
		if fmt.Sprintf("%v", c["name"]) == nameOrID {
			return fmt.Sprintf("%v", c["id"]), nil
		}
	}

	return "", fmt.Errorf("companion %q not found", nameOrID)
}

var companionsIdentityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Load the active Companion's full identity via MCP",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newMCPClient()
		if err != nil {
			return err
		}

		result, err := client.CallTool(context.Background(), "get_companion_identity", map[string]any{})
		if err != nil {
			return fmt.Errorf("loading companion identity: %w", err)
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
		if jsonErr := json.Unmarshal(result, &callResult); jsonErr == nil && len(callResult.Content) > 0 {
			for _, c := range callResult.Content {
				ui.Println(c.Text)
			}
			return nil
		}

		// Fallback: print raw
		ui.Println(string(result))
		return nil
	},
}

// companionsDayCmd reads the companion's day ring (WA-2421): its last 24 hours
// as arcs (talked, dreaming, reached_out, own_time) and dots (remembered,
// acted), times and kinds only. Owner-only; anyone else gets the 404.
var companionsDayCmd = &cobra.Command{
	Use:   "day <id|name>",
	Short: "Show what a companion did in the last 24 hours",
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
		if err := client.Get(context.Background(), "/companions/"+companionID+"/day", &result); err != nil {
			return fmt.Errorf("reading companion day: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		printCompanionDay(result)
		return nil
	},
}

// printCompanionDay renders the ring as one row per mark, in the owner's zone
// the server already applied. A resting companion has no marks.
func printCompanionDay(day map[string]any) {
	fmt.Printf("Window:   %s → %s (%v)\n", clockTime(day["window_start"]), clockTime(day["window_end"]), day["timezone"])
	if since, ok := day["resting_since"].(string); ok && since != "" {
		fmt.Printf("Resting since %s\n", clockTime(since))
	}
	var rows [][]string
	for _, item := range anySlice(day["arcs"]) {
		arc, _ := item.(map[string]any)
		detail := ""
		if room, ok := arc["room_id"]; ok {
			detail = fmt.Sprintf("room %v", room)
		}
		rows = append(rows, []string{"arc", fmt.Sprintf("%v", arc["kind"]), clockTime(arc["start"]) + "–" + clockTime(arc["end"]), detail})
	}
	for _, item := range anySlice(day["dots"]) {
		dot, _ := item.(map[string]any)
		detail := ""
		if category, ok := dot["category"]; ok {
			detail = fmt.Sprintf("%v", category)
		}
		rows = append(rows, []string{"dot", fmt.Sprintf("%v", dot["kind"]), clockTime(dot["at"]), detail})
	}
	ui.PrintTable([]string{"MARK", "KIND", "TIME", "DETAIL"}, rows)
}

// clockTime shortens an ISO-8601 instant to its date and wall-clock minute
// ("2026-10-04 14:05"), keeping the offset-free local reading the server sent.
func clockTime(value any) string {
	s, _ := value.(string)
	if len(s) < 16 {
		return s
	}
	return strings.Replace(s[:16], "T", " ", 1)
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

// companionsSleepCmd puts a companion to sleep on the owner's instruction —
// the counterpart of `companions resume`. Idempotent server-side: sleeping a
// sleeping companion keeps its first `suspended_at`.
var companionsSleepCmd = &cobra.Command{
	Use:   "sleep <id|name>",
	Short: "Put a companion to sleep",
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
		if err := client.Post(context.Background(), "/companions/"+companionID+"/presence/sleep", nil, &result); err != nil {
			return fmt.Errorf("putting companion to sleep: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Companion %s is asleep.", args[0])
		return nil
	},
}

func init() {
	companionsCreateCmd.Flags().StringVar(&compName, "name", "", "companion name")
	companionsCreateCmd.Flags().StringVar(&compPersonality, "personality", "", "companion personality description")

	companionsUpdateCmd.Flags().StringVar(&compUpdateName, "name", "", "new companion name")
	companionsUpdateCmd.Flags().StringVar(&compUpdatePersonality, "personality", "", "new personality description")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPrompt, "system-prompt", "", "new system prompt (use '-' to read from stdin)")
	companionsUpdateCmd.Flags().StringVar(&compUpdateSystemPromptFile, "system-prompt-file", "", "path to file containing new system prompt")
	companionsUpdateCmd.Flags().StringVar(&compUpdateShortDescription, "short-description", "", "short description for Experts/Circle")
	companionsUpdateCmd.Flags().StringVar(&compUpdateCategory, "category", "", "category (e.g. experts, wellness)")
	companionsUpdateCmd.Flags().StringVar(&compUpdateTags, "tags", "", "comma-separated tags (e.g. 'ai,productivity')")
	companionsUpdateCmd.Flags().BoolVar(&compUpdatePublish, "publish", false, "publish companion to Experts/Circle")
	companionsUpdateCmd.Flags().BoolVar(&compUpdateUnpublish, "unpublish", false, "unpublish companion from Experts/Circle")

	companionsDeleteCmd.Flags().BoolVarP(&compDeleteYes, "yes", "y", false, "confirm deletion without prompt")

	companionsCmd.AddCommand(companionsListCmd)
	companionsCmd.AddCommand(companionsShowCmd)
	companionsCmd.AddCommand(companionsCreateCmd)
	companionsCmd.AddCommand(companionsSelectCmd)
	companionsCmd.AddCommand(companionsIdentityCmd)
	companionsCmd.AddCommand(companionsUpdateCmd)
	companionsCmd.AddCommand(companionsDeleteCmd)
	companionsCmd.AddCommand(companionsDayCmd)
	companionsCmd.AddCommand(companionsSleepCmd)
	rootCmd.AddCommand(companionsCmd)
}

// imageURL returns the signed URL of an image set's original, or "" when the
// companion has none. The server sends each picture as `{original, thumb}`,
// each a presigned media-store URL valid for 20–30 minutes, and null when
// there is no picture or the store did not answer (WA-2349).
func imageURL(set any) string {
	images, ok := set.(map[string]any)
	if !ok {
		return ""
	}
	original, ok := images["original"].(map[string]any)
	if !ok {
		return ""
	}
	url, _ := original["url"].(string)
	return url
}
