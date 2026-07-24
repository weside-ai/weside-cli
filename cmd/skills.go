package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	skillCompanion string
	skillConfig    string
	skillEnabled   bool
	skillLimit     int
)

var companionSkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage skills installed on a companion",
}

var skillsAvailableCmd = &cobra.Command{
	Use:   "available",
	Short: "List published skill definitions you can install",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/companions/available?limit=%d", skillLimit)
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("listing available skills: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		skills, _ := result["skills"].([]any)
		headers := []string{"ID", "NAME", "VERSION", "CATEGORY", "INSTALLS"}
		var rows [][]string
		for _, item := range skills {
			s, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", s["id"]),
				truncate(fmt.Sprintf("%v", s["name"]), 30),
				fmt.Sprintf("%v", s["version"]),
				fmt.Sprintf("%v", s["category"]),
				fmt.Sprintf("%v", s["install_count"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List skills installed on a companion",
	RunE: func(_ *cobra.Command, _ []string) error {
		companionID, err := resolveCompanion(skillCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/companions/"+companionID+"/skills", &result); err != nil {
			return fmt.Errorf("listing skills: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		skills, _ := result["skills"].([]any)
		headers := []string{"DEF ID", "NAME", "VERSION", "ENABLED"}
		var rows [][]string
		for _, item := range skills {
			s, _ := item.(map[string]any)
			def, _ := s["skill"].(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", s["skill_definition_id"]),
				truncate(fmt.Sprintf("%v", def["name"]), 30),
				fmt.Sprintf("%v", def["version"]),
				fmt.Sprintf("%v", s["enabled"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install <skill_definition_id>",
	Short: "Install a skill on a companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(skillCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		defID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid skill definition id %q: %w", args[0], err)
		}
		body := map[string]any{"skill_definition_id": defID}
		if cfg, ok := parseJSONConfig(skillConfig); ok {
			body["config_json"] = cfg
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/companions/"+companionID+"/skills", body, &result); err != nil {
			return fmt.Errorf("installing skill: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Skill %s installed.", args[0])
		return nil
	},
}

var skillsSetCmd = &cobra.Command{
	Use:   "set <skill_definition_id>",
	Short: "Reconfigure or toggle an installed skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(skillCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if cfg, ok := parseJSONConfig(skillConfig); ok {
			body["config_json"] = cfg
		}
		if cmd.Flags().Changed("enabled") {
			body["enabled"] = skillEnabled
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to set (use --enabled and/or --config)")
		}

		if err := client.Patch(context.Background(), "/companions/"+companionID+"/skills/"+args[0], body, nil); err != nil {
			return fmt.Errorf("updating skill: %w", err)
		}

		ui.PrintSuccess("Skill %s updated.", args[0])
		return nil
	},
}

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall <skill_definition_id>",
	Short: "Uninstall a skill from a companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		companionID, err := resolveCompanion(skillCompanion)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		if err := client.Delete(context.Background(), "/companions/"+companionID+"/skills/"+args[0], nil); err != nil {
			return fmt.Errorf("uninstalling skill: %w", err)
		}

		ui.PrintSuccess("Skill %s uninstalled.", args[0])
		return nil
	},
}

// parseJSONConfig parses --config JSON into a map; returns ok=false if empty.
func parseJSONConfig(s string) (map[string]any, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(s), &cfg); err != nil {
		return nil, false
	}
	return cfg, true
}

func init() {
	companionSkillsCmd.PersistentFlags().StringVar(&skillCompanion, "companion", "", "companion id or name (default: selected companion)")
	skillsAvailableCmd.Flags().IntVar(&skillLimit, "limit", 50, "max results")
	skillsInstallCmd.Flags().StringVar(&skillConfig, "config", "", "skill config as JSON object")
	skillsSetCmd.Flags().StringVar(&skillConfig, "config", "", "skill config as JSON object (clears with null)")
	skillsSetCmd.Flags().BoolVar(&skillEnabled, "enabled", false, "enable or disable the skill")

	companionSkillsCmd.AddCommand(skillsAvailableCmd)
	companionSkillsCmd.AddCommand(skillsListCmd)
	companionSkillsCmd.AddCommand(skillsInstallCmd)
	companionSkillsCmd.AddCommand(skillsSetCmd)
	companionSkillsCmd.AddCommand(skillsUninstallCmd)
	companionsCmd.AddCommand(companionSkillsCmd)
}
