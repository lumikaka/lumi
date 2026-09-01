package story

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"lumi/internal/project"
	"lumi/internal/promptcatalog"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	maxChapterBytes = 2 << 20
	maxStoryMDBytes = 2 << 20
	maxPromptBytes  = 256 << 10
)

var chapterCodePattern = regexp.MustCompile(`^vol([0-9]{2,})\.ch([0-9]{2,})$`)

type Service struct {
	store           *project.Store
	now             func() time.Time
	writeProjection func(string) error
	events          EventPublisher
}

type EventPublisher interface {
	Broadcast(topic, event string, payload any)
}

func NewService(store *project.Store) *Service {
	service := &Service{store: store, now: time.Now}
	service.writeProjection = service.atomicWriteStoryMD
	return service
}

func (service *Service) WithEvents(events EventPublisher) *Service {
	service.events = events
	return service
}

func (service *Service) emit(event string, payload map[string]any) {
	if service.events == nil {
		return
	}
	payload["project_uuid"] = service.store.ProjectUUID()
	service.events.Broadcast("project:"+service.store.ProjectUUID(), event, payload)
}

func (service *Service) PictureBookProfile() (project.PictureBookProfile, error) {
	return service.store.RequirePictureBookProfile()
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

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func validateText(content string, maxBytes int, field string) error {
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return storyError(CodeValidationFailed, field+"编码无效", "内容必须是有效 UTF-8 文本且不能包含 NUL。", nil)
	}
	if len(content) > maxBytes {
		return storyError(CodeValidationFailed, field+"过大", fmt.Sprintf("内容不能超过 %d 字节。", maxBytes), nil)
	}
	if strings.TrimSpace(content) == "" {
		return storyError(CodeValidationFailed, field+"不能为空", "请输入至少一个非空白字符。", nil)
	}
	return nil
}

func normalizeFormat(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "markdown" {
		value = "md"
	}
	if value != "txt" && value != "md" {
		return "", storyError(CodeValidationFailed, "正文格式无效", "content_format 只支持 txt 或 md。", nil)
	}
	return value, nil
}

func parseChapterCode(value string) (string, int, int, int, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	matches := chapterCodePattern.FindStringSubmatch(normalized)
	if matches == nil {
		return "", 0, 0, 0, storyError(CodeValidationFailed, "章节编号无效", "章节编号必须使用 vol01.ch01 格式。", nil)
	}
	var volume, chapter int
	if _, err := fmt.Sscanf(matches[1], "%d", &volume); err != nil {
		return "", 0, 0, 0, storyError(CodeValidationFailed, "章节编号无效", "无法读取卷号。", err)
	}
	if _, err := fmt.Sscanf(matches[2], "%d", &chapter); err != nil {
		return "", 0, 0, 0, storyError(CodeValidationFailed, "章节编号无效", "无法读取章号。", err)
	}
	if volume <= 0 || chapter <= 0 || chapter >= 100000 {
		return "", 0, 0, 0, storyError(CodeValidationFailed, "章节编号超出范围", "卷号和章号必须为正数，章号必须小于 100000。", nil)
	}
	return normalized, volume, chapter, volume*100000 + chapter, nil
}

func (service *Service) projectAndActor(ctx context.Context, db *gorm.DB) (project.Project, project.Actor, error) {
	var projectRecord project.Project
	if err := db.WithContext(ctx).Where("uuid = ?", service.store.ProjectUUID()).First(&projectRecord).Error; err != nil {
		return projectRecord, project.Actor{}, err
	}
	var actor project.Actor
	if err := db.WithContext(ctx).Where("kind = ?", "local_user").Order("id ASC").First(&actor).Error; err != nil {
		return projectRecord, actor, err
	}
	return projectRecord, actor, nil
}

