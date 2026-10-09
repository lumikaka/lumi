package modeltest

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lumi/internal/imagegen"
	"lumi/internal/llm"
	"lumi/internal/provider"
	"lumi/internal/providerdiag"
	"lumi/internal/providerlimit"
	"lumi/internal/sitesettings"

	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
)

const textPrompt = "请用一句简短中文确认：文本生成正常。"
const imagePrompt = "纯白背景，居中的蓝色圆点，扁平图形，无文字。"

func (s *Service) run(ctx context.Context, testUUID string) error {
	row, ctx, cancel, err := s.claim(ctx, testUUID)
	if err != nil || cancel == nil {
		return err
	}
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.active, testUUID)
		s.mu.Unlock()
	}()
	s.changed(row.UUID)
	for _, step := range row.Steps {
		started := time.Now().UTC()
		step.Status, step.StartedAt = "running", &started
		claimed, err := s.startStep(ctx, step)
		if err != nil || !claimed {
			return err
		}
		s.changed(row.UUID)
		stepCtx, cancel := context.WithTimeout(ctx, s.timeouts[step.Position-1])
		stepErr := s.perform(stepCtx, row, &step)
		if stepErr == nil {
			stepErr = stepCtx.Err()
		}
		cancel()
		if ctx.Err() != nil {
			// Stop has persisted this test's interruption; Close reconciles all
			// outstanding tests after the worker exits. Never stop other tests here.
			return nil
		}
		completed := time.Now().UTC()
		step.CompletedAt, step.DurationMS, step.Status = &completed, completed.Sub(started).Milliseconds(), "succeeded"
		if stepErr != nil {
			step.Status = "failed"
			// perform records sanitized diagnostics when credentials are available.
			if step.ErrorCode == "" {
				setFailure(&step, stepErr, "")
			}
		}
		if err := s.finish(row, step); err != nil {
			return err
		}
		if stepErr != nil {
			return nil
		}
	}
	return nil
}

// Claim and cancellation registration must be atomic with respect to Stop.
func (s *Service) claim(ctx context.Context, testUUID string) (Test, context.Context, context.CancelFunc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil {
		return Test{}, nil, nil, nil
	}
	row, err := s.Get(ctx, testUUID)
	if err != nil {
		return Test{}, nil, nil, err
	}
	now := time.Now().UTC()
	claimed := s.app.DB().WithContext(ctx).Model(&Test{}).Where("id = ? AND status = 'queued'", row.ID).
		Updates(map[string]any{"status": "running", "started_at": now, "updated_at": now})
	if claimed.Error != nil {
		return Test{}, nil, nil, claimed.Error
	}
	if claimed.RowsAffected != 1 {
		return Test{}, nil, nil, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	s.active[testUUID] = cancel
	return row, ctx, cancel, nil
}

func (s *Service) startStep(ctx context.Context, step Step) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return false, nil
	}
	result := s.app.DB().WithContext(ctx).Model(&Step{}).Where("id = ? AND status = 'pending'", step.ID).
		Updates(map[string]any{"status": step.Status, "started_at": step.StartedAt})
	return result.RowsAffected == 1, result.Error
}

