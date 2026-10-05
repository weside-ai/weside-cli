package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// Stage app verbs (WA-2363): an app's data, its tool routes, and the shares
// that open it to a room. Two route families serve the same tools:
//
//   - owner:  /stage/artifacts/{artifact_id}/tools/{tool}        (WA-2379)
//   - member: /rooms/{room_id}/apps/{artifact_id}/tools/{tool}   (WA-2383)
//
// --room picks the member route, so a room member who does not own the app
// reads and writes through the grant the owner shared into that room.

// appToolPath builds the tool route for one artifact, owner or room scoped.
func appToolPath(artifactID string, roomID int, tool string) string {
	a := url.PathEscape(artifactID)
	t := url.PathEscape(tool)
	if roomID > 0 {
		return fmt.Sprintf("/rooms/%d/apps/%s/tools/%s", roomID, a, t)
	}
	return fmt.Sprintf("/stage/artifacts/%s/tools/%s", a, t)
}

// callAppTool POSTs a raw JSON body to one app tool route.
func callAppTool(ctx context.Context, c *api.Client, artifactID string, roomID int, tool string, body json.RawMessage) (map[string]any, error) {
	var result map[string]any
	if err := c.Post(ctx, appToolPath(artifactID, roomID, tool), body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// readAppData calls app_data_read; an empty key sends no key and lists the keys.
func readAppData(ctx context.Context, c *api.Client, artifactID string, roomID int, key string) (map[string]any, error) {
	body := map[string]any{}
	if key != "" {
		body["key"] = key
	}
	raw, _ := json.Marshal(body)
	return callAppTool(ctx, c, artifactID, roomID, "app_data_read", raw)
}

// writeAppData calls app_data_write; a nil expectedRevision omits the key, so
// the write always lands.
func writeAppData(
	ctx context.Context, c *api.Client, artifactID string, roomID int,
	key string, value json.RawMessage, expectedRevision *int,
) (map[string]any, error) {
	body := map[string]any{"key": key, "value": value}
	if expectedRevision != nil {
		body["expected_revision"] = *expectedRevision
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return callAppTool(ctx, c, artifactID, roomID, "app_data_write", raw)
}

// shareCapabilities is what members of the room may do with the app's data.
func shareCapabilities(viewOnly bool) []string {
	if viewOnly {
		return []string{"app_data_read"}
	}
	return []string{"app_data_read", "app_data_write"}
}

func shareApp(ctx context.Context, c *api.Client, artifactID string, roomID int, viewOnly bool) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/stage/artifacts/%s/shares/%d", url.PathEscape(artifactID), roomID)
	body := map[string]any{"capabilities": shareCapabilities(viewOnly)}
	if err := c.Put(ctx, path, body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func unshareApp(ctx context.Context, c *api.Client, artifactID string, roomID int) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/stage/artifacts/%s/shares/%d", url.PathEscape(artifactID), roomID)
	if err := c.Delete(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func listRoomApps(ctx context.Context, c *api.Client, roomID int) (map[string]any, error) {
	var result map[string]any
	if err := c.Get(ctx, fmt.Sprintf("/rooms/%d/apps", roomID), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// parseJSONArg rejects a body that is not valid JSON before it reaches the server.
func parseJSONArg(flag, spec string) (json.RawMessage, error) {
	raw, err := readAPICmdBody(spec)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", flag, err)
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s is not valid JSON: %q", flag, truncate(strings.TrimSpace(string(raw)), 80))
	}
	return raw, nil
}

func printAppResult(result map[string]any, summary string) {
	if IsJSON() {
		ui.PrintJSON(result)
		return
	}
	ui.Println(summary)
}

var (
	stageDataKey       string
	stageDataRoom      string
	stageDataSetKey    string
	stageDataSetValue  string
	stageDataSetRev    int
	stageDataSetRoom   string
	stageCallBody      string
	stageCallRoom      string
	stageShareRoom     string
	stageShareViewOnly bool
	stageUnshareRoom   string
)

var stageDataCmd = &cobra.Command{
	Use:   "data <artifact_id>",
	Short: "Read an app's data: one key, or the list of keys",
	Long: `Read a Stage app's data through app_data_read.

Without --key the answer lists the keys. --room reads as a member of that room,
through the share its owner granted, instead of as the app's owner.

Examples:
  weside stage data 812
  weside stage data 812 --key scores
  weside stage data 812 --key scores --room 42 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		roomID, err := parseRoomFlag(stageDataRoom)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := readAppData(cmd.Context(), client, args[0], roomID, stageDataKey)
		if err != nil {
			return fmt.Errorf("reading app data: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		printAppData(result, stageDataKey)
		return nil
	},
}

// printAppData renders an app_data_read answer. Keys and values are written by
// any member a share lets write, so every line goes through ui.SafeText.
func printAppData(result map[string]any, key string) {
	if key == "" {
		keys, _ := result["keys"].([]any)
		if len(keys) == 0 {
			ui.Println("This app holds no data yet.")
			return
		}
		for _, k := range keys {
			ui.Println(ui.SafeText(fmt.Sprintf("%v", k)))
		}
		return
	}
	if exists, _ := result["exists"].(bool); !exists {
		ui.Printf("Key %s is not set.\n", ui.SafeText(key))
		return
	}
	value, _ := json.MarshalIndent(result["value"], "", "  ")
	ui.Printf("%s (revision %v)\n%s\n", ui.SafeText(key), result["revision"], ui.SafeText(string(value)))
}

var stageDataSetCmd = &cobra.Command{
	Use:   "set <artifact_id>",
	Short: "Write one key of an app's data (null deletes it)",
	Long: `Write one key through app_data_write.

--value is any JSON value (inline, @file, or - for stdin); null deletes the key.
--expected-revision writes only while the key still has that revision (0 = the key must not exist yet);
a mismatch answers 409 app-data-conflict.

Examples:
  weside stage data set 812 --key scores --value '{"foxy": 3}'
  weside stage data set 812 --key scores --value @scores.json --expected-revision 4
  weside stage data set 812 --key scores --value null --room 42`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		roomID, err := parseRoomFlag(stageDataSetRoom)
		if err != nil {
			return err
		}
		value, err := parseJSONArg("--value", stageDataSetValue)
		if err != nil {
			return err
		}
		var expected *int
		if cmd.Flags().Changed("expected-revision") {
			if stageDataSetRev < 0 {
				return fmt.Errorf("--expected-revision must be 0 or more, got %d", stageDataSetRev)
			}
			expected = &stageDataSetRev
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := writeAppData(cmd.Context(), client, args[0], roomID, stageDataSetKey, value, expected)
		if err != nil {
			return fmt.Errorf("writing app data: %w", err)
		}
		if deleted, _ := result["deleted"].(bool); deleted {
			printAppResult(result, fmt.Sprintf("Key %s deleted.", stageDataSetKey))
			return nil
		}
		printAppResult(result, fmt.Sprintf("Key %s written, revision %v.", stageDataSetKey, result["revision"]))
		return nil
	},
}

var stageCallCmd = &cobra.Command{
	Use:   "call <artifact_id> <tool>",
	Short: "Call one of an app's tool routes with a raw JSON body",
	Long: `POST a raw JSON body to an app tool route and print the answer.

Without --room it calls the owner route /stage/artifacts/<id>/tools/<tool>
(app_data_read, app_data_write, companion_event, open_link). --room calls the
member route /rooms/<room>/apps/<id>/tools/<tool>, which serves the data tools.

Examples:
  weside stage call 812 companion_event --body '{"name":"won","payload":{}}'
  weside stage call 812 app_data_read --body '{"key":"scores"}' --room 42`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		roomID, err := parseRoomFlag(stageCallRoom)
		if err != nil {
			return err
		}
		body, err := parseJSONArg("--body", stageCallBody)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := callAppTool(cmd.Context(), client, args[0], roomID, args[1], body)
		if err != nil {
			return fmt.Errorf("calling %s: %w", args[1], err)
		}
		// A tool answer has no common shape, so the plain output is its JSON too.
		ui.PrintJSON(result)
		return nil
	},
}

var stageShareCmd = &cobra.Command{
	Use:   "share <artifact_id>",
	Short: "Share an app into a room (members may read and write its data)",
	Long: `Share an app you own into a room. Members may read and write its data;
--view-only lets them read only. Sharing again replaces the capabilities.

Examples:
  weside stage share 812 --room 42
  weside stage share 812 --room 42 --view-only`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		roomID, err := requiredRoom(stageShareRoom)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := shareApp(cmd.Context(), client, args[0], roomID, stageShareViewOnly)
		if err != nil {
			return fmt.Errorf("sharing app: %w", err)
		}
		mode := "read and write"
		if stageShareViewOnly {
			mode = "read only"
		}
		printAppResult(result, fmt.Sprintf("App %s shared into room %d (%s).", args[0], roomID, mode))
		return nil
	},
}

var stageUnshareCmd = &cobra.Command{
	Use:   "unshare <artifact_id>",
	Short: "Stop sharing an app into a room",
	Long: `Stop sharing an app you own into a room; its members lose access.

Example:
  weside stage unshare 812 --room 42`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		roomID, err := requiredRoom(stageUnshareRoom)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := unshareApp(cmd.Context(), client, args[0], roomID)
		if err != nil {
			return fmt.Errorf("unsharing app: %w", err)
		}
		printAppResult(result, fmt.Sprintf("App %s no longer shared into room %d.", args[0], roomID))
		return nil
	},
}

var roomsAppsCmd = &cobra.Command{
	Use:   "apps <room_id>",
	Short: "List the apps shared into a room that you may open",
	Long: `List the apps shared into a room that you may open, with the artifact id
the room's Stage opens.

Example:
  weside rooms apps 42 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		roomID, err := strconv.Atoi(args[0])
		if err != nil || roomID <= 0 {
			return fmt.Errorf("room_id must be a positive room id, got %q", args[0])
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := listRoomApps(cmd.Context(), client, roomID)
		if err != nil {
			return fmt.Errorf("listing room apps: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		items, _ := result["items"].([]any)
		if len(items) == 0 {
			ui.Println("No apps are shared into this room.")
			return nil
		}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			a, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", a["artifact_id"]),
				truncate(ui.SafeText(fmt.Sprintf("%v", a["title"])), 40),
				ui.SafeText(fmt.Sprintf("%v", a["kind"])),
				ui.SafeText(fmt.Sprintf("%v", a["companion_name"])),
			})
		}
		ui.PrintTable([]string{"Artifact", "Title", "Kind", "Companion"}, rows)
		return nil
	},
}

// requiredRoom parses a room id that must be present.
func requiredRoom(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("a room id is required (--room <id>)")
	}
	return parseRoomFlag(s)
}

func init() {
	stageDataCmd.Flags().StringVar(&stageDataKey, "key", "", "the key to read; omitted lists the keys")
	stageDataCmd.Flags().StringVar(&stageDataRoom, "room", "", "read as a member of this room (shared app)")

	stageDataSetCmd.Flags().StringVar(&stageDataSetKey, "key", "", "the key to write (required)")
	stageDataSetCmd.Flags().StringVar(&stageDataSetValue, "value", "", "JSON value: inline, @file, or - for stdin; null deletes (required)")
	stageDataSetCmd.Flags().IntVar(&stageDataSetRev, "expected-revision", 0, "write only while the key has this revision (0 = the key must not exist yet)")
	stageDataSetCmd.Flags().StringVar(&stageDataSetRoom, "room", "", "write as a member of this room (shared app)")
	_ = stageDataSetCmd.MarkFlagRequired("key")
	_ = stageDataSetCmd.MarkFlagRequired("value")
	stageDataCmd.AddCommand(stageDataSetCmd)

	stageCallCmd.Flags().StringVar(&stageCallBody, "body", "{}", "JSON body: inline, @file, or - for stdin")
	stageCallCmd.Flags().StringVar(&stageCallRoom, "room", "", "call the room member route of this room")

	stageShareCmd.Flags().StringVar(&stageShareRoom, "room", "", "the room to share into (required)")
	stageShareCmd.Flags().BoolVar(&stageShareViewOnly, "view-only", false, "members may read the data but not write it")
	stageUnshareCmd.Flags().StringVar(&stageUnshareRoom, "room", "", "the room to stop sharing into (required)")
	_ = stageShareCmd.MarkFlagRequired("room")
	_ = stageUnshareCmd.MarkFlagRequired("room")

	stageCmd.AddCommand(stageDataCmd, stageCallCmd, stageShareCmd, stageUnshareCmd)
	roomsCmd.AddCommand(roomsAppsCmd)
}
