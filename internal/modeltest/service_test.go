package modeltest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/provider"
	"lumi/internal/providerdiag"
	"lumi/internal/providerlimit"
	"lumi/internal/sitesettings"
)

type fakeModels struct {
	mu           sync.Mutex
	calls        []string
	fail         string
	emptyText    bool
	invalidImage bool
	block        string
	entered      chan struct{}
	release      <-chan struct{}
	textRequest  llm.Request
	imageRequest imagegen.Request
	imageBytes   []byte
}

func (f *fakeModels) call(ctx context.Context, kind string) error {
	f.mu.Lock()
	f.calls = append(f.calls, kind)
	f.mu.Unlock()
	if f.block == kind {
		if f.entered != nil {
			select {
			case f.entered <- struct{}{}:
			default:
			}
		}
		select {
		case <-f.release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.fail == kind {
		return &llm.Error{Code: llm.CodeAuthentication, SafeMessage: "denied",
			Diagnostic: providerdiag.Details{HTTPStatus: 401, ProviderCode: "denied", RequestID: "test-request",
				Message: "bad secret cloud-secret api_key=echoed-key Bearer bearer-key"}}
	}
	return nil
}

func (f *fakeModels) CheckConnection(ctx context.Context, _ string, request llm.Request) error {
	f.textRequest = request
	return f.call(ctx, "connection")
}

func (f *fakeModels) Generate(ctx context.Context, request llm.Request, _ func(string) error) (llm.Response, error) {
	f.textRequest = request
	if err := f.call(ctx, "text"); err != nil {
		return llm.Response{}, err
	}
	if f.emptyText {
		return llm.Response{Content: " \n"}, nil
	}
	return llm.Response{Content: "文本生成正常。"}, nil
}

type fakeImages struct{ models *fakeModels }

func (f fakeImages) Generate(ctx context.Context, request imagegen.Request) (imagegen.Response, error) {
	f.models.imageRequest = request
	if err := f.models.call(ctx, "image"); err != nil {
		return imagegen.Response{}, err
	}
	if f.models.invalidImage {
		return imagegen.Response{Bytes: []byte("not an image"), MIMEType: "image/png"}, nil
	}
	return imagegen.Response{Bytes: f.models.imageBytes, MIMEType: "image/png"}, nil
}

type fakeEvents struct {
	mu      sync.Mutex
	values  []string
	updates chan struct{}
}

func (f *fakeEvents) Broadcast(topic, event string, payload any) {
	value, _ := json.Marshal(map[string]any{"topic": topic, "event": event, "payload": payload})
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = append(f.values, string(value))
	if f.updates != nil {
		select {
		case f.updates <- struct{}{}:
		default:
		}
	}
}

type fakeLimiter struct {
	mu       sync.Mutex
	classes  []providerlimit.RequestClass
	finished int
}
type fakeLease struct{ owner *fakeLimiter }

func (f *fakeLimiter) Acquire(_ context.Context, _, _, _ string, class providerlimit.RequestClass) (providerlimit.Lease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.classes = append(f.classes, class)
	return fakeLease{f}, nil
}
func (f fakeLease) Finish(error) { f.owner.mu.Lock(); defer f.owner.mu.Unlock(); f.owner.finished++ }

type harness struct {
	s         *Service
	models    *fakeModels
	providers *provider.Service
	app       *appstore.Store
	input     ExecutionInput
	events    *fakeEvents
	limiter   *fakeLimiter
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	providers := provider.NewService(app, provider.NewMemorySecretStore())
	cloud, err := providers.Create(context.Background(), provider.CreateInput{AccountID: "0123456789abcdef0123456789abcdef", DefaultModel: "openai/gpt-6-sol", DefaultImageModel: "openai/gpt-5.6-terra", APIKey: "cloud-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := providers.Settings().UpdateSystem(context.Background(), map[string]any{sitesettings.CloudflareVerifiedKey: false, sitesettings.ActiveProviderKey: "none"}); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	models := &fakeModels{imageBytes: buffer.Bytes()}
	events, limiter := &fakeEvents{}, &fakeLimiter{}
	s := NewService(app, providers, models, fakeImages{models}, limiter, events)
	t.Cleanup(func() { _ = s.Close(); providers.Close(); _ = app.Close() })
	return &harness{s, models, providers, app, ExecutionInput{TextModel: Selection{cloud.UUID, cloud.DefaultModel}, ImageModel: Selection{cloud.UUID, cloud.DefaultImageModel}}, events, limiter}
}

func (h *harness) queued(t *testing.T) Test {
	t.Helper()
	row, err := h.s.Create(context.Background(), h.input)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (h *harness) run(t *testing.T) Test {
	t.Helper()
	row := h.queued(t)
	if err := h.s.run(context.Background(), row.UUID); err != nil {
		t.Fatal(err)
	}
	row, err := h.s.Get(context.Background(), row.UUID)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestRunsAllStepsAndPersistsIndependentSelectionsAndImage(t *testing.T) {
	h := newHarness(t)
	row := h.run(t)
	if row.Status != "succeeded" || row.StartedAt == nil || row.CompletedAt == nil || len(row.Steps) != 3 {
		t.Fatalf("test=%+v", row)
	}
	if !reflect.DeepEqual(h.models.calls, []string{"connection", "text", "image"}) {
		t.Fatal(h.models.calls)
	}
	for _, step := range row.Steps {
		if step.Status != "succeeded" || step.StartedAt == nil || step.CompletedAt == nil {
			t.Fatalf("step=%+v", step)
		}
	}
	if row.TextModel.Selection != h.input.TextModel || row.ImageModel.Selection != h.input.ImageModel {
		t.Fatalf("models=%+v %+v", row.TextModel, row.ImageModel)
	}
	if h.models.textRequest.Model != h.input.TextModel.Model || h.models.textRequest.Prompt != textPrompt || h.models.imageRequest.Model != h.input.ImageModel.Model || h.models.imageRequest.Prompt != imagePrompt {
		t.Fatal("did not use selected models and fixed prompts")
	}
	if h.models.imageRequest.Size != "1024x1024" || h.models.imageRequest.Quality != "low" || len(h.models.imageRequest.Images) != 0 {
		t.Fatal(h.models.imageRequest)
	}
	if !reflect.DeepEqual(h.limiter.classes, []providerlimit.RequestClass{providerlimit.RequestText, providerlimit.RequestText, providerlimit.RequestImage}) || h.limiter.finished != 3 {
		t.Fatalf("limits=%+v", h.limiter)
	}
	// A new service reads the same metadata and file without a project or a worker.
	reopened := NewService(h.app, h.providers, h.models, fakeImages{h.models}, nil, nil)
	file, step, err := reopened.OpenImage(context.Background(), row.UUID, row.Steps[2].UUID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(content, h.models.imageBytes) || step.ImageWidth != 2 || step.ContentURL == "" {
		t.Fatal("image did not persist", err)
	}
	providerState, err := h.providers.Get(context.Background(), h.input.TextModel.ProviderUUID)
	if err != nil || providerState.Verified || providerState.Active {
		t.Fatal("diagnostic changed provider verification or activation", err)
	}
	encoded, _ := json.Marshal(row)
	for _, secret := range []string{`"id":`, `"model_test_id":`, h.app.DataDir(), "cloud-secret", "base64"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("public data leaked %q", secret)
		}
	}
	for _, raw := range h.events.values {
		var envelope struct {
			Topic, Event string
			Payload      map[string]string
		}
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Topic != "system" || envelope.Event != EventChanged || len(envelope.Payload) != 1 || envelope.Payload["test_uuid"] != row.UUID {
			t.Fatalf("event=%s", raw)
		}
	}
}

func TestStopsAtEachFailureAndSanitizesProviderDiagnostics(t *testing.T) {
	for failedIndex, kind := range []string{"connection", "text", "image"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t)
			h.models.fail = kind
			row := h.run(t)
			if row.Status != "failed" || len(h.models.calls) != failedIndex+1 {
				t.Fatalf("test=%+v calls=%v", row, h.models.calls)
			}
			for i, step := range row.Steps {
				want := "succeeded"
				if i == failedIndex {
					want = "failed"
				} else if i > failedIndex {
					want = "skipped"
				}
				if step.Status != want {
					t.Fatalf("step=%+v want %s", step, want)
				}
			}
			step := row.Steps[failedIndex]
			if step.HTTPStatus != 401 || step.ProviderRequestID != "test-request" || step.ErrorCode != llm.CodeAuthentication {
				t.Fatalf("diagnostics=%+v", step)
			}
			encoded, _ := json.Marshal(row)
			for _, secret := range []string{"cloud-secret", "echoed-key", "bearer-key"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("leaked %s", secret)
				}
			}
		})
	}
}

func TestEmptyTextInvalidImageAndTimeoutFail(t *testing.T) {
	for _, kind := range []string{"empty_text", "invalid_image", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t)
			h.models.emptyText, h.models.invalidImage = kind == "empty_text", kind == "invalid_image"
			if kind == "timeout" {
				h.models.block = "text"
				h.s.timeouts[1] = 5 * time.Millisecond
			}
			row := h.run(t)
			index, code := 1, llm.CodeInvalidContent
			if kind == "invalid_image" {
				index, code = 2, "image_invalid_response"
			}
			if kind == "timeout" {
				code = "model_test_timeout"
			}
			if row.Status != "failed" || row.Steps[index].ErrorCode != code || row.Steps[2].ContentURL != "" {
				t.Fatalf("test=%+v", row)
			}
		})
	}
}

