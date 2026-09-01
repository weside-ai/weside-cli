package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var roomsCmd = &cobra.Command{
	Use:   "rooms",
	Short: "Manage conversation rooms",
	Long: `Inspect the v2 conversation rooms.

Rooms replace the v1 threads model (WA-1548): a conversation lives in a room,
not a thread. Use these commands to debug room state and read timelines.

Examples:
  weside rooms list
  weside rooms show 42
  weside rooms mute 42
  weside rooms unmute 42
  weside rooms activity 42
  weside rooms delete 42`,
}

var roomsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your rooms",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/rooms", &result); err != nil {
			return fmt.Errorf("listing rooms: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		rooms, _ := result["rooms"].([]any)
		total := result["total"]

		headers := []string{"ID", "KIND", "TITLE", "MUTED", "LAST MESSAGE", "UPDATED"}
		var rows [][]string
		for _, item := range rooms {
			r, _ := item.(map[string]any)
			id := fmt.Sprintf("%v", r["id"])
			kind := fmt.Sprintf("%v", r["kind"])
			title := fmt.Sprintf("%v", r["title"])
			if title == "<nil>" || title == "" {
				title = fmt.Sprintf("%v", r["auto_title"])
			}
			lastMsg := ""
			if lm, ok := r["last_message"].(map[string]any); ok {
				lastMsg = truncate(fmt.Sprintf("%v", lm["snippet"]), 40)
			}
			updated := fmt.Sprintf("%v", r["updated_at"])
			muted := ""
			if r["muted"] == true {
				muted = "yes"
			}
			rows = append(rows, []string{id, kind, truncate(title, 30), muted, lastMsg, updated})
		}

		ui.PrintTable(headers, rows)
		fmt.Printf("\n%v room(s)\n", total)
		return nil
	},
}

func setRoomMute(ctx context.Context, client *api.Client, roomID string, muted bool) (map[string]any, error) {
	result := map[string]any{}
	path := "/rooms/" + roomID + "/mute"
	var err error
	if muted {
		err = client.Put(ctx, path, nil, &result)
	} else {
		err = client.Delete(ctx, path, &result)
	}
	return result, err
}

func newRoomsMuteCommand(muted bool) *cobra.Command {
	verb := "mute"
	action := "Muting"
	success := "muted"
	if !muted {
		verb = "unmute"
		action = "Unmuting"
		success = "unmuted"
	}
	return &cobra.Command{
		Use:   verb + " <room_id>",
		Short: action + " proactive notifications for a room",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAuthenticatedClientV2()
			if err != nil {
				return err
			}
			result, err := setRoomMute(cmd.Context(), client, args[0], muted)
			if err != nil {
				return fmt.Errorf("%s room: %w", verb, err)
			}
			if IsJSON() {
				ui.PrintJSON(result)
				return nil
			}
			ui.PrintSuccess("Room %s %s.", args[0], success)
			return nil
		},
	}
}

var (
	roomsShowCursor string
	roomsShowAfter  string
	roomsShowLimit  int
)

