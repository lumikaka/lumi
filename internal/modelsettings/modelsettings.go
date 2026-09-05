package modelsettings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"lumi/internal/project"
	"lumi/internal/provider"

	"gorm.io/gorm"
)

const (
	KindText  = "text"
	KindImage = "image"

	ProjectText             = "project_text"
	ProjectImage            = "project_image"
	ChatArea                = "chat_area"
	StoryText               = "story_text"
	SectionPremiseSelection = "section_premise_selection"

	SourceExplicitTask         = "explicit_task"
	SourceScenarioOverride     = "scenario_override"
	SourceProjectTextOverride  = "project_text_override"
	SourceProjectImageOverride = "project_image_override"
	SourceGlobalDefault        = "global_provider_default"
)

const (
	CodeInvalid  = "project_model_settings_invalid"
	CodeConflict = "project_model_settings_conflict"
	CodeNoModel  = "project_model_unavailable"
)

type Error struct {
	Code, Message, Details string
	Cause                  error
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %v", err.Message, err.Cause)
	}
	return err.Message
}

func (err *Error) Unwrap() error { return err.Cause }

func domainError(code, message, details string, cause error) error {
	return &Error{Code: code, Message: message, Details: details, Cause: cause}
}

type Selection struct {
	ProviderUUID string `json:"provider_uuid"`
	Model        string `json:"model"`
}

type ModelOption struct {
	ProviderUUID string `json:"provider_uuid"`
	ProviderType string `json:"provider_type"`
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
	Kind         string `json:"kind"`
	Ready        bool   `json:"ready"`
	Active       bool   `json:"active"`
}

type Options struct {
	TextModels  []ModelOption `json:"text_models"`
	ImageModels []ModelOption `json:"image_models"`
}

type SettingView struct {
	Kind           string     `json:"kind"`
	Override       *Selection `json:"override"`
	Inherited      *Selection `json:"inherited"`
	Effective      *Selection `json:"effective"`
	Source         string     `json:"source"`
	OverrideStatus string     `json:"override_status"`
}

type View struct {
	Revision int                    `json:"revision"`
	Settings map[string]SettingView `json:"settings"`
	Options  Options                `json:"options"`
}

type PatchInput struct {
	ExpectedRevision int
	Changes          map[string]*Selection
}

type Resolved struct {
	Provider provider.Resolved
	Model    string
	Source   string
}

type Resolver struct{ providers *provider.Service }

func NewResolver(providers *provider.Service) *Resolver { return &Resolver{providers: providers} }

type record struct {
	ID                                          int64 `gorm:"primaryKey"`
	ProjectID                                   int64
	ProjectTextProviderUUID, ProjectTextModel   string
	ProjectImageProviderUUID, ProjectImageModel string
	ChatAreaProviderUUID, ChatAreaModel         string
	StoryTextProviderUUID, StoryTextModel       string
	SectionPremiseSelectionProviderUUID         string
	SectionPremiseSelectionModel                string
	Revision                                    int
	CreatedAt, UpdatedAt                        time.Time
}

func (record) TableName() string { return "project_model_settings" }

var settingKinds = map[string]string{
	ProjectText: KindText, ProjectImage: KindImage, ChatArea: KindText,
	StoryText: KindText, SectionPremiseSelection: KindText,
}

func ValidSettingKey(key string) bool {
	_, ok := settingKinds[key]
	return ok
}

func (resolver *Resolver) Get(ctx context.Context, store *project.Store) (View, error) {
	if resolver == nil || resolver.providers == nil {
		return View{}, domainError(CodeNoModel, "模型服务不可用", "Provider 服务尚未初始化。", nil)
	}
	projectID, err := projectID(ctx, store)
	if err != nil {
		return View{}, err
	}
	var row record
	err = store.DB().WithContext(ctx).Where("project_id = ?", projectID).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return View{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = record{ProjectID: projectID}
	}
	options, err := resolver.options(ctx)
	if err != nil {
		return View{}, err
	}
	return buildView(row, options), nil
}

