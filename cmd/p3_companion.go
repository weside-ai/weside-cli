package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var meDisplayName string

// --- evolution -------------------------------------------------------------

var evolutionCmd = &cobra.Command{Use: "evolution", Short: "Inspect/start companion evolution runs"}

var evolutionCurrentCmd = &cobra.Command{
	Use:   "current <companion>",
	Short: "Show the pending evolution run for a companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+cid+"/evolution/current", &result); err != nil {
			return fmt.Errorf("evolution current: %w", err)
		}
		ui.PrintJSON(result)
		return nil
	},
}

var evolutionStartCmd = &cobra.Command{
	Use:   "start <companion>",
	Short: "Start a new evolution run",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/companions/"+cid+"/evolution/start", nil, &result); err != nil {
			return fmt.Errorf("starting evolution: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Evolution run started (id %v).", result["id"])
		return nil
	},
}

var evolutionDismissCmd = &cobra.Command{
	Use:   "dismiss <companion> <evolution_id>",
	Short: "Clear a pending evolution run after review",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		if err := client.Post(context.Background(), "/companions/"+cid+"/evolution/"+args[1]+"/dismiss", nil, nil); err != nil {
			return fmt.Errorf("dismissing evolution: %w", err)
		}
		ui.PrintSuccess("Evolution %s dismissed.", args[1])
		return nil
	},
}

var evolutionPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List enabled evolution style presets",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/evolution/presets", &result); err != nil {
			return fmt.Errorf("evolution presets: %w", err)
		}
		printListOrJSON(result, []string{"id", "name"})
		return nil
	},
}

// --- reminders / mentor-sessions / subscriptions ---------------------------

var remindersCmd = &cobra.Command{Use: "reminders", Short: "List/dismiss companion reminders"}

var remindersListCmd = &cobra.Command{
	Use:   "list <companion>",
	Short: "List pending reminders for a companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+cid+"/reminders", &result); err != nil {
			return fmt.Errorf("listing reminders: %w", err)
		}
		printListOrJSON(result, []string{"id", "title", "due_at"})
		return nil
	},
}

var remindersDismissCmd = &cobra.Command{
	Use:   "dismiss <companion> <reminder_id>",
	Short: "Dismiss a pending reminder",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		if err := client.Patch(context.Background(), "/companions/"+cid+"/reminders/"+args[1]+"/dismiss", nil, nil); err != nil {
			return fmt.Errorf("dismissing reminder: %w", err)
		}
		ui.PrintSuccess("Reminder %s dismissed.", args[1])
		return nil
	},
}

var mentorSessionsCmd = &cobra.Command{
	Use:   "mentor-sessions <companion>",
	Short: "List completed mentor-session reports",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+cid+"/mentor-sessions", &result); err != nil {
			return fmt.Errorf("listing mentor sessions: %w", err)
		}
		printListOrJSON(result, []string{"id", "created_at"})
		return nil
	},
}

var subscriptionsCmd = &cobra.Command{Use: "subscriptions", Short: "List/toggle companion attention subscriptions"}

var subscriptionsListCmd = &cobra.Command{
	Use:   "list <companion>",
	Short: "List a companion's attention-tag subscriptions",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+cid+"/subscriptions", &result); err != nil {
			return fmt.Errorf("listing subscriptions: %w", err)
		}
		printListOrJSON(result, []string{"tag", "enabled"})
		return nil
	},
}

var subscriptionsToggleCmd = &cobra.Command{
	Use:   "toggle <companion> <tag>",
	Short: "Enable or disable an attention-tag subscription",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		cid, err := resolveCompanion(args[0])
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		body := map[string]any{"enabled": true}
		if err := client.Patch(context.Background(), "/companions/"+cid+"/subscriptions/"+args[1], body, nil); err != nil {
			return fmt.Errorf("toggling subscription: %w", err)
		}
		ui.PrintSuccess("Subscription %s toggled.", args[1])
		return nil
	},
}

// --- me account ------------------------------------------------------------

var meAccountCmd = &cobra.Command{Use: "me-account", Short: "Account profile, locale, depth, export, deactivate"}

var meProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Show your profile",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/users/me", &result); err != nil {
			return fmt.Errorf("reading profile: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		fmt.Printf("ID:       %v\n", result["id"])
		fmt.Printf("Name:     %v\n", result["display_name"])
		fmt.Printf("Email:    %v\n", result["email"])
		return nil
	},
}

