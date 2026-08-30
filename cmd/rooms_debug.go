package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// P0 rooms-debug surface — the inspection/control layer over /api/v2/rooms/*.
// list/show/delete live in rooms.go; the subcommands here are the debugging
// affordances: trace, participants, tool-call detail, cancel/undo/context-break,
// rename, and room creation (dm/group).

var (
	roomsTraceLimit        int
	roomsTraceFull         bool
	roomsConfirm           bool
	roomsCancelServerMsgID string
	roomsCancelPartial     string
	roomsRenameClear       bool
	roomsGroupCompanions   string
	roomsGroupTitle        string
	eventsSince            string
	eventsRaw              bool
	eventsNDJSON           bool
)

var roomsTraceCmd = &cobra.Command{
	Use:   "trace <room_id>",
	Short: "Show the companion checkpoint trace of a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/rooms/%s/trace?limit=%d", args[0], roomsTraceLimit)
		var result map[string]any
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("reading room trace: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		messages, _ := result["messages"].([]any)
		if len(messages) == 0 {
			fmt.Println("No trace (room has no caller-owned companion checkpoint).")
			return nil
		}
		for _, item := range messages {
			m, _ := item.(map[string]any)
			role := roleLabel(fmt.Sprintf("%v", m["role"]))
			created := fmt.Sprintf("%v", m["created_at"])
			msgID, _ := m["message_id"].(string)
			fmt.Printf("[%s] %s", role, created)
			if msgID != "" && msgID != "<nil>" {
				fmt.Printf("  (%s)", msgID)
			}
			fmt.Println()
			printTraceItems(m["trace_items"], roomsTraceFull)
			fmt.Println()
		}
		return nil
	},
}

// printTraceItems renders the slim trace (tool calls + reminders) under a message.
// Tool output is truncated unless full is true.
func printTraceItems(raw any, full bool) {
	items, _ := raw.([]any)
	for _, it := range items {
		item, _ := it.(map[string]any)
		switch fmt.Sprintf("%v", item["type"]) {
		case "tool_call":
			name, _ := item["tool_name"].(string)
			args := prettyJSON(item["tool_args"])
			output, _ := item["tool_output"].(string)
			fmt.Printf("  - tool_call %s(%s)", name, args)
			if output != "" && output != "<nil>" {
				if full {
					fmt.Printf(" -> %s", output)
				} else {
					fmt.Printf(" -> %s", truncate(output, 200))
				}
			}
			fmt.Println()
		case "reminder":
			category, _ := item["category"].(string)
			message, _ := item["message"].(string)
			if category != "" && category != "<nil>" {
				fmt.Printf("  - reminder [%s]: %s\n", category, message)
			} else {
				fmt.Printf("  - reminder: %s\n", message)
			}
		default:
			fmt.Printf("  - %v\n", item["type"])
		}
	}
}

func prettyJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

