package cmd

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var stageCmd = &cobra.Command{
	Use:   "stage",
	Short: "Stage artifacts a companion built for you",
	Long: `Read and remove the stage artifacts your companions rendered (WA-1783).

An artifact belongs to YOU. The room and the companion are where it was born,
recorded as origin rather than ownership — so it outlives the room, exactly the
way a file or a note does.

Examples:
  weside stage list
  weside stage list --room 42
  weside stage delete abc123`,
}

var (
	stageListRoom   string
	stageListCursor string
	stageListLimit  int
)

var stageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your stage artifacts, newest first",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Set("limit", strconv.Itoa(stageListLimit))
		if cmd.Flags().Changed("room") {
			q.Set("room_id", stageListRoom)
		}
		if cmd.Flags().Changed("cursor") {
			q.Set("cursor", stageListCursor)
		}

		var result map[string]any
		if err := client.Get(cmd.Context(), "/stage/artifacts?"+q.Encode(), &result); err != nil {
			return fmt.Errorf("listing stage artifacts: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		items, _ := result["items"].([]any)
		if len(items) == 0 {
			fmt.Println("Nothing is kept on the stage yet.")
			return nil
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			a, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", a["id"]),
				fmt.Sprintf("%v", a["title"]),
				fmt.Sprintf("%v", a["origin_companion_name"]),
				fmt.Sprintf("%v", a["created_at"]),
			})
		}
		ui.PrintTable([]string{"ID", "Title", "Built by", "When"}, rows)
		if next, _ := result["next_cursor"].(string); next != "" {
			fmt.Printf("(older: --cursor %s)\n", next)
		}
		return nil
	},
}

var stageDeleteCmd = &cobra.Command{
	Use:   "delete <artifact_id>",
	Short: "Delete a stage artifact and its stored bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Delete(cmd.Context(), "/stage/artifacts/"+args[0], &result); err != nil {
			return fmt.Errorf("deleting stage artifact: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Artifact %s deleted.", args[0])
		return nil
	},
}

func init() {
	stageListCmd.Flags().StringVar(&stageListRoom, "room", "", "only artifacts born in this room")
	stageListCmd.Flags().StringVar(&stageListCursor, "cursor", "", "keyset cursor from a previous page")
	stageListCmd.Flags().IntVar(&stageListLimit, "limit", 50, "maximum artifacts to return")
	stageCmd.AddCommand(stageListCmd)
	stageCmd.AddCommand(stageDeleteCmd)
	rootCmd.AddCommand(stageCmd)
}
