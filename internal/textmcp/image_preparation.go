package textmcp

import (
	"context"

	"lumi/internal/httpapi"
	"lumi/internal/production"
)

type settingImageReadinessItem struct {
	SettingUUID      string `json:"setting_uuid"`
	Title            string `json:"title"`
	AssetType        string `json:"asset_type"`
	ExpectedRevision int64  `json:"expected_revision"`
	HasImage         bool   `json:"has_image"`
	AssetUUID        string `json:"asset_uuid,omitempty"`
	VariantUUID      string `json:"variant_uuid,omitempty"`
}

type settingImageReadiness struct {
	Scope          string                      `json:"scope"`
	Status         string                      `json:"status"`
	Items          []settingImageReadinessItem `json:"items"`
	AvailableCount int                         `json:"available_count"`
	MissingCount   int                         `json:"missing_count"`
	NextAction     string                      `json:"next_action"`
	StylePrompt    string                      `json:"style_prompt"`
	StyleDigest    string                      `json:"style_digest"`
}

type pageImageTarget struct {
	production.HostImageTarget
	SettingReadiness settingImageReadiness `json:"setting_readiness"`
}

// This reports persisted image facts without guessing which settings a page
// needs. An omitted selection is an inventory, never a readiness approval.
func imageTarget(ctx context.Context, prod *production.Service, a arguments) (any, error) {
	if a.ImageSettings != nil && a.Slot != "page_image" {
		return nil, httpapi.NewError(400, "invalid_arguments", "设定检查只用于页面", "setting_uuids 仅适用于 slot=page_image。", nil)
	}
	target, err := prod.HostImageTarget(ctx, a.TargetUUID, a.Slot)
	if err != nil || a.Slot != "page_image" {
		return target, err
	}
	settings, err := prod.ListPremiseAssets(ctx, "", "active")
	if err != nil {
		return nil, err
	}
	readiness := settingImageReadiness{Scope: "project_candidates", Status: "selection_required", NextAction: "select_page_settings", Items: []settingImageReadinessItem{}}
	readiness.StylePrompt, err = prod.HostSettingReviewStyle(ctx)
	if err != nil {
		return nil, err
	}
	readiness.StyleDigest = production.SettingReviewStyleDigest(readiness.StylePrompt)
	if a.ImageSettings != nil {
		byUUID := make(map[string]production.PremiseAsset, len(settings))
		for _, setting := range settings {
			byUUID[setting.UUID] = setting
		}
		settings = nil
		for _, uuid := range *a.ImageSettings {
			setting, exists := byUUID[uuid]
			if !exists {
				return nil, httpapi.NewError(400, "invalid_arguments", "设定选择无效", "setting_uuids 必须来自当前项目的 active 设定，且不得重复。", nil)
			}
			settings = append(settings, setting)
			delete(byUUID, uuid)
		}
		readiness.Scope, readiness.Status, readiness.NextAction = "selected_settings", "review_required", "review_setting_images"
	}
	for _, setting := range settings {
		hasImage := setting.CurrentVariant != nil && setting.CurrentVariant.Asset.UUID != "" && setting.CurrentVariant.Asset.Kind == "image"
		// Source notes are not visual subjects. Existing reference images remain
		// candidates, and callers may explicitly select a reference to illustrate.
		if a.ImageSettings == nil && setting.AssetType == "reference" && !hasImage {
			continue
		}
		item := settingImageReadinessItem{SettingUUID: setting.UUID, Title: setting.Title, AssetType: setting.AssetType, ExpectedRevision: setting.Revision, HasImage: hasImage}
		if hasImage {
			item.AssetUUID = setting.CurrentVariant.Asset.UUID
			item.VariantUUID = setting.CurrentVariant.UUID
			readiness.AvailableCount++
		} else {
			readiness.MissingCount++
		}
		readiness.Items = append(readiness.Items, item)
	}
	if a.ImageSettings != nil {
		if readiness.MissingCount > 0 {
			readiness.Status, readiness.NextAction = "missing_images", "prepare_setting_images"
		} else if len(readiness.Items) == 0 {
			readiness.Status, readiness.NextAction = "no_settings_selected", "review_setting_scope"
		}
	} else if len(readiness.Items) == 0 {
		readiness.NextAction = "plan_visual_settings"
	}
	return pageImageTarget{HostImageTarget: target, SettingReadiness: readiness}, nil
}
