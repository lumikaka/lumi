package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"lumi/internal/processlock"
)

const instanceRoute = "/api/v1/cli-instance"

var localClient = &http.Client{
	Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
	Timeout:       2 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

func requestInstance(ctx context.Context, value instance, method string) (instanceStatus, error) {
	var status instanceStatus
	r, err := http.NewRequestWithContext(ctx, method, value.Endpoint+instanceRoute, nil)
	if err != nil {
		return status, err
	}
	// A public generation identifier prevents stale discovery from stopping a
	// replacement process. This is not a credential or an authentication scheme.
	r.Header.Set("If-Match", value.Nonce)
	response, err := localClient.Do(r)
	if err != nil {
		return status, err
	}
	defer response.Body.Close()
	var envelope struct {
		Success bool           `json:"success"`
		Data    instanceStatus `json:"data"`
	}
	if response.StatusCode != 200 {
		return status, fmt.Errorf("CLI identity endpoint returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&envelope); err != nil {
		return status, err
	}
	status = envelope.Data
	if !envelope.Success || status.instance != value || !status.Running {
		return status, errors.New("CLI instance identity changed")
	}
	return status, nil
}

func inspectInstance(ctx context.Context, opt options) (instanceStatus, error) {
	value, err := readInstance(opt)
	if err == nil {
		status, probeErr := requestInstance(ctx, value, http.MethodGet)
		if probeErr == nil {
			return status, nil
		}
		err = probeErr
	}
	// A file can be absent during startup/shutdown while the lifetime lock is
	// still held. Do not report a stopped instance until that lock is free.
	if _, statErr := os.Stat(opt.dataDir); errors.Is(statErr, os.ErrNotExist) {
		return stoppedStatus(opt), errNotRunning
	}
	lock, lockErr := processlock.Try(opt.path(".lock"))
	if lockErr == nil {
		_ = lock.Close()
		return stoppedStatus(opt), errNotRunning
	}
	if !errors.Is(lockErr, processlock.ErrBusy) {
		return instanceStatus{}, lockErr
	}
	return instanceStatus{}, fmt.Errorf("CLI owns its lock but is unavailable: %w", err)
}

func ensureInstance(ctx context.Context, opt options) (instanceStatus, error) {
	guard, err := processlock.Acquire(ctx, opt.path(".start.lock"))
	if err != nil {
		return instanceStatus{}, err
	}
	defer guard.Close()
	if status, err := inspectInstance(ctx, opt); err == nil {
		return status, nil
	}
	lock, err := processlock.Try(opt.path(".lock"))
	if errors.Is(err, processlock.ErrBusy) {
		return waitReady(ctx, opt, nil)
	}
	if err != nil {
		return instanceStatus{}, err
	}
	_ = lock.Close()

	executable, err := os.Executable()
	if err != nil {
		return instanceStatus{}, err
	}
	cmd := exec.Command(executable, "__serve", "--environment", opt.environment, "--data-dir", opt.dataDir)
	cmd.Env = withoutDesktopToken(os.Environ())
	detach(cmd)
	log, err := os.OpenFile(opt.path(".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return instanceStatus{}, err
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return instanceStatus{}, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	status, err := waitReady(ctx, opt, exited)
	if err != nil {
		_ = cmd.Process.Kill()
		return status, fmt.Errorf("start CLI (see %s): %w", opt.path(".log"), err)
	}
	return status, nil
}

func waitReady(ctx context.Context, opt options, exited <-chan error) (instanceStatus, error) {
	for {
		if value, err := readInstance(opt); err == nil {
			if status, err := requestInstance(ctx, value, http.MethodGet); err == nil {
				return status, nil
			}
		}
		select {
		case <-ctx.Done():
			return instanceStatus{}, ctx.Err()
		case err := <-exited:
			return instanceStatus{}, fmt.Errorf("background server exited: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func stopInstance(ctx context.Context, opt options) (instanceStatus, error) {
	guard, err := processlock.Acquire(ctx, opt.path(".start.lock"))
	if err != nil {
		return instanceStatus{}, err
	}
	defer guard.Close()
	status, err := inspectInstance(ctx, opt)
	if errors.Is(err, errNotRunning) {
		if stale, readErr := readInstance(opt); readErr == nil {
			removeInstance(opt, stale)
		}
		return stoppedStatus(opt), nil
	}
	if err != nil {
		return status, err
	}
	if _, err := requestInstance(ctx, status.instance, http.MethodDelete); err != nil {
		return status, err
	}
	// Waiting for the lifetime lock also waits for project and SQLite shutdown.
	lock, err := processlock.Acquire(ctx, opt.path(".lock"))
	if err != nil {
		return status, err
	}
	defer lock.Close()
	removeInstance(opt, status.instance)
	return stoppedStatus(opt), nil
}

func withoutDesktopToken(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(strings.ToUpper(item), "LUMI_DESKTOP_ACCESS_TOKEN=") {
			clean = append(clean, item)
		}
	}
	return clean
}
