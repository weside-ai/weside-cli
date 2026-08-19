package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// P3 ops surface (lower-frequency). Tables are best-effort (the first list
// array in the response); use --json for the exact wire shape.

// firstList finds the first []any value in a map response (best-effort table).
func firstList(m map[string]any) []any {
	for _, v := range m {
		if l, ok := v.([]any); ok && len(l) > 0 {
			return l
		}
	}
	return nil
}

func printListOrJSON(result map[string]any, cols []string) {
	if IsJSON() {
		ui.PrintJSON(result)
		return
	}
	list := firstList(result)
	if list == nil {
		ui.PrintJSON(result)
		return
	}
	var rows [][]string
	for _, item := range list {
		m, _ := item.(map[string]any)
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = truncate(fmt.Sprintf("%v", m[c]), 30)
		}
		rows = append(rows, row)
	}
	ui.PrintTable(cols, rows)
}

// --- referrals -------------------------------------------------------------

var referralsCmd = &cobra.Command{Use: "referrals", Short: "Manage referral codes"}

var referralsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your referral codes",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		// GET /referrals/codes returns a bare JSON array, not an envelope.
		var items []any
		if err := client.Get(context.Background(), "/referrals/codes", &items); err != nil {
			return fmt.Errorf("listing referrals: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(items)
			return nil
		}
		headers := []string{"CODE", "STATUS", "CREATED"}
		var rows [][]string
		for _, item := range items {
			m, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", m["code"]),
				fmt.Sprintf("%v", m["status"]),
				truncate(fmt.Sprintf("%v", m["created_at"]), 25),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var referralsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Issue a new 1:1 referral code",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/referrals/codes", nil, &result); err != nil {
			return fmt.Errorf("creating referral: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Referral code: %v", result["code"])
		return nil
	},
}

var referralsRevokeCmd = &cobra.Command{
	Use:   "revoke <code>",
	Short: "Revoke an unclaimed referral code",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		if err := client.Delete(context.Background(), "/referrals/codes/"+args[0], nil); err != nil {
			return fmt.Errorf("revoking referral: %w", err)
		}
		ui.PrintSuccess("Referral %s revoked.", args[0])
		return nil
	},
}

var referralsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show referral stats",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/referrals/stats", &result); err != nil {
			return fmt.Errorf("referral stats: %w", err)
		}
		ui.PrintJSON(result)
		return nil
	},
}

// --- circles ---------------------------------------------------------------

var circleVisibility string

var circlesCmd = &cobra.Command{Use: "circles", Short: "Manage Trust Circles"}

var circlesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your circles",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/circles", &result); err != nil {
			return fmt.Errorf("listing circles: %w", err)
		}
		printListOrJSON(result, []string{"id", "name", "is_default"})
		return nil
	},
}

var circlesCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a non-default circle",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		body := map[string]any{"name": args[0]}
		if circleVisibility != "" {
			body["visibility"] = circleVisibility
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/circles", body, &result); err != nil {
			return fmt.Errorf("creating circle: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Circle %v created (id %v).", result["name"], result["id"])
		return nil
	},
}

var circlesDeleteCmd = &cobra.Command{
	Use:   "delete <circle_id>",
	Short: "Delete an empty non-default circle",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		if err := client.Delete(context.Background(), "/circles/"+args[0], nil); err != nil {
			return fmt.Errorf("deleting circle: %w", err)
		}
		ui.PrintSuccess("Circle %s deleted.", args[0])
		return nil
	},
}

// --- plans + billing (read subset) -----------------------------------------

var plansCmd = &cobra.Command{Use: "plans", Short: "Show subscription plans"}

var plansShowCmd = &cobra.Command{
	Use:   "show",
	Short: "List all plans",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/plans", &result); err != nil {
			return fmt.Errorf("listing plans: %w", err)
		}
		printListOrJSON(result, []string{"id", "name", "tier"})
		return nil
	},
}

var plansMeCmd = &cobra.Command{
	Use:   "me",
	Short: "Show your current plan and feature flags",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/plans/me", &result); err != nil {
			return fmt.Errorf("reading plan: %w", err)
		}
		ui.PrintJSON(result)
		return nil
	},
}

var billingCmd = &cobra.Command{Use: "billing", Short: "Billing reads (usage, eligibility)"}

var billingUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show credit balance and usage for the current period",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/billing/usage", &result); err != nil {
			return fmt.Errorf("billing usage: %w", err)
		}
		ui.PrintJSON(result)
		return nil
	},
}

var billingEligibilityCmd = &cobra.Command{
	Use:   "purchase-eligibility",
	Short: "Show subscription purchase guard state",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/billing/purchase-eligibility", &result); err != nil {
			return fmt.Errorf("purchase-eligibility: %w", err)
		}
		ui.PrintJSON(result)
		return nil
	},
}

// --- channels --------------------------------------------------------------

var channelsCmd = &cobra.Command{Use: "channels", Short: "List and set active companion on channels"}

var channelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your channels",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/channels", &result); err != nil {
			return fmt.Errorf("listing channels: %w", err)
		}
		printListOrJSON(result, []string{"id", "channel_kind", "active_companion_id"})
		return nil
	},
}

var channelsSetActiveCmd = &cobra.Command{
	Use:   "set-active <channel_id> <companion_id>",
	Short: "Set the active companion for a channel",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		compID, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid companion id: %w", err)
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		body := map[string]any{"active_companion_id": compID}
		if err := client.Patch(context.Background(), "/channels/"+args[0]+"/active-companion", body, nil); err != nil {
			return fmt.Errorf("setting active companion: %w", err)
		}
		ui.PrintSuccess("Channel %s active companion set to %s.", args[0], args[1])
		return nil
	},
}

// --- experts ---------------------------------------------------------------

