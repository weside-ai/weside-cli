package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// Ablage ("filing") is the one surface where vault (notes repo) rows of
// every kind and plain-storage files are searched and browsed together —
// see WA-2109. This is a new command group; it does not replace `notes` or
// `files`, which keep their own read/write surfaces.

var ablageCmd = &cobra.Command{
	Use:   "ablage",
	Short: "Search across the vault (every kind) and plain-storage files",
}

var ablageSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search vault entries and plain-storage files by name",
	Long: `Search the Ablage: vault rows of every kind (note, image, document,
code, audio, video) plus plain-storage files, matched by name across
folders with no recency window.

Example:
  weside ablage search vertrag --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Set("q", args[0])
		var result map[string]any
		if err := client.Get(context.Background(), "/ablage/search?"+q.Encode(), &result); err != nil {
			return fmt.Errorf("searching the ablage: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		items, _ := result["items"].([]any)
		headers := []string{"KIND", "PATH", "TITLE"}
		var rows [][]string
		for _, item := range items {
			n, _ := item.(map[string]any)
			title := fmt.Sprintf("%v", n["title"])
			if title == "<nil>" {
				title = fmt.Sprintf("%v", n["name"])
			}
			rows = append(rows, []string{
				fmt.Sprintf("%v", n["kind"]),
				truncate(fmt.Sprintf("%v", n["path"]), 40),
				truncate(title, 30),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

func init() {
	ablageCmd.AddCommand(ablageSearchCmd)
	rootCmd.AddCommand(ablageCmd)
}
