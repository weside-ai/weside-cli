package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	chatMessage    string
	chatStream     bool
	chatFile       string
	chatAbortAfter string
)

// abortBound is a parsed --abort-after value: either a wall-clock deadline
// measured from the moment the message was sent, or a number of received
// delta frames. Exactly one of the two is set.
type abortBound struct {
	after  time.Duration
	deltas int
}

// parseAbortAfter reads a duration ("2s", "1500ms") or a plain chunk count
// ("3"). A bare integer is a count, not seconds: "3" meaning three seconds
// would silently make every count-based verification time-based on a slow
// provider, which is the one thing this flag exists to avoid.
func parseAbortAfter(raw string) (abortBound, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return abortBound{}, nil
	}
	if n, err := strconv.Atoi(raw); err == nil {
		if n <= 0 {
			return abortBound{}, fmt.Errorf("--abort-after chunk count must be > 0, got %d", n)
		}
		return abortBound{deltas: n}, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return abortBound{}, fmt.Errorf("--abort-after %q is neither a chunk count nor a duration (try 3 or 2s)", raw)
	}
	if d <= 0 {
		return abortBound{}, fmt.Errorf("--abort-after duration must be > 0, got %s", d)
	}
	return abortBound{after: d}, nil
}

var chatCmd = &cobra.Command{
	Use:   "chat [companion]",
	Short: "Chat with your Companion",
	Long: `Send a message to your Companion and get a response.

If no companion is specified, the default companion is used (set via: weside companions select).

The v2 chat model is room-based: the message is sent to the companion's DM
room, and the companion's reply arrives over the room event stream.

Examples:
  weside chat -m "Hello!"
  weside chat nox -m "Tell me a story" --stream
  echo "Hi there" | weside chat nox

--abort-after cancels the running turn mid-answer, the way the app's Stop button
does, and drops the stream on top of it. Use it to verify that a cancelled turn
is still billed (WA-2125): the command prints the room id and the turn's
server_message_id, and exits 0, so you can look that turn up in usage_ledger.

Compare against an uncancelled run of the same prompt — a single aborted run
cannot tell "cancelled and billed" from "ran to completion and billed". If the
server had nothing to cancel (the turn already finished, or none was running),
the command exits NON-ZERO and says so: that run proves nothing.

  weside chat nox -m "Erzähl mir eine lange Geschichte" --stream --abort-after 3
  weside chat nox -m "Erzähl mir eine lange Geschichte" --stream --abort-after 2s`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Companion resolution still uses v1 (companions list is v1).
		companionArg := ""
		if len(args) > 0 {
			companionArg = args[0]
		}
		companionID, err := resolveCompanion(companionArg)
		if err != nil {
			return err
		}
		companionIDInt, err := strconv.Atoi(companionID)
		if err != nil {
			return fmt.Errorf("invalid companion id %q: %w", companionID, err)
		}

		message, err := getMessage()
		if err != nil {
			return err
		}
		if message == "" {
			return fmt.Errorf("no message provided (use -m, -f, or pipe via stdin)")
		}

		// v2 surface: resolve the DM room, then send + receive over room events.
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		roomID, err := resolveDMRoomID(client, companionIDInt)
		if err != nil {
			return err
		}

		bound, err := parseAbortAfter(chatAbortAfter)
		if err != nil {
			return err
		}

		return sendChat(cmd.Context(), client, roomID, message, bound)
	},
}

func resolveCompanion(nameOrID string) (string, error) {
	if nameOrID != "" {
		client, err := newAuthenticatedClient()
		if err != nil {
			return "", err
		}
		return resolveCompanionID(client, nameOrID)
	}

	// Use default companion from config
	defaultID := viper.GetString("default_companion_id")
	if defaultID != "" {
		return defaultID, nil
	}
	defaultName := viper.GetString("default_companion")
	if defaultName != "" {
		client, err := newAuthenticatedClient()
		if err != nil {
			return "", err
		}
		return resolveCompanionID(client, defaultName)
	}

	return "", fmt.Errorf("no companion specified and no default set (use: weside companions select <name>)")
}

