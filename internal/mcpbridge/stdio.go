package mcpbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Run forwards newline-delimited JSON-RPC. Discovery is reread per message;
// failed requests are never silently replayed after an ambiguous disconnect.
func Run(ctx context.Context, in io.Reader, out io.Writer, path, environment, token string) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 320<<10)
	client := &http.Client{Timeout: 130 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	initialized, ready := false, false
	version := ""
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  struct {
				Meta map[string]any `json:"_meta"`
				Name string         `json:"name"`
			} `json:"params"`
		}
		parseErr := json.Unmarshal(raw, &msg)
		replyError := func(code int, message string) error {
			id := msg.ID
			if len(id) == 0 {
				id = json.RawMessage("null")
			}
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
			_, err := fmt.Fprintln(out, string(b))
			return err
		}
		if parseErr != nil {
			code, message := -32700, "Parse error"
			if json.Valid(raw) {
				code, message = -32600, "Invalid Request"
			}
			if err := replyError(code, message); err != nil {
				return err
			}
			continue
		}
		if msg.JSONRPC != "2.0" || msg.Method == "" {
			if err := replyError(-32600, "Invalid Request"); err != nil {
				return err
			}
			continue
		}
		if msg.Method == "notifications/initialized" {
			if initialized {
				ready = true
			}
			continue
		}
		if len(msg.ID) == 0 {
			continue
		} // no advertised notification side effects
		requestVersion, _ := msg.Params.Meta["io.modelcontextprotocol/protocolVersion"].(string)
		if msg.Method != "initialize" && msg.Method != "server/discover" && msg.Method != "ping" && requestVersion == "" && !ready {
			if err := replyError(-32000, "Initialize Lumi MCP first"); err != nil {
				return err
			}
			continue
		}
		instance, err := Read(path, environment)
		if err != nil {
			if err = replyError(-32001, "Lumi is unavailable. Start the correct Lumi environment and open the authorized project."); err != nil {
				return err
			}
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, instance.Endpoint, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Lumi-Instance", instance.Nonce)
		if requestVersion != "" {
			req.Header.Set("MCP-Protocol-Version", requestVersion)
		} else if version != "" {
			req.Header.Set("MCP-Protocol-Version", version)
		}
		req.Header.Set("Mcp-Method", msg.Method)
		if msg.Params.Name != "" {
			req.Header.Set("Mcp-Name", msg.Params.Name)
		}
		response, err := client.Do(req)
		if err != nil {
			if err = replyError(-32001, "Lumi backend disconnected. Do not repeat writes with a new idempotency_key; reconnect and retrieve the call."); err != nil {
				return err
			}
			continue
		}
		b, readErr := io.ReadAll(io.LimitReader(response.Body, 7<<20))
		response.Body.Close()
		var rpcResponse struct {
			JSONRPC string `json:"jsonrpc"`
		}
		validRPC := json.Unmarshal(b, &rpcResponse) == nil && rpcResponse.JSONRPC == "2.0"
		if readErr == nil && !validRPC && response.StatusCode == http.StatusBadRequest {
			code, message := -32600, "Invalid Request"
			if strings.Contains(string(b), "unsupported") {
				code, message = -32601, "Method not found"
			}
			if err = replyError(code, message); err != nil {
				return err
			}
			continue
		}
		if readErr != nil || (response.StatusCode != http.StatusOK && !validRPC) {
			message := "Lumi backend unavailable or stale discovery"
			if response.StatusCode == 401 {
				message = "MCP credential missing, invalid or revoked"
			}
			if err = replyError(-32001, message); err != nil {
				return err
			}
			continue
		}
		if !json.Valid(b) {
			if err = replyError(-32603, "Invalid backend response"); err != nil {
				return err
			}
			continue
		}
		if msg.Method == "initialize" {
			var reply struct {
				Result struct {
					ProtocolVersion string `json:"protocolVersion"`
				} `json:"result"`
			}
			_ = json.Unmarshal(b, &reply)
			if reply.Result.ProtocolVersion != "" {
				initialized = true
				version = reply.Result.ProtocolVersion
			}
		}
		if _, err = fmt.Fprintln(out, string(b)); err != nil {
			return err
		}
	}
	return scanner.Err()
}
