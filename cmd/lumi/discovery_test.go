package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDiscoveryCleanupOnlyRemovesItsOwnGeneration(t *testing.T) {
	opt := options{dataDir: t.TempDir(), environment: "production"}
	first := instance{Endpoint: "http://127.0.0.1:12345", PID: 123, Environment: opt.environment, Nonce: strings.Repeat("a", 64)}
	cleanupFirst, err := writeInstance(opt, first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Nonce = strings.Repeat("b", 64)
	cleanupSecond, err := writeInstance(opt, second)
	if err != nil {
		t.Fatal(err)
	}
	cleanupFirst()
	current, err := readInstance(opt)
	if err != nil || current != second {
		t.Fatalf("old cleanup removed replacement: %+v %v", current, err)
	}
	cleanupSecond()
	if _, err := os.Stat(opt.path(".json")); !os.IsNotExist(err) {
		t.Fatalf("current cleanup did not remove discovery: %v", err)
	}
}

func TestDiscoveryRejectsNonLoopbackAndMalformedIdentities(t *testing.T) {
	opt := options{dataDir: t.TempDir(), environment: "production"}
	valid := instance{Endpoint: "http://127.0.0.1:12345", PID: 123, Environment: opt.environment, Nonce: strings.Repeat("a", 64)}
	for _, endpoint := range []string{"http://evil.example:12345", "http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:65536", "http://127.0.0.1:12345/mcp", "http://user@127.0.0.1:12345", "http://127.0.0.1:12345?query=1"} {
		value := valid
		value.Endpoint = endpoint
		b, _ := json.Marshal(value)
		if err := os.WriteFile(opt.path(".json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readInstance(opt); err == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
}

func TestCommandErrorsAndHelp(t *testing.T) {
	for _, args := range [][]string{{"unknown", "--json"}, {"serve", "--environment", "other", "--json"}, {"serve", "--unknown", "--json"}, {"serve", "--timeout", "0", "--json"}} {
		var stdout, stderr bytes.Buffer
		if code := command(args, &stdout, &stderr); code != 2 {
			t.Fatalf("%v: exit=%d", args, code)
		}
		var envelope struct {
			Success bool `json:"success"`
			Data    any  `json:"data"`
			Error   struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope.Success || envelope.Data != nil || envelope.Error.Code != "invalid_arguments" || stderr.Len() != 0 {
			t.Fatalf("%v: stdout=%s stderr=%s err=%v", args, &stdout, &stderr, err)
		}
	}
	for _, args := range [][]string{{"--help"}, {"serve", "--help"}} {
		var stdout, stderr bytes.Buffer
		if code := command(args, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Usage:") {
			t.Fatalf("help exit=%d output=%s", code, &stdout)
		}
	}
}
