// Package mcpbridge contains only instance discovery and protocol forwarding.
// It must not import application storage, project, agent or business packages.
package mcpbridge

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
)

type Instance struct {
	Endpoint    string `json:"endpoint"`
	Nonce       string `json:"nonce"`
	Environment string `json:"environment"`
}

func Path(dataDir, environment string) string {
	return filepath.Join(dataDir, "mcp-"+environment+".json")
}
func Read(path, environment string) (Instance, error) {
	var instance Instance
	info, err := os.Lstat(path)
	if err != nil {
		return instance, fmt.Errorf("Lumi is not running; start Lumi (%s): %w", environment, err)
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return instance, fmt.Errorf("invalid Lumi discovery file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return instance, err
	}
	if err = json.Unmarshal(b, &instance); err != nil {
		return instance, err
	}
	u, err := url.Parse(instance.Endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path != "/mcp" || u.RawQuery != "" || u.User != nil || u.Fragment != "" || instance.Environment != environment || len(instance.Nonce) != 64 {
		return instance, fmt.Errorf("invalid Lumi instance discovery")
	}
	if _, _, err = net.SplitHostPort(u.Host); err != nil {
		return instance, err
	}
	return instance, nil
}
func Write(path string, instance Instance) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	b, err := json.Marshal(instance)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".mcp-instance-*")
	if err != nil {
		return nil, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return func() {
		current, err := Read(path, instance.Environment)
		if err == nil && current.Nonce == instance.Nonce {
			_ = os.Remove(path)
		}
	}, nil
}