var meProfileSetCmd = &cobra.Command{
	Use:   "profile-set --name N",
	Short: "Update your display name",
	RunE: func(_ *cobra.Command, _ []string) error {
		if meDisplayName == "" {
			return fmt.Errorf("--name is required")
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		body := map[string]any{"display_name": meDisplayName}
		var result map[string]any
		if err := client.Put(context.Background(), "/users/me", body, &result); err != nil {
			return fmt.Errorf("updating profile: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Profile updated.")
		return nil
	},
}

var meLocaleCmd = &cobra.Command{
	Use:   "locale <locale>",
	Short: "Set your preferred locale (e.g. de, en)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		body := map[string]any{"locale": args[0]}
		if err := client.Patch(context.Background(), "/users/me/locale", body, nil); err != nil {
			return fmt.Errorf("setting locale: %w", err)
		}
		ui.PrintSuccess("Locale set to %s.", args[0])
		return nil
	},
}

var meSlidingWindowCmd = &cobra.Command{
	Use:   "sliding-window [tokens]",
	Short: "Show or set conversation depth (tokens)",
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		if len(args) == 0 {
			var result map[string]any
			if err := client.Get(context.Background(), "/users/me/sliding-window", &result); err != nil {
				return fmt.Errorf("reading sliding window: %w", err)
			}
			ui.PrintJSON(result)
			return nil
		}
		tokens, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid tokens: %w", err)
		}
		body := map[string]any{"tokens": tokens}
		var result map[string]any
		if err := client.Patch(context.Background(), "/users/me/sliding-window", body, &result); err != nil {
			return fmt.Errorf("setting sliding window: %w", err)
		}
		ui.PrintSuccess("Sliding window set (effective %v).", result["effective_tokens"])
		return nil
	},
}

var meExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Request a GDPR data export",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/users/me/export", nil, &result); err != nil {
			return fmt.Errorf("requesting export: %w", err)
		}
		ui.PrintJSON(result)
		return nil
	},
}

var meDeactivateCmd = &cobra.Command{
	Use:   "deactivate",
	Short: "GDPR soft-delete your account (destructive)",
	RunE: func(_ *cobra.Command, _ []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this soft-deletes your account — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/users/me/deactivate", nil, &result); err != nil {
			return fmt.Errorf("deactivating: %w", err)
		}
		ui.PrintSuccess("Account deactivated.")
		return nil
	},
}

// --- integrations ---------------------------------------------------------

var integrationsCmd = &cobra.Command{Use: "integrations", Short: "List/disconnect/reconcile integrations"}

var integrationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your connected integrations",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/integrations", &result); err != nil {
			return fmt.Errorf("listing integrations: %w", err)
		}
		printListOrJSON(result, []string{"id", "toolkit_name", "status"})
		return nil
	},
}

var integrationsCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Show the integration catalog",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/integrations/catalog", &result); err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
		printListOrJSON(result, []string{"slug", "name", "category"})
		return nil
	},
}

var integrationsDisconnectCmd = &cobra.Command{
	Use:   "disconnect <integration_id>",
	Short: "Disconnect an integration",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		if err := client.Delete(context.Background(), "/integrations/"+args[0], nil); err != nil {
			return fmt.Errorf("disconnecting: %w", err)
		}
		ui.PrintSuccess("Integration %s disconnected.", args[0])
		return nil
	},
}

var integrationsReconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Reconcile integration status against Composio",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/integrations/reconcile", nil, &result); err != nil {
			return fmt.Errorf("reconciling: %w", err)
		}
		ui.PrintJSON(result)
		return nil
	},
}

func init() {
	meProfileSetCmd.Flags().StringVar(&meDisplayName, "name", "", "new display name")
	meDeactivateCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")

	evolutionCmd.AddCommand(evolutionCurrentCmd)
	evolutionCmd.AddCommand(evolutionStartCmd)
	evolutionCmd.AddCommand(evolutionDismissCmd)
	evolutionCmd.AddCommand(evolutionPresetsCmd)
	rootCmd.AddCommand(evolutionCmd)

	remindersCmd.AddCommand(remindersListCmd)
	remindersCmd.AddCommand(remindersDismissCmd)
	rootCmd.AddCommand(remindersCmd)
	rootCmd.AddCommand(mentorSessionsCmd)

	subscriptionsCmd.AddCommand(subscriptionsListCmd)
	subscriptionsCmd.AddCommand(subscriptionsToggleCmd)
	rootCmd.AddCommand(subscriptionsCmd)

	meAccountCmd.AddCommand(meProfileCmd)
	meAccountCmd.AddCommand(meProfileSetCmd)
	meAccountCmd.AddCommand(meLocaleCmd)
	meAccountCmd.AddCommand(meSlidingWindowCmd)
	meAccountCmd.AddCommand(meExportCmd)
	meAccountCmd.AddCommand(meDeactivateCmd)
	rootCmd.AddCommand(meAccountCmd)

	integrationsCmd.AddCommand(integrationsListCmd)
	integrationsCmd.AddCommand(integrationsCatalogCmd)
	integrationsCmd.AddCommand(integrationsDisconnectCmd)
	integrationsCmd.AddCommand(integrationsReconcileCmd)
	rootCmd.AddCommand(integrationsCmd)
}
