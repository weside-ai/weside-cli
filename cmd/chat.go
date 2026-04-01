package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/ui"
)

var (
	chatMessage   string
	chatStream    bool
	chatNewThread bool
	chatThreadID  string
	chatFile      string
)

var chatCmd = &cobra.Command{
	Use:   "chat [companion]",
	Short: "Chat with your Companion",
	Long: `Send a message to your Companion and get a response.

If no companion is specified, the default companion is used (set via: weside companions select).

Examples:
  weside chat -m "Hello!"
  weside chat nox -m "Tell me a story" --stream
  weside chat --new -m "Fresh start"
  echo "Hi there" | weside chat nox`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		client, err := newAuthenticatedClient()
		if err != nil {
			return err
		}

		// Resolve companion
		companionArg := ""
		if len(args) > 0 {
			companionArg = args[0]
		}

		companionID, err := resolveCompanion(companionArg)
		if err != nil {
			return err
		}

		// Get message from flag, file, or stdin
		message, err := getMessage()
		if err != nil {
			return err
		}

		if message == "" {
			return fmt.Errorf("no message provided (use -m, -f, or pipe via stdin)")
		}

		// Build request body
		companionIDInt, _ := strconv.Atoi(companionID)
		body := map[string]any{
			"companion_id": companionIDInt,
			"content":      message,
			"stream":       chatStream,
		}
		if chatNewThread {
			body["new_thread"] = true
		}
		if chatThreadID != "" {
			body["thread_id"] = chatThreadID
		}

		if chatStream {
			return sendStreaming(client, body)
		}
		return sendNonStreaming(client, body)
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

func sendNonStreaming(client interface {
	Post(ctx context.Context, path string, body any, result any) error
}, body map[string]any,
) error {
	var result map[string]any
	if err := client.Post(context.Background(), "/chat/send", body, &result); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	if IsJSON() {
		ui.PrintJSON(result)
		return nil
	}

	// Response: assistant_message.content is [{type: "text", text: "..."}]
	if msg, ok := result["assistant_message"].(map[string]any); ok {
		if content, ok := msg["content"].([]any); ok {
			for _, block := range content {
				if b, ok := block.(map[string]any); ok {
					if text, ok := b["text"].(string); ok {
						fmt.Println(text)
					}
				}
			}
		}
	}
	return nil
}

func sendStreaming(client interface {
	DoRaw(ctx context.Context, method, path string, body any) (*http.Response, error)
}, body map[string]any,
) error {
	resp, err := client.DoRaw(context.Background(), http.MethodPost, "/chat/send", body)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		// Parse SSE event - extract text content
		if strings.Contains(data, `"type":"content"`) || strings.Contains(data, `"text"`) {
			// Simple extraction: find "text":"..." in the JSON
			if idx := strings.Index(data, `"text":"`); idx >= 0 {
				rest := data[idx+8:]
				if endIdx := strings.Index(rest, `"`); endIdx >= 0 {
					chunk := rest[:endIdx]
					// Unescape basic JSON escapes
					chunk = strings.ReplaceAll(chunk, `\n`, "\n")
					chunk = strings.ReplaceAll(chunk, `\"`, `"`)
					chunk = strings.ReplaceAll(chunk, `\\`, `\`)
					fmt.Print(chunk)
				}
			}
		}
	}
	fmt.Println()

	return scanner.Err()
}

func init() {
	chatCmd.Flags().StringVarP(&chatMessage, "message", "m", "", "message to send")
	chatCmd.Flags().BoolVar(&chatStream, "stream", false, "enable streaming response")
	chatCmd.Flags().BoolVar(&chatNewThread, "new", false, "start a new thread")
	chatCmd.Flags().StringVarP(&chatThreadID, "thread", "t", "", "continue a specific thread")
	chatCmd.Flags().StringVarP(&chatFile, "file", "f", "", "read message from file")
	rootCmd.AddCommand(chatCmd)
}
