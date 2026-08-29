package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

const (
	SetupStatusDraft = "draft"
	SetupStatusReady = "ready"

	SetupDraftStatusDraft               = "draft"
	SetupDraftStatusPendingConfirmation = "pending_confirmation"
	SetupDraftStatusFinalized           = "finalized"
	SetupDraftStatusFailed              = "failed"

	SetupSourceSystemDefault = "system_default"
	SetupSourceAgentProposed = "agent_proposed"
	SetupSourceUserConfirmed = "user_confirmed"
)

var setupDraftFields = []string{
	"project_name", "generation_language", "overall_style", "format", "aspect_ratio",
	"large_image_minimal_text", "interaction_mode", "comic_layout",
}

type SetupDraftInitialization struct {
	UUID               string
	OriginalInput      string
	GenerationLanguage string
}

type setupDraftRecord struct {
	ID                    int64 `gorm:"primaryKey;autoIncrement"`
	UUID                  string
	ProjectID             int64
	Status                string
	Revision              int64
	OriginalInput         string
	ProjectName           *string
	GenerationLanguage    *string
	OverallStyle          *string
	Format                *string
	AspectRatioMode       *string
	AspectWidth           *int
	AspectHeight          *int
	LargeImageMinimalText *bool
	InteractionMode       *string
	ComicLayout           *string
	FieldSourcesJSON      string
	MissingFieldsJSON     string
	ErrorCode             string
	ErrorMessage          string
	FinalizedRevision     *int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	FinalizedAt           *time.Time
	FailedAt              *time.Time
}

func (setupDraftRecord) TableName() string { return "project_setup_drafts" }

type SetupDraftValues struct {
	ProjectName        string              `json:"project_name,omitempty"`
	GenerationLanguage string              `json:"generation_language,omitempty"`
	OverallStyle       string              `json:"overall_style,omitempty"`
	PictureBook        *PictureBookProfile `json:"picture_book,omitempty"`
}

type SetupState struct {
	UUID               string              `json:"uuid,omitempty"`
	ProjectUUID        string              `json:"project_uuid"`
	SetupStatus        string              `json:"setup_status"`
	Status             string              `json:"status"`
	Revision           int64               `json:"revision"`
	OriginalInput      string              `json:"original_input,omitempty"`
	DraftValues        SetupDraftValues    `json:"draft_values"`
	FieldSources       map[string]string   `json:"field_sources"`
	MissingInformation []string            `json:"missing_information"`
	FinalPictureBook   *PictureBookProfile `json:"final_picture_book,omitempty"`
	ErrorCode          string              `json:"error_code,omitempty"`
	ErrorMessage       string              `json:"error_message,omitempty"`
	CreatedAt          time.Time           `json:"created_at,omitempty"`
	UpdatedAt          time.Time           `json:"updated_at,omitempty"`
	FinalizedAt        *time.Time          `json:"finalized_at,omitempty"`
}

type SetupDraftPatchInput struct {
	ExpectedRevision   int64
	ProjectName        *string
	GenerationLanguage *string
	OverallStyle       *string
	PictureBook        *PictureBookInput
}

func createSetupDraftRecord(tx *gorm.DB, projectID int64, input SetupDraftInitialization, now time.Time) error {
	original := input.OriginalInput
	if !isUUIDv7(input.UUID) || strings.TrimSpace(original) == "" || len(original) > 256<<10 || !utf8.ValidString(original) || strings.ContainsRune(original, 0) {
		return projectError(CodeProjectSetupInvalid, "项目设置草稿无效", "原始输入和设置 UUID 必须有效。", nil)
	}
	language, valid := NormalizeGenerationLanguage(input.GenerationLanguage)
	if !valid {
		language = DefaultGenerationLanguage
	}
	sources, _ := json.Marshal(map[string]string{"generation_language": SetupSourceSystemDefault})
	missing, _ := json.Marshal([]string{"project_name", "overall_style", "picture_book.format"})
	record := setupDraftRecord{
		UUID: input.UUID, ProjectID: projectID, Status: SetupDraftStatusDraft, Revision: 1,
		OriginalInput: original, GenerationLanguage: &language,
		FieldSourcesJSON: string(sources), MissingFieldsJSON: string(missing), CreatedAt: now, UpdatedAt: now,
	}
	return tx.Create(&record).Error
}

