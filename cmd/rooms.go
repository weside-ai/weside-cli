package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
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

		headers := []string{"ID", "KIND", "TITLE", "LAST MESSAGE", "UPDATED"}
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
			rows = append(rows, []string{id, kind, truncate(title, 30), lastMsg, updated})
		}

		ui.PrintTable(headers, rows)
		fmt.Printf("\n%v room(s)\n", total)
		return nil
	},
}

var roomsShowCmd = &cobra.Command{
	Use:   "show <room_id>",
	Short: "Show messages in a room timeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/messages", &result); err != nil {
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
			fmt.Printf("(more: --cursor %s)\n", next)
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
	roomsCmd.AddCommand(roomsListCmd)
	roomsCmd.AddCommand(roomsShowCmd)
	roomsCmd.AddCommand(roomsDeleteCmd)
	rootCmd.AddCommand(roomsCmd)
}