func (s *Service) perform(ctx context.Context, row Test, step *Step) (err error) {
	selection, class := row.TextModel.Selection, providerlimit.RequestText
	if step.Kind == "image" {
		selection, class = row.ImageModel.Selection, providerlimit.RequestImage
	}
	resolved, err := s.resolve(ctx, selection, step.Kind == "image")
	if err != nil {
		return err
	}
	apiKey := resolved.APIKey
	if step.Kind == "image" {
		apiKey = resolved.ImageAPIKey
	}
	defer func() {
		if err != nil {
			setFailure(step, err, apiKey)
		}
	}()
	baseURL := resolved.BaseURL
	if step.Kind == "image" {
		baseURL = resolved.ImageBaseURL
	}
	lease, err := s.limiter.Acquire(ctx, resolved.ProviderType, baseURL, selection.Model, class)
	if err != nil {
		return err
	}
	defer func() { lease.Finish(err) }()
	switch step.Kind {
	case "connection":
		return s.text.CheckConnection(ctx, resolved.ProviderType, llm.Request{ProviderType: resolved.ProviderType, BaseURL: baseURL, APIKey: apiKey, Model: selection.Model})
	case "text":
		response, callErr := s.text.Generate(ctx, llm.Request{ProviderType: resolved.ProviderType, BaseURL: baseURL, APIKey: apiKey,
			Model: selection.Model, Prompt: textPrompt, MaxTokens: 2048}, nil)
		if callErr != nil {
			return callErr
		}
		if strings.TrimSpace(response.Content) == "" {
			return &llm.Error{Code: llm.CodeInvalidContent, SafeMessage: "文本模型没有返回有效文本。"}
		}
		step.OutputText = providerdiag.RedactPreview(response.Content, apiKey, 16384)
		return nil
	case "image":
		input := imagegen.Request{ImageAPIMode: resolved.ImageAPIMode, ProviderType: resolved.ProviderType, BaseURL: baseURL, APIKey: apiKey,
			Model: selection.Model, Prompt: imagePrompt, Size: "1024x1024", Quality: "low"}
		if resolved.ProviderType == provider.TypeAliyunBailian {
			disabled := false
			input.Size, input.Quality = "1536x1536", ""
			input.EnablePromptExtend, input.EnableThinking = &disabled, &disabled
		}
		if resolved.ProviderType == provider.TypeOpenAIRelay && resolved.ImageAPIMode == sitesettings.ImageAPIImages {
			// Provider connection checks cover generations. Model Test exercises
			// edits independently using a local, deterministic reference image.
			reference := image.NewRGBA(image.Rect(0, 0, 64, 64))
			draw.Draw(reference, reference.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
			draw.Draw(reference, image.Rect(16, 16, 48, 48), image.NewUniform(color.RGBA{B: 255, A: 255}), image.Point{}, draw.Src)
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, reference); err != nil {
				return err
			}
			input.Images = []imagegen.ImageInput{{MIMEType: "image/png", Data: encoded.Bytes()}}
			input.Prompt = "将参考图里的蓝色方块改成红色，保持白色背景，无文字。"
		}
		response, callErr := s.images.Generate(ctx, input)
		if callErr != nil {
			return callErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return s.saveImage(row.UUID, step, response)
	default:
		return ErrInvalid
	}
}

func setFailure(step *Step, err error, apiKey string) {
	code, message := "model_test_failed", "模型测试失败。"
	var textErr *llm.Error
	var imageErr *imagegen.Error
	var providerErr *provider.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code, message = "model_test_timeout", "模型测试超时。"
	case errors.Is(err, context.Canceled):
		code, message = "model_test_interrupted", "模型测试已中断。"
	case errors.As(err, &textErr):
		code, message = textErr.Code, textErr.SafeMessage
	case errors.As(err, &imageErr):
		code, message = imageErr.Code, imageErr.SafeMessage
	case errors.As(err, &providerErr):
		code, message = providerErr.Code, providerErr.Message
	case errors.Is(err, ErrInvalid):
		code, message = "model_test_invalid", "所选模型当前不可用。"
	}
	diagnostic := providerdiag.FromError(err)
	if diagnostic.Message != "" {
		message = diagnostic.Message
	}
	step.ErrorCode = providerdiag.RedactPreview(code, apiKey, 255)
	step.ErrorMessage = providerdiag.RedactPreview(message, apiKey, 2000)
	step.HTTPStatus = diagnostic.HTTPStatus
	step.ProviderErrorCode = providerdiag.RedactPreview(diagnostic.ProviderCode, apiKey, 255)
	step.ProviderRequestID = providerdiag.RedactPreview(diagnostic.RequestID, apiKey, 255)
}

func (s *Service) finish(row Test, step Step) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.app.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Step{}).Where("id = ? AND status = 'running'", step.ID).Select("*").Omit("id", "uuid", "model_test_id", "position", "kind").Updates(&step)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		updates := map[string]any{"updated_at": step.CompletedAt}
		if step.Status == "failed" || step.Position == 3 {
			updates["status"], updates["completed_at"] = step.Status, step.CompletedAt
		}
		if step.Status == "failed" {
			if err := tx.Model(&Step{}).Where("model_test_id = ? AND status = 'pending'", row.ID).
				Updates(map[string]any{"status": "skipped", "completed_at": step.CompletedAt}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&Test{}).Where("id = ? AND status = 'running'", row.ID).Updates(updates).Error
	})
	if err == nil {
		s.changed(row.UUID)
	}
	return err
}

func (s *Service) imagePath(testUUID, stepUUID string) string {
	return filepath.Join(s.app.DataDir(), "model-tests", testUUID, stepUUID+".image")
}

func (s *Service) saveImage(testUUID string, step *Step, response imagegen.Response) error {
	invalid := &imagegen.Error{Code: "image_invalid_response", SafeMessage: "图片模型没有返回有效图片。"}
	if len(response.Bytes) == 0 || len(response.Bytes) > 64<<20 {
		return invalid
	}
	mime := http.DetectContentType(response.Bytes)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" {
		return invalid
	}
	if response.MIMEType != mime {
		return invalid
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(response.Bytes))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16<<20 {
		return invalid
	}
	if _, _, err := image.Decode(bytes.NewReader(response.Bytes)); err != nil {
		return invalid
	}
	path := s.imagePath(testUUID, step.UUID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".image-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(response.Bytes); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	step.ImageMIMEType, step.ImageWidth, step.ImageHeight = mime, config.Width, config.Height
	return nil
}

func (s *Service) OpenImage(ctx context.Context, testUUID, stepUUID string) (*os.File, Step, error) {
	if !validUUID(stepUUID) {
		return nil, Step{}, ErrNotFound
	}
	row, err := s.Get(ctx, testUUID)
	if err != nil {
		return nil, Step{}, err
	}
	for _, step := range row.Steps {
		if step.UUID == stepUUID && step.ContentURL != "" {
			file, err := os.Open(s.imagePath(row.UUID, step.UUID))
			if errors.Is(err, os.ErrNotExist) {
				err = ErrNotFound
			}
			return file, step, err
		}
	}
	return nil, Step{}, ErrNotFound
}
