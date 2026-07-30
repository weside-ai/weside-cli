package cmd

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var searchLimit int

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search your memories, notes and files",
	Long: `Search everything you own — conversations and memories, the notes vault,
and file names (WA-1785).

Three engines answer and the result stays GROUPED: cosine distance and Postgres
full-text rank are not on the same scale, so a merged order would be arbitrary
and would look authoritative. Each group carries its own availability, so one
engine being down costs you that group and not the search — which is also the
only way to tell "nothing there" from "it did not run".`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Set("q", args[0])
		q.Set("limit", strconv.Itoa(searchLimit))

		var result map[string]any
		if err := client.Get(cmd.Context(), "/search?"+q.Encode(), &result); err != nil {
			return fmt.Errorf("searching: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		printed := false
		for _, group := range []struct{ key, label string }{
			{"semantic", "Conversations and memories"},
			{"notes", "Notes"},
			{"files", "Files"},
		} {
			block, _ := result[group.key].(map[string]any)
			if block == nil {
				continue
			}
			if unavailable, _ := block["unavailable"].(bool); unavailable {
				ui.PrintError("%s could not be reached.", group.label)
				printed = true
				continue
			}
			items, _ := block["items"].([]any)
			if len(items) == 0 {
				continue
			}
			printed = true
			fmt.Printf("\n%s\n", group.label)
			rows := make([][]string, 0, len(items))
			for _, item := range items {
				hit, _ := item.(map[string]any)
				rows = append(rows, []string{
					fmt.Sprintf("%v", hit["title"]),
					fmt.Sprintf("%v", hit["snippet"]),
				})
			}
			ui.PrintTable([]string{"Title", "Where it matched"}, rows)
		}
		if !printed {
			fmt.Printf("Nothing found for %q.\n", args[0])
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "maximum hits per group")
	rootCmd.AddCommand(searchCmd)
}