var expertsCmd = &cobra.Command{Use: "experts", Short: "Discover published experts"}

var expertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List published experts",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/experts", &result); err != nil {
			return fmt.Errorf("listing experts: %w", err)
		}
		printListOrJSON(result, []string{"id", "name", "category"})
		return nil
	},
}

var expertsBefriendCmd = &cobra.Command{
	Use:   "befriend <companion_id>",
	Short: "Befriend an expert (create a personal copy)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/experts/"+args[0]+"/befriend", nil, &result); err != nil {
			return fmt.Errorf("befriending expert: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Befriended expert %s (new companion %v).", args[0], result["id"])
		return nil
	},
}

// --- safety (v2) -----------------------------------------------------------

var safetyCmd = &cobra.Command{Use: "safety", Short: "Trust & safety actions"}

type blockedUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type safetyReportRequest struct {
	TargetType   string `json:"target_type"`
	TargetUserID string `json:"target_user_id"`
	ReasonCode   string `json:"reason_code"`
	// The API historically calls the reporter's own words "reason"; the
	// category is the separate reason_code field exposed as --reason-code.
	Reason string `json:"reason"`
}

var (
	safetyReportUser       string
	safetyReportReasonCode string
	safetyReportText       string
)

var allowedSafetyReportReasonCodes = []string{
	"harassment",
	"unwanted_sexual",
	"impersonation",
	"spam_scam",
	"other",
}

var safetyBlockCmd = &cobra.Command{
	Use:   "block <user_id>",
	Short: "Block another user (bidirectional, idempotent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		if err := client.Post(context.Background(), "/users/"+args[0]+"/block", nil, nil); err != nil {
			return fmt.Errorf("blocking user: %w", err)
		}
		ui.PrintSuccess("User %s blocked.", args[0])
		return nil
	},
}

var safetyUnblockCmd = &cobra.Command{
	Use:   "unblock <user_id>",
	Short: "Remove a block",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		if err := client.Delete(context.Background(), "/users/"+args[0]+"/block", nil); err != nil {
			return fmt.Errorf("unblocking user: %w", err)
		}
		ui.PrintSuccess("User %s unblocked.", args[0])
		return nil
	},
}

var safetyBlockedCmd = &cobra.Command{
	Use:   "blocked",
	Short: "List blocked people",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		var result []blockedUser
		if err := client.Get(context.Background(), "/users/blocked", &result); err != nil {
			return fmt.Errorf("listing blocked users: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		rows := make([][]string, 0, len(result))
		for _, user := range result {
			rows = append(rows, []string{user.ID, user.DisplayName})
		}
		ui.PrintTable([]string{"ID", "DISPLAY NAME"}, rows)
		return nil
	},
}

var safetyReportCmd = &cobra.Command{
	Use:   "report --user <id> --reason-code <key> [--text <sentence>]",
	Short: "Report another user",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := validateSafetyReport(safetyReportUser, safetyReportReasonCode, safetyReportText); err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		body := safetyReportRequest{
			TargetType:   "user",
			TargetUserID: safetyReportUser,
			ReasonCode:   safetyReportReasonCode,
			Reason:       safetyReportText,
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/reports", body, &result); err != nil {
			return fmt.Errorf("reporting user: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("User %s reported.", safetyReportUser)
		return nil
	},
}

func validateSafetyReport(userID, reasonCode, text string) error {
	if userID == "" {
		return fmt.Errorf("--user is required")
	}
	allowed := false
	for _, candidate := range allowedSafetyReportReasonCodes {
		if reasonCode == candidate {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("--reason-code must be one of: %s", strings.Join(allowedSafetyReportReasonCodes, ", "))
	}
	if reasonCode == "other" && strings.TrimSpace(text) == "" {
		return fmt.Errorf("--text is required when --reason-code is other")
	}
	return nil
}

func init() {
	circlesCreateCmd.Flags().StringVar(&circleVisibility, "visibility", "", "circle visibility")

	referralsCmd.AddCommand(referralsListCmd)
	referralsCmd.AddCommand(referralsCreateCmd)
	referralsCmd.AddCommand(referralsRevokeCmd)
	referralsCmd.AddCommand(referralsStatsCmd)
	rootCmd.AddCommand(referralsCmd)

	circlesCmd.AddCommand(circlesListCmd)
	circlesCmd.AddCommand(circlesCreateCmd)
	circlesCmd.AddCommand(circlesDeleteCmd)
	rootCmd.AddCommand(circlesCmd)

	plansCmd.AddCommand(plansShowCmd)
	plansCmd.AddCommand(plansMeCmd)
	rootCmd.AddCommand(plansCmd)

	billingCmd.AddCommand(billingUsageCmd)
	billingCmd.AddCommand(billingEligibilityCmd)
	rootCmd.AddCommand(billingCmd)

	channelsCmd.AddCommand(channelsListCmd)
	channelsCmd.AddCommand(channelsSetActiveCmd)
	rootCmd.AddCommand(channelsCmd)

	expertsCmd.AddCommand(expertsListCmd)
	expertsCmd.AddCommand(expertsBefriendCmd)
	rootCmd.AddCommand(expertsCmd)

	safetyCmd.AddCommand(safetyBlockCmd)
	safetyCmd.AddCommand(safetyUnblockCmd)
	safetyCmd.AddCommand(safetyBlockedCmd)
	safetyCmd.AddCommand(safetyReportCmd)
	rootCmd.AddCommand(safetyCmd)

	safetyReportCmd.Flags().StringVar(&safetyReportUser, "user", "", "user ID to report")
	safetyReportCmd.Flags().StringVar(&safetyReportReasonCode, "reason-code", "", "report category")
	safetyReportCmd.Flags().StringVar(&safetyReportText, "text", "", "reporter's own description")
}
