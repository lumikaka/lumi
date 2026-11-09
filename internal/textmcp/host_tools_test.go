package textmcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
)

// fakePageImages stands in for the CLI production task manager. It never
// calls an image provider; tests choose the task outcome.
type fakePageImages struct {
	mu      sync.Mutex
	created []PageImageRequest
	tasks   map[string]PageImageTask
	status  string
}

func (f *fakePageImages) CreatePageImage(_ context.Context, input PageImageRequest) (PageImageTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, input)
	task := PageImageTask{UUID: "01989abc-def0-7000-8000-0000000000aa", ResourceUUID: input.PageUUID, Status: f.status, ErrorCode: map[bool]string{true: "provider_rate_limited"}[f.status == "failed"]}
	if f.tasks == nil {
		f.tasks = map[string]PageImageTask{}
	}
	f.tasks[input.IdempotencyKey] = task
	return task, nil
}

func (f *fakePageImages) FindPageImage(_ context.Context, _, key string) (PageImageTask, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[key]
	return task, ok, nil
}

func (f *fakePageImages) PageImage(_ context.Context, _, taskUUID string) (PageImageTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, task := range f.tasks {
		if task.UUID == taskUUID {
			return task, nil
		}
	}
	return PageImageTask{}, httpapi.NewError(404, "task_not_found", "missing", "", nil)
}

func (f *fakePageImages) createdCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

func claudeCall(t *testing.T, s *Service, root, name string, args map[string]any) httpapi.Envelope {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return s.Call(WithHost(t.Context(), HostClaude), root, name, b)
}

func listedTools(t *testing.T, host string) ([]string, bool) {
	t.Helper()
	server := ProtocolFor(host, func(context.Context, string, json.RawMessage) httpapi.Envelope {
		return httpapi.Envelope{Success: true}
	})
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "host-tools", Version: "1"}, nil).Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	listed, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	resources, err := client.ListResources(t.Context(), nil)
	hasAppResource := err == nil && len(resources.Resources) > 0
	return names, hasAppResource
}

func TestToolListsPerHost(t *testing.T) {
	historical, historicalApp := listedTools(t, "")
	if !historicalApp {
		t.Fatal("unknown host must keep the MCP Apps resource")
	}
	for _, host := range []string{HostCodex, HostCindy} {
		names, app := listedTools(t, host)
		if !reflect.DeepEqual(names, historical) || !app {
			t.Fatalf("%s tool list changed: %v", host, names)
		}
	}
	for _, name := range historical {
		if name == "generate_page_image" || name == "web_url" {
			t.Fatalf("non-claude hosts must not list %s", name)
		}
	}
	claude, claudeApp := listedTools(t, HostClaude)
	if claudeApp {
		t.Fatal("claude host must not declare the MCP Apps resource")
	}
	has := map[string]bool{}
	for _, name := range claude {
		has[name] = true
		if strings.HasPrefix(name, "review_") || strings.HasPrefix(name, "read_app_") || name == "write_app_resource" || name == "watch_app_changes" {
			t.Fatalf("claude host lists app-only tool %s", name)
		}
	}
	for _, want := range []string{"generate_page_image", "web_url", "prepare_page_image", "image_target", "read_image", "import_image", "pages"} {
		if !has[want] {
			t.Fatalf("claude host missing %s: %v", want, claude)
		}
	}
	// The Cindy contract snapshot stays host-neutral.
	for _, spec := range Specs() {
		if spec.Name == "generate_page_image" || spec.Name == "web_url" {
			t.Fatalf("Specs() must not include Claude-only %s", spec.Name)
		}
	}
}

