package modelsettings

import (
	"context"
	"errors"

	"lumi/internal/appstore"
)

func (resolver *Resolver) GetGlobal(ctx context.Context) (View, error) {
	if resolver == nil || resolver.providers == nil {
		return View{}, domainError(CodeNoModel, "模型服务不可用", "Provider 服务尚未初始化。", nil)
	}
	options, err := resolver.options(ctx)
	if err != nil {
		return View{}, err
	}
	return resolver.globalView(ctx, options)
}

func (resolver *Resolver) globalView(ctx context.Context, options Options) (View, error) {
	row, err := resolver.providers.GlobalModelSettings(ctx)
	if err != nil {
		return View{}, err
	}
	return buildGlobalView(row, options), nil
}

func (resolver *Resolver) PatchGlobal(ctx context.Context, input PatchInput) (View, error) {
	options, err := resolver.options(ctx)
	if err != nil {
		return View{}, err
	}
	if err := validateChanges(options, input); err != nil {
		return View{}, err
	}
	changes := make(map[string]*appstore.GlobalModelSelection, len(input.Changes))
	for key, selection := range input.Changes {
		changes[key] = nil
		if selection == nil {
			continue
		}
		items := options.TextModels
		if settingKinds[key] == KindImage {
			items = options.ImageModels
		}
		for _, option := range items {
			if option.ProviderUUID == selection.ProviderUUID && option.Model == selection.Model {
				changes[key] = &appstore.GlobalModelSelection{ProviderType: option.ProviderType, Model: selection.Model, EnableThinking: selection.EnableThinking, PromptExtend: selection.PromptExtend}
				break
			}
		}
	}
	row, err := resolver.providers.PatchGlobalModelSettings(ctx, input.ExpectedRevision, changes)
	if errors.Is(err, appstore.ErrGlobalModelSettingsConflict) {
		return View{}, domainError(CodeConflict, "模型设置已变化", "刷新后基于最新 revision 重试。", err)
	}
	if err != nil {
		return View{}, err
	}
	return buildGlobalView(row, options), nil
}

func buildGlobalView(row appstore.GlobalModelSettings, options Options) View {
	overrides := make(map[string]*Selection, len(row.Settings))
	items := append(append([]ModelOption{}, options.TextModels...), options.ImageModels...)
	for key, saved := range row.Settings {
		if saved == nil {
			continue
		}
		selection := &Selection{Model: saved.Model, EnableThinking: saved.EnableThinking, PromptExtend: saved.PromptExtend}
		// Resolve identities even when the saved model or provider isn't ready,
		// preserving the invalid selection so the UI can show it and reset it.
		for _, option := range items {
			if option.ProviderType == saved.ProviderType {
				selection.ProviderUUID = option.ProviderUUID
				break
			}
		}
		overrides[key] = selection
	}
	text := settingView(KindText, overrides[ProjectText], activeSelection(options.TextModels), SourceGlobalTextDefault, SourceGlobalDefault, options)
	picture := settingView(KindImage, overrides[ProjectImage], activeSelection(options.ImageModels), SourceGlobalImageDefault, SourceGlobalDefault, options)
	settings := map[string]SettingView{ProjectText: text, ProjectImage: picture}
	for _, key := range []string{ChatArea, StoryText, SectionPremiseSelection} {
		settings[key] = settingView(KindText, overrides[key], text.Effective, SourceGlobalScenario, text.Source, options)
	}
	return View{Revision: row.Revision, Settings: settings, Options: options}
}

// ResolveGlobal is used by preflight checks before a project exists.
func (resolver *Resolver) ResolveGlobal(ctx context.Context, settingKey, kind string) (Resolved, error) {
	if settingKinds[settingKey] != kind {
		return Resolved{}, domainError(CodeInvalid, "模型场景与能力类型不匹配", "请求的模型场景不支持该能力类型。", nil)
	}
	view, err := resolver.GetGlobal(ctx)
	if err != nil {
		return Resolved{}, err
	}
	return resolver.resolveView(ctx, view, settingKey, kind, "", "")
}
