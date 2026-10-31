package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/processlock"
	"lumi/internal/project"
	"lumi/internal/server"
)

// Re-execute the test binary as a real CLI/desktop process, without depending
// on a prebuilt binary or a frontend build in a clean checkout.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve", "status", "stop", "__serve", "init", "open", "project", "mcp":
			os.Exit(command(os.Args[1:], os.Stdout, os.Stderr))
		case "__desktop":
			if err := testDesktop(os.Args[2]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		case "__migrate":
			if err := testMigrateProcess(os.Args[2], os.Args[3]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func cliOptions(t *testing.T) options {
	t.Helper()
	opt := options{environment: "production", dataDir: t.TempDir(), timeout: 20 * time.Second, json: true}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := stopInstance(ctx, opt); err != nil {
			t.Errorf("cleanup CLI: %v", err)
		}
	})
	return opt
}

func cliProcess(opt options, action string) *exec.Cmd {
	return exec.Command(os.Args[0], action, "--json", "--environment", opt.environment, "--data-dir", opt.dataDir, "--timeout", "20s")
}

func cliRun(t *testing.T, opt options, action string, wantExit int) instanceStatus {
	t.Helper()
	cmd := cliProcess(opt, action)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	exit := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			exit = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if exit != wantExit {
		log, _ := os.ReadFile(opt.path(".log"))
		t.Fatalf("%s exit %d: stdout=%s stderr=%s log=%s", action, exit, output, stderr.String(), log)
	}
	var result struct {
		Success bool           `json:"success"`
		Data    instanceStatus `json:"data"`
	}
	if err := json.Unmarshal(output, &result); err != nil || !result.Success {
		t.Fatalf("%s JSON=%s: %v", action, output, err)
	}
	return result.Data
}

func TestServeReusesInstanceAndRestartsAfterCrash(t *testing.T) {
	opt := cliOptions(t)
	cliRun(t, opt, "status", 3)
	first := cliRun(t, opt, "serve", 0)
	second := cliRun(t, opt, "serve", 0)
	if !first.Running || first.instance != second.instance {
		t.Fatalf("instances: %+v %+v", first, second)
	}
	info, err := os.Stat(opt.path(".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("discovery permissions: %v %v", info, err)
	}
	process, err := os.FindProcess(first.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	lock, err := processlock.Acquire(ctx, opt.path(".lock"))
	if err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	// SIGKILL leaves discovery behind; a reused PID/address is never trusted
	// without the matching live HTTP identity and lifetime lock.
	if _, err := readInstance(opt); err != nil {
		t.Fatal(err)
	}
	replacement := cliRun(t, opt, "serve", 0)
	if replacement.Nonce == first.Nonce || replacement.PID == first.PID {
		t.Fatalf("stale instance reused: %+v", replacement)
	}
	cliRun(t, opt, "stop", 0)
	if _, err := os.Stat(opt.path(".json")); !os.IsNotExist(err) {
		t.Fatalf("discovery remains: %v", err)
	}
	cliRun(t, opt, "stop", 0)
	cliRun(t, opt, "status", 3)
}

func TestConcurrentServeStartsOneProcess(t *testing.T) {
	opt := cliOptions(t)
	const count = 6
	outputs := make([][]byte, count)
	errors := make([]error, count)
	var group sync.WaitGroup
	for i := range count {
		group.Add(1)
		go func() { defer group.Done(); outputs[i], errors[i] = cliProcess(opt, "serve").CombinedOutput() }()
	}
	group.Wait()
	var expected instance
	for i := range count {
		if errors[i] != nil {
			t.Fatalf("serve %d: %v %s", i, errors[i], outputs[i])
		}
		var result struct {
			Data instanceStatus `json:"data"`
		}
		if err := json.Unmarshal(outputs[i], &result); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			expected = result.Data.instance
		}
		if result.Data.instance != expected {
			t.Fatalf("serve %d returned different process", i)
		}
	}
}

func TestDesktopAndCLIShareAppStoreButKeepIndependentInstancesAndProjectLocks(t *testing.T) {
	opt := cliOptions(t)
	desktop := exec.Command(os.Args[0], "__desktop", opt.dataDir)
	desktop.Env = append(withoutDesktopToken(os.Environ()), "LUMI_DESKTOP_ACCESS_TOKEN=desktop-test-token", "APP_ENV=production", "LUMI_DATA_DIR="+opt.dataDir)
	log, err := os.Create(filepath.Join(opt.dataDir, "desktop-test.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	desktop.Stdout, desktop.Stderr = log, log
	if err := desktop.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Process.Kill(); _ = desktop.Wait() })
	var desktopURL string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(filepath.Join(opt.dataDir, "desktop-ready"))
		if err == nil {
			desktopURL = string(b)
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if desktopURL == "" {
		b, _ := os.ReadFile(log.Name())
		t.Fatalf("desktop not ready: %s", b)
	}
	// Even an inherited desktop token and APP_ENV cannot turn CLI into a desktop.
	cmd := exec.Command(os.Args[0], "serve", "--json", "--data-dir", opt.dataDir, "--timeout", "20s")
	cmd.Env = append(os.Environ(), "LUMI_DESKTOP_ACCESS_TOKEN=inherited-secret", "APP_ENV=development")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("serve with desktop running: %v %s", err, output)
	}
	cli := cliRun(t, opt, "status", 0)
	if cli.PID == desktop.Process.Pid || cli.Endpoint == desktopURL || cli.Environment != "production" {
		t.Fatalf("not independent: %+v desktop=%s", cli, desktopURL)
	}
	if _, err := os.Stat(filepath.Join(opt.dataDir, "mcp-production.json")); !os.IsNotExist(err) {
		t.Fatalf("CLI wrote legacy discovery: %v", err)
	}

	apiRequest(t, desktopURL, "GET", "/api/v1/providers", "", "", "", "", 401)
	session := apiRequest(t, desktopURL, "POST", "/api/v1/desktop-sessions", `{"token":"desktop-test-token"}`, desktopURL, "", "", 201)
	if len(session.cookies) != 1 {
		t.Fatalf("desktop session cookies: %v", session.cookies)
	}
	cookie := session.cookies[0].Name + "=" + session.cookies[0].Value
	parent, _ := json.Marshal(t.TempDir())
	created := apiRequest(t, desktopURL, "POST", "/api/v1/projects", `{"name":"Desktop project","parent_path":`+string(parent)+`}`, desktopURL, cookie, "", 201)
	var projectInfo project.Summary
	if err := json.Unmarshal(created.data, &projectInfo); err != nil {
		t.Fatal(err)
	}
	root, _ := json.Marshal(projectInfo.RootPath)
	locked := apiRequest(t, cli.Endpoint, "POST", "/api/v1/open-projects", `{"root_path":`+string(root)+`}`, "", "", "", http.StatusLocked)
	if locked.errorCode != "project_locked" || !strings.Contains(locked.details, fmt.Sprint(desktop.Process.Pid)) || !strings.Contains(locked.details, "lumi stop") {
		t.Fatalf("lock error missing owner/release advice: %+v", locked)
	}
	apiRequest(t, desktopURL, "DELETE", "/api/v1/open-projects/"+projectInfo.UUID, "", desktopURL, cookie, "", 200)
	apiRequest(t, cli.Endpoint, "POST", "/api/v1/open-projects", `{"root_path":`+string(root)+`}`, "", "", "", 200)
	second := apiRequest(t, cli.Endpoint, "POST", "/api/v1/projects", `{"name":"CLI second project","parent_path":`+string(parent)+`}`, "", "", "", 201)
	if len(second.data) == 0 {
		t.Fatal("CLI write returned no object")
	}
	status := cliRun(t, opt, "status", 0)
	if len(status.OpenProjects) != 2 {
		t.Fatalf("open projects: %+v", status.OpenProjects)
	}
	apiRequest(t, cli.Endpoint, "GET", "/api/v1/providers", "", "", "", "", 200)
	apiRequest(t, desktopURL, "GET", "/api/v1/recent-projects", "", "", cookie, "", 200)
	for _, base := range []string{desktopURL, cli.Endpoint} {
		for _, path := range []string{"/api/v1/health", "/media/missing", "/"} {
			apiRequest(t, base, "GET", path, "", "", "", "evil.example:"+strings.Split(base, ":")[2], 403)
		}
		apiRequest(t, base, "POST", "/api/v1/open-projects", `{"root_path":""}`, "https://evil.example", cookie, "", 403)
	}
	cliRun(t, opt, "stop", 0)
	// stop waits for project shutdown, so the desktop can reopen immediately.
	apiRequest(t, desktopURL, "PUT", "/api/v1/open-projects/"+projectInfo.UUID, "", desktopURL, cookie, "", 200)
}

type apiResult struct {
	data               json.RawMessage
	errorCode, details string
	cookies            []*http.Cookie
}

func apiRequest(t *testing.T, base, method, path, body, origin, cookie, host string, want int) apiResult {
	t.Helper()
	r, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != "" {
		r.Header.Set("Cookie", cookie)
	}
	if host != "" {
		r.Host = host
	}
	response, err := localClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	b, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		t.Fatalf("%s %s: %d want %d: %s", method, path, response.StatusCode, want, b)
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Details string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatalf("API envelope: %v %s", err, b)
	}
	return apiResult{data: envelope.Data, errorCode: envelope.Error.Code, details: envelope.Error.Details, cookies: response.Cookies()}
}