func TestExecutionIsIdempotentAndRejectsChangedSelections(t *testing.T) {
	h := newHarness(t)
	row, err := h.s.Create(context.Background(), h.input)
	if err != nil {
		t.Fatal(err)
	}
	// A saved draft from the previous version can still be started exactly once.
	if err := h.app.DB().Model(&Test{}).Where("id = ?", row.ID).Update("status", "draft").Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.s.Execute(context.Background(), row.UUID, h.input); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := h.s.run(context.Background(), row.UUID); err != nil {
		t.Fatal(err)
	}
	if err := h.s.run(context.Background(), row.UUID); err != nil {
		t.Fatal(err)
	}
	if len(h.models.calls) != 3 {
		t.Fatal(h.models.calls)
	}
	changed := h.input
	changed.TextModel.Model = "openai/gpt-5.6-sol"
	if _, err := h.s.Execute(context.Background(), row.UUID, changed); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	// Idempotent re-reads remain possible even if credentials are later removed.
	if _, _, err := h.providers.Settings().Update(context.Background(), map[string]any{sitesettings.CloudflareAPITokenKey: "changed-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.Execute(context.Background(), row.UUID, h.input); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionValidationDoesNotCreateUnstartedRecords(t *testing.T) {
	h := newHarness(t)
	for _, input := range []ExecutionInput{{}, {TextModel: h.input.ImageModel, ImageModel: Selection{h.input.ImageModel.ProviderUUID, "unsupported"}}, {TextModel: Selection{"internal-1", "x"}, ImageModel: h.input.ImageModel}} {
		if _, err := h.s.Create(context.Background(), input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("input=%+v err=%v", input, err)
		}
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.Create(context.Background(), h.input); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	for _, table := range []any{&Test{}, &Step{}} {
		var count int64
		if err := h.app.DB().Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("unexpected saved records: %d, %v", count, err)
		}
	}
	if len(h.models.calls) != 0 || len(h.events.values) != 0 {
		t.Fatal(h.models.calls, h.events.values)
	}
}

func TestBailianImageUsesIndependentModelAndLowOverheadOptions(t *testing.T) {
	h := newHarness(t)
	if _, _, err := h.providers.Settings().Update(context.Background(), map[string]any{
		sitesettings.BailianWorkspaceKey: "workspace-test", sitesettings.BailianRegionKey: "cn-beijing", sitesettings.BailianAPIKeyKey: "bailian-secret",
	}); err != nil {
		t.Fatal(err)
	}
	providers, err := h.providers.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range providers {
		if item.ProviderType == provider.TypeAliyunBailian {
			h.input.ImageModel = Selection{item.UUID, provider.BailianImageModel}
		}
	}
	row := h.run(t)
	request := h.models.imageRequest
	if row.Status != "succeeded" || request.ProviderType != provider.TypeAliyunBailian || request.Size != "1536x1536" || request.Quality != "" || request.EnableThinking == nil || *request.EnableThinking || request.EnablePromptExtend == nil || *request.EnablePromptExtend {
		t.Fatalf("request=%+v test=%s", request, row.Status)
	}
}

func TestShutdownCancelsRunningTestAndSkipsQueuedTests(t *testing.T) {
	h := newHarness(t)
	h.models.block, h.models.entered = "text", make(chan struct{}, 1)
	if err := h.s.Start(); err != nil {
		t.Fatal(err)
	}
	row := h.queued(t)
	select {
	case <-h.models.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	queued := h.queued(t)
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	current, err := h.s.Get(context.Background(), row.UUID)
	if err != nil || current.Status != "interrupted" || current.Steps[0].Status != "succeeded" || current.Steps[1].Status != "interrupted" || current.Steps[2].Status != "skipped" {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	queued, err = h.s.Get(context.Background(), queued.UUID)
	if err != nil || queued.Status != "interrupted" || queued.Steps[0].Status != "skipped" {
		t.Fatalf("queued=%+v err=%v", queued, err)
	}
	if !reflect.DeepEqual(h.models.calls, []string{"connection", "text"}) {
		t.Fatal(h.models.calls)
	}
}

func TestWorkerFinishesQueuedTestsAfterHTTPContextIsCancelled(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.models.block, h.models.entered, h.models.release = "text", make(chan struct{}, 1), release
	h.events.updates = make(chan struct{}, 128)
	if err := h.s.Start(); err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	row, err := h.s.Create(requestCtx, h.input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.models.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not reach text step")
	}
	cancel()
	second := h.queued(t)
	close(release)
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		first, err := h.s.Get(context.Background(), row.UUID)
		if err != nil {
			t.Fatal(err)
		}
		last, err := h.s.Get(context.Background(), second.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if first.Status == "succeeded" && last.Status == "succeeded" {
			break
		}
		select {
		case <-h.events.updates:
		case <-timeout.C:
			t.Fatalf("tests did not finish: %s, %s", first.Status, last.Status)
		}
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(h.models.calls, []string{"connection", "text", "image", "connection", "text", "image"}) {
		t.Fatal(h.models.calls)
	}
}

func TestStartupReconcilesAbandonedWorkWithoutRetry(t *testing.T) {
	h := newHarness(t)
	row := h.queued(t)
	if err := h.app.DB().Model(&Test{}).Where("id = ?", row.ID).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	if err := h.app.DB().Model(&Step{}).Where("id = ?", row.Steps[0].ID).Update("status", "succeeded").Error; err != nil {
		t.Fatal(err)
	}
	if err := h.app.DB().Model(&Step{}).Where("id = ?", row.Steps[1].ID).Updates(map[string]any{"status": "running", "started_at": time.Now().Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.s.Start(); err != nil {
		t.Fatal(err)
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	current, _ := h.s.Get(context.Background(), row.UUID)
	if current.Status != "interrupted" || current.Steps[0].Status != "succeeded" || current.Steps[1].Status != "interrupted" || len(h.models.calls) != 0 {
		t.Fatalf("current=%+v calls=%v", current, h.models.calls)
	}
}

func TestHistoryPaginationAndImageOwnership(t *testing.T) {
	h := newHarness(t)
	first := h.run(t)
	legacyDraft := Test{UUID: "01900000-0000-7000-8000-000000000001", Status: "draft", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := h.app.DB().Create(&legacyDraft).Error; err != nil {
		t.Fatal(err)
	}
	second, err := h.s.Create(context.Background(), h.input)
	if err != nil {
		t.Fatal(err)
	}
	page, err := h.s.List(context.Background(), 1, 1)
	if err != nil || page.Pagination.Total != 2 || page.Pagination.LastPage != 2 || len(page.Items) != 1 || page.Items[0].UUID != second.UUID {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	page, _ = h.s.List(context.Background(), 2, 1)
	if page.Items[0].UUID != first.UUID {
		t.Fatal(page)
	}
	if _, _, err := h.s.OpenImage(context.Background(), second.UUID, first.Steps[2].UUID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, _, err := h.s.OpenImage(context.Background(), first.UUID, "../../app.sqlite"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
