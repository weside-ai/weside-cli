package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	filesLimit      int
	filesUploadName string
)

var filesCmd = &cobra.Command{
	Use:   "files",
	Short: "Inspect persistent storage (JuiceFS)",
}

var filesTreeCmd = &cobra.Command{
	Use:   "tree [path]",
	Short: "List directory contents of persistent storage",
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		path := "/"
		if len(args) > 0 {
			path = args[0]
		}
		q := url.Values{}
		q.Set("path", path)
		q.Set("limit", fmt.Sprintf("%d", filesLimit))
		var result map[string]any
		if err := client.Get(context.Background(), "/files/tree?"+q.Encode(), &result); err != nil {
			return fmt.Errorf("listing files: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		entries, _ := result["entries"].([]any)
		headers := []string{"TYPE", "NAME", "SIZE", "CHILDREN"}
		var rows [][]string
		for _, item := range entries {
			e, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", e["type"]),
				truncate(fmt.Sprintf("%v", e["name"]), 50),
				fmt.Sprintf("%v", e["size_bytes"]),
				fmt.Sprintf("%v", e["children_count"]),
			})
		}
		ui.PrintTable(headers, rows)
		fmt.Printf("\n%v total\n", result["total_count"])
		return nil
	},
}

var filesQuotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "Show persistent storage quota",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/files/quota", &result); err != nil {
			return fmt.Errorf("reading quota: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		fmt.Printf("Used:   %s / %s  (%.1f%%)\n", result["used_formatted"], result["limit_formatted"], result["percent"])
		if warning, _ := result["warning"].(bool); warning {
			fmt.Println("Warning: approaching quota limit.")
		}
		if over, _ := result["over_limit"].(bool); over {
			fmt.Println("Over limit.")
		}
		return nil
	},
}

var filesDeleteCmd = &cobra.Command{
	Use:   "delete <path>",
	Short: "Delete a file from persistent storage",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/files/"+args[0], nil); err != nil {
			return fmt.Errorf("deleting file: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(map[string]any{"deleted": true, "path": args[0]})
			return nil
		}
		ui.PrintSuccess("Deleted %s.", args[0])
		return nil
	},
}

var filesUploadCmd = &cobra.Command{
	Use:   "upload <local-path>",
	Short: "Upload a local file into persistent storage",
	Long: `Upload a local file into the current user's temp/ area in persistent
storage (JuiceFS). The remote subdirectory is resolved server-side, so the
client does not need to know the configured path.

The stored name defaults to the local basename; override it with --name.

Examples:
  weside files upload ./notes.pdf
  weside files upload ./scratch.txt --name seed.txt`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		localPath := args[0]
		handle, err := os.Open(localPath) //nolint:gosec // user-supplied path is the point
		if err != nil {
			return fmt.Errorf("opening %s: %w", localPath, err)
		}
		defer func() { _ = handle.Close() }()

		name := filesUploadName
		if name == "" {
			name = filepath.Base(localPath)
		}

		q := url.Values{}
		q.Set("name", name)
		var result map[string]any
		if err := client.PostStream(
			context.Background(),
			"/files/upload-to-temp?"+q.Encode(),
			handle,
			&result,
		); err != nil {
			return fmt.Errorf("uploading file: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Uploaded to %v (%v bytes).", result["path"], result["size_bytes"])
		return nil
	},
}

func init() {
	filesTreeCmd.Flags().IntVar(&filesLimit, "limit", 500, "max entries (1-2000)")
	filesUploadCmd.Flags().StringVar(&filesUploadName, "name", "", "stored filename (default: local basename)")

	filesCmd.AddCommand(filesTreeCmd)
	filesCmd.AddCommand(filesUploadCmd)
	filesCmd.AddCommand(filesQuotaCmd)
	filesCmd.AddCommand(filesDeleteCmd)
	rootCmd.AddCommand(filesCmd)
}
