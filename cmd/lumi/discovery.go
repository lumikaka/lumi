package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"lumi/internal/project"
)

var errNotRunning = errors.New("CLI instance is not running")

type instance struct {
	Endpoint    string `json:"endpoint"`
	PID         int    `json:"pid"`
	Environment string `json:"environment"`
	Nonce       string `json:"nonce"`
}

type instanceStatus struct {
	instance
	Running      bool              `json:"running"`
	OpenProjects []project.Summary `json:"open_projects"`
}

func stoppedStatus(opt options) instanceStatus {
	return instanceStatus{instance: instance{Environment: opt.environment}, OpenProjects: []project.Summary{}}
}

func (opt options) path(suffix string) string {
	return filepath.Join(opt.dataDir, "cli-"+opt.environment+suffix)
}

func readInstance(opt options) (instance, error) {
	var value instance
	info, err := os.Lstat(opt.path(".json"))
	if err != nil {
		return value, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return value, errors.New("invalid CLI discovery file")
	}
	b, err := os.ReadFile(opt.path(".json"))
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(b, &value); err != nil {
		return value, err
	}
	u, err := url.Parse(value.Endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || value.PID <= 0 || value.Environment != opt.environment {
		return value, errors.New("invalid CLI discovery identity")
	}
	_, port, err := net.SplitHostPort(u.Host)
	p, portErr := strconv.Atoi(port)
	nonce, nonceErr := hex.DecodeString(value.Nonce)
	if err != nil || portErr != nil || p < 1 || p > 65535 || nonceErr != nil || len(nonce) != 32 {
		return value, errors.New("invalid CLI discovery address or nonce")
	}
	return value, nil
}

func writeInstance(opt options, value instance) (func(), error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(opt.dataDir, ".cli-instance-*") // 0600, including replacement of a stale file.
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return nil, err
	}
	if err := f.Sync(); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(f.Name(), opt.path(".json")); err != nil {
		return nil, err
	}
	return func() { removeInstance(opt, value) }, nil
}

func removeInstance(opt options, value instance) {
	if current, err := readInstance(opt); err == nil && current.Nonce == value.Nonce {
		_ = os.Remove(opt.path(".json"))
	}
}

func newNonce() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create instance nonce: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
