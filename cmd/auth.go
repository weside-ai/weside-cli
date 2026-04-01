package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/auth"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var devMode bool

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to weside.ai",
	RunE: func(_ *cobra.Command, _ []string) error {
		if devMode {
			return loginDev()
		}
		// TODO(WA-698 Phase 5): Supabase PKCE flow
		fmt.Println("Production login (Supabase PKCE) is not yet implemented.")
		fmt.Println("Use --dev for local development.")
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and remove stored credentials",
	RunE: func(_ *cobra.Command, _ []string) error {
		storage := auth.NewStorage()
		if err := storage.Delete(); err != nil {
			return fmt.Errorf("logging out: %w", err)
		}
		ui.PrintSuccess("Logged out successfully.")
		return nil
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authenticated user",
	RunE: func(_ *cobra.Command, _ []string) error {
		token, err := auth.GetToken()
		if err != nil {
			return err
		}

		client := api.NewClient(GetAPIURL()+"/api/v1", token)
		var user map[string]any
		if err := client.Get(context.Background(), "/auth/me", &user); err != nil {
			return fmt.Errorf("getting user info: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(user)
			return nil
		}

		fmt.Printf("Logged in as: %s\n", user["email"])
		if name, ok := user["display_name"]; ok && name != nil {
			fmt.Printf("Name: %s\n", name)
		}
		if id, ok := user["id"]; ok {
			fmt.Printf("User ID: %v\n", id)
		}
		return nil
	},
}

var authTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the current access token (for scripting)",
	RunE: func(_ *cobra.Command, _ []string) error {
		token, err := auth.GetToken()
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, token)
		return nil
	},
}

func loginDev() error {
	email := "test@weside.ai"
	fmt.Printf("Logging in as %s (dev mode)...\n", email)

	client := api.NewClient(GetAPIURL(), "")
	var result map[string]any
	body := map[string]string{"email": email}
	if err := client.Post(context.Background(), "/dev/auth/token", body, &result); err != nil {
		return fmt.Errorf("dev login failed: %w", err)
	}

	token, ok := result["access_token"].(string)
	if !ok || token == "" {
		return fmt.Errorf("no access token in response")
	}

	storage := auth.NewStorage()
	if err := storage.Save(&auth.Tokens{AccessToken: token}); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}

	ui.PrintSuccess("Logged in as %s (dev mode)", email)
	return nil
}

func init() {
	authLoginCmd.Flags().BoolVar(&devMode, "dev", false, "use dev authentication (local only)")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
	authCmd.AddCommand(authTokenCmd)
	rootCmd.AddCommand(authCmd)
}
