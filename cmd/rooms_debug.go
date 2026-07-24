package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// P0 rooms-debug surface — the inspection/control layer over /api/v2/rooms/*.
// list/show/delete live in rooms.go; the subcommands here are the debugging
// affordances: trace, participants, tool-call detail, cancel/undo/context-break,
// rename, and room creation (dm/group).

var (
	roomsTraceLimit        int
	roomsConfirm           bool
	roomsCancelServerMsgID string
	roomsCancelPartial     string
	roomsRenameClear       bool
	roomsGroupCompanions   string
	roomsGroupTitle        string
)

var roomsTraceCmd = &cobra.Command{
	Use:   "trace <room_id>",
	Short: "Show the companion checkpoint trace of a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/rooms/%s/trace?limit=%d", args[0], roomsTraceLimit)
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("reading room trace: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		messages, _ := result["messages"].([]any)
		if len(messages) == 0 {
			fmt.Println("No trace (room has no caller-owned companion checkpoint).")
			return nil
		}
		for _, item := range messages {
			m, _ := item.(map[string]any)
			role := roleLabel(fmt.Sprintf("%v", m["role"]))
			created := fmt.Sprintf("%v", m["created_at"])
			msgID, _ := m["message_id"].(string)
			fmt.Printf("[%s] %s", role, created)
			if msgID != "" && msgID != "<nil>" {
				fmt.Printf("  (%s)", msgID)
			}
			fmt.Println()
			printTraceItems(m["trace_items"])
			fmt.Println()
		}
		return nil
	},
}

// printTraceItems renders the slim trace (tool calls + reminders) under a message.
func printTraceItems(raw any) {
	items, _ := raw.([]any)
	for _, it := range items {
		item, _ := it.(map[string]any)
		switch fmt.Sprintf("%v", item["type"]) {
		case "tool_call":
			name, _ := item["tool_name"].(string)
			args := prettyJSON(item["tool_args"])
			output, _ := item["tool_output"].(string)
			fmt.Printf("  - tool_call %s(%s)", name, args)
			if output != "" && output != "<nil>" {
				fmt.Printf(" -> %s", truncate(output, 200))
			}
			fmt.Println()
		case "reminder":
			category, _ := item["category"].(string)
			message, _ := item["message"].(string)
			if category != "" && category != "<nil>" {
				fmt.Printf("  - reminder [%s]: %s\n", category, message)
			} else {
				fmt.Printf("  - reminder: %s\n", message)
			}
		default:
			fmt.Printf("  - %v\n", item["type"])
		}
	}
}

func prettyJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

var roomsParticipantsCmd = &cobra.Command{
	Use:   "participants <room_id>",
	Short: "List the participants of a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/participants", &result); err != nil {
			return fmt.Errorf("listing participants: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		participants, _ := result["participants"].([]any)
		headers := []string{"KIND", "NAME", "ID", "ROLE", "JOINED"}
		var rows [][]string
		for _, item := range participants {
			p, _ := item.(map[string]any)
			kind := fmt.Sprintf("%v", p["kind"])
			name := fmt.Sprintf("%v", p["display_name"])
			id := participantID(p)
			role := fmt.Sprintf("%v", p["role"])
			joined := fmt.Sprintf("%v", p["joined_at"])
			rows = append(rows, []string{kind, name, id, role, joined})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

// participantID picks the relevant id field for a participant's kind.
func participantID(p map[string]any) string {
	for _, k := range []string{"user_id", "companion_id", "external_id"} {
		if v, ok := p[k]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return "-"
}

var roomsToolCallCmd = &cobra.Command{
	Use:   "tool-call <room_id> <tool_call_id>",
	Short: "Show full detail of a companion tool call in a room",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/tool-calls/"+args[1], &result); err != nil {
			return fmt.Errorf("reading tool call: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		if fmt.Sprintf("%v", result["detail_level"]) == "metadata_only" {
			fmt.Println("metadata only (tool call not visible to you)")
			return nil
		}
		fmt.Printf("Tool:    %s\n", result["tool_name"])
		fmt.Printf("Args:    %s\n", prettyJSON(result["args"]))
		out, _ := result["output"].(string)
		if pending, _ := result["pending"].(bool); pending {
			fmt.Println("Output:  (still running)")
		} else if out != "" && out != "<nil>" {
			fmt.Printf("Output:  %s\n", out)
		} else {
			fmt.Println("Output:  (none)")
		}
		return nil
	},
}

var roomsCancelCmd = &cobra.Command{
	Use:   "cancel <room_id>",
	Short: "Cancel the running companion turn in a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this turns off a running companion turn — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if roomsCancelServerMsgID != "" {
			body["server_message_id"] = roomsCancelServerMsgID
		}
		if roomsCancelPartial != "" {
			body["partial_content"] = roomsCancelPartial
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/turns/cancel", body, &result); err != nil {
			return fmt.Errorf("cancelling turn: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		if cancelled, _ := result["cancelled"].(bool); cancelled {
			ui.PrintSuccess("Turn cancelled.")
		} else {
			fmt.Println("No running turn to cancel.")
		}
		return nil
	},
}

var roomsUndoCmd = &cobra.Command{
	Use:   "undo <room_id>",
	Short: "Undo the newest 1:1 turn in a room (checkpoints + timeline)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this rolls back the last turn — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/undo", nil, &result); err != nil {
			return fmt.Errorf("undoing turn: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Undid turn (%d message(s) removed).", len(asSlice(result["removed_message_ids"])))
		if txt, _ := result["user_message_text"].(string); txt != "" {
			fmt.Printf("Your message: %s\n", txt)
		}
		return nil
	},
}

var roomsContextBreakCmd = &cobra.Command{
	Use:   "context-break <room_id>",
	Short: "Rotate the room's companion thread (force a fresh context)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this rotates the companion's context — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/context-break", nil, &result); err != nil {
			return fmt.Errorf("breaking context: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Context broken (marker message %v).", result["message_id"])
		return nil
	},
}

