package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryEnvironmentValidationAndOwnership(t *testing.T) {
	path := Path(t.TempDir(), "production")
	original := Instance{Endpoint: "http://127.0.0.1:1234/mcp", Nonce: strings.Repeat("a", 64), Environment: "production"}
	cleanup, err := Write(path, original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Read(path, "development"); err == nil {
		t.Fatal("mixed environments")
	}
	newer := original
	newer.Nonce = strings.Repeat("b", 64)
	newCleanup, err := Write(path, newer)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if got, err := Read(path, "production"); err != nil || got.Nonce != newer.Nonce {
		t.Fatal("old process removed replacement discovery")
	}
	newCleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("discovery not removed")
	}
	for _, endpoint := range []string{"http://evil.test/mcp", "http://127.0.0.1:1234/mcp?secret=1", "https://127.0.0.1:1234/mcp", "http://user@127.0.0.1:1234/mcp", "http://127.0.0.1:1234/other"} {
		original.Endpoint = endpoint
		b, _ := json.Marshal(original)
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(path, "production"); err == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
}
func TestStdioRejectsMalformedAndUnavailableWithoutStdoutLogs(t *testing.T) {
	var out bytes.Buffer
	input := "{broken\n[]\n" + `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/cancelled"}` + "\n"
	if err := Run(context.Background(), strings.NewReader(input), &out, filepath.Join(t.TempDir(), "missing"), "production", "test"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatal(out.String())
	}
	for i, line := range lines {
		var v struct {
			JSONRPC string `json:"jsonrpc"`
			Error   struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &v) != nil || v.JSONRPC != "2.0" || v.Error.Code != []int{-32700, -32600, -32000, -32001}[i] {
			t.Fatal(line)
		}
	}
}
