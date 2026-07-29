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
	roomsCmd.AddCommand(roomsDeleteCmd)
	rootCmd.AddCommand(roomsCmd)
}