func (resolver *Resolver) Patch(ctx context.Context, store *project.Store, input PatchInput) (View, error) {
	if input.ExpectedRevision < 0 || len(input.Changes) == 0 {
		return View{}, domainError(CodeInvalid, "模型设置更新无效", "expected_revision 必须非负且至少更新一项。", nil)
	}
	options, err := resolver.options(ctx)
	if err != nil {
		return View{}, err
	}
	for key, selection := range input.Changes {
		kind, ok := settingKinds[key]
		if !ok {
			return View{}, domainError(CodeInvalid, "模型设置项无效", "不支持设置项 "+key+"。", nil)
		}
		if selection == nil {
			continue
		}
		selection.ProviderUUID = strings.TrimSpace(selection.ProviderUUID)
		selection.Model = strings.TrimSpace(selection.Model)
		if selection.ProviderUUID == "" || selection.Model == "" || len([]rune(selection.Model)) > 512 || !optionAvailable(options, kind, *selection) {
			return View{}, domainError(CodeInvalid, "模型不可用或能力类型不匹配", "只能选择当前已就绪 Provider 对应类型的模型。", nil)
		}
	}
	projectID, err := projectID(ctx, store)
	if err != nil {
		return View{}, err
	}
	now := time.Now().UTC()
	err = store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lockErr := tx.Model(&project.Project{}).Where("id = ?", projectID).UpdateColumn("updated_at", gorm.Expr("updated_at")).Error; lockErr != nil {
			return lockErr
		}
		var row record
		findErr := tx.Where("project_id = ?", projectID).First(&row).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			if input.ExpectedRevision != 0 {
				return domainError(CodeConflict, "模型设置已变化", "刷新后基于最新 revision 重试。", nil)
			}
			row = record{ProjectID: projectID, CreatedAt: now, UpdatedAt: now}
			applyChanges(&row, input.Changes)
			row.Revision = 1
			return tx.Create(&row).Error
		}
		if findErr != nil {
			return findErr
		}
		if row.Revision != input.ExpectedRevision {
			return domainError(CodeConflict, "模型设置已变化", "刷新后基于最新 revision 重试。", nil)
		}
		applyChanges(&row, input.Changes)
		result := tx.Model(&record{}).Where("id = ? AND revision = ?", row.ID, input.ExpectedRevision).Updates(map[string]any{
			"project_text_provider_uuid": row.ProjectTextProviderUUID, "project_text_model": row.ProjectTextModel,
			"project_image_provider_uuid": row.ProjectImageProviderUUID, "project_image_model": row.ProjectImageModel,
			"chat_area_provider_uuid": row.ChatAreaProviderUUID, "chat_area_model": row.ChatAreaModel,
			"story_text_provider_uuid": row.StoryTextProviderUUID, "story_text_model": row.StoryTextModel,
			"section_premise_selection_provider_uuid": row.SectionPremiseSelectionProviderUUID,
			"section_premise_selection_model":         row.SectionPremiseSelectionModel,
			"revision":                                gorm.Expr("revision + 1"), "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainError(CodeConflict, "模型设置已变化", "刷新后基于最新 revision 重试。", nil)
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return resolver.Get(ctx, store)
}

func (resolver *Resolver) Resolve(ctx context.Context, store *project.Store, settingKey, kind, explicitProviderUUID, explicitModel string) (Resolved, error) {
	if settingKinds[settingKey] != kind {
		return Resolved{}, domainError(CodeInvalid, "模型场景与能力类型不匹配", "请求的模型场景不支持该能力类型。", nil)
	}
	view, err := resolver.Get(ctx, store)
	if err != nil {
		return Resolved{}, err
	}
	selection := Selection{ProviderUUID: strings.TrimSpace(explicitProviderUUID), Model: strings.TrimSpace(explicitModel)}
	source := SourceExplicitTask
	if selection.ProviderUUID == "" && selection.Model == "" {
		setting := view.Settings[settingKey]
		if setting.Effective == nil {
			return Resolved{}, domainError(CodeNoModel, "没有可用模型", "请先配置并验证全局 Provider，或选择有效的项目模型覆盖。", nil)
		}
		selection = *setting.Effective
		source = setting.Source
	} else {
		selection, err = selectExplicit(view.Options, kind, selection, view.Settings[settingKey].Effective)
		if err != nil {
			return Resolved{}, err
		}
	}
	item, err := resolver.providers.Resolve(ctx, selection.ProviderUUID)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Provider: item, Model: selection.Model, Source: source}, nil
}

func (resolver *Resolver) options(ctx context.Context) (Options, error) {
	providers, err := resolver.providers.List(ctx)
	if err != nil {
		return Options{}, err
	}
	result := Options{TextModels: []ModelOption{}, ImageModels: []ModelOption{}}
	for _, item := range providers {
		for _, model := range provider.SupportedTextModels(item) {
			result.TextModels = append(result.TextModels, ModelOption{ProviderUUID: item.UUID, ProviderType: item.ProviderType, ProviderName: item.DisplayName, Model: model, Kind: KindText, Ready: item.Ready, Active: item.Active})
		}
		for _, model := range provider.SupportedImageModels(item) {
			result.ImageModels = append(result.ImageModels, ModelOption{ProviderUUID: item.UUID, ProviderType: item.ProviderType, ProviderName: item.DisplayName, Model: model, Kind: KindImage, Ready: item.Ready, Active: item.Active})
		}
	}
	return result, nil
}

