// Package mcp provides a client for the weside MCP (Model Context Protocol) server.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Client communicates with the weside MCP server via HTTP JSON-RPC.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	reqID      atomic.Int64
}

// NewClient creates a new MCP client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int64  `json:"id"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      int64           `json:"id"`
}

// JSONRPCError is a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// Call sends a JSON-RPC request and returns the result.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.reqID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("session expired (run: weside auth login)")
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP server error %d: %s", resp.StatusCode, string(respBody))
	}

	rpcResp, err := parseResponse(resp.Header.Get("Content-Type"), respBody)
	if err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}

func parseResponse(contentType string, body []byte) (JSONRPCResponse, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("Content-Type %q: %w", contentType, err)
	}

	var rpcResp JSONRPCResponse
	switch mediaType {
	case "application/json":
		err = json.Unmarshal(body, &rpcResp)
	case "text/event-stream":
		rpcResp, err = parseSSEResponse(body)
	default:
		err = fmt.Errorf("unsupported Content-Type %q", contentType)
	}

	return rpcResp, err
}

func parseSSEResponse(body []byte) (JSONRPCResponse, error) {
	normalized := strings.ReplaceAll(string(body), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	var dataLines []string
	for _, line := range strings.Split(normalized, "\n") {
		if line == "" {
			if rpcResp, ok := parseSSEData(dataLines); ok {
				return rpcResp, nil
			}
			dataLines = nil
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if !found {
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		if field == "data" {
			dataLines = append(dataLines, value)
		}
	}

	if rpcResp, ok := parseSSEData(dataLines); ok {
		return rpcResp, nil
	}
	return JSONRPCResponse{}, fmt.Errorf("no JSON-RPC response event found")
}

func parseSSEData(dataLines []string) (JSONRPCResponse, bool) {
	if len(dataLines) == 0 {
		return JSONRPCResponse{}, false
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &rpcResp); err != nil {
		return JSONRPCResponse{}, false
	}
	return rpcResp, rpcResp.JSONRPC == "2.0"
}

// ListTools calls tools/list on the MCP server.
func (c *Client) ListTools(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, "tools/list", nil)
}

// CallTool calls tools/call with the given tool name and arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (json.RawMessage, error) {
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	return c.Call(ctx, "tools/call", params)
}
