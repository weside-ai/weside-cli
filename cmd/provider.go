package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage AI provider and data residency",
}

var providerShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current provider configuration",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		// Trailing slash required (307 redirect without it)
		var result map[string]any
		if err := client.Get(context.Background(), "/data-residency/", &result); err != nil {
			return fmt.Errorf("getting provider config: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		ui.Printf("Type:   %v\n", result["type"])
		ui.Printf("Preset: %v\n", result["preset_display_name"])
		ui.Printf("Model:  %v\n", result["model_name"])
		if region := result["region"]; region != nil {
			ui.Printf("Region: %v\n", region)
		}
		if result["has_api_key"] == true {
			ui.Printf("BYOK:   yes\n")
		}
		printEuOnly(result)
		return nil
	},
}

// printEuOnly renders the WA-2257 switch, and only when the server sent it —
// an older backend has no such field and must not print "EU only: false",
// which would be a claim about a setting that does not exist there.
//
// The count is the point of the line. `eu_only_disables` is what the switch
// takes away, so a user who sees "on (9 sources hidden)" knows there is
// something to look at; `--json` carries the rows themselves.
func printEuOnly(result map[string]any) {
	value, present := result["eu_only"]
	if !present {
		return
	}

	state := "off"
	if value == true {
		state = "on"
	}

	hidden, _ := result["eu_only_disables"].([]any)
	if value == true && len(hidden) > 0 {
		ui.Printf("EU only: %s (%d sources hidden)\n", state, len(hidden))
		return
	}
	if value != true && result["eu_only_available"] == false {
		ui.Printf("EU only: %s (needs the EU region)\n", state)
		return
	}
	ui.Printf("EU only: %s\n", state)
}

var providerEuOnlyCmd = &cobra.Command{
	Use:   "eu-only <on|off>",
	Short: "Turn EU only on or off (WA-2257)",
	// The 409's sentence IS this command's most useful output, and cobra
	// prints fifteen lines of usage above any RunE error by default — which
	// buries it. Usage belongs to a malformed invocation, not to a server
	// that answered clearly. Measured against staging rc.3.
	SilenceUsage: true,
	Long: `Turn the EU-only switch on or off.

With it on, nothing weside CHOOSES for you leaves the EU: no platform tool
whose processor sits outside it, no image model on a non-EU endpoint, no
speech output. What you connected yourself is your own choice and stays.

It can only be turned ON while your inference runs in the EU — pick the EU
region first with 'weside provider presets' and 'weside provider set <id>'.
Moving to any other region clears it again.

'weside provider show' prints the current state and how many sources it hides.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		var value bool
		switch args[0] {
		case "on":
			value = true
		case "off":
			value = false
		default:
			return fmt.Errorf("expected 'on' or 'off', got %q", args[0])
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		// The discriminated union's fourth shape: it modifies the current
		// selection instead of replacing it, so region, quality and preset id
		// are not restated here.
		body := map[string]any{"type": "eu_only", "eu_only": value}
		var result map[string]any
		if err := client.Put(context.Background(), "/data-residency/", body, &result); err != nil {
			var apiErr *api.Error
			// 409 is the one refusal a user can act on, and the server's own
			// sentence says what to do — pass it through rather than wrapping
			// it in "setting EU only: …", which buries the instruction.
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
				return fmt.Errorf("%s", apiErr.Detail)
			}
			return fmt.Errorf("setting EU only: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		if value {
			hidden, _ := result["eu_only_disables"].([]any)
			ui.PrintSuccess("EU only is on — %d sources are now hidden", len(hidden))
			return nil
		}
		ui.PrintSuccess("EU only is off")
		return nil
	},
}

var providerPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List available regional presets",
	RunE: func(_ *cobra.Command, _ []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/data-residency/presets", &result); err != nil {
			return fmt.Errorf("listing presets: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		// API returns {groups: [{region: "EUR", presets: [...]}]}
		groups, _ := result["groups"].([]any)
		for _, gItem := range groups {
			group, _ := gItem.(map[string]any)
			region := fmt.Sprintf("%v", group["region"])
			ui.Printf("\n%s:\n", region)

			presets, _ := group["presets"].([]any)
			headers := []string{"ID", "TIER", "NAME", "DESCRIPTION"}
			var rows [][]string
			for _, pItem := range presets {
				p, _ := pItem.(map[string]any)
				id := fmt.Sprintf("%v", p["id"])
				tier := fmt.Sprintf("%v", p["tier"])
				name := fmt.Sprintf("%v", p["display_name"])
				desc := truncate(fmt.Sprintf("%v", p["description"]), 50)
				rows = append(rows, []string{id, tier, name, desc})
			}
			ui.PrintTable(headers, rows)
		}
		return nil
	},
}

var providerSetCmd = &cobra.Command{
	Use:   "set <preset_id>",
	Short: "Set provider preset (use numeric ID from 'presets')",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		presetID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("preset_id must be a number (use 'weside provider presets' to see IDs)")
		}

		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/data-residency/presets", &result); err != nil {
			return fmt.Errorf("listing presets: %w", err)
		}

		groups, _ := result["groups"].([]any)
		body, err := buildSetRequestBody(presetID, groups)
		if err != nil {
			return err
		}
		if err := client.Put(context.Background(), "/data-residency/", body, nil); err != nil {
			return fmt.Errorf("setting provider: %w", err)
		}

		ui.PrintSuccess("Provider preset set to %d", presetID)
		return nil
	},
}

func buildSetRequestBody(presetID int, groups []any) (map[string]any, error) {
	for _, gItem := range groups {
		group, _ := gItem.(map[string]any)
		region := fmt.Sprintf("%v", group["region"])
		if region != "EUR" && region != "USA" && region != "WESIDE" {
			continue
		}

		presets, _ := group["presets"].([]any)
		for _, pItem := range presets {
			preset, _ := pItem.(map[string]any)
			// encoding/json decodes numbers into float64; comparing their
			// %v rendering would break at seven digits ("1e+06").
			id, ok := preset["id"].(float64)
			if !ok || int(id) != presetID {
				continue
			}

			if region == "WESIDE" {
				return map[string]any{"type": "weside", "preset_id": presetID}, nil
			}
			return map[string]any{
				"type":      "region",
				"region":    region,
				"preset_id": presetID,
			}, nil
		}
	}

	return nil, fmt.Errorf(
		"preset_id %d not found (use 'weside provider presets' to see valid IDs)",
		presetID,
	)
}

var providerByokCmd = &cobra.Command{
	Use:   "byok <provider> [key] [--key-stdin]",
	Short: "Bring Your Own Key",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := ""
		if len(args) == 2 {
			key = args[1]
		}
		key, supplied, err := secretInput(cmd, "key-stdin", key, len(args) == 2)
		if err != nil {
			return err
		}
		if !supplied {
			return fmt.Errorf("a key argument or --key-stdin is required")
		}
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		body := map[string]any{
			"type":     "byok",
			"provider": args[0],
			"api_key":  key,
		}
		if err := client.Put(context.Background(), "/data-residency/", body, nil); err != nil {
			return fmt.Errorf("setting BYOK: %w", err)
		}

		ui.PrintSuccess("BYOK configured for %s", args[0])
		return nil
	},
}

func init() {
	addSecretStdinFlag(providerByokCmd, "key-stdin")
	providerCmd.AddCommand(providerShowCmd)
	providerCmd.AddCommand(providerEuOnlyCmd)
	providerCmd.AddCommand(providerPresetsCmd)
	providerCmd.AddCommand(providerSetCmd)
	providerCmd.AddCommand(providerByokCmd)
	rootCmd.AddCommand(providerCmd)
}