var roomsShowCmd = &cobra.Command{
	Use:   "show <room_id>",
	Short: "Show messages in a room timeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		path := "/rooms/" + args[0] + "/messages?"
		q := url.Values{}
		q.Set("limit", strconv.Itoa(roomsShowLimit))
		if cmd.Flags().Changed("cursor") {
			q.Set("before", roomsShowCursor)
		}
		if cmd.Flags().Changed("after") {
			q.Set("after", roomsShowAfter)
		}

		var result map[string]any
		if err := client.Get(context.Background(), path+q.Encode(), &result); err != nil {
			return fmt.Errorf("getting room timeline: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		messages, _ := result["messages"].([]any)
		for _, item := range messages {
			msg, _ := item.(map[string]any)
			role := fmt.Sprintf("%v", msg["role"])
			prefix := roleLabel(role)
			if content, ok := msg["content"].([]any); ok {
				for _, block := range content {
					if b, ok := block.(map[string]any); ok {
						if text, ok := b["text"].(string); ok {
							fmt.Printf("[%s] %s\n\n", prefix, text)
						}
					}
				}
			}
		}
		if next, _ := result["next_cursor"].(string); next != "" {
			fmt.Printf("(older: --cursor %s)\n", next)
		}
		if prev, _ := result["prev_cursor"].(string); prev != "" {
			fmt.Printf("(newer: --after %s)\n", prev)
		}
		return nil
	},
}

var (
	roomsActivityLimit int
	roomsActivityScope string
)

// roomActivityQuery builds the activity request's query string. Extracted so a
// flag reaching the wire is a test rather than a claim — a --scope that never
// arrives looks exactly like a --scope the server ignores.
func roomActivityQuery(limit int, scope string) url.Values {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	// "all" is the server's default; sending it would only add noise.
	if scope != "" && scope != "all" {
		q.Set("scope", scope)
	}
	return q
}

var roomsActivityCmd = &cobra.Command{
	Use:   "activity <room_id>",
	Short: "Show what happened in a room — tools, notes, memories",
	Long: `Read the room's durable activity feed (WA-1784).

One source: every event is a tool-audit row, so a memory save and a note write
appear here as what they are rather than as three merged feeds. The feed is
scoped to your own companions' activity — a foreign companion's arguments and
output are never returned.

This feed is a newest-first slice, never a page. WA-2145 removed the cursor
along with the app's Verlauf screen, which was its only pager; --limit is the
whole surface, and the server caps it at 100.

--scope last_turn narrows it to the newest turn of each of your companions,
which is what the in-chat toolbox shows.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		q := roomActivityQuery(roomsActivityLimit, roomsActivityScope)

		var result map[string]any
		path := "/rooms/" + args[0] + "/activity?" + q.Encode()
		if err := client.Get(cmd.Context(), path, &result); err != nil {
			return fmt.Errorf("getting room activity: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		events, _ := result["events"].([]any)
		if len(events) == 0 {
			fmt.Println("No activity yet.")
			return nil
		}
		rows := make([][]string, 0, len(events))
		for _, item := range events {
			e, _ := item.(map[string]any)
			// `outcome` is nil while an invocation has no terminal row, and
			// "error" when it failed. The feed reads as a history, so a failed
			// tool must not be shown as a thing that happened.
			outcome := "…"
			switch e["outcome"] {
			case "success":
				outcome = "ok"
			case "error":
				outcome = "failed"
			}
			rows = append(rows, []string{
				fmt.Sprintf("%v", e["created_at"]),
				fmt.Sprintf("%v", e["event_kind"]),
				fmt.Sprintf("%v", e["tool_name"]),
				outcome,
				fmt.Sprintf("%v", e["companion_name"]),
			})
		}
		ui.PrintTable([]string{"When", "Kind", "Tool", "Outcome", "Who"}, rows)
		return nil
	},
}

var roomsDeleteCmd = &cobra.Command{
	Use:   "delete <room_id>",
	Short: "Delete a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/rooms/"+args[0], nil); err != nil {
			return fmt.Errorf("deleting room: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(map[string]any{"deleted": true, "id": args[0]})
			return nil
		}

		ui.PrintSuccess("Room %s deleted.", args[0])
		return nil
	},
}

// roleLabel renders a message role as a human-readable speaker prefix.
func roleLabel(role string) string {
	switch role {
	case "user":
		return "You"
	case "assistant":
		return "Companion"
	case "mentor":
		return "Mentor"
	case "system":
		return "System"
	default:
		return role
	}
}

func init() {
	roomsShowCmd.Flags().IntVar(&roomsShowLimit, "limit", 50, "max messages (1-100)")
	roomsShowCmd.Flags().StringVar(&roomsShowCursor, "cursor", "", "older-page cursor (from next_cursor)")
	roomsShowCmd.Flags().StringVar(&roomsShowAfter, "after", "", "newer-page cursor (from prev_cursor)")
	roomsCmd.AddCommand(roomsListCmd)
	roomsCmd.AddCommand(roomsShowCmd)
	roomsCmd.AddCommand(newRoomsMuteCommand(true))
	roomsCmd.AddCommand(newRoomsMuteCommand(false))
	roomsActivityCmd.Flags().IntVar(&roomsActivityLimit, "limit", 50, "maximum events to return")
	roomsActivityCmd.Flags().StringVar(&roomsActivityScope, "scope", "all", "all | last_turn (newest turn per own companion)")
	roomsCmd.AddCommand(roomsActivityCmd)
	roomsCmd.AddCommand(roomsDeleteCmd)
	rootCmd.AddCommand(roomsCmd)
}