func getMessage() (string, error) {
	if chatMessage != "" {
		return chatMessage, nil
	}

	if chatFile != "" {
		data, err := os.ReadFile(chatFile)
		if err != nil {
			return "", fmt.Errorf("reading file %s: %w", chatFile, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Check stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	return "", nil
}

// resolveDMRoomID resolves (or lazily creates) the canonical 1:1 room between
// the caller and the companion via POST /api/v2/rooms/dm/{companion_id}.
func resolveDMRoomID(client *api.Client, companionID int) (int, error) {
	var result map[string]any
	if err := client.Post(context.Background(), fmt.Sprintf("/rooms/dm/%d", companionID), nil, &result); err != nil {
		return 0, fmt.Errorf("resolving DM room: %w", err)
	}
	switch v := result["id"].(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("resolving DM room: unexpected room id type %T", result["id"])
	}
}

// newClientMessageID generates an idempotency key for a room message send.
// Replaying the same key against the v2 endpoint returns the original
// persisted message (200) instead of a duplicate or a 409.
func newClientMessageID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "cli-" + hex.EncodeToString(b[:])
}

// sendChat opens the room SSE subscription first, waits for the `connected`
// frame, then POSTs the message. Subscribing before sending guarantees the
// companion's turn (room_message_start → deltas → room_message_complete) is
// not missed — the background turn only starts after the POST returns.
// Events are correlated by server_message_id so a pre-existing or concurrent
// turn's events don't leak into this invocation.
func sendChat(ctx context.Context, client *api.Client, roomID int, content string, bound abortBound) error {
	// A separate, cancellable context for the SSE request only: cancelling it
	// tears the connection down mid-body, which is exactly the client
	// disconnect --abort-after is here to reproduce. The POST keeps the
	// caller's context so an abort can never cancel the send itself.
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	resp, err := client.Subscribe(streamCtx, fmt.Sprintf("/rooms/%d/events", roomID))
	if err != nil {
		return fmt.Errorf("opening event stream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Written by the scanner loop, read by the timer's goroutine.
	var abortTurnID atomic.Pointer[string]
	emptyTurnID := ""
	abortTurnID.Store(&emptyTurnID)

	aborted := false
	// The cancel is issued exactly once, from whichever abort path fires
	// first, and it runs on the CALLER's context — never on streamCtx, which
	// the teardown below cancels; a cancel on that context would abort itself.
	var cancelOnce sync.Once
	var cancelErr error
	cancelTurn := func(id *string) error {
		cancelOnce.Do(func() { cancelErr = cancelRoomTurn(ctx, client, roomID, *id) })
		return cancelErr
	}
	abort := func() {
		aborted = true
		// Cancel BEFORE the teardown, not instead of it: the turn runs in a
		// server-side background task publishing onto the room bus, so
		// dropping the subscription ends nothing (WA-2140). A closed app drops
		// both, which is why the teardown stays.
		_ = cancelTurn(abortTurnID.Load())
		cancelStream()
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	sent := false
	streamed := false
	deltas := 0

	// The deadline timer is created once, inside the loop, at the moment the
	// message goes out — but its Stop belongs here. A `defer` inside the loop
	// is what golangci-lint's deferInLoop refuses, and rightly: the guard that
	// makes it fire once today is three branches away from the defer, so the
	// shape survives only as long as nobody moves the send.
	var abortTimer *time.Timer
	defer func() {
		if abortTimer != nil {
			abortTimer.Stop()
		}
	}()
	var turnID string // server_message_id of our companion's turn
	// The timer fires on its own goroutine while this loop writes turnID, so
	// the abort path reads it through an atomic rather than racing the scanner.
	preActive := map[string]bool{} // active_turns from connected — ignore these

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event map[string]any
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		smid, _ := event["server_message_id"].(string)

		switch event["type"] {
		case "connected":
			// Note pre-existing active turns so we don't adopt their events.
			if at, ok := event["active_turns"].([]any); ok {
				for _, id := range at {
					if s, ok := id.(string); ok {
						preActive[s] = true
					}
				}
			}
			if !sent {
				if err := postMessage(ctx, client, roomID, content); err != nil {
					return err
				}
				sent = true
				if bound.after > 0 {
					// Measured from the send, not from subscription: the
					// provider's first token is what the clock is racing.
					abortTimer = time.AfterFunc(bound.after, abort)
				}
			}
		case "room_message_start":
			// Capture our turn's correlation ID — the first start after we
			// sent that isn't a pre-existing active turn.
			if sent && smid != "" && !preActive[smid] && turnID == "" {
				turnID = smid
				captured := smid
				abortTurnID.Store(&captured)
			}
		case "room_message_delta":
			// Only accept deltas for our turn, and never in JSON mode
			// (deltas would corrupt the final JSON document).
			if !chatStream || IsJSON() {
				continue
			}
			if turnID != "" && smid != turnID {
				continue
			}
			if delta, ok := event["delta"].(string); ok && delta != "" {
				fmt.Print(delta)
				streamed = true
				deltas++
				if bound.deltas > 0 && deltas >= bound.deltas {
					fmt.Println()
					aborted = true
					if err := cancelTurn(&turnID); err != nil {
						return err
					}
					return reportAbort(roomID, turnID, deltas)
				}
			}
		case "room_message_complete":
			msg, _ := event["message"].(map[string]any)
			role, _ := msg["role"].(string)
			if role != "assistant" && role != "mentor" {
				continue
			}
			if turnID != "" && smid != turnID {
				continue
			}
			fallback := extractCompleteText(event)
			if IsJSON() {
				ui.PrintJSON(msg)
				return nil
			}
			if chatStream {
				if !streamed && fallback != "" {
					fmt.Print(fallback)
				}
			} else {
				fmt.Print(ui.RenderMarkdown(fallback))
			}
			fmt.Println()
			return nil
		case "room_turn_ended":
			// Turn ended without a complete — cancelled, failed, timed_out.
			if turnID != "" && smid != turnID {
				continue
			}
			reason, _ := event["reason"].(string)
			if reason == "" {
				reason, _ = event["error_kind"].(string)
			}
			if reason == "" {
				reason = "ended"
			}
			return fmt.Errorf("companion turn ended without a reply (%s)", reason)
		case "error":
			detail, _ := event["error"].(string)
			if detail == "" {
				detail, _ = event["message"].(string) // fallback for non-standard senders
			}
			if ec, ok := event["error_code"].(string); ok && ec != "" {
				return fmt.Errorf("companion error: %s (%s)", detail, ec)
			}
			if detail != "" {
				return fmt.Errorf("companion error: %s", detail)
			}
			return fmt.Errorf("companion error event received")
		}
	}
	// An abort tears the connection down, so the scanner ends with a read
	// error on a cancelled request. That is the success path here, and it is
	// checked BEFORE scanner.Err() — otherwise the intended abort would be
	// reported as a transport failure.
	if aborted {
		if streamed {
			fmt.Println()
		}
		if cancelErr != nil {
			return cancelErr
		}
		return reportAbort(roomID, turnID, deltas)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading event stream: %w", err)
	}
	if !sent {
		return fmt.Errorf("event stream closed before subscription was established")
	}
	return fmt.Errorf("event stream closed before the companion replied")
}

// reportAbort names the turn that was cancelled and exits successfully — the
// cancellation IS the outcome the caller asked for. The identifiers go to
// stderr in text mode so they never mix into the streamed reply on stdout;
// --json puts them on stdout as the command's document.
func reportAbort(roomID int, turnID string, deltas int) error {
	if IsJSON() {
		ui.PrintJSON(map[string]any{
			"aborted":           true,
			"turn_cancelled":    true,
			"room_id":           roomID,
			"server_message_id": turnID,
			"deltas_received":   deltas,
		})
		return nil
	}
	if turnID == "" {
		turnID = "(no room_message_start seen — the turn may not have started)"
	}
	fmt.Fprintf(
		os.Stderr,
		"aborted after %d delta(s): room_id=%d server_message_id=%s\n"+
			"the server-side turn was cancelled; look it up in usage_ledger (WA-2125)\n",
		deltas, roomID, turnID,
	)
	return nil
}

// cancelRoomTurn stops the companion turn the server is running for us.
//
// WA-2140: tearing the SSE subscription down does NOT end the turn — it runs in
// a background task publishing onto the room bus, and a listener that walks
// away ends nothing. `POST /rooms/{id}/turns/cancel` sets the flag the turn
// loop polls, which is what the app itself does when the user hits Stop.
//
// An empty turnID is sent as an omitted `server_message_id`, which the endpoint
// documents as "offer the cancel to every companion thread in this room" — the
// right semantic when the deadline fired before `room_message_start` arrived,
// and precisely the zero-token case WA-2125 exists for.
//
// A `cancelled: false` answer is an ERROR here, not a shrug: it means no turn
// was running or the id did not match, so nothing was cancelled and any ledger
// row a caller reads afterwards proves nothing. Returning nil would rebuild the
// false green this function was written to remove.
func cancelRoomTurn(ctx context.Context, client *api.Client, roomID int, turnID string) error {
	body := map[string]any{}
	if turnID != "" {
		body["server_message_id"] = turnID
	}
	var result map[string]any
	if err := client.Post(ctx, fmt.Sprintf("/rooms/%d/turns/cancel", roomID), body, &result); err != nil {
		return fmt.Errorf("cancelling turn: %w", err)
	}
	if cancelled, _ := result["cancelled"].(bool); !cancelled {
		return fmt.Errorf(
			"the server cancelled nothing (room_id=%d server_message_id=%q): no turn was "+
				"running, or it had already finished — this run proves nothing about the "+
				"cancellation path",
			roomID, turnID,
		)
	}
	return nil
}

// postMessage sends the user message to the room. The v2 endpoint returns the
// persisted user message immediately; the companion's reply arrives over the
// room event stream that sendChat is already reading.
func postMessage(ctx context.Context, client *api.Client, roomID int, content string) error {
	body := map[string]any{
		"content":           content,
		"client_message_id": newClientMessageID(),
		"stream":            true,
	}
	var result map[string]any
	if err := client.Post(ctx, fmt.Sprintf("/rooms/%d/messages", roomID), body, &result); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	return nil
}

// extractCompleteText pulls the companion text from a room_message_complete
// event: {message: {content: [{type: "text", text: "..."}]}}.
func extractCompleteText(event map[string]any) string {
	msg, ok := event["message"].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, block := range content {
		if b, ok := block.(map[string]any); ok {
			if text, ok := b["text"].(string); ok {
				sb.WriteString(text)
			}
		}
	}
	return sb.String()
}

func init() {
	chatCmd.Flags().StringVarP(&chatMessage, "message", "m", "", "message to send")
	chatCmd.Flags().BoolVar(&chatStream, "stream", false, "print the reply token-by-token as it streams")
	chatCmd.Flags().StringVarP(&chatFile, "file", "f", "", "read message from file")
	chatCmd.Flags().StringVar(&chatAbortAfter, "abort-after", "", "cancel the turn after N delta chunks (e.g. 3) or a duration (e.g. 2s), then exit 0")
	rootCmd.AddCommand(chatCmd)
}
