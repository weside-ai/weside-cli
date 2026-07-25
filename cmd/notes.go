package cmd

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	notesScope    string
	notesAudience string
	notesLimit    int
	patLabel      string
	patTTL        int
	patCompanion  int

	notesWriteBody    string
	notesWriteFile    string
	notesWriteMessage string
)

// --- notes -----------------------------------------------------------------

var notesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Read notes from the notes repo",
}

var notesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notes",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Set("scope", notesScope)
		if notesAudience != "" {
			q.Set("audience", notesAudience)
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/notes/list?"+q.Encode(), &result); err != nil {
			return fmt.Errorf("listing notes: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		items, _ := result["items"].([]any)
		headers := []string{"PATH", "TITLE", "TAGS", "MODIFIED"}
		var rows [][]string
		for _, item := range items {
			n, _ := item.(map[string]any)
			rows = append(rows, []string{
				truncate(fmt.Sprintf("%v", n["path"]), 40),
				truncate(fmt.Sprintf("%v", n["title"]), 30),
				joinAnySlice(n["tags"]),
				fmt.Sprintf("%v", n["modified_at"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var notesGetCmd = &cobra.Command{
	Use:   "get <path>",
	Short: "Read a single note",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Set("path", args[0])
		var result map[string]any
		if err := client.Get(context.Background(), "/notes/get?"+q.Encode(), &result); err != nil {
			return fmt.Errorf("reading note: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		if title, _ := result["title"].(string); title != "" {
			fmt.Printf("# %s\n\n", title)
		}
		fmt.Printf("%v\n", result["body"])
		return nil
	},
}

var notesSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search across notes",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Set("q", args[0])
		q.Set("scope", notesScope)
		q.Set("limit", fmt.Sprintf("%d", notesLimit))
		var result map[string]any
		if err := client.Get(context.Background(), "/notes/search?"+q.Encode(), &result); err != nil {
			return fmt.Errorf("searching notes: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		items, _ := result["items"].([]any)
		headers := []string{"PATH", "TITLE", "SNIPPET", "SCORE"}
		var rows [][]string
		for _, item := range items {
			n, _ := item.(map[string]any)
			rows = append(rows, []string{
				truncate(fmt.Sprintf("%v", n["path"]), 30),
				truncate(fmt.Sprintf("%v", n["title"]), 25),
				truncate(fmt.Sprintf("%v", n["snippet"]), 40),
				fmt.Sprintf("%.2f", n["score"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

// --- notes-repo ------------------------------------------------------------

var notesRepoCmd = &cobra.Command{
	Use:   "notes-repo",
	Short: "Inspect or repair the notes repo",
}

var notesRepoStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show notes repo URL and sync status",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/me/notes-repo", &result); err != nil {
			return fmt.Errorf("reading notes repo: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		fmt.Printf("URL:    %v\n", result["repo_url"])
		fmt.Printf("Clone:  %v\n", result["clone_url"])
		fmt.Printf("Status: %v\n", result["status"])
		fmt.Printf("Enabled: %v\n", result["enabled"])
		if companions, ok := result["companions"].([]any); ok {
			fmt.Println("\nCompanion subdirs:")
			for _, item := range companions {
				c, _ := item.(map[string]any)
				fmt.Printf("  %v → %v\n", c["companion_name"], c["subdir_path"])
			}
		}
		return nil
	},
}

var notesRepoRepairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Force a clean re-clone of the notes repo",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/me/notes-repo/repair", nil, &result); err != nil {
			return fmt.Errorf("repairing notes repo: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Notes repo repair scheduled (%v).", result["status"])
		return nil
	},
}

// --- notes-pat -------------------------------------------------------------

var notesPatCmd = &cobra.Command{
	Use:   "notes-pat",
	Short: "Manage notes-repo personal access tokens",
}

var notesPatListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active notes PATs",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result []any
		if err := client.Get(context.Background(), "/me/notes-pat", &result); err != nil {
			return fmt.Errorf("listing PATs: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		headers := []string{"ID", "LABEL", "EXPIRES", "LAST USED"}
		var rows [][]string
		for _, item := range result {
			p, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", p["id"]),
				truncate(fmt.Sprintf("%v", p["label"]), 25),
				fmt.Sprintf("%v", p["expires_at"]),
				fmt.Sprintf("%v", p["last_used_at"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var notesPatMintCmd = &cobra.Command{
	Use:   "mint --label L",
	Short: "Mint a new notes PAT (plaintext returned once)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if patLabel == "" {
			return fmt.Errorf("--label is required")
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{"label": patLabel, "ttl_days": patTTL}
		if cmd.Flags().Changed("companion") {
			body["companion_id"] = patCompanion
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/me/notes-pat", body, &result); err != nil {
			return fmt.Errorf("minting PAT: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("PAT %v created. Token (shown once):", result["id"])
		fmt.Printf("%v\n", result["token"])
		fmt.Printf("Clone URL: %v\n", result["clone_url"])
		return nil
	},
}

var notesPatRevokeCmd = &cobra.Command{
	Use:   "revoke <pat_id>",
	Short: "Revoke a notes PAT",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/me/notes-pat/"+args[0], nil); err != nil {
			return fmt.Errorf("revoking PAT: %w", err)
		}

		ui.PrintSuccess("PAT %s revoked.", args[0])
		return nil
	},
}

var notesWriteCmd = &cobra.Command{
	Use:   "write <path>",
	Short: "Create or overwrite a note",
	Long: `Write a note into the notes repo at a repo-relative path.

Content comes from --body, from --file, or from stdin when neither is given.
The path must not contain ".." or absolute components.

Note: notes repos are a platform capability. Against a backend that runs
without one configured, every notes-repo route answers 503 — the command
reports that rather than pretending the write landed.

Examples:
  weside notes write scratch/idea.md --body "# Idea"
  weside notes write scratch/idea.md --file ./idea.md
  echo "# Idea" | weside notes write scratch/idea.md`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		content, err := resolveNoteContent()
		if err != nil {
			return err
		}

		payload := map[string]any{"path": args[0], "body": content}
		if notesWriteMessage != "" {
			payload["commit_message"] = notesWriteMessage
		}

		var result map[string]any
		if err := client.Put(context.Background(), "/notes", payload, &result); err != nil {
			return fmt.Errorf("writing note: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Wrote %v (sha %v).", result["path"], result["sha"])
		return nil
	},
}

// resolveNoteContent picks the note body from exactly one source: --body,
// --file, or stdin. Two sources at once is a mistake worth failing on rather
// than silently preferring one.
func resolveNoteContent() (string, error) {
	if notesWriteBody != "" && notesWriteFile != "" {
		return "", fmt.Errorf("pass either --body or --file, not both")
	}
	if notesWriteBody != "" {
		return notesWriteBody, nil
	}
	if notesWriteFile != "" {
		data, err := os.ReadFile(notesWriteFile) //nolint:gosec // user-supplied path is the point
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", notesWriteFile, err)
		}
		return string(data), nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no content: pass --body, --file, or pipe it on stdin")
	}
	return string(data), nil
}

func init() {
	notesListCmd.Flags().StringVar(&notesScope, "scope", "all", "all|mine|shared|user-private")
	notesListCmd.Flags().StringVar(&notesAudience, "audience", "", "companion slug")
	notesSearchCmd.Flags().StringVar(&notesScope, "scope", "all", "scope filter")
	notesSearchCmd.Flags().IntVar(&notesLimit, "limit", 25, "max results (1-50)")
	notesPatMintCmd.Flags().StringVar(&patLabel, "label", "", "PAT label (required)")
	notesPatMintCmd.Flags().IntVar(&patTTL, "ttl", 90, "days: 30|90|365")
	notesPatMintCmd.Flags().IntVar(&patCompanion, "companion", 0, "companion id (optional)")

	notesWriteCmd.Flags().StringVar(&notesWriteBody, "body", "", "note content (inline)")
	notesWriteCmd.Flags().StringVar(&notesWriteFile, "file", "", "read note content from a local file")
	notesWriteCmd.Flags().StringVar(&notesWriteMessage, "message", "", "commit message")

	notesCmd.AddCommand(notesListCmd)
	notesCmd.AddCommand(notesWriteCmd)
	notesCmd.AddCommand(notesGetCmd)
	notesCmd.AddCommand(notesSearchCmd)
	notesRepoCmd.AddCommand(notesRepoStatusCmd)
	notesRepoCmd.AddCommand(notesRepoRepairCmd)
	notesPatCmd.AddCommand(notesPatListCmd)
	notesPatCmd.AddCommand(notesPatMintCmd)
	notesPatCmd.AddCommand(notesPatRevokeCmd)
	rootCmd.AddCommand(notesCmd)
	rootCmd.AddCommand(notesRepoCmd)
	rootCmd.AddCommand(notesPatCmd)
}
