package modelsettings

import (
	"context"
	"errors"
	"sort"

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
	if err := validateChanges(options, input, true); err != nil {
		return View{}, err
	}
	limitChanges, providerLimitChanges, err := validateRateLimitUpdates(options, input.RateLimitUpdates)
	if err != nil {
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
	row, err := resolver.providers.PatchGlobalModelSettings(ctx, input.ExpectedRevision, changes, limitChanges, providerLimitChanges)
	if errors.Is(err, appstore.ErrGlobalModelSettingsConflict) {
		return View{}, domainError(CodeConflict, "模型设置已变化", "刷新后基于最新 revision 重试。", err)
	}
	if err != nil {
		return View{}, err
	}
	if resolver.onRateLimitsChanged != nil {
		resolver.onRateLimitsChanged(row)
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
	rateLimits := make([]RateLimit, 0)
	providerUUIDs := make(map[string]string)
	for _, option := range items {
		providerUUIDs[option.ProviderType] = option.ProviderUUID
	}
	for providerType, limit := range row.ProviderRateLimits {
		if providerUUID := providerUUIDs[providerType]; providerUUID != "" {
			rateLimits = append(rateLimits, RateLimit{Scope: "provider", ProviderUUID: providerUUID, RPM: limit.RPM, RPS: limit.RPS})
		}
	}
	for providerType, models := range row.RateLimits {
		providerUUID := providerUUIDs[providerType]
		if providerUUID == "" {
			continue
		}
		for model, limit := range models {
			rateLimits = append(rateLimits, RateLimit{Scope: "model", ProviderUUID: providerUUID, Model: model, RPM: limit.RPM, RPS: limit.RPS})
		}
	}
	sort.Slice(rateLimits, func(i, j int) bool {
		if rateLimits[i].ProviderUUID != rateLimits[j].ProviderUUID {
			return rateLimits[i].ProviderUUID < rateLimits[j].ProviderUUID
		}
		if rateLimits[i].Scope != rateLimits[j].Scope {
			return rateLimits[i].Scope == "provider"
		}
		return rateLimits[i].Model < rateLimits[j].Model
	})
	return View{Revision: row.Revision, Settings: settings, Options: options, RateLimits: &rateLimits}
}

func validateRateLimitUpdates(options Options, updates []RateLimit) (map[string]map[string]*appstore.GlobalModelRateLimit, map[string]*appstore.GlobalModelRateLimit, error) {
	changes := make(map[string]map[string]*appstore.GlobalModelRateLimit)
	providerChanges := make(map[string]*appstore.GlobalModelRateLimit)
	all := append(append([]ModelOption{}, options.TextModels...), options.ImageModels...)
	for _, update := range updates {
		if (update.RPM != nil && *update.RPM <= 0) || (update.RPS != nil && *update.RPS <= 0) {
			return nil, nil, domainError(CodeInvalid, "模型限流设置无效", "rpm 和 rps 必须是正整数或 null。", nil)
		}
		scope := update.Scope
		if scope == "" {
			scope = "model"
		}
		if scope != "provider" && scope != "model" {
			return nil, nil, domainError(CodeInvalid, "模型限流设置无效", "scope 必须是 provider 或 model。", nil)
		}
		matched := false
		for _, option := range all {
			if option.ProviderUUID == update.ProviderUUID && (scope == "provider" || option.Model == update.Model) {
				if scope == "provider" {
					if update.Model != "" {
						return nil, nil, domainError(CodeInvalid, "供应商限流设置无效", "供应商总池规则不能指定 model。", nil)
					}
					if _, exists := providerChanges[option.ProviderType]; exists {
						return nil, nil, domainError(CodeInvalid, "供应商限流设置重复", "同一服务商只能更新一次总池规则。", nil)
					}
					if update.RPM != nil || update.RPS != nil {
						providerChanges[option.ProviderType] = &appstore.GlobalModelRateLimit{RPM: update.RPM, RPS: update.RPS}
					} else {
						providerChanges[option.ProviderType] = nil
					}
					matched = true
					break
				}
				if changes[option.ProviderType] == nil {
					changes[option.ProviderType] = make(map[string]*appstore.GlobalModelRateLimit)
				}
				if _, exists := changes[option.ProviderType][option.Model]; exists {
					return nil, nil, domainError(CodeInvalid, "模型限流设置重复", "同一服务商模型只能更新一次。", nil)
				}
				if update.RPM != nil || update.RPS != nil {
					changes[option.ProviderType][option.Model] = &appstore.GlobalModelRateLimit{RPM: update.RPM, RPS: update.RPS}
				} else {
					changes[option.ProviderType][option.Model] = nil
				}
				matched = true
				break
			}
		}
		if !matched {
			return nil, nil, domainError(CodeInvalid, "模型限流设置无效", "只能配置当前服务商或其支持的模型。", nil)
		}
	}
	return changes, providerChanges, nil
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
