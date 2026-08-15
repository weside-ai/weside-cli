package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/mcp"
)

func TestClientCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		var req mcp.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
			return
		}

		if req.JSONRPC != "2.0" {
			t.Errorf("jsonrpc = %q, want %q", req.JSONRPC, "2.0")
		}
		if req.Method != "tools/list" {
			t.Errorf("method = %q, want %q", req.Method, "tools/list")
		}

		// Check auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}

		// Return response
		resp := map[string]any{
			"jsonrpc": "2.0",
			"result":  map[string]any{"tools": []any{}},
			"id":      req.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := mcp.NewClient(server.URL, "test-token")
	result, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if _, ok := parsed["tools"]; !ok {
		t.Error("result missing 'tools' key")
	}
}

func TestClientCallSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(": keepalive\nevent: message\nid: 7\nretry: 1000\ndata: {\"jsonrpc\":\"2.0\",\"result\":{\"tools\":[]},\"id\":1}\n\n"))
	}))
	defer server.Close()

	client := mcp.NewClient(server.URL, "token")
	result, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}
	if _, ok := parsed["tools"]; !ok {
		t.Error("result missing 'tools' key")
	}
}

func TestClientCallSSEMultilineData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"result\":{\ndata: \"content\":\"joined\"},\"id\":1}\n\n"))
	}))
	defer server.Close()

	client := mcp.NewClient(server.URL, "token")
	result, err := client.Call(context.Background(), "tools/call", nil)
	if err != nil {
		t.Fatalf("Call() error: %v", err)
	}
	if got, want := string(result), "{\n\"content\":\"joined\"}"; got != want {
		t.Fatalf("result = %q, want newline-joined %q", got, want)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}
	if parsed["content"] != "joined" {
		t.Errorf("content = %v, want %q", parsed["content"], "joined")
	}
}

func TestClientCallUnknownContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("not JSON"))
	}))
	defer server.Close()

	client := mcp.NewClient(server.URL, "token")
	_, err := client.Call(context.Background(), "tools/list", nil)
	if err == nil {
		t.Fatal("Call() error = nil, want unsupported Content-Type error")
	}
	if !strings.Contains(err.Error(), "text/plain") {
		t.Fatalf("Call() error = %q, want it to name %q", err, "text/plain")
	}
}

func TestClientCallTool(t *testing.T) {
	var gotName string
	var gotArgs map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		params, _ := req.Params.(map[string]any)
		gotName, _ = params["name"].(string)
		gotArgs, _ = params["arguments"].(map[string]any)

		resp := map[string]any{
			"jsonrpc": "2.0",
			"result": map[string]any{
				"content": []map[string]string{
					{"type": "text", "text": "result text"},
				},
			},
			"id": req.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := mcp.NewClient(server.URL, "token")
	_, err := client.CallTool(context.Background(), "search_memories", map[string]any{"query": "test"})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}

	if gotName != "search_memories" {
		t.Errorf("tool name = %q, want %q", gotName, "search_memories")
	}
	if gotArgs["query"] != "test" {
		t.Errorf("args.query = %v, want %q", gotArgs["query"], "test")
	}
}

func TestClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		resp := map[string]any{
			"jsonrpc": "2.0",
			"error":   map[string]any{"code": -32600, "message": "invalid request"},
			"id":      req.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := mcp.NewClient(server.URL, "token")
	_, err := client.Call(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