var roomsParticipantsCmd = &cobra.Command{
	Use:   "participants <room_id>",
	Short: "List the participants of a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/participants", &result); err != nil {
			return fmt.Errorf("listing participants: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		participants, _ := result["participants"].([]any)
		headers := []string{"KIND", "NAME", "ID", "ROLE", "JOINED"}
		var rows [][]string
		for _, item := range participants {
			p, _ := item.(map[string]any)
			kind := fmt.Sprintf("%v", p["kind"])
			name := fmt.Sprintf("%v", p["display_name"])
			id := participantID(p)
			role := fmt.Sprintf("%v", p["role"])
			joined := fmt.Sprintf("%v", p["joined_at"])
			rows = append(rows, []string{kind, name, id, role, joined})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

// participantID picks the relevant id field for a participant's kind.
func participantID(p map[string]any) string {
	for _, k := range []string{"user_id", "companion_id", "external_id"} {
		if v, ok := p[k]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return "-"
}

var roomsToolCallCmd = &cobra.Command{
	Use:   "tool-call <room_id> <tool_call_id>",
	Short: "Show full detail of a companion tool call in a room",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/tool-calls/"+args[1], &result); err != nil {
			return fmt.Errorf("reading tool call: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}

		if fmt.Sprintf("%v", result["detail_level"]) == "metadata_only" {
			fmt.Println("metadata only (tool call not visible to you)")
			return nil
		}
		fmt.Printf("Tool:    %s\n", result["tool_name"])
		fmt.Printf("Args:    %s\n", prettyJSON(result["args"]))
		out, _ := result["output"].(string)
		if pending, _ := result["pending"].(bool); pending {
			fmt.Println("Output:  (still running)")
		} else if out != "" && out != "<nil>" {
			fmt.Printf("Output:  %s\n", out)
		} else {
			fmt.Println("Output:  (none)")
		}
		return nil
	},
}

var roomsCancelCmd = &cobra.Command{
	Use:   "cancel <room_id>",
	Short: "Cancel the running companion turn in a room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this turns off a running companion turn — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if roomsCancelServerMsgID != "" {
			body["server_message_id"] = roomsCancelServerMsgID
		}
		if roomsCancelPartial != "" {
			body["partial_content"] = roomsCancelPartial
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/turns/cancel", body, &result); err != nil {
			return fmt.Errorf("cancelling turn: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		if cancelled, _ := result["cancelled"].(bool); cancelled {
			ui.PrintSuccess("Turn cancelled.")
		} else {
			fmt.Println("No running turn to cancel.")
		}
		return nil
	},
}

var roomsUndoCmd = &cobra.Command{
	Use:   "undo <room_id>",
	Short: "Undo the newest 1:1 turn in a room (checkpoints + timeline)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this rolls back the last turn — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		// The endpoint requires the caller to NAME the turn it means: it undoes
		// the room's newest turn and refuses with 409 when that turn does not
		// contain the id. So read the newest message first and hand its id
		// back. A turn landing between the read and the undo turns into that
		// 409 instead of silently rolling back a different turn.
		var timeline map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/messages?limit=1", &timeline); err != nil {
			return fmt.Errorf("reading room timeline: %w", err)
		}
		messages := asSlice(timeline["messages"])
		if len(messages) == 0 {
			return fmt.Errorf("room %s has no messages to undo", args[0])
		}
		newest, _ := messages[len(messages)-1].(map[string]any)
		expectedID, _ := newest["id"].(string)
		if expectedID == "" {
			return fmt.Errorf("room %s: newest message carries no id", args[0])
		}

		var result map[string]any
		body := map[string]any{"expected_message_id": expectedID}
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/undo", body, &result); err != nil {
			return fmt.Errorf("undoing turn: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Undid turn (%d message(s) removed).", len(asSlice(result["removed_message_ids"])))
		if txt, _ := result["user_message_text"].(string); txt != "" {
			fmt.Printf("Your message: %s\n", txt)
		}
		return nil
	},
}

var roomsRegenerateCmd = &cobra.Command{
	Use:   "regenerate <room_id>",
	Short: "Regenerate the newest 1:1 turn in a room (delete + re-send, idempotent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this discards the last answer and buys a new one — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		// Same "name the turn you mean" contract as undo: the endpoint acts on
		// the room's newest turn and answers 409 when that turn does not
		// contain the id. Read the newest message and hand its id back, so a
		// turn landing in between becomes that 409 rather than a regenerate of
		// something the caller never saw.
		var timeline map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/messages?limit=1", &timeline); err != nil {
			return fmt.Errorf("reading room timeline: %w", err)
		}
		messages := asSlice(timeline["messages"])
		if len(messages) == 0 {
			return fmt.Errorf("room %s has no messages to regenerate", args[0])
		}
		newest, _ := messages[len(messages)-1].(map[string]any)
		expectedID, _ := newest["id"].(string)
		if expectedID == "" {
			return fmt.Errorf("room %s: newest message carries no id", args[0])
		}

		var result map[string]any
		body := map[string]any{"expected_message_id": expectedID}
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/regenerate", body, &result); err != nil {
			return fmt.Errorf("regenerating turn: %w", err)
		}

		if IsJSON() {
			// `replayed` is the point of the JSON shape: it is the
			// machine-readable proof that a second call performed no side
			// effect. Printing the whole body keeps that assertable.
			ui.PrintJSON(result)
			return nil
		}
		if replayed, _ := result["replayed"].(bool); replayed {
			fmt.Println("Already regenerated — replayed the first result, nothing deleted, nothing charged.")
			return nil
		}
		ui.PrintSuccess("Regenerating (%d message(s) removed).", len(asSlice(result["removed_message_ids"])))
		return nil
	},
}

