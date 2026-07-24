package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

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

var authTokenDecode bool

var authTokenCmd = &cobra.Command{
	Use:   "token [--decode]",
	Short: "Print the current access token (for scripting)",
	RunE: func(_ *cobra.Command, _ []string) error {
		token, err := auth.GetToken()
		if err != nil {
			return err
		}
		if authTokenDecode {
			return printDecodedJWT(token)
		}
		_, _ = fmt.Fprint(os.Stdout, token)
		return nil
	},
}

// printDecodedJWT decodes the JWT payload (middle segment) and prints its
// claims. No signature verification — display only, for debugging 401s.
func printDecodedJWT(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("not a JWT (expected 3 dot-separated parts, got %d)", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decoding JWT payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("parsing JWT claims: %w", err)
	}

	if IsJSON() {
		ui.PrintJSON(claims)
		return nil
	}
	fmt.Printf("sub:        %v\n", claims["sub"])
	if email, ok := claims["email"]; ok {
		fmt.Printf("email:      %v\n", email)
	}
	if role, ok := claims["role"]; ok {
		fmt.Printf("role:       %v\n", role)
	}
	if isAnon, ok := claims["is_anonymous"]; ok {
		fmt.Printf("anonymous:  %v\n", isAnon)
	}
	if exp, ok := claims["exp"]; ok {
		printJWTExp(exp)
	}
	if iat, ok := claims["iat"]; ok {
		fmt.Printf("issued:     %v\n", timeFromUnix(iat))
	}
	return nil
}

func printJWTExp(exp any) {
	switch v := exp.(type) {
	case float64:
		t := time.Unix(int64(v), 0).UTC()
		remaining := time.Until(t).Round(time.Second)
		if remaining > 0 {
			fmt.Printf("expires:    %s (in %s)\n", t.Format(time.RFC3339), remaining)
		} else {
			fmt.Printf("expires:    %s (EXPIRED %s ago)\n", t.Format(time.RFC3339), -remaining)
		}
	default:
		fmt.Printf("expires:    %v\n", exp)
	}
}

func timeFromUnix(v any) string {
	if f, ok := v.(float64); ok {
		return time.Unix(int64(f), 0).UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("%v", v)
}

func loginPKCE() error {
	// Resolve auth-config (override → cache → live → fallback) before anything else.
	resolveCtx, resolveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer resolveCancel()
	res := auth.Resolve(resolveCtx, GetAPIURL())
	// Partial override (only one of supabase_url / supabase_anon_key set) is
	// always a misconfiguration — warn unconditionally so the user notices
	// that login is silently proceeding against the prod defaults.
	if errors.Is(res.FetchError, auth.ErrPartialOverride) {
		fmt.Fprintf(os.Stderr, "auth-config: %v — falling back to hardcoded defaults\n", res.FetchError)
	} else if IsVerbose() {
		switch res.Source {
		case auth.SourceFallback:
			fmt.Fprintf(os.Stderr, "auth-config: using hardcoded fallback (well-known fetch failed: %v)\n", res.FetchError)
		default:
			fmt.Fprintf(os.Stderr, "auth-config: source=%s\n", res.Source)
		}
	}
	cfg := res.Config

	// Generate PKCE verifier + challenge and an OAuth state (CSRF binding).
	verifier, err := auth.GenerateVerifier()
	if err != nil {
		return err
	}
	challenge := auth.GenerateChallenge(verifier)
	state, err := auth.GenerateState()
	if err != nil {
		return err
	}

	// Start callback server. Try the resolved port first, then the two
	// fallback ports — all three must be registered redirect_uris on the
	// OAuth client (the OAuth 2.1 server validates redirect_uri exactly).
	server, err := auth.NewCallbackServer(cfg.CallbackPort, cfg.CallbackPort+1, cfg.CallbackPort+2)
	if err != nil {
		return err
	}
	server.SetExpectedState(state)

	// Open browser to the weside OAuth login page (provider choice happens there).
	authURL := auth.AuthorizeURL(cfg.SupabaseURL, cfg.OAuthClientID, challenge, server.RedirectURI(), state)
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
	result, err := auth.ExchangeCode(cfg.SupabaseURL, cfg.SupabaseAnonKey, cfg.OAuthClientID, code, verifier, server.RedirectURI())
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
	authTokenCmd.Flags().BoolVar(&authTokenDecode, "decode", false, "decode the JWT and print its claims")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
	authCmd.AddCommand(authTokenCmd)
	rootCmd.AddCommand(authCmd)
}