func buildView(row record, options Options) View {
	globalText := activeSelection(options.TextModels)
	globalImage := activeSelection(options.ImageModels)
	projectText := settingView(KindText, overrideFor(row, ProjectText), globalText, SourceProjectTextOverride, SourceGlobalDefault, options)
	projectImage := settingView(KindImage, overrideFor(row, ProjectImage), globalImage, SourceProjectImageOverride, SourceGlobalDefault, options)
	settings := map[string]SettingView{
		ProjectText:             projectText,
		ProjectImage:            projectImage,
		ChatArea:                settingView(KindText, overrideFor(row, ChatArea), projectText.Effective, SourceScenarioOverride, projectText.Source, options),
		StoryText:               settingView(KindText, overrideFor(row, StoryText), projectText.Effective, SourceScenarioOverride, projectText.Source, options),
		SectionPremiseSelection: settingView(KindText, overrideFor(row, SectionPremiseSelection), projectText.Effective, SourceScenarioOverride, projectText.Source, options),
	}
	return View{Revision: row.Revision, Settings: settings, Options: options}
}

func settingView(kind string, override, inherited *Selection, overrideSource, inheritedSource string, options Options) SettingView {
	result := SettingView{Kind: kind, Override: override, Inherited: cloneSelection(inherited), Effective: cloneSelection(inherited), Source: inheritedSource, OverrideStatus: "inherit"}
	if override == nil {
		return result
	}
	if optionAvailable(options, kind, *override) {
		result.Effective, result.Source, result.OverrideStatus = cloneSelection(override), overrideSource, "valid"
		return result
	}
	result.OverrideStatus = "invalid"
	return result
}

func activeSelection(options []ModelOption) *Selection {
	for _, option := range options {
		if option.Active && option.Ready {
			return &Selection{ProviderUUID: option.ProviderUUID, Model: option.Model}
		}
	}
	return nil
}

func optionAvailable(options Options, kind string, selection Selection) bool {
	items := options.TextModels
	if kind == KindImage {
		items = options.ImageModels
	}
	for _, option := range items {
		if option.Ready && option.ProviderUUID == selection.ProviderUUID && option.Model == selection.Model {
			return true
		}
	}
	return false
}

func selectExplicit(options Options, kind string, selection Selection, effective *Selection) (Selection, error) {
	items := options.TextModels
	if kind == KindImage {
		items = options.ImageModels
	}
	if selection.ProviderUUID != "" {
		for _, option := range items {
			if option.Ready && option.ProviderUUID == selection.ProviderUUID && (selection.Model == "" || option.Model == selection.Model) {
				return Selection{ProviderUUID: option.ProviderUUID, Model: option.Model}, nil
			}
		}
	} else if selection.Model != "" {
		if effective != nil {
			for _, option := range items {
				if option.Ready && option.ProviderUUID == effective.ProviderUUID && option.Model == selection.Model {
					return Selection{ProviderUUID: option.ProviderUUID, Model: option.Model}, nil
				}
			}
		}
		for _, option := range items {
			if option.Ready && option.Model == selection.Model {
				return Selection{ProviderUUID: option.ProviderUUID, Model: option.Model}, nil
			}
		}
	}
	return Selection{}, domainError(CodeInvalid, "显式模型不可用或能力类型不匹配", "请从当前场景的可用模型选项中选择。", nil)
}

func overrideFor(row record, key string) *Selection {
	selection := Selection{}
	switch key {
	case ProjectText:
		selection = Selection{row.ProjectTextProviderUUID, row.ProjectTextModel}
	case ProjectImage:
		selection = Selection{row.ProjectImageProviderUUID, row.ProjectImageModel}
	case ChatArea:
		selection = Selection{row.ChatAreaProviderUUID, row.ChatAreaModel}
	case StoryText:
		selection = Selection{row.StoryTextProviderUUID, row.StoryTextModel}
	case SectionPremiseSelection:
		selection = Selection{row.SectionPremiseSelectionProviderUUID, row.SectionPremiseSelectionModel}
	}
	if selection.ProviderUUID == "" && selection.Model == "" {
		return nil
	}
	return &selection
}

func applyChanges(row *record, changes map[string]*Selection) {
	for key, value := range changes {
		selection := Selection{}
		if value != nil {
			selection = *value
		}
		switch key {
		case ProjectText:
			row.ProjectTextProviderUUID, row.ProjectTextModel = selection.ProviderUUID, selection.Model
		case ProjectImage:
			row.ProjectImageProviderUUID, row.ProjectImageModel = selection.ProviderUUID, selection.Model
		case ChatArea:
			row.ChatAreaProviderUUID, row.ChatAreaModel = selection.ProviderUUID, selection.Model
		case StoryText:
			row.StoryTextProviderUUID, row.StoryTextModel = selection.ProviderUUID, selection.Model
		case SectionPremiseSelection:
			row.SectionPremiseSelectionProviderUUID, row.SectionPremiseSelectionModel = selection.ProviderUUID, selection.Model
		}
	}
}

func cloneSelection(value *Selection) *Selection {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func projectID(ctx context.Context, store *project.Store) (int64, error) {
	var id int64
	if err := store.DB().WithContext(ctx).Table("projects").Where("uuid = ?", store.ProjectUUID()).Pluck("id", &id).Error; err != nil || id == 0 {
		return 0, domainError(CodeInvalid, "项目模型设置不可用", "当前项目身份无效。", err)
	}
	return id, nil
}