func (service *Service) GetProject(ctx context.Context) (ProjectDetail, error) {
	projectRecord, _, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return ProjectDetail{}, err
	}
	var activeCount, trashCount int64
	db := service.store.DB().WithContext(ctx).Model(&chapterRecord{}).Where("project_id = ?", projectRecord.ID)
	if err := db.Where("deleted_at IS NULL").Count(&activeCount).Error; err != nil {
		return ProjectDetail{}, err
	}
	if err := db.Where("deleted_at IS NOT NULL").Count(&trashCount).Error; err != nil {
		return ProjectDetail{}, err
	}
	return ProjectDetail{UUID: projectRecord.UUID, Name: projectRecord.Name, Description: projectRecord.Description, GenerationLanguage: projectRecord.GenerationLanguage, Revision: projectRecord.Revision, ChapterCount: activeCount, TrashCount: trashCount, UpdatedAt: projectRecord.UpdatedAt, SetupStatus: projectRecord.SetupStatus, PictureBook: service.store.OptionalPictureBookProfile()}, nil
}

type UpdateProjectInput struct {
	Name               string
	Description        string
	GenerationLanguage *string
	ExpectedRevision   int64
}

func (service *Service) UpdateProject(ctx context.Context, input UpdateProjectInput) (ProjectDetail, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || len([]rune(input.Name)) > 120 || len([]rune(input.Description)) > 2000 {
		return ProjectDetail{}, storyError(CodeValidationFailed, "项目信息无效", "名称需为 1 到 120 个字符，说明不能超过 2000 个字符。", nil)
	}
	now := service.now().UTC()
	var requestedLanguage *string
	if input.GenerationLanguage != nil {
		language, valid := project.NormalizeGenerationLanguage(*input.GenerationLanguage)
		if !valid {
			return ProjectDetail{}, storyError(CodeValidationFailed, "项目生成语言无效", "generation_language 只支持 zh-Hans 或 en。", nil)
		}
		requestedLanguage = &language
	}
	err := service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectRecord, actor, err := service.projectAndActor(ctx, tx)
		if err != nil {
			return err
		}
		if projectRecord.Revision != input.ExpectedRevision {
			return storyError(CodeProjectRevisionConflict, "项目信息版本冲突", "项目已被其他操作更新，请刷新后重试。", nil)
		}
		updates := map[string]any{"name": input.Name, "description": input.Description, "revision": gorm.Expr("revision + 1"), "updated_at": now}
		if requestedLanguage != nil {
			updates["generation_language"] = *requestedLanguage
			if *requestedLanguage != projectRecord.GenerationLanguage {
				if err := service.migratePromptLanguage(ctx, tx, projectRecord, actor, *requestedLanguage, now); err != nil {
					return err
				}
			}
		}
		result := tx.WithContext(ctx).Model(&project.Project{}).
			Where("id = ? AND revision = ?", projectRecord.ID, input.ExpectedRevision).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return storyError(CodeProjectRevisionConflict, "项目信息版本冲突", "项目已被其他操作更新，请刷新后重试。", nil)
		}
		return nil
	})
	if err != nil {
		return ProjectDetail{}, err
	}
	if err := service.store.RefreshProject(ctx); err != nil {
		return ProjectDetail{}, err
	}
	detail, err := service.GetProject(ctx)
	if err == nil {
		service.emit("story:project_changed", map[string]any{"revision": detail.Revision})
	}
	return detail, err
}

