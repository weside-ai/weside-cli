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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	chatMessage string
	chatStream  bool
	chatFile    string
)

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
  echo "Hi there" | weside chat nox`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
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

		return sendChat(client, roomID, message)
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
func sendChat(client *api.Client, roomID int, content string) error {
	ctx := context.Background()
	resp, err := client.Subscribe(ctx, fmt.Sprintf("/rooms/%d/events", roomID))
	if err != nil {
		return fmt.Errorf("opening event stream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	// A room_message_complete frame carries the full message and can exceed
	// bufio.Scanner's default 64 KiB line cap — give it room so large replies
	// don't abort the stream mid-read.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	sent := false
	streamed := false
	for scanner.Scan() {
		line := scanner.Text()
		// v2 SSE frame: `id: <cursor>\nevent: <type>\ndata: <json>\n\n`.
		// Route on the `type` field inside the data payload; `id:`/`event:`
		// lines are skipped by the `data: ` prefix filter.
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

		switch event["type"] {
		case "connected":
			// Subscription is live — now send the message (exactly once).
			if !sent {
				if err := postMessage(client, roomID, content); err != nil {
					return err
				}
				sent = true
			}
		case "room_message_delta":
			if chatStream {
				if delta, ok := event["delta"].(string); ok && delta != "" {
					fmt.Print(delta)
					streamed = true
				}
			}
		case "room_message_complete":
			msg, _ := event["message"].(map[string]any)
			role, _ := msg["role"].(string)
			// Only the companion's reply finishes a turn. Skip our own
			// user message if the server echoes it on the stream.
			if role != "assistant" && role != "mentor" {
				continue
			}
			fallback := extractCompleteText(event)

			if IsJSON() {
				ui.PrintJSON(msg)
				return nil
			}
			if chatStream {
				// Deltas are raw; if none arrived (a room without
				// streams_deltas capability), print the full text once.
				if !streamed && fallback != "" {
					fmt.Print(fallback)
				}
			} else {
				fmt.Print(ui.RenderMarkdown(fallback))
			}
			fmt.Println()
			return nil
		case "error":
			if detail, _ := event["message"].(string); detail != "" {
				return fmt.Errorf("companion error: %s", detail)
			}
			return fmt.Errorf("companion error event received")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading event stream: %w", err)
	}
	if !sent {
		return fmt.Errorf("event stream closed before subscription was established")
	}
	return fmt.Errorf("event stream closed before the companion replied")
}

// postMessage sends the user message to the room. The v2 endpoint returns the
// persisted user message immediately; the companion's reply arrives over the
// room event stream that sendChat is already reading.
func postMessage(client *api.Client, roomID int, content string) error {
	body := map[string]any{
		"content":           content, // bare string — v2 accepts it
		"client_message_id": newClientMessageID(),
		"stream":            true,
	}
	var result map[string]any
	if err := client.Post(context.Background(), fmt.Sprintf("/rooms/%d/messages", roomID), body, &result); err != nil {
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
	rootCmd.AddCommand(chatCmd)
}
