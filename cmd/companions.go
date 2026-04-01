package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/auth"
	"github.com/weside-ai/weside-cli/internal/config"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var companionsCmd = &cobra.Command{
	Use:   "companions",
	Short: "Manage your Companions",
}

var companionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all your Companions",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		}
		if err := client.Get(context.Background(), "/companions", &result); err != nil {
			return fmt.Errorf("listing companions: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		defaultComp := config.GetDefaultCompanion()

		headers := []string{"ID", "NAME", "PERSONALITY", "DEFAULT"}
		var rows [][]string
		for _, c := range result.Items {
			id := fmt.Sprintf("%v", c["id"])
			name := fmt.Sprintf("%v", c["name"])
			personality := truncate(fmt.Sprintf("%v", c["personality"]), 40)
			def := ""
			if name == defaultComp {
				def = "*"
			}
			rows = append(rows, []string{id, name, personality, def})
		}

		ui.PrintTable(headers, rows)
		fmt.Printf("\n%d companion(s)\n", result.Total)
		return nil
	},
}

var companionsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show Companion details",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var companion map[string]any
		if err := client.Get(context.Background(), "/companions/"+args[0], &companion); err != nil {
			return fmt.Errorf("getting companion: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(companion)
			return nil
		}

		fmt.Printf("ID:          %v\n", companion["id"])
		fmt.Printf("Name:        %v\n", companion["name"])
		fmt.Printf("Personality: %v\n", companion["personality"])
		if model, ok := companion["llm_model"]; ok && model != nil {
			fmt.Printf("Model:       %v\n", model)
		}
		if created, ok := companion["created_at"]; ok {
			fmt.Printf("Created:     %v\n", created)
		}
		return nil
	},
}

var (
	compName        string
	compPersonality string
)

var companionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Companion",
	RunE: func(_ *cobra.Command, _ []string) error {
		if compName == "" {
			return fmt.Errorf("--name is required")
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]string{
			"name":        compName,
			"personality": compPersonality,
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/companions", body, &result); err != nil {
			return fmt.Errorf("creating companion: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		ui.PrintSuccess("Companion %q created (ID: %v)", result["name"], result["id"])
		return nil
	},
}

var companionsSelectCmd = &cobra.Command{
	Use:   "select <name>",
	Short: "Set the default Companion for chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]

		// Verify the companion exists
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result struct {
			Items []map[string]any `json:"items"`
		}
		if err := client.Get(context.Background(), "/companions", &result); err != nil {
			return fmt.Errorf("listing companions: %w", err)
		}

		found := false
		var foundID string
		for _, c := range result.Items {
			if fmt.Sprintf("%v", c["name"]) == name {
				found = true
				foundID = fmt.Sprintf("%v", c["id"])
				break
			}
		}

		if !found {
			// Try by ID
			for _, c := range result.Items {
				if fmt.Sprintf("%v", c["id"]) == name {
					found = true
					foundID = name
					name = fmt.Sprintf("%v", c["name"])
					break
				}
			}
		}

		if !found {
			return fmt.Errorf("companion %q not found", name)
		}

		viper.Set("default_companion", name)
		viper.Set("default_companion_id", foundID)
		if err := config.SetDefaultCompanion(name); err != nil {
			return fmt.Errorf("saving default companion: %w", err)
		}

		ui.PrintSuccess("Default Companion set to %q (ID: %s)", name, foundID)
		return nil
	},
}

func newAuthenticatedClient() (*api.Client, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, err
	}
	return api.NewClient(GetAPIURL()+"/api/v1", token), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func resolveCompanionID(client *api.Client, nameOrID string) (string, error) {
	// If it's a number, assume ID
	if _, err := strconv.Atoi(nameOrID); err == nil {
		return nameOrID, nil
	}

	// Otherwise, look up by name
	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := client.Get(context.Background(), "/companions", &result); err != nil {
		return "", fmt.Errorf("listing companions: %w", err)
	}

	for _, c := range result.Items {
		if fmt.Sprintf("%v", c["name"]) == nameOrID {
			return fmt.Sprintf("%v", c["id"]), nil
		}
	}

	return "", fmt.Errorf("companion %q not found", nameOrID)
}

func init() {
	companionsCreateCmd.Flags().StringVar(&compName, "name", "", "companion name")
	companionsCreateCmd.Flags().StringVar(&compPersonality, "personality", "", "companion personality description")
	companionsCmd.AddCommand(companionsListCmd)
	companionsCmd.AddCommand(companionsShowCmd)
	companionsCmd.AddCommand(companionsCreateCmd)
	companionsCmd.AddCommand(companionsSelectCmd)
	rootCmd.AddCommand(companionsCmd)
}
