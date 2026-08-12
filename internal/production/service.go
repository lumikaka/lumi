package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/files"
	"lumi/internal/project"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventPublisher interface {
	Broadcast(topic, event string, payload any)
}

type Service struct {
	store  *project.Store
	files  *files.Service
	events EventPublisher
	now    func() time.Time
}

func NewService(store *project.Store, events EventPublisher) *Service {
	return &Service{store: store, files: files.NewService(store, events), events: events, now: time.Now}
}

func (service *Service) Files() *files.Service { return service.files }

func (service *Service) projectActor(ctx context.Context, db *gorm.DB) (project.Project, project.Actor, error) {
	var p project.Project
	if err := db.WithContext(ctx).Where("uuid = ?", service.store.ProjectUUID()).First(&p).Error; err != nil {
		return p, project.Actor{}, err
	}
	var actor project.Actor
	if err := db.WithContext(ctx).Where("kind = ?", "local_user").Order("id ASC").First(&actor).Error; err != nil {
		return p, actor, err
	}
	return p, actor, nil
}

func (service *Service) emit(event string, payload map[string]any) {
	if service.events == nil {
		return
	}
	payload["project_uuid"] = service.store.ProjectUUID()
	service.events.Broadcast("project:"+service.store.ProjectUUID(), event, payload)
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return value.String(), nil
}

func isUUIDv7(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == 7
}

func encodeJSON(value any, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", domainError(CodeValidation, "JSON 字段无效", "字段必须能编码为 JSON。", err)
	}
	if !json.Valid(data) {
		return "", domainError(CodeValidation, "JSON 字段无效", "字段不是有效 JSON。", nil)
	}
	return string(data), nil
}

func hashJSON(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func normalizeTags(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if len([]rune(value)) > 64 {
			return nil, domainError(CodeValidation, "标签过长", "每个标签最多 64 个字符。", nil)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) > 64 {
		return nil, domainError(CodeValidation, "标签过多", "每个资产最多 64 个标签。", nil)
	}
	return result, nil
}

func validAssetType(value string) bool {
	switch value {
	case AssetCharacter, AssetScene, AssetProp, AssetReference:
		return true
	}
	return false
}

func notFound(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainError(CodeNotFound, message, "资源不存在或不属于当前项目。", err)
	}
	return err
}

func ensureProductionTaskRunning(tx *gorm.DB, taskUUID string) error {
	if !isUUIDv7(taskUUID) {
		return domainError(CodeStateConflict, "生产任务身份无效", "生成结果没有有效 task_uuid。", nil)
	}
	var allowed bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM production_task_runs WHERE uuid=? AND status='running' AND cancel_requested_at IS NULL)`, taskUUID).Scan(&allowed).Error; err != nil {
		return err
	}
	if !allowed {
		return context.Canceled
	}
	return nil
}