var roomsRenameCmd = &cobra.Command{
	Use:   "rename <room_id> <title>",
	Short: "Set or clear a room's title",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		body := map[string]any{"title": args[1]}
		if roomsRenameClear {
			body["title"] = nil
		}
		var result map[string]any
		if err := client.Patch(context.Background(), "/rooms/"+args[0], body, &result); err != nil {
			return fmt.Errorf("renaming room: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Room %s renamed.", args[0])
		return nil
	},
}

var roomsGroupCmd = &cobra.Command{
	Use:   "group --companions 1,2 [--title T]",
	Short: "Create a group room seated with your companions",
	RunE: func(_ *cobra.Command, _ []string) error {
		ids, err := parseIntCSV(roomsGroupCompanions)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		body := map[string]any{"companion_ids": ids}
		if roomsGroupTitle != "" {
			body["title"] = roomsGroupTitle
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/group", body, &result); err != nil {
			return fmt.Errorf("creating group room: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Group room created (ID: %v).", result["id"])
		return nil
	},
}

var roomsDmCmd = &cobra.Command{
	Use:   "dm <companion_id>",
	Short: "Resolve (or create) the 1:1 DM room with a companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/dm/"+args[0], nil, &result); err != nil {
			return fmt.Errorf("resolving DM room: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("DM room with companion %s (ID: %v).", args[0], result["id"])
		return nil
	},
}

// parseIntCSV turns "1,2,3" into []int.
func parseIntCSV(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("no ids provided")
	}
	parts := strings.Split(s, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", p, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// asSlice coerces an any into a []any (nil if not a slice).
func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func init() {
	roomsTraceCmd.Flags().IntVar(&roomsTraceLimit, "limit", 50, "max trace rows (1-200)")
	roomsCancelCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsCancelCmd.Flags().StringVar(&roomsCancelServerMsgID, "server-message-id", "", "cancel a specific turn")
	roomsCancelCmd.Flags().StringVar(&roomsCancelPartial, "partial-content", "", "persist partial content before cancelling")
	roomsUndoCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsContextBreakCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsRenameCmd.Flags().BoolVar(&roomsRenameClear, "clear", false, "clear the title instead of setting it")
	roomsGroupCmd.Flags().StringVar(&roomsGroupCompanions, "companions", "", "comma-separated companion ids (required)")
	roomsGroupCmd.Flags().StringVar(&roomsGroupTitle, "title", "", "optional room title")
	_ = roomsGroupCmd.MarkFlagRequired("companions")

	roomsCmd.AddCommand(roomsTraceCmd)
	roomsCmd.AddCommand(roomsParticipantsCmd)
	roomsCmd.AddCommand(roomsToolCallCmd)
	roomsCmd.AddCommand(roomsCancelCmd)
	roomsCmd.AddCommand(roomsUndoCmd)
	roomsCmd.AddCommand(roomsContextBreakCmd)
	roomsCmd.AddCommand(roomsRenameCmd)
	roomsCmd.AddCommand(roomsGroupCmd)
	roomsCmd.AddCommand(roomsDmCmd)
}
