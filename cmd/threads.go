package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var threadCompanionFilter string

var threadsCmd = &cobra.Command{
	Use:   "threads",
	Short: "Manage conversation threads",
}

var threadsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your conversation threads",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		path := "/chat/threads"
		if threadCompanionFilter != "" {
			path += "?companion_id=" + threadCompanionFilter
		}

		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing threads: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		threads, _ := result["threads"].([]any)
		total := result["total"]

		headers := []string{"THREAD ID", "TITLE", "MESSAGES", "LAST MESSAGE", "CREATED"}
		var rows [][]string
		for _, item := range threads {
			t, _ := item.(map[string]any)
			id := fmt.Sprintf("%v", t["id"])
			title := truncate(fmt.Sprintf("%v", t["title"]), 30)
			msgCount := fmt.Sprintf("%v", t["message_count"])
			lastMsg := truncate(fmt.Sprintf("%v", t["last_message"]), 30)
			created := fmt.Sprintf("%v", t["created_at"])
			rows = append(rows, []string{id, title, msgCount, lastMsg, created})
		}

		ui.PrintTable(headers, rows)
		fmt.Printf("\n%v thread(s)\n", total)
		return nil
	},
}

var threadsShowCmd = &cobra.Command{
	Use:   "show <thread_id>",
	Short: "Show messages in a thread",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/chat/threads/"+args[0]+"/messages", &result); err != nil {
			return fmt.Errorf("getting thread messages: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		messages, _ := result["messages"].([]any)
		for _, item := range messages {
			msg, _ := item.(map[string]any)
			role := fmt.Sprintf("%v", msg["role"])
			prefix := "You"
			if role == "assistant" {
				prefix = "Companion"
			}
			// Content is [{type: "text", text: "..."}]
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
		return nil
	},
}

var threadsDeleteCmd = &cobra.Command{
	Use:   "delete <thread_id>",
	Short: "Delete a thread",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/chat/threads/"+args[0], nil); err != nil {
			return fmt.Errorf("deleting thread: %w", err)
		}

		ui.PrintSuccess("Thread %s deleted.", args[0])
		return nil
	},
}

func init() {
	threadsListCmd.Flags().StringVar(&threadCompanionFilter, "companion", "", "filter by companion ID")
	threadsCmd.AddCommand(threadsListCmd)
	threadsCmd.AddCommand(threadsShowCmd)
	threadsCmd.AddCommand(threadsDeleteCmd)
	rootCmd.AddCommand(threadsCmd)
}