func testDesktop(dataDir string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DesktopAuth == nil || cfg.Environment != "production" {
		return fmt.Errorf("desktop environment/authentication not configured")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	cfg.Address = listener.Addr().String()
	cfg.FrontendURL = "http://" + cfg.Address
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		return err
	}
	defer store.Close()
	application, err := server.New(cfg, store, project.NewManager(store))
	if err != nil {
		return err
	}
	defer application.Close()
	if err := os.WriteFile(filepath.Join(dataDir, "desktop-ready"), []byte(cfg.FrontendURL), 0600); err != nil {
		return err
	}
	application.Echo.Server.Handler = application
	return application.Echo.Server.Serve(listener)
}

func TestTwoProcessesMigrateSameApplicationDatabase(t *testing.T) {
	dataDir := t.TempDir()
	gate := filepath.Join(dataDir, "start")
	commands := []*exec.Cmd{exec.Command(os.Args[0], "__migrate", dataDir, gate), exec.Command(os.Args[0], "__migrate", dataDir, gate)}
	logs := []*bytes.Buffer{{}, {}}
	for i, cmd := range commands {
		cmd.Stdout, cmd.Stderr = logs[i], logs[i]
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
	}
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("migration process %d: %v %s", i, err, logs[i])
		}
	}
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var dirty bool
	if err := store.DB().Raw("SELECT dirty FROM schema_migrations").Scan(&dirty).Error; err != nil || dirty {
		t.Fatalf("migration dirty=%v err=%v", dirty, err)
	}
}

func testMigrateProcess(dataDir, gate string) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(gate); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("migration gate timed out")
		}
		time.Sleep(time.Millisecond)
	}
	store, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		return err
	}
	defer store.Close()
	time.Sleep(100 * time.Millisecond) // Keep one live app connection while the other starts.
	_, err = store.RecentProjects(context.Background())
	return err
}
