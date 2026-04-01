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

		var result struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		}
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing threads: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		headers := []string{"THREAD ID", "COMPANION", "LAST MESSAGE", "CREATED"}
		var rows [][]string
		for _, t := range result.Items {
			id := fmt.Sprintf("%v", t["id"])
			companion := fmt.Sprintf("%v", t["companion_name"])
			lastMsg := truncate(fmt.Sprintf("%v", t["last_message_preview"]), 40)
			created := fmt.Sprintf("%v", t["created_at"])
			rows = append(rows, []string{id, companion, lastMsg, created})
		}

		ui.PrintTable(headers, rows)
		fmt.Printf("\n%d thread(s)\n", result.Total)
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

		var result struct {
			Items []map[string]any `json:"items"`
		}
		if err := client.Get(context.Background(), "/chat/threads/"+args[0]+"/messages", &result); err != nil {
			return fmt.Errorf("getting thread messages: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		for _, msg := range result.Items {
			role := fmt.Sprintf("%v", msg["role"])
			text := fmt.Sprintf("%v", msg["text"])
			prefix := "You"
			if role == "assistant" {
				prefix = "Companion"
			}
			fmt.Printf("[%s] %s\n\n", prefix, text)
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
