package textmcp

import (
	"context"
	"fmt"

	"lumi/internal/httpapi"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

type reviewCandidate struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type reviewVariant struct {
	UUID      string `json:"uuid,omitempty"`
	AssetUUID string `json:"asset_uuid,omitempty"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Text      string `json:"text"`
}

type reviewItem struct {
	UUID        string          `json:"uuid"`
	Revision    int64           `json:"revision"`
	Title       string          `json:"title"`
	Text        string          `json:"text"`
	Scene       string          `json:"scene"`
	Kind        string          `json:"kind"`
	ChapterUUID string          `json:"chapter_uuid,omitempty"`
	AssetUUID   string          `json:"asset_uuid,omitempty"`
	OpenURL     string          `json:"open_url"`
	Variants    []reviewVariant `json:"variants,omitempty"`
}

func pageReview(p production.ComicSection, base string) reviewItem {
	item := reviewItem{UUID: p.UUID, Revision: p.Revision, Title: p.Title, Text: p.DescriptionMD, Kind: p.PageRole, ChapterUUID: p.ChapterUUID, OpenURL: base + "/chapters/" + p.ChapterUUID + "/sections/" + p.UUID}
	if p.CurrentStoryboard != nil {
		item.Scene = p.CurrentStoryboard.ContentMD
	}
	if p.CurrentImage != nil {
		item.AssetUUID = p.CurrentImage.Asset.UUID
	}
	return item
}

func settingReview(p production.PremiseAsset, base string) reviewItem {
	item := reviewItem{UUID: p.UUID, Revision: p.Revision, Title: p.Title, Text: p.Summary, Kind: p.AssetType, OpenURL: base + "/premise/assets/" + p.UUID}
	if p.CurrentVariant != nil {
		item.AssetUUID = p.CurrentVariant.Asset.UUID
	}
	return item
}

func (s *Service) review(ctx context.Context, store *project.Store, st *story.Service, prod *production.Service, name string, a arguments) (any, error) {
	p, err := st.GetProject(ctx)
	if err != nil {
		return nil, err
	}
	base := s.Endpoint + "/projects/" + store.ProjectUUID()
	// Keep a usable command even in protocol/service tests without a web origin.
	open := base + "/chapters"
	data := map[string]any{"view": name[len("review_"):], "project_uuid": store.ProjectUUID(), "target_uuid": store.ProjectUUID(), "revision": p.Revision, "title": p.Name, "open_url": open, "open_command": "lumi open", "request": map[string]any{"name": name, "arguments": aReviewArgs(name, a)}, "selection_notice": "选择只保留在当前界面。请在对话中回复候选编号，由模型重读后提交。"}
	items := []reviewItem{}
	switch name {
	case "review_book":
		chapters, err := st.ListChapters(ctx, "active")
		if err != nil {
			return nil, err
		}
		for _, ch := range chapters {
			pages, err := prod.ListSections(ctx, ch.UUID)
			if err != nil {
				return nil, err
			}
			for _, page := range pages {
				items = append(items, pageReview(page, base))
			}
		}
	case "review_settings":
		settings, err := prod.ListPremiseAssets(ctx, "", "active")
		if err != nil {
			return nil, err
		}
		for _, setting := range settings {
			if a.SettingUUID == "" || setting.UUID == a.SettingUUID {
				items = append(items, settingReview(setting, base))
			}
		}
		if a.SettingUUID != "" && len(items) == 0 {
			_, err = prod.GetPremiseAsset(ctx, a.SettingUUID)
			return nil, err
		}
	case "review_page", "review_candidates":
		if name == "review_page" || a.SlotKind == "page" {
			if a.PageUUID == "" || a.ChapterUUID == "" {
				return nil, reviewArguments("需要 chapter_uuid 和 page_uuid。")
			}
			page, err := prod.GetSection(ctx, a.ChapterUUID, a.PageUUID)
			if err != nil {
				return nil, err
			}
			item := pageReview(page, base)
			variants, err := prod.ListImageVariants(ctx, a.ChapterUUID, a.PageUUID)
			if err != nil {
				return nil, err
			}
			// Version numbers are immutable, so candidate numbers survive new imports.
			for _, v := range variants {
				item.Variants = append(item.Variants, reviewVariant{UUID: v.UUID, AssetUUID: v.Asset.UUID, Number: v.VersionNo, Title: fmt.Sprintf("版本 %d", v.VersionNo), Text: page.DescriptionMD})
			}
			items = append(items, item)
			data["submit"] = map[string]any{"tool": "pages", "action": "select_image", "chapter_uuid": a.ChapterUUID, "page_uuid": a.PageUUID, "expected_revision": page.Revision}
		} else if a.SlotKind == "setting" {
			if a.SettingUUID == "" {
				return nil, reviewArguments("需要 setting_uuid。")
			}
			setting, err := prod.GetPremiseAsset(ctx, a.SettingUUID)
			if err != nil {
				return nil, err
			}
			item := settingReview(setting, base)
			variants, err := prod.ListAssetVariants(ctx, a.SettingUUID)
			if err != nil {
				return nil, err
			}
			for _, v := range variants {
				item.Variants = append(item.Variants, reviewVariant{UUID: v.UUID, AssetUUID: v.Asset.UUID, Number: v.VersionNo, Title: fmt.Sprintf("版本 %d", v.VersionNo), Text: setting.Summary})
			}
			items = append(items, item)
			data["submit"] = map[string]any{"tool": "settings", "action": "select_image", "setting_uuid": a.SettingUUID, "expected_revision": setting.Revision}
		} else {
			if len(a.Candidates) < 2 {
				return nil, reviewArguments("text 类型需要 2–12 个 candidates。")
			}
			item := reviewItem{UUID: store.ProjectUUID(), Revision: p.Revision, Title: "文本候选", Kind: "text", OpenURL: open}
			for i, c := range a.Candidates {
				item.Variants = append(item.Variants, reviewVariant{Number: i + 1, Title: c.Title, Text: c.Text})
			}
			items = append(items, item)
		}
		data["target_uuid"], data["revision"] = items[0].UUID, items[0].Revision
		data["open_url"] = items[0].OpenURL
	}
	if name == "review_book" || name == "review_settings" {
		paged := paginate(items, a)
		data["items"] = paged["items"]
		data["cursor_pagination"] = paged["cursor_pagination"]
	} else {
		data["items"] = items
	}
	return data, nil
}

func reviewArguments(details string) error {
	return httpapi.NewError(400, "invalid_arguments", "审阅目标不完整", details, nil)
}
func aReviewArgs(name string, a arguments) map[string]any {
	args := map[string]any{}
	if a.After != "" {
		args["after"] = a.After
	}
	if a.Before != "" {
		args["before"] = a.Before
	}
	if a.Limit != 0 {
		args["limit"] = a.Limit
	}
	if a.ChapterUUID != "" {
		args["chapter_uuid"] = a.ChapterUUID
	}
	if a.PageUUID != "" {
		args["page_uuid"] = a.PageUUID
	}
	if a.SettingUUID != "" {
		args["setting_uuid"] = a.SettingUUID
	}
	if name == "review_candidates" {
		args["kind"] = a.SlotKind
		if len(a.Candidates) > 0 {
			args["candidates"] = a.Candidates
		}
	}
	return args
}
