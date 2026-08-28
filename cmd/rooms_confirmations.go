package cmd

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// sentinelRe mirrors the app's parser (apps/mobile/utils/actionConfirmation.ts).
// The ask is a wire format on the FIRST line of a companion message; `remaining`
// is a number or the literal `unlimited` (a plan with no monthly ceiling).
var sentinelRe = regexp.MustCompile(
	`<action-confirmation id=(\d+) weight=(\d+) remaining=(\d+|unlimited) tool="([^"]*)" />`,
)

// Confirmation is one Gefallen ask as the CLI reports it: what the timeline
// carries plus the durable status only the server knows.
type Confirmation struct {
	ID        int    `json:"confirmation_id"`
	Weight    int    `json:"weight"`
	Remaining string `json:"remaining"`
	Tool      string `json:"tool"`
	Status    string `json:"status"`
	// The v2 timeline names a message's server id `id`; the field is omitted
	// rather than printed as "<nil>" when a payload has none.
	MessageID string `json:"message_id,omitempty"`
}

// stringField reads one string member, answering "" when it is absent or of
// another type — never the "<nil>" a %v of a missing key produces.
func stringField(msg map[string]any, key string) string {
	value, _ := msg[key].(string)
	return value
}

// parseConfirmationSentinels reads every ask out of a room timeline payload,
// newest first as the endpoint returns it. Deliberately timeline-driven: the
// backend exposes an ask by id only, so the timeline is the sole place a
// caller can learn which ids exist in a room.
func parseConfirmationSentinels(result map[string]any) []Confirmation {
	messages, _ := result["messages"].([]any)
	var found []Confirmation
	for _, item := range messages {
		msg, _ := item.(map[string]any)
		content, _ := msg["content"].([]any)
		for _, block := range content {
			b, _ := block.(map[string]any)
			text, _ := b["text"].(string)
			m := sentinelRe.FindStringSubmatch(text)
			if m == nil {
				continue
			}
			id, _ := strconv.Atoi(m[1])
			weight, _ := strconv.Atoi(m[2])
			found = append(found, Confirmation{
				ID:        id,
				Weight:    weight,
				Remaining: m[3],
				Tool:      m[4],
				MessageID: stringField(msg, "id"),
			})
		}
	}
	return found
}

var roomsConfirmationsLimit int

var roomsConfirmationsCmd = &cobra.Command{
	Use:   "confirmations <room_id>",
	Short: "List the Gefallen asks in a room and their durable status",
	Long: `Show every inline Gefallen ask the room's timeline carries, with the
status the server holds for it (pending, accepted, consumed, declined, expired).

The ask ids live in the timeline — the API exposes an ask by id only — so this
reads the newest --limit messages, parses their sentinels and asks the server
about each one. That makes an ask assertable from a script: whether a card is
still answerable, and whether pressing Zulassen actually consumed it.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}
		roomID := args[0]

		q := url.Values{}
		q.Set("limit", strconv.Itoa(roomsConfirmationsLimit))
		var timeline map[string]any
		if err := client.Get(
			context.Background(), "/rooms/"+roomID+"/messages?"+q.Encode(), &timeline,
		); err != nil {
			return fmt.Errorf("getting room timeline: %w", err)
		}

		found := parseConfirmationSentinels(timeline)
		for i := range found {
			found[i].Status = confirmationStatus(client, roomID, found[i].ID)
		}

		if IsJSON() {
			ui.PrintJSON(map[string]any{"room_id": roomID, "confirmations": found})
			return nil
		}
		if len(found) == 0 {
			fmt.Printf("No Gefallen ask in the newest %d messages.\n", roomsConfirmationsLimit)
			return nil
		}
		headers := []string{"ID", "STATUS", "WEIGHT", "REMAINING", "TOOL"}
		var rows [][]string
		for _, c := range found {
			rows = append(rows, []string{
				strconv.Itoa(c.ID), c.Status, strconv.Itoa(c.Weight), c.Remaining, c.Tool,
			})
		}
		ui.PrintTable(headers, rows)
		return nil
	},
}

// confirmationStatus reads one ask's durable status. A 409 is the server's
// answer for an ask that is gone or not this room's — a durable fact, not a
// fetch failure, so it is reported rather than turned into an error that would
// hide the other rows.
func confirmationStatus(client *api.Client, roomID string, id int) string {
	var result map[string]any
	path := fmt.Sprintf("/rooms/%s/action-confirmations/%d", roomID, id)
	if err := client.Get(context.Background(), path, &result); err != nil {
		return "unreadable"
	}
	status, _ := result["status"].(string)
	if status == "" {
		return "-"
	}
	return status
}

func init() {
	roomsConfirmationsCmd.Flags().IntVar(
		&roomsConfirmationsLimit, "limit", 50, "How many of the newest messages to scan",
	)
	roomsCmd.AddCommand(roomsConfirmationsCmd)
}