var roomsContextBreakCmd = &cobra.Command{
	Use:   "context-break <room_id>",
	Short: "Rotate the room's companion thread (force a fresh context)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !roomsConfirm {
			return fmt.Errorf("this rotates the companion's context — pass --confirm to proceed")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/context-break", nil, &result); err != nil {
			return fmt.Errorf("breaking context: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Context broken (marker message %v).", result["message_id"])
		return nil
	},
}

var roomsRenameCmd = &cobra.Command{
	Use:   "rename <room_id> [title]",
	Short: "Set or clear a room's title",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("clear") && len(args) < 2 {
			return fmt.Errorf("title is required (or use --clear to remove it)")
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		body := map[string]any{}
		if roomsRenameClear {
			body["title"] = nil
		} else {
			body["title"] = args[1]
		}
		var result map[string]any
		if err := client.Patch(context.Background(), "/rooms/"+args[0], body, &result); err != nil {
			return fmt.Errorf("renaming room: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Room %s renamed.", args[0])
		return nil
	},
}

var roomsGroupCmd = &cobra.Command{
	Use:   "group --companions 1,2 [--title T]",
	Short: "Create a group room seated with your companions",
	RunE: func(_ *cobra.Command, _ []string) error {
		ids, err := parseIntCSV(roomsGroupCompanions)
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		body := map[string]any{"companion_ids": ids}
		if roomsGroupTitle != "" {
			body["title"] = roomsGroupTitle
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/group", body, &result); err != nil {
			return fmt.Errorf("creating group room: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Group room created (ID: %v).", result["id"])
		return nil
	},
}

var roomsDmCmd = &cobra.Command{
	Use:   "dm <companion_id>",
	Short: "Resolve (or create) the 1:1 DM room with a companion",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/dm/"+args[0], nil, &result); err != nil {
			return fmt.Errorf("resolving DM room: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("DM room with companion %s (ID: %v).", args[0], result["id"])
		return nil
	},
}

// parseIntCSV turns "1,2,3" into []int.
func parseIntCSV(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("no ids provided")
	}
	parts := strings.Split(s, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", p, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// asSlice coerces an any into a []any (nil if not a slice).
func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// ndjsonFrame is one line of `rooms events --ndjson`'s output — a verification
// instrument's shape: `event`/`id` alongside `data` parsed as an object (not
// a re-encoded string) so a script (or a second concurrent `rooms events
// --ndjson` on another device) can diff frames without a second parse pass.
type ndjsonFrame struct {
	Event string `json:"event"`
	ID    string `json:"id,omitempty"`
	Data  any    `json:"data"`
}

// roomsEventsCmd — raw SSE mitschnitt of a room's event stream. Prints every
// frame (known and unknown types) for debugging; unlike `chat` it does not
// interpret or wait for a turn to end. Three output forms share one SSE
// frame parse: the default pretty form, `--raw` (`<type> <cursor> <json>`),
// and `--ndjson` (one JSON object per line, for `jq`/diffing two devices'
// subscriptions to the same room) — mutually exclusive with `--raw`.
var roomsEventsCmd = &cobra.Command{
	Use:   "events <room_id>",
	Short: "Stream raw room SSE events (debug)",
	Long: `Stream a room's SSE events for debugging or verification.

Default form prints each frame as id:/event:/data: lines, like the wire
format. --raw prints one line per frame as "<type> <cursor> <json>".
--ndjson prints one JSON object per line — {"event":...,"id":...,"data":{...}}
with "data" parsed as an object, "id" omitted when absent — for a script (or
a second "weside rooms events --ndjson" on another device) to consume or diff.
--raw and --ndjson are mutually exclusive.

Under --ndjson, heartbeat comments are counted rather than printed to
stdout, and a one-line summary (frames, heartbeats, close reason) goes to
stderr on exit, keeping stdout pure NDJSON. All forms exit cleanly (0) on
Ctrl-C. There is no retry: a broken stream is reported, not papered over.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if eventsRaw && eventsNDJSON {
			return fmt.Errorf("--raw and --ndjson are mutually exclusive")
		}

		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer stop()

		path := "/rooms/" + args[0] + "/events"
		if eventsSince != "" {
			// Cursors are opaque tokens — encode so any character survives.
			path += "?since=" + url.QueryEscape(eventsSince)
		}
		resp, err := client.Subscribe(ctx, path)
		if err != nil {
			return fmt.Errorf("opening event stream: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		return streamRoomEvents(ctx, resp.Body, os.Stdout, os.Stderr, eventsRaw, eventsNDJSON)
	},
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// prettyData re-indents a JSON data line for readability; returns the input
// unchanged if it is not JSON.
func prettyData(data string) string {
	var pretty bytes.Buffer
	if json.Indent(&pretty, []byte(data), "  ", "  ") == nil {
		return "\n  " + pretty.String()
	}
	return data
}

// streamRoomEvents reads SSE frames from r and writes them to out in one of
// three forms (default id:/event:/data: lines, raw "<type> <cursor> <json>",
// or one-JSON-object-per-line ndjson), returning once r is exhausted (server
// closed the stream) or ctx is cancelled (SIGINT — reported, not returned as
// an error). Under ndjson, heartbeat comments are counted rather than
// written to out, and a one-line summary (frames, heartbeats, close reason)
// goes to stderr — the default/raw forms keep their prior stdout contract
// unchanged and stay silent on stderr. A malformed ndjson data payload, a
// genuine (non-cancellation) read error, or a failed write to out comes
// back as err — deliberately no retry, so a caller sees a break as a break.
// The stderr summary is best-effort: a failed write there never masks the
// error being reported.
func streamRoomEvents(ctx context.Context, r io.Reader, out, stderr io.Writer, raw, ndjson bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var ndjsonOut *bufio.Writer
	var ndjsonEnc *json.Encoder
	if ndjson {
		ndjsonOut = bufio.NewWriter(out)
		ndjsonEnc = json.NewEncoder(ndjsonOut)
	}

	var (
		curID, eventType   string
		dataLines          []string
		frames, heartbeats int
	)
	flush := func() error {
		if eventType == "" && curID == "" && len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		switch {
		case ndjson:
			frame := ndjsonFrame{Event: eventType, ID: curID}
			if data != "" {
				if err := json.Unmarshal([]byte(data), &frame.Data); err != nil {
					return fmt.Errorf("parsing data for event %q: %w", eventType, err)
				}
			}
			if err := ndjsonEnc.Encode(&frame); err != nil {
				return err
			}
			if err := ndjsonOut.Flush(); err != nil {
				return err
			}
		case raw:
			if _, err := fmt.Fprintf(out, "%s %s %s\n", orDash(eventType), orDash(curID), data); err != nil {
				return err
			}
		default:
			if curID != "" {
				if _, err := fmt.Fprintf(out, "id: %s\n", curID); err != nil {
					return err
				}
			}
			if eventType != "" {
				if _, err := fmt.Fprintf(out, "event: %s\n", eventType); err != nil {
					return err
				}
			}
			if data != "" {
				if _, err := fmt.Fprintf(out, "data: %s\n", prettyData(data)); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
		frames++
		curID, eventType = "", ""
		dataLines = nil
		return nil
	}

	var flushErr error
scanLoop:
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				flushErr = err
				break scanLoop
			}
		case strings.HasPrefix(line, ":"):
			// SSE comment (e.g. ":heartbeat"). NDJSON counts it and keeps
			// stdout pure; the other forms surface it as before.
			if ndjson {
				heartbeats++
			} else {
				if _, err := fmt.Fprintf(out, "%s\n", line); err != nil {
					flushErr = err
					break scanLoop
				}
			}
		case strings.HasPrefix(line, "id: "):
			curID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if flushErr == nil {
		flushErr = flush()
	}
	readErr := scanner.Err()

	if !ndjson {
		if flushErr != nil {
			return flushErr
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("reading event stream: %w", readErr)
		}
		return nil
	}

	switch {
	case flushErr != nil:
		_, _ = fmt.Fprintf(stderr, "events: %s (frames=%d heartbeats=%d)\n", flushErr, frames, heartbeats)
		return flushErr
	case readErr != nil && ctx.Err() != nil:
		_, _ = fmt.Fprintf(stderr, "events: interrupted (frames=%d heartbeats=%d)\n", frames, heartbeats)
		return nil
	case readErr != nil:
		wrapped := fmt.Errorf("reading event stream: %w", readErr)
		_, _ = fmt.Fprintf(stderr, "events: %s (frames=%d heartbeats=%d)\n", wrapped, frames, heartbeats)
		return wrapped
	default:
		_, _ = fmt.Fprintf(stderr, "events: stream closed by server (frames=%d heartbeats=%d)\n", frames, heartbeats)
		return nil
	}
}

// rooms invites subgroup — list/create/revoke room invites.
var roomsInvitesCmd = &cobra.Command{
	Use:   "invites",
	Short: "Manage a group room's invites",
}

var roomsInvitesListCmd = &cobra.Command{
	Use:   "list <room_id>",
	Short: "List a room's active invites",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Get(context.Background(), "/rooms/"+args[0]+"/invites", &result); err != nil {
			return fmt.Errorf("listing invites: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		invites, _ := result["invites"].([]any)
		headers := []string{"ID", "CODE", "EXPIRES"}
		var rows [][]string
		for _, item := range invites {
			inv, _ := item.(map[string]any)
			rows = append(rows, []string{
				fmt.Sprintf("%v", inv["id"]),
				fmt.Sprintf("%v", inv["code"]),
				fmt.Sprintf("%v", inv["expires_at"]),
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

var roomsInvitesCreateCmd = &cobra.Command{
	Use:   "create <room_id>",
	Short: "Mint a 7-day single-use invite for a group room",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		var result map[string]any
		if err := client.Post(context.Background(), "/rooms/"+args[0]+"/invites", nil, &result); err != nil {
			return fmt.Errorf("creating invite: %w", err)
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Invite created: %v (expires %v)", result["code"], result["expires_at"])
		return nil
	},
}

// acceptInvite / previewInvite are extracted from their commands so a test can
// drive the REAL path construction against an httptest server. A test that
// posts a hand-written path only ever proves itself — and the mistake worth
// catching here is precisely a wrong path: an accept is code-scoped
// (/rooms/invites/<code>/accept), not room-scoped like its siblings, and a
// wrong one returns a 404 indistinguishable from the deliberate uniform 404
// for an invalid code.
func acceptInvite(
	ctx context.Context, client *api.Client, code string,
) (map[string]any, error) {
	var result map[string]any
	if err := client.Post(ctx, "/rooms/invites/"+code+"/accept", nil, &result); err != nil {
		return nil, fmt.Errorf("accepting invite: %w", err)
	}
	return result, nil
}

func previewInvite(
	ctx context.Context, client *api.Client, code string,
) (map[string]any, error) {
	var result map[string]any
	if err := client.Get(ctx, "/rooms/invites/"+code, &result); err != nil {
		return nil, fmt.Errorf("previewing invite: %w", err)
	}
	return result, nil
}

var roomsInvitesAcceptCmd = &cobra.Command{
	Use:   "accept <code>",
	Short: "Redeem an invite code and join the group room",
	Long: `Redeem an invite code as the logged-in user and join that group room.

The other half of ` + "`invites create`" + `: minting could be driven from the CLI
while redeeming could not, so verifying human-to-human rooms needed a raw API
call for the one step that requires the SECOND identity — exactly the step worth
having a verb for.

An unknown, expired, revoked or block-gated code all answer the same 404 by
design, so the error here cannot tell you which it was.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := acceptInvite(context.Background(), client, args[0])
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess("Joined room %v (%v).", result["id"], result["title"])
		return nil
	},
}

var roomsInvitesPreviewCmd = &cobra.Command{
	Use:   "preview <code>",
	Short: "Show what an invite code reveals before joining",
	Long: `Read the four public fields a held code exposes: room title, who invited,
how many humans and how many companions are in the room.

Deliberately minimal — a code is not a directory entry, so this shows enough to
decide whether to join and nothing more.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		result, err := previewInvite(context.Background(), client, args[0])
		if err != nil {
			return err
		}
		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		ui.PrintSuccess(
			"%v — invited by %v · %v humans, %v companions",
			result["room_title"], result["inviter_display_name"],
			result["human_count"], result["companion_count"],
		)
		return nil
	},
}

var roomsInvitesRevokeCmd = &cobra.Command{
	Use:   "revoke <room_id> <invite_id>",
	Short: "Revoke an active invite",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		if err := client.Delete(context.Background(), "/rooms/"+args[0]+"/invites/"+args[1], nil); err != nil {
			return fmt.Errorf("revoking invite: %w", err)
		}
		ui.PrintSuccess("Invite %s revoked.", args[1])
		return nil
	},
}

func init() {
	roomsTraceCmd.Flags().IntVar(&roomsTraceLimit, "limit", 50, "max trace rows (1-200)")
	roomsTraceCmd.Flags().BoolVar(&roomsTraceFull, "full", false, "show full tool output (no truncation)")
	roomsCancelCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsCancelCmd.Flags().StringVar(&roomsCancelServerMsgID, "server-message-id", "", "cancel a specific turn")
	roomsCancelCmd.Flags().StringVar(&roomsCancelPartial, "partial-content", "", "persist partial content before cancelling")
	roomsUndoCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsRegenerateCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsContextBreakCmd.Flags().BoolVar(&roomsConfirm, "confirm", false, "confirm the destructive action")
	roomsRenameCmd.Flags().BoolVar(&roomsRenameClear, "clear", false, "clear the title instead of setting it")
	roomsGroupCmd.Flags().StringVar(&roomsGroupCompanions, "companions", "", "comma-separated companion ids (required)")
	roomsGroupCmd.Flags().StringVar(&roomsGroupTitle, "title", "", "optional room title")
	_ = roomsGroupCmd.MarkFlagRequired("companions")
	roomsEventsCmd.Flags().StringVar(&eventsSince, "since", "", "resume from an SSE cursor")
	roomsEventsCmd.Flags().BoolVar(&eventsRaw, "raw", false, "one line per frame: <type> <cursor> <json>")
	roomsEventsCmd.Flags().BoolVar(&eventsNDJSON, "ndjson", false, "one JSON object per frame: {\"event\":...,\"id\":...,\"data\":{...}} (mutually exclusive with --raw)")

	roomsCmd.AddCommand(roomsTraceCmd)
	roomsCmd.AddCommand(roomsParticipantsCmd)
	roomsCmd.AddCommand(roomsToolCallCmd)
	roomsCmd.AddCommand(roomsCancelCmd)
	roomsCmd.AddCommand(roomsUndoCmd)
	roomsCmd.AddCommand(roomsRegenerateCmd)
	roomsCmd.AddCommand(roomsContextBreakCmd)
	roomsCmd.AddCommand(roomsRenameCmd)
	roomsCmd.AddCommand(roomsGroupCmd)
	roomsCmd.AddCommand(roomsDmCmd)
	roomsCmd.AddCommand(roomsEventsCmd)
	roomsCmd.AddCommand(roomsInvitesCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesListCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesCreateCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesRevokeCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesAcceptCmd)
	roomsInvitesCmd.AddCommand(roomsInvitesPreviewCmd)
}