func decodeSetupSources(raw string) map[string]string {
	result := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func decodeMissingFields(raw string) []string {
	result := []string{}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func setupRecordPictureBook(record setupDraftRecord) *PictureBookProfile {
	if record.Format == nil || record.AspectRatioMode == nil || record.AspectWidth == nil || record.AspectHeight == nil {
		return nil
	}
	return &PictureBookProfile{
		Format:                *record.Format,
		AspectRatio:           AspectRatio{Mode: *record.AspectRatioMode, Width: *record.AspectWidth, Height: *record.AspectHeight},
		LargeImageMinimalText: record.LargeImageMinimalText, InteractionMode: record.InteractionMode, ComicLayout: record.ComicLayout,
	}
}

func setupState(projectRecord Project, record *setupDraftRecord, profile *PictureBookProfile) SetupState {
	state := SetupState{
		ProjectUUID: projectRecord.UUID, SetupStatus: projectRecord.SetupStatus,
		Status: SetupDraftStatusFinalized, FieldSources: map[string]string{}, MissingInformation: []string{},
		FinalPictureBook: profile,
	}
	if record == nil {
		state.DraftValues = SetupDraftValues{ProjectName: projectRecord.Name, GenerationLanguage: projectRecord.GenerationLanguage, PictureBook: profile}
		for _, field := range setupDraftFields {
			state.FieldSources[field] = SetupSourceUserConfirmed
		}
		return state
	}
	state.UUID, state.Status, state.Revision = record.UUID, record.Status, record.Revision
	state.OriginalInput, state.FieldSources = record.OriginalInput, decodeSetupSources(record.FieldSourcesJSON)
	state.MissingInformation = decodeMissingFields(record.MissingFieldsJSON)
	state.ErrorCode, state.ErrorMessage = record.ErrorCode, record.ErrorMessage
	state.CreatedAt, state.UpdatedAt, state.FinalizedAt = record.CreatedAt, record.UpdatedAt, record.FinalizedAt
	state.DraftValues.PictureBook = setupRecordPictureBook(*record)
	if record.ProjectName != nil {
		state.DraftValues.ProjectName = *record.ProjectName
	}
	if record.GenerationLanguage != nil {
		state.DraftValues.GenerationLanguage = *record.GenerationLanguage
	}
	if record.OverallStyle != nil {
		state.DraftValues.OverallStyle = *record.OverallStyle
	}
	return state
}

func (store *Store) ProjectSetup(ctx context.Context) (SetupState, error) {
	var projectRecord Project
	if err := store.db.WithContext(ctx).Where("uuid = ?", store.ProjectUUID()).First(&projectRecord).Error; err != nil {
		return SetupState{}, err
	}
	var record setupDraftRecord
	result := store.db.WithContext(ctx).Where("project_id = ?", projectRecord.ID).Limit(1).Find(&record)
	if result.Error != nil {
		return SetupState{}, result.Error
	}
	var recordPointer *setupDraftRecord
	if result.RowsAffected == 1 {
		recordPointer = &record
	}
	return setupState(projectRecord, recordPointer, store.OptionalPictureBookProfile()), nil
}

func validateSetupText(value string, maximum int, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len([]rune(value)) > maximum {
		return "", projectError(CodeProjectSetupInvalid, "项目设置无效", fmt.Sprintf("%s 必须为 1 到 %d 个有效字符。", field, maximum), nil)
	}
	return value, nil
}

func setupMissing(record setupDraftRecord) []string {
	missing := []string{}
	if record.ProjectName == nil || strings.TrimSpace(*record.ProjectName) == "" {
		missing = append(missing, "project_name")
	}
	if record.GenerationLanguage == nil || strings.TrimSpace(*record.GenerationLanguage) == "" {
		missing = append(missing, "generation_language")
	}
	if record.OverallStyle == nil || strings.TrimSpace(*record.OverallStyle) == "" {
		missing = append(missing, "overall_style")
	}
	if setupRecordPictureBook(record) == nil {
		missing = append(missing, "picture_book.format")
	}
	return missing
}

func (store *Store) UpdateProjectSetupDraft(ctx context.Context, input SetupDraftPatchInput) (SetupState, error) {
	if input.ExpectedRevision < 1 {
		return SetupState{}, projectError(CodeProjectSetupConflict, "项目设置版本冲突", "expected_revision 必须是刚读取到的正整数 revision。", nil)
	}
	now := time.Now().UTC()
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var projectRecord Project
		if err := tx.Where("uuid = ?", store.ProjectUUID()).First(&projectRecord).Error; err != nil {
			return err
		}
		if projectRecord.SetupStatus != SetupStatusDraft {
			return projectError(CodePictureBookImmutable, "绘本规格已经定稿", "ready 项目的正式绘本规格不可修改。", nil)
		}
		var record setupDraftRecord
		if err := tx.Where("project_id = ?", projectRecord.ID).First(&record).Error; err != nil {
			return err
		}
		if record.Status == SetupDraftStatusFinalized || record.Revision != input.ExpectedRevision {
			return projectError(CodeProjectSetupConflict, "项目设置版本冲突", "项目设置已更新，请重新读取后再修改。", nil)
		}
		sources := decodeSetupSources(record.FieldSourcesJSON)
		changed := false
		if input.ProjectName != nil {
			value, err := validateSetupText(*input.ProjectName, 120, "project_name")
			if err != nil {
				return err
			}
			record.ProjectName, sources["project_name"], changed = &value, SetupSourceAgentProposed, true
		}
		if input.GenerationLanguage != nil {
			value, valid := NormalizeGenerationLanguage(*input.GenerationLanguage)
			if !valid {
				return projectError(CodeProjectSetupInvalid, "项目生成语言无效", "generation_language 只支持 zh-Hans 或 en。", nil)
			}
			record.GenerationLanguage, sources["generation_language"], changed = &value, SetupSourceAgentProposed, true
		}
		if input.OverallStyle != nil {
			value, err := validateSetupText(*input.OverallStyle, 12000, "overall_style")
			if err != nil {
				return err
			}
			record.OverallStyle, sources["overall_style"], changed = &value, SetupSourceAgentProposed, true
		}
		if input.PictureBook != nil {
			if strings.TrimSpace(input.PictureBook.Format) == "" {
				return projectError(CodeProjectSetupInvalid, "绘本形式缺失", "picture_book.format 不能为空。", nil)
			}
			profile, err := NormalizePictureBookInput(input.PictureBook)
			if err != nil {
				return err
			}
			record.Format, record.AspectRatioMode = &profile.Format, &profile.AspectRatio.Mode
			record.AspectWidth, record.AspectHeight = &profile.AspectRatio.Width, &profile.AspectRatio.Height
			record.LargeImageMinimalText, record.InteractionMode, record.ComicLayout = profile.LargeImageMinimalText, profile.InteractionMode, profile.ComicLayout
			sources["format"] = SetupSourceAgentProposed
			if input.PictureBook.AspectRatio == nil {
				sources["aspect_ratio"] = SetupSourceSystemDefault
			} else {
				sources["aspect_ratio"] = SetupSourceAgentProposed
			}
			delete(sources, "large_image_minimal_text")
			delete(sources, "interaction_mode")
			delete(sources, "comic_layout")
			switch profile.Format {
			case PictureBookClassic:
				if input.PictureBook.LargeImageMinimalText == nil {
					sources["large_image_minimal_text"] = SetupSourceSystemDefault
				} else {
					sources["large_image_minimal_text"] = SetupSourceAgentProposed
				}
			case PictureBookInteractive:
				if input.PictureBook.InteractionMode == nil {
					sources["interaction_mode"] = SetupSourceSystemDefault
				} else {
					sources["interaction_mode"] = SetupSourceAgentProposed
				}
			case PictureBookComicStory:
				if input.PictureBook.ComicLayout == nil {
					sources["comic_layout"] = SetupSourceSystemDefault
				} else {
					sources["comic_layout"] = SetupSourceAgentProposed
				}
			}
			changed = true
		}
		if !changed {
			return projectError(CodeProjectSetupInvalid, "项目设置没有变化", "至少提供一个初始化草稿字段。", nil)
		}
		missing := setupMissing(record)
		sourcesJSON, _ := json.Marshal(sources)
		missingJSON, _ := json.Marshal(missing)
		status := SetupDraftStatusDraft
		if len(missing) == 0 {
			status = SetupDraftStatusPendingConfirmation
		}
		result := tx.Model(&setupDraftRecord{}).Where("id = ? AND revision = ?", record.ID, input.ExpectedRevision).Updates(map[string]any{
			"status": status, "revision": gorm.Expr("revision + 1"), "project_name": record.ProjectName,
			"generation_language": record.GenerationLanguage, "overall_style": record.OverallStyle, "format": record.Format,
			"aspect_ratio_mode": record.AspectRatioMode, "aspect_width": record.AspectWidth, "aspect_height": record.AspectHeight,
			"large_image_minimal_text": record.LargeImageMinimalText, "interaction_mode": record.InteractionMode, "comic_layout": record.ComicLayout,
			"field_sources_json": string(sourcesJSON), "missing_fields_json": string(missingJSON), "error_code": "", "error_message": "", "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return projectError(CodeProjectSetupConflict, "项目设置版本冲突", "项目设置已更新，请重新读取后再修改。", nil)
		}
		return nil
	})
	if err != nil {
		return SetupState{}, err
	}
	return store.ProjectSetup(ctx)
}

func (store *Store) FinalizeProjectSetup(ctx context.Context, expectedRevision int64) (SetupState, error) {
	if expectedRevision < 1 {
		return SetupState{}, projectError(CodeProjectSetupConflict, "项目设置版本冲突", "expected_revision 必须是刚读取到的正整数 revision。", nil)
	}
	now := time.Now().UTC()
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var projectRecord Project
		if err := tx.Where("uuid = ?", store.ProjectUUID()).First(&projectRecord).Error; err != nil {
			return err
		}
		var record setupDraftRecord
		if err := tx.Where("project_id = ?", projectRecord.ID).First(&record).Error; err != nil {
			return err
		}
		if projectRecord.SetupStatus == SetupStatusReady && record.Status == SetupDraftStatusFinalized {
			if record.FinalizedRevision != nil && *record.FinalizedRevision == expectedRevision {
				return nil
			}
			return projectError(CodeProjectSetupConflict, "项目设置已经定稿", "该请求与已经完成的定稿 revision 不一致。", nil)
		}
		if projectRecord.SetupStatus != SetupStatusDraft || record.Status == SetupDraftStatusFinalized || record.Revision != expectedRevision {
			return projectError(CodeProjectSetupConflict, "项目设置版本冲突", "项目设置已更新，请重新读取后再定稿。", nil)
		}
		missing := setupMissing(record)
		if len(missing) > 0 {
			return projectError(CodeProjectSetupInvalid, "项目设置尚不完整", "仍缺少："+strings.Join(missing, "、")+"。", nil)
		}
		profile := setupRecordPictureBook(record)
		if profile == nil {
			return projectError(CodeProjectSetupInvalid, "绘本规格无效", "初始化草稿中的绘本规格不完整。", nil)
		}
		input := &PictureBookInput{Format: profile.Format, LargeImageMinimalText: profile.LargeImageMinimalText, InteractionMode: profile.InteractionMode, ComicLayout: profile.ComicLayout}
		if profile.Format != PictureBookVertical && profile.Format != PictureBookInteractive {
			input.AspectRatio = &AspectRatioInput{Mode: profile.AspectRatio.Mode}
			if profile.AspectRatio.Mode == AspectCustom {
				input.AspectRatio.Width, input.AspectRatio.Height = profile.AspectRatio.Width, profile.AspectRatio.Height
			}
		}
		normalized, err := NormalizePictureBookInput(input)
		if err != nil {
			return err
		}
		if normalized.AspectRatio != profile.AspectRatio {
			return projectError(CodeProjectSetupInvalid, "绘本比例不是规范值", "请重新提交初始化草稿中的绘本规格。", nil)
		}
		if err := tx.Create(&pictureBookProfileRecord{
			ProjectID: projectRecord.ID, Format: normalized.Format, AspectRatioMode: normalized.AspectRatio.Mode,
			AspectWidth: normalized.AspectRatio.Width, AspectHeight: normalized.AspectRatio.Height,
			LargeImageMinimalText: normalized.LargeImageMinimalText, InteractionMode: normalized.InteractionMode,
			ComicLayout: normalized.ComicLayout, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		projectResult := tx.Model(&Project{}).Where("id = ? AND setup_status = ?", projectRecord.ID, SetupStatusDraft).Updates(map[string]any{
			"name": *record.ProjectName, "generation_language": *record.GenerationLanguage,
			"setup_status": SetupStatusReady, "revision": gorm.Expr("revision + 1"), "updated_at": now,
		})
		if projectResult.Error != nil {
			return projectResult.Error
		}
		if projectResult.RowsAffected != 1 {
			return projectError(CodeProjectSetupConflict, "项目设置版本冲突", "项目生命周期已变化。", nil)
		}
		if err := tx.Table("premise_profiles").Where("project_id = ?", projectRecord.ID).Updates(map[string]any{
			"default_style": *record.OverallStyle, "revision": gorm.Expr("revision + 1"), "updated_at": now,
		}).Error; err != nil {
			return err
		}
		sources := decodeSetupSources(record.FieldSourcesJSON)
		for key := range sources {
			sources[key] = SetupSourceUserConfirmed
		}
		sourcesJSON, _ := json.Marshal(sources)
		finalizedRevision := record.Revision
		return tx.Model(&setupDraftRecord{}).Where("id = ? AND revision = ?", record.ID, expectedRevision).Updates(map[string]any{
			"status": SetupDraftStatusFinalized, "field_sources_json": string(sourcesJSON), "missing_fields_json": "[]",
			"finalized_revision": finalizedRevision, "finalized_at": now, "updated_at": now,
		}).Error
	})
	if err != nil {
		var projectErr *Error
		if errors.As(err, &projectErr) {
			return SetupState{}, err
		}
		return SetupState{}, projectError(CodeProjectSetupInvalid, "项目设置定稿失败", "正式绘本规格未写入。", err)
	}
	if err := store.RefreshProject(ctx); err != nil {
		return SetupState{}, err
	}
	return store.ProjectSetup(ctx)
}
