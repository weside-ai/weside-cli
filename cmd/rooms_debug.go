package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// P0 rooms-debug surface — the inspection/control layer over /api/v2/rooms/*.
// list/show/delete live in rooms.go; the subcommands here are the debugging
// affordances: trace, participants, tool-call detail, cancel/undo/context-break,
// rename, and room creation (dm/group).

var (
	roomsTraceLimit        int
	roomsTraceFull         bool
	roomsConfirm           bool
	roomsCancelServerMsgID string
	roomsCancelPartial     string
	roomsRenameClear       bool
	roomsGroupCompanions   string
	roomsGroupTitle        string
	eventsSince            string
	eventsRaw              bool
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
			printTraceItems(m["trace_items"], roomsTraceFull)
			fmt.Println()
		}
		return nil
	},
}

// printTraceItems renders the slim trace (tool calls + reminders) under a message.
// Tool output is truncated unless full is true.
func printTraceItems(raw any, full bool) {
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
				if full {
					fmt.Printf(" -> %s", output)
				} else {
					fmt.Printf(" -> %s", truncate(output, 200))
				}
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
	Use:   "rename <room_id> [title]",
	Short: "Set or clear a room's title",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("clear") && len(args) < 2 {
			return fmt.Errorf("title is required (or use --clear to remove it)")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if roomsRenameClear {
			body["title"] = nil
		} else {
			body["title"] = args[1]
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

// roomsEventsCmd — raw SSE mitschnitt of a room's event stream. Prints every
// frame (known and unknown types) for debugging; unlike `chat` it does not
// interpret or wait for a turn to end.
var roomsEventsCmd = &cobra.Command{
	Use:   "events <room_id>",
	Short: "Stream raw room SSE events (debug)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		path := "/rooms/" + args[0] + "/events"
		if eventsSince != "" {
			// Cursors are opaque tokens — encode so any character survives.
			path += "?since=" + url.QueryEscape(eventsSince)
		}
		resp, err := client.Subscribe(cmd.Context(), path)
		if err != nil {
			return fmt.Errorf("opening event stream: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var (
			curID, eventType string
			dataLines        []string
			flush            = func() {
				if eventType == "" && curID == "" && len(dataLines) == 0 {
					return
				}
				data := strings.Join(dataLines, "\n")
				if eventsRaw {
					fmt.Printf("%s %s %s\n", orDash(eventType), orDash(curID), data)
				} else {
					if curID != "" {
						fmt.Printf("id: %s\n", curID)
					}
					if eventType != "" {
						fmt.Printf("event: %s\n", eventType)
					}
					if data != "" {
						fmt.Printf("data: %s\n", prettyData(data))
					}
					fmt.Println()
				}
				curID, eventType = "", ""
				dataLines = nil
			}
		)
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				flush()
			case strings.HasPrefix(line, ":"):
				// SSE comment (e.g. ":heartbeat") — surface it.
				if !eventsRaw {
					fmt.Printf("%s\n", line)
				}
			case strings.HasPrefix(line, "id: "):
				curID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				eventType = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			}
		}
		flush()
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading event stream: %w", err)
		}
		return nil
	},
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// prettyData re-indents a JSON data line for readability; returns the input
// unchanged if it is not JSON.
func prettyData(data string) string {
	var pretty bytes.Buffer
	if json.Indent(&pretty, []byte(data), "  ", "  ") == nil {
		return "\n  " + pretty.String()
	}
	return data
}

// rooms invites subgroup — list/create/revoke room invites.
var roomsInvitesCmd = &cobra.Command{
	Use:   "invites",
	Short: "Manage a group room's invites",
}

var roomsInvitesListCmd = &cobra.Command{
	Use:   "list <room_id>",
	Short: "List a room's active invites",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/invites", &result); err != nil {
			return fmt.Errorf("listing invites: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		invites, _ := result["invites"].([]any)
		headers := []string{"ID", "CODE", "EXPIRES"}
		var rows [][]string
		for _, item := range invites {
			inv, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", inv["id"]),
				fmt.Sprintf("%v", inv["code"]),
				fmt.Sprintf("%v", inv["expires_at"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var roomsInvitesCreateCmd = &cobra.Command{
	Use:   "create <room_id>",
	Short: "Mint a 7-day multi-use invite for a group room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/invites", nil, &result); err != nil {
			return fmt.Errorf("creating invite: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Invite created: %v (expires %v)", result["code"], result["expires_at"])
		return nil
	},
}

// acceptInvite / previewInvite are extracted from their commands so a test can
// drive the REAL path construction against an httptest server. A test that
// posts a hand-written path only ever proves itself — and the mistake worth
// catching here is precisely a wrong path: an accept is code-scoped
// (/rooms/invites/<code>/accept), not room-scoped like its siblings, and a
// wrong one returns a 404 indistinguishable from the deliberate uniform 404
// for an invalid code.
func acceptInvite(
	ctx context.Context, client *api.Client, code string,
) (map[string]any, error) {
	var result map[string]any
	if err := client.Post(ctx, "/rooms/invites/"+code+"/accept", nil, &result); err != nil {
		return nil, fmt.Errorf("accepting invite: %w", err)
	}
	return result, nil
}

func previewInvite(
	ctx context.Context, client *api.Client, code string,
) (map[string]any, error) {
	var result map[string]any
	if err := client.Get(ctx, "/rooms/invites/"+code, &result); err != nil {
		return nil, fmt.Errorf("previewing invite: %w", err)
	}
	return result, nil
}

var roomsInvitesAcceptCmd = &cobra.Command{
	Use:   "accept <code>",
	Short: "Redeem an invite code and join the group room",
	Long: `Redeem an invite code as the logged-in user and join that group room.

The other half of ` + "`invites create`" + `: minting could be driven from the CLI
while redeeming could not, so verifying human-to-human rooms needed a raw API
call for the one step that requires the SECOND identity — exactly the step worth
having a verb for.

An unknown, expired, revoked or block-gated code all answer the same 404 by
design, so the error here cannot tell you which it was.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := acceptInvite(context.Background(), client, args[0])
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Joined room %v (%v).", result["id"], result["title"])
		return nil
	},
}

var roomsInvitesPreviewCmd = &cobra.Command{
	Use:   "preview <code>",
	Short: "Show what an invite code reveals before joining",
	Long: `Read the four public fields a held code exposes: room title, who invited,
how many humans and how many companions are in the room.

Deliberately minimal — a code is not a directory entry, so this shows enough to
decide whether to join and nothing more.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := previewInvite(context.Background(), client, args[0])
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess(
			"%v — invited by %v · %v humans, %v companions",
			result["room_title"], result["inviter_display_name"],
			result["human_count"], result["companion_count"],
		)
		return nil
	},
}

var roomsInvitesRevokeCmd = &cobra.Command{
	Use:   "revoke <room_id> <invite_id>",
	Short: "Revoke an active invite",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		if err := client.Delete(context.Background(), "/rooms/"+args[0]+"/invites/"+args[1], nil); err != nil {
			return fmt.Errorf("revoking invite: %w", err)
		}
		ui.PrintSuccess("Invite %s revoked.", args[1])
		return nil
	},
}

func init() {
	roomsTraceCmd.Flags().IntVar(&roomsTraceLimit, "limit", 50, "max trace rows (1-200)")
	roomsTraceCmd.Flags().BoolVar(&roomsTraceFull, "full", false, "show full tool output (no truncation)")
	roomsCancelCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsCancelCmd.Flags().StringVar(&roomsCancelServerMsgID, "server-message-id", "", "cancel a specific turn")
	roomsCancelCmd.Flags().StringVar(&roomsCancelPartial, "partial-content", "", "persist partial content before cancelling")
	roomsUndoCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsContextBreakCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsRenameCmd.Flags().BoolVar(&roomsRenameClear, "clear", false, "clear the title instead of setting it")
	roomsGroupCmd.Flags().StringVar(&roomsGroupCompanions, "companions", "", "comma-separated companion ids (required)")
	roomsGroupCmd.Flags().StringVar(&roomsGroupTitle, "title", "", "optional room title")
	_ = roomsGroupCmd.MarkFlagRequired("companions")
	roomsEventsCmd.Flags().StringVar(&eventsSince, "since", "", "resume from an SSE cursor")
	roomsEventsCmd.Flags().BoolVar(&eventsRaw, "raw", false, "one line per frame: <type> <cursor> <json>")

	roomsCmd.AddCommand(roomsTraceCmd)
	roomsCmd.AddCommand(roomsParticipantsCmd)
	roomsCmd.AddCommand(roomsToolCallCmd)
	roomsCmd.AddCommand(roomsCancelCmd)
	roomsCmd.AddCommand(roomsUndoCmd)
	roomsCmd.AddCommand(roomsContextBreakCmd)
	roomsCmd.AddCommand(roomsRenameCmd)
	roomsCmd.AddCommand(roomsGroupCmd)
	roomsCmd.AddCommand(roomsDmCmd)
	roomsCmd.AddCommand(roomsEventsCmd)
	roomsCmd.AddCommand(roomsInvitesCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesListCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesCreateCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesRevokeCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesAcceptCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesPreviewCmd)
}
