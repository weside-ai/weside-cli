package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// One invite code for the app and for a room (WA-2235).
//
// The v2 `rooms invites …` subtree these verbs replace drove the retired
// `room_invites` table; the unified code lives on `bonus_codes` with a nullable
// `room_id` and is served by `/api/v1/invites`. Scope is a flag, not a
// separate verb family: `--room <id>` mints a room-scoped code, no flag mints
// an app-scoped one. Everything else — single use, 7-day TTL, one live code
// per (user, scope), the uniform 404 on any invalid code — is the same on
// either scope, and the verbs say nothing that would distinguish them.

var inviteCmd = &cobra.Command{
	Use:   "invite",
	Short: "Invite someone to weside, or into one of your rooms",
	Long: `Draw, share and redeem the one invite code.

  weside invite mint                # your live app-scoped code (draws one if none)
  weside invite mint --room 42      # your live code for room 42 (owner only)
  weside invite rotate [--room 42]  # retire the live code and draw a fresh one
  weside invite show <code>         # what a held code reveals before redeeming
  weside invite accept <code>       # redeem as the logged-in user

A code works once and expires after 7 days. Unknown, expired, revoked and spent
codes all answer the same 404 by design — the error cannot tell you which.`,
}

var (
	inviteMintRoom   int
	inviteRotateRoom int
)

// inviteScopeBody is the request body both mint and rotate send. A zero room id
// means app scope and is sent as an absent key, never as `"room_id": 0` — the
// server would try to resolve room 0 and refuse it (a 403, the same answer a
// non-owner and a non-member get, so nothing about room 0 leaks).
func inviteScopeBody(roomID int) map[string]any {
	if roomID <= 0 {
		return map[string]any{}
	}
	return map[string]any{"room_id": roomID}
}

// The helpers below are what the commands call, extracted so a test can drive
// the REAL path construction against an httptest server — the mistake worth
// catching is a wrong path, which answers the same 404 as an invalid code and
// would survive a manual round unnoticed.

func mintInvite(ctx context.Context, client *api.Client, roomID int) (map[string]any, error) {
	var result map[string]any
	if err := client.Post(ctx, "/invites", inviteScopeBody(roomID), &result); err != nil {
		return nil, fmt.Errorf("minting invite: %w", err)
	}
	return result, nil
}

func rotateInvite(ctx context.Context, client *api.Client, roomID int) (map[string]any, error) {
	var result map[string]any
	if err := client.Post(ctx, "/invites/rotate", inviteScopeBody(roomID), &result); err != nil {
		return nil, fmt.Errorf("rotating invite: %w", err)
	}
	return result, nil
}

func showInvite(ctx context.Context, client *api.Client, code string) (map[string]any, error) {
	var result map[string]any
	if err := client.Get(ctx, "/invites/"+code+"/preview", &result); err != nil {
		return nil, fmt.Errorf("previewing invite: %w", err)
	}
	return result, nil
}

func acceptInvite(ctx context.Context, client *api.Client, code string) (map[string]any, error) {
	var result map[string]any
	if err := client.Post(ctx, "/invites/"+code+"/accept", nil, &result); err != nil {
		return nil, fmt.Errorf("accepting invite: %w", err)
	}
	return result, nil
}

func printInviteCode(result map[string]any) {
	scope := "weside"
	if rid, ok := result["room_id"]; ok && rid != nil {
		scope = "room " + fmt.Sprintf("%v", rid)
	}
	ui.PrintSuccess("%v — invite to %s, works once, expires %v", result["code"], scope, result["expires_at"])
	if u, ok := result["url"]; ok && u != nil {
		ui.Printf("%v\n", u)
	}
}

var inviteMintCmd = &cobra.Command{
	Use: "mint",
	// An API error is the answer, not a usage mistake — no usage block after it.
	SilenceUsage: true,
	Short:        "Show your live invite code, drawing one if none is live",
	Args:         cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		result, err := mintInvite(cmd.Context(), client, inviteMintRoom)
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		printInviteCode(result)
		return nil
	},
}

var inviteRotateCmd = &cobra.Command{
	Use: "rotate",
	// An API error is the answer, not a usage mistake — no usage block after it.
	SilenceUsage: true,
	Short:        "Retire the live code and draw a fresh one",
	Args:         cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		result, err := rotateInvite(cmd.Context(), client, inviteRotateRoom)
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		printInviteCode(result)
		return nil
	},
}

var inviteShowCmd = &cobra.Command{
	Use: "show <code>",
	// An API error is the answer, not a usage mistake — no usage block after it.
	SilenceUsage: true,
	Short:        "Show what a held code reveals before redeeming",
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		result, err := showInvite(cmd.Context(), client, args[0])
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		if rt, ok := result["room_title"]; ok && rt != nil {
			ui.PrintSuccess(
				"%v — invited by %v · %v humans, %v companions",
				rt, result["inviter_display_name"], result["human_count"], result["companion_count"],
			)
			return nil
		}
		if result["valid"] == false {
			// Every rejection is one constant body — no name, no scope; rendering
			// either would suggest the server knew something about the code.
			ui.PrintError("This invite is no longer valid.")
			return nil
		}
		ui.PrintSuccess("Invited to weside by %v", result["inviter_display_name"])
		return nil
	},
}

var inviteAcceptCmd = &cobra.Command{
	Use: "accept <code>",
	// An API error is the answer, not a usage mistake — no usage block after it.
	SilenceUsage: true,
	Short:        "Redeem an invite code as the logged-in user",
	Long: `Redeem a code as the logged-in user.

The other half of ` + "`invite mint`" + `: this is the step that needs the SECOND
identity, and the one a verification round cannot do without a verb. A room
code seats you in the room; an app code does nothing for an account that
already exists (you are not a referee), and says so.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		result, err := acceptInvite(cmd.Context(), client, args[0])
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		if room, ok := result["room"].(map[string]any); ok && room != nil {
			ui.PrintSuccess("Joined room %v (%v).", room["id"], room["title"])
			return nil
		}
		ui.PrintSuccess("Code accepted: %v", result["outcome"])
		return nil
	},
}

// parseRoomFlag keeps a typed room id out of the flag machinery so a bad value
// fails at parse time, not as a 404 that looks like an invalid code.
func parseRoomFlag(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("--room must be a positive room id, got %q", s)
	}
	return n, nil
}

func init() {
	inviteMintCmd.Flags().IntVar(&inviteMintRoom, "room", 0, "room id — mint a room-scoped code (owner only)")
	inviteRotateCmd.Flags().IntVar(&inviteRotateRoom, "room", 0, "room id — rotate the room-scoped code (owner only)")
	inviteCmd.AddCommand(inviteMintCmd)
	inviteCmd.AddCommand(inviteRotateCmd)
	inviteCmd.AddCommand(inviteShowCmd)
	inviteCmd.AddCommand(inviteAcceptCmd)
	rootCmd.AddCommand(inviteCmd)
}
