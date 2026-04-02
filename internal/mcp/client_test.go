package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
