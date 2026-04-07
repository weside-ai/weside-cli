package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/auth"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	devMode      bool
	authProvider string
)

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
		return loginPKCE()
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
		_, _ = fmt.Fprint(os.Stdout, token)
		return nil
	},
}

func loginPKCE() error {
	// Generate PKCE verifier + challenge
	verifier, err := auth.GenerateVerifier()
	if err != nil {
		return err
	}
	challenge := auth.GenerateChallenge(verifier)

	// Start callback server (finds free port)
	server, err := auth.NewCallbackServer()
	if err != nil {
		return err
	}

	// Open browser
	authURL := auth.AuthorizeURL(challenge, server.RedirectURI(), authProvider)
	fmt.Println("Opening browser for login...")
	fmt.Printf("\nIf the browser doesn't open, visit:\n%s\n\n", authURL)
	_ = openBrowser(authURL)

	// Wait for callback (2 min timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("Waiting for login...")
	code, err := server.WaitForCode(ctx)
	if err != nil {
		return err
	}

	// Exchange code for tokens
	result, err := auth.ExchangeCode(code, verifier)
	if err != nil {
		return err
	}

	// Save tokens
	storage := auth.NewStorage()
	if err := storage.Save(&auth.Tokens{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
	}); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	ui.PrintSuccess("Login successful!")
	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
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
	authLoginCmd.Flags().StringVar(&authProvider, "provider", "google", "OAuth provider (google)")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
	authCmd.AddCommand(authTokenCmd)
	rootCmd.AddCommand(authCmd)
}
