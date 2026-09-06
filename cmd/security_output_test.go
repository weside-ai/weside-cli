package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/api"
	"github.com/weside-ai/weside-cli/internal/ui"
)

func TestChatTerminalControlsAcrossChunks(t *testing.T) {
	previous := chatStream
	chatStream = true
	t.Cleanup(func() { chatStream = previous })
	viper.Set("json", false)
	t.Cleanup(func() { viper.Set("json", false) })
	sse := "data: {\"type\":\"connected\"}\n\n"
	for _, chunk := range []string{"Grüße 世界\n\t", "\x1b", "]52;c;payload", "\a", "\u009b", "2J", "\r"} {
		data, _ := json.Marshal(map[string]string{"type": "room_message_delta", "delta": chunk})
		sse += "data: " + string(data) + "\n\n"
	}
	sse += "data: {\"type\":\"room_message_complete\",\"message\":{\"role\":\"assistant\",\"content\":[]}}\n\n"
	srv := chatTestServer(t, sse)
	defer srv.Close()
	out := captureStdout(t, func() error {
		return sendChat(context.Background(), api.NewClient(srv.URL, "token"), 1, "hi", abortBound{})
	})
	for _, r := range out {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			t.Fatalf("active control %U in streamed output %q", r, out)
		}
	}
	if !strings.Contains(out, "Grüße 世界\n\t") {
		t.Fatalf("legitimate stream changed: %q", out)
	}
}

func TestRoomsCommandAndErrorTerminalControls(t *testing.T) {
	text := "Grüße 世界\n\t\x1b]52;c;payload\a\u009b2J\r"
	result := map[string]any{"messages": []any{map[string]any{
		"role": "user", "content": []any{map[string]string{"type": "text", "text": text}},
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer srv.Close()
	t.Setenv("WESIDE_TOKEN", "token")
	viper.Set("api_url", srv.URL)
	viper.Set("json", false)
	t.Cleanup(func() { viper.Set("api_url", ""); viper.Set("json", false) })
	out := captureStdout(t, func() error { return roomsShowCmd.RunE(roomsShowCmd, []string{"1"}) })
	stderr := captureStderr(t, func() { ui.PrintError("remote error: %s", text) })
	for _, output := range []string{out, stderr} {
		if !strings.Contains(output, "Grüße 世界\n\t") {
			t.Fatalf("legitimate text changed: %q", output)
		}
		for _, r := range output {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				t.Fatalf("active control %U in %q", r, output)
			}
		}
	}
	viper.Set("json", true)
	out = captureStdout(t, func() error { return roomsShowCmd.RunE(roomsShowCmd, []string{"1"}) })
	var decoded struct {
		Messages []struct{ Content []struct{ Text string } }
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Messages[0].Content[0].Text != text {
		t.Fatal("JSON text was changed")
	}
}