func (service *Service) migratePromptLanguage(ctx context.Context, tx *gorm.DB, projectRecord project.Project, actor project.Actor, newLanguage string, now time.Time) error {
	oldDefinitions := service.promptDefinitions(projectRecord.GenerationLanguage)
	newDefinitions := service.promptDefinitions(newLanguage)
	if len(oldDefinitions) != len(newDefinitions) {
		return fmt.Errorf("prompt catalog language drift: %d != %d", len(oldDefinitions), len(newDefinitions))
	}
	for index, oldDefinition := range oldDefinitions {
		newDefinition := newDefinitions[index]
		if oldDefinition.Group != newDefinition.Group || oldDefinition.Key != newDefinition.Key {
			return fmt.Errorf("prompt catalog identity drift at %s/%s", oldDefinition.Group, oldDefinition.Key)
		}
		var current promptVersionRecord
		err := tx.WithContext(ctx).
			Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, oldDefinition.Group, oldDefinition.Key).
			Order("version_no DESC").First(&current).Error
		currentVersion := 0
		if errors.Is(err, gorm.ErrRecordNotFound) {
			legacyFound := false
			for _, legacyKey := range oldDefinition.LegacyKeys {
				var count int64
				if countErr := tx.WithContext(ctx).Model(&promptVersionRecord{}).
					Where("project_id = ? AND prompt_group = ? AND prompt_key = ?", projectRecord.ID, oldDefinition.Group, legacyKey).
					Count(&count).Error; countErr != nil {
					return countErr
				}
				legacyFound = legacyFound || count > 0
			}
			if legacyFound {
				continue
			}
		} else if err != nil {
			return err
		} else {
			currentVersion = current.VersionNo
			if !tracksBuiltinPrompt(current.SourceType) {
				continue
			}
			currentHash := contentHash(strings.TrimSpace(current.Prompt))
			matchesOldDefault := currentHash == contentHash(strings.TrimSpace(oldDefinition.DefaultValue))
			for _, previous := range oldDefinition.PreviousDefaultValues {
				if currentHash == contentHash(strings.TrimSpace(previous)) {
					matchesOldDefault = true
					break
				}
			}
			if !matchesOldDefault {
				continue
			}
			if currentHash == contentHash(strings.TrimSpace(newDefinition.DefaultValue)) {
				continue
			}
		}
		uuid, err := newUUIDv7()
		if err != nil {
			return err
		}
		next := promptVersionRecord{
			UUID: uuid, ProjectID: projectRecord.ID, ActorID: actor.ID,
			PromptGroup: newDefinition.Group, PromptKey: newDefinition.Key,
			VersionNo: currentVersion + 1, Prompt: strings.TrimSpace(newDefinition.DefaultValue),
			PromptHash: contentHash(newDefinition.DefaultValue), SourceType: "project_language_changed", CreatedAt: now,
		}
		if err := tx.WithContext(ctx).Create(&next).Error; err != nil {
			return err
		}
		if newDefinition.Group == promptcatalog.GroupPremiseStyle && newDefinition.Key == "project_overall_style" {
			if err := syncPremiseStyleProjection(ctx, tx, projectRecord.ID, strings.TrimSpace(newDefinition.DefaultValue), now); err != nil {
				return err
			}
		}
	}
	return nil
}

func uniqueConflict(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func (service *Service) sourceAndItem(ctx context.Context, tx *gorm.DB, projectID, actorID, chapterID int64, ordinal int, sourceType, requestHash, filename, format, content string) (storySourceRecord, storySourceItemRecord, error) {
	now := service.now().UTC()
	sourceUUID, err := newUUIDv7()
	if err != nil {
		return storySourceRecord{}, storySourceItemRecord{}, err
	}
	itemUUID, err := newUUIDv7()
	if err != nil {
		return storySourceRecord{}, storySourceItemRecord{}, err
	}
	source := storySourceRecord{UUID: sourceUUID, ProjectID: projectID, ActorID: actorID, SourceType: sourceType, RequestHash: requestHash, ItemCount: 1, CreatedAt: now}
	if err := tx.WithContext(ctx).Create(&source).Error; err != nil {
		return source, storySourceItemRecord{}, err
	}
	var filenamePointer *string
	if strings.TrimSpace(filename) != "" {
		cleaned := strings.TrimSpace(filename)
		filenamePointer = &cleaned
	}
	item := storySourceItemRecord{UUID: itemUUID, SourceID: source.ID, ChapterID: &chapterID, Ordinal: ordinal, OriginalFilename: filenamePointer, ContentFormat: format, ContentHash: contentHash(content), ByteSize: int64(len([]byte(content))), CreatedAt: now}
	if err := tx.WithContext(ctx).Create(&item).Error; err != nil {
		return source, item, err
	}
	return source, item, nil
}

func randomRequestHash() (string, error) {
	value, err := newUUIDv7()
	if err != nil {
		return "", err
	}
	return contentHash(value), nil
}

func recordNotFound(err error, code, message, details string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storyError(code, message, details, err)
	}
	return err
}
