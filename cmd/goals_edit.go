package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	goalEditCompanion    string
	goalEditTitle        string
	goalEditContent      string
	goalEditTags         string
	goalEditDue          string
	goalEditFollowUp     string
	goalReorderCompanion string
	goalReorderIDs       string
)

// goalsEditCmd — PATCH .../goal-content (content/meta of a goal by group_id)
var goalsEditCmd = &cobra.Command{
	Use:   "edit <group_id>",
	Short: "Edit a goal's content, title, tags, or dates",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(goalEditCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if cmd.Flags().Changed("title") {
			body["title"] = goalEditTitle
		}
		if cmd.Flags().Changed("content") {
			body["content"] = goalEditContent
		}
		if cmd.Flags().Changed("tags") {
			body["tags"] = csvSlice(goalEditTags)
		}
		if cmd.Flags().Changed("due") {
			body["due_date"] = goalEditDue
		}
		if cmd.Flags().Changed("follow-up") {
			body["follow_up_date"] = goalEditFollowUp
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to edit (use --title/--content/--tags/--due/--follow-up)")
		}

		var result map[string]any
		if err := client.Patch(context.Background(), "/companions/"+companionID+"/memories/"+args[0]+"/goal-content", body, &result); err != nil {
			return fmt.Errorf("editing goal: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Goal %s updated.", args[0])
		return nil
	},
}

// goalsReorderCmd — PUT .../goals/reorder with memory_ids_ordered
var goalsReorderCmd = &cobra.Command{
	Use:   "reorder --ids 1,2,3",
	Short: "Reorder active goals",
	RunE: func(_ *cobra.Command, _ []string) error {
		ids, err := parseIntCSV(goalReorderIDs)
		if err != nil {
			return err
		}
		companionID, err := resolveCompanion(goalReorderCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"memory_ids_ordered": ids}
		if err := client.Put(context.Background(), "/companions/"+companionID+"/memories/goals/reorder", body, nil); err != nil {
			return fmt.Errorf("reordering goals: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(map[string]any{"reordered": ids})
			return nil
		}
		ui.PrintSuccess("Goals reordered (%d).", len(ids))
		return nil
	},
}

func init() {
	goalsEditCmd.Flags().StringVar(&goalEditCompanion, "companion", "", "companion id or name (default: selected companion)")
	goalsEditCmd.Flags().StringVar(&goalEditTitle, "title", "", "new title")
	goalsEditCmd.Flags().StringVar(&goalEditContent, "content", "", "new content")
	goalsEditCmd.Flags().StringVar(&goalEditTags, "tags", "", "comma-separated tags")
	goalsEditCmd.Flags().StringVar(&goalEditDue, "due", "", "due date (YYYY-MM-DD; empty clears)")
	goalsEditCmd.Flags().StringVar(&goalEditFollowUp, "follow-up", "", "follow-up date (YYYY-MM-DD; empty clears)")
	goalsReorderCmd.Flags().StringVar(&goalReorderCompanion, "companion", "", "companion id or name (default: selected companion)")
	goalsReorderCmd.Flags().StringVar(&goalReorderIDs, "ids", "", "comma-ordered goal memory ids (required)")
	_ = goalsReorderCmd.MarkFlagRequired("ids")

	goalsCmd.AddCommand(goalsEditCmd)
	goalsCmd.AddCommand(goalsReorderCmd)
}