func TestGeneratePageImageHostGatingAndValidation(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	fake := &fakePageImages{status: "completed"}
	s.PageImages = fake
	target := object(t, invoke(t, s, root, "image_target", map[string]any{"target_uuid": page, "slot": "page_image", "setting_uuids": []string{}}))
	revision := target["expected_revision"]
	args := map[string]any{"page_uuid": page, "setting_uuids": []string{}, "setting_reviews": []any{}, "idempotency_key": "gate-1", "expected_revision": revision}

	for _, host := range []string{"", HostCodex, HostCindy} {
		b, _ := json.Marshal(args)
		if result := s.Call(WithHost(t.Context(), host), root, "generate_page_image", b); code(result) != "host_not_supported" {
			t.Fatalf("host %q: %+v", host, result.Error)
		}
	}
	if result := claudeCall(t, s, root, "generate_page_image", map[string]any{"page_uuid": page, "setting_uuids": []string{}}); code(result) != "invalid_arguments" {
		t.Fatalf("missing key/revision accepted: %+v", result.Error)
	}
	stale := map[string]any{}
	for k, v := range args {
		stale[k] = v
	}
	stale["expected_revision"], stale["idempotency_key"] = revision.(float64)+1, "gate-stale"
	if result := claudeCall(t, s, root, "generate_page_image", stale); code(result) != "production_conflict" && code(result) != "conflict" {
		t.Fatalf("stale revision accepted: %+v", result.Error)
	}
	// Selected settings without reviews are rejected by the same validation
	// as prepare_page_image, before any task.
	setting := createReadinessSetting(t, s, root, "character", "月光小狐狸")
	object(t, invoke(t, s, root, "import_image", map[string]any{"path": preparedFixtureImage(t), "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
	unreviewed := map[string]any{"page_uuid": page, "setting_uuids": []string{setting["uuid"].(string)}, "setting_reviews": []any{}, "idempotency_key": "gate-unreviewed", "expected_revision": revision}
	if result := claudeCall(t, s, root, "generate_page_image", unreviewed); result.Success {
		t.Fatal("unreviewed setting accepted")
	}
	reviews := fixtureSettingReviews(t, s, root, page, []string{setting["uuid"].(string)})
	reviews[0].(map[string]any)["verdict"] = "needs_revision"
	unreviewed["setting_reviews"], unreviewed["idempotency_key"] = reviews, "gate-needs-revision"
	if result := claudeCall(t, s, root, "generate_page_image", unreviewed); result.Success {
		t.Fatal("needs_revision setting accepted")
	}
	if fake.createdCount() != 0 {
		t.Fatalf("rejected calls created %d tasks", fake.createdCount())
	}

	s.PageImages = nil
	if result := claudeCall(t, s, root, "generate_page_image", args); code(result) != "page_image_generation_unavailable" {
		t.Fatalf("nil task manager: %+v", result.Error)
	}
	_ = chapter
}

func TestGeneratePageImageRunningReplayAndFailure(t *testing.T) {
	previous := PageImageWait
	PageImageWait = 50 * time.Millisecond
	t.Cleanup(func() { PageImageWait = previous })
	s, root, _, page := imagePromptPage(t)
	fake := &fakePageImages{status: "running"}
	s.PageImages = fake
	target := object(t, invoke(t, s, root, "image_target", map[string]any{"target_uuid": page, "slot": "page_image", "setting_uuids": []string{}}))
	args := map[string]any{"page_uuid": page, "setting_uuids": []string{}, "setting_reviews": []any{}, "additional_direction": "四周留白", "idempotency_key": "run-1", "expected_revision": target["expected_revision"]}
	running := object(t, claudeCall(t, s, root, "generate_page_image", args))
	if running["done"] != false || running["task"].(map[string]any)["status"] != "running" || !strings.Contains(running["next_action"].(string), "idempotency_key") {
		t.Fatalf("running=%+v", running)
	}
	request := fake.created[0]
	if request.IdempotencyKey != pageImageKeyPrefix+"run-1" || request.HostPagePrompt == "" || request.AdditionalDirection != "四周留白" || len(request.SettingUUIDs) != 0 || request.PageUUID != page {
		t.Fatalf("request=%+v", request)
	}
	// Same key replays the existing task without a second create, even if the
	// caller sends a revision that a completed task would have advanced.
	args["expected_revision"] = target["expected_revision"].(float64) + 5
	object(t, claudeCall(t, s, root, "generate_page_image", args))
	if fake.createdCount() != 1 {
		t.Fatalf("replay created %d tasks", fake.createdCount())
	}
	// The same key on another page is a conflict, never a silent reuse.
	fake.mu.Lock()
	task := fake.tasks[pageImageKeyPrefix+"run-1"]
	task.ResourceUUID = "01989abc-def0-7000-8000-0000000000bb"
	fake.tasks[pageImageKeyPrefix+"run-1"] = task
	fake.mu.Unlock()
	if result := claudeCall(t, s, root, "generate_page_image", args); code(result) != "idempotency_key_conflict" {
		t.Fatalf("cross-page replay: %+v", result.Error)
	}

	fake.status = "failed"
	args["expected_revision"], args["idempotency_key"] = target["expected_revision"], "run-2"
	failed := claudeCall(t, s, root, "generate_page_image", args)
	if failed.Success || code(failed) != "provider_rate_limited" || !strings.Contains(failed.Error.Details, "01989abc-def0-7000-8000-0000000000aa") {
		t.Fatalf("failed=%+v", failed.Error)
	}
	wire, _ := json.Marshal(running)
	for _, forbidden := range []string{`"id"`, "resource_uuid", "project.sqlite", "/.lumi/"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("result leaked %s: %s", forbidden, wire)
		}
	}
}

func TestWebURLScopesUseExistingWebRoutes(t *testing.T) {
	s, root, chapter, page := imagePromptPage(t)
	s.Endpoint = "http://127.0.0.1:43210"
	project := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)["uuid"].(string)
	setting := createReadinessSetting(t, s, root, "character", "月光小狐狸")
	base := s.Endpoint + "/projects/" + project
	for _, tc := range []struct {
		args  map[string]any
		url   string
		scope string
	}{
		{map[string]any{}, base + "/", "book"},
		{map[string]any{"view": "book"}, base + "/", "book"},
		{map[string]any{"view": "chapters"}, base + "/chapters", "chapters"},
		{map[string]any{"view": "settings"}, base + "/premise", "settings"},
		{map[string]any{"view": "exports"}, base + "/exports", "exports"},
		{map[string]any{"view": "providers"}, s.Endpoint + "/settings/providers", "providers"},
		{map[string]any{"chapter_uuid": chapter}, base + "/chapters/" + chapter, "chapter"},
		{map[string]any{"page_uuid": page}, base + "/chapters/" + chapter + "/sections/" + page, "page"},
		{map[string]any{"page_uuid": page, "chapter_uuid": chapter}, base + "/chapters/" + chapter + "/sections/" + page, "page"},
		{map[string]any{"setting_uuid": setting["uuid"]}, base + "/premise/assets/" + setting["uuid"].(string), "setting"},
	} {
		got := object(t, claudeCall(t, s, root, "web_url", tc.args))
		if got["url"] != tc.url || got["scope"] != tc.scope {
			t.Fatalf("%v: got %v %v", tc.args, got["url"], got["scope"])
		}
	}
	for _, args := range []map[string]any{
		{"page_uuid": page, "setting_uuid": setting["uuid"]},
		{"chapter_uuid": chapter, "view": "exports"},
		{"page_uuid": page, "chapter_uuid": "01989abc-def0-7000-8000-000000000001"},
		{"view": "dashboard"},
	} {
		if result := claudeCall(t, s, root, "web_url", args); result.Success {
			t.Fatalf("accepted conflicting scope %v", args)
		}
	}
	if !readOnlyOperation("web_url", "") {
		t.Fatal("web_url must be read-only")
	}
}
