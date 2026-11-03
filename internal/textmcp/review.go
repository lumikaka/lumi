package textmcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"

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
	UUID         string          `json:"uuid"`
	Revision     int64           `json:"revision"`
	Title        string          `json:"title"`
	Text         string          `json:"text"`
	Scene        string          `json:"scene"`
	Kind         string          `json:"kind"`
	ChapterUUID  string          `json:"chapter_uuid,omitempty"`
	ChapterTitle string          `json:"chapter_title,omitempty"`
	PageNumber   int             `json:"page_number,omitempty"`
	AssetUUID    string          `json:"asset_uuid,omitempty"`
	OpenURL      string          `json:"open_url"`
	Variants     []reviewVariant `json:"variants,omitempty"`
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

func (s *Service) reviewLegacy(ctx context.Context, store *project.Store, st *story.Service, prod *production.Service, name string, a arguments) (any, error) {
	p, err := st.GetProject(ctx)
	if err != nil {
		return nil, err
	}
	base := s.Endpoint + "/projects/" + store.ProjectUUID()
	// Keep a usable command even in protocol/service tests without a web origin.
	open := base + "/chapters"
	data := map[string]any{"contract_version": 2, "view": name[len("review_"):], "project_uuid": store.ProjectUUID(), "target_uuid": store.ProjectUUID(), "revision": p.Revision, "title": p.Name, "open_url": open, "open_command": "lumi open", "request": map[string]any{"name": name, "arguments": aReviewArgs(name, a)}, "selection_notice": "选择只保留在当前界面。请在对话中回复候选编号，由模型重读后提交。"}
	items := []reviewItem{}
	switch name {
	case "review_book", "review_chapters", "review_chapter":
		if name == "review_book" && a.Mode == "overview" && a.ChapterUUID != "" {
			return nil, reviewArguments("作品总览不能指定章节；请使用 review_chapter 或 mode=preview。")
		}
		reader := name == "review_book" && (a.Mode == "preview" || a.ChapterUUID != "")
		chapters, err := st.ListChapters(ctx, "active")
		if err != nil {
			return nil, err
		}
		summaries := []reviewChapter{}
		pageCount, imageCount := 0, 0
		found := a.ChapterUUID == ""
		for _, ch := range chapters {
			if a.ChapterUUID != "" && ch.UUID != a.ChapterUUID {
				continue
			}
			found = true
			pages, err := prod.ListSections(ctx, ch.UUID)
			if err != nil {
				return nil, err
			}
			summary := chapterReview(ch, pages, base)
			summaries = append(summaries, summary)
			pageCount += summary.PageCount
			imageCount += summary.ImageCount
			if name == "review_chapter" || reader {
				bodyNumber := 0
				for _, page := range pages {
					item := pageReview(page, base)
					item.ChapterTitle = ch.Title
					if page.PageRole != "front_cover" && page.PageRole != "back_cover" {
						bodyNumber++
						item.PageNumber = bodyNumber
					}
					items = append(items, item)
				}
			}
			if name == "review_chapter" {
				summary.Story = ""
				if ch.CurrentStory != nil {
					summary.Story = ch.CurrentStory.Content
				}
				data["chapter"] = summary
				data["target_uuid"], data["revision"], data["title"], data["open_url"] = ch.UUID, ch.Revision, ch.Title, summary.OpenURL
			}
		}
		if !found {
			return nil, reviewArguments("指定章节不在当前项目的有效章节中。")
		}
		switch {
		case reader:
			data["view"], data["scope"] = "reader", "book"
			data["selection_notice"] = "只读预览当前采用的页面图片；本地翻页和版本预览不修改采用结果。"
			if a.ChapterUUID != "" {
				data["scope"], data["title"], data["open_url"] = "chapter", summaries[0].Title, summaries[0].OpenURL+"/preview"
			} else {
				data["open_url"] = base + "/preview"
			}
			if a.Mode == "" {
				data["compatibility"] = "legacy_chapter_preview"
			}
			if err := reviewPageSlice(data, "items", items, a); err != nil {
				return nil, err
			}
			return data, nil
		case name == "review_chapters":
			data["open_url"] = base + "/chapters"
			if err := reviewPageSlice(data, "chapters", summaries, a); err != nil {
				return nil, err
			}
			data["items"] = items
			return data, nil
		case name == "review_chapter":
			if err := reviewPageSlice(data, "items", items, a); err != nil {
				return nil, err
			}
			return data, nil
		default:
			data["open_url"] = base + "/"
			setup, err := store.ProjectSetup(ctx)
			if err != nil {
				return nil, err
			}
			description := p.Description
			if description == "" {
				description = setup.DraftValues.GenerationBrief
			}
			if description == "" {
				description = setup.OriginalInput
			}
			data["book"] = map[string]any{"description": description, "picture_book": p.PictureBook, "generation_language": p.GenerationLanguage, "setup_status": p.SetupStatus, "chapter_count": len(chapters), "page_count": pageCount, "image_count": imageCount}
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
			return nil, reviewArguments("指定设定不在当前项目的有效设定中。")
		}
	case "review_page", "review_candidates":
		if name == "review_page" || a.SlotKind == "page" {
			if a.PageUUID == "" || a.ChapterUUID == "" {
				return nil, reviewArguments("需要 chapter_uuid 和 page_uuid。")
			}
			ch, err := st.GetChapter(ctx, a.ChapterUUID)
			if err != nil {
				return nil, err
			}
			pages, err := prod.ListSections(ctx, ch.UUID)
			if err != nil {
				return nil, err
			}
			data["chapter"] = chapterReview(ch, pages, base)
			position := -1
			for i, sibling := range pages {
				if sibling.UUID == a.PageUUID {
					position = i
					break
				}
			}
			if position < 0 {
				return nil, reviewArguments("页面不在当前章节的有效页面中。")
			}
			data["position"], data["total"] = position+1, len(pages)
			if position > 0 {
				data["previous_page_uuid"] = pages[position-1].UUID
			}
			if position+1 < len(pages) {
				data["next_page_uuid"] = pages[position+1].UUID
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
			if setting.DeletedAt != nil {
				return nil, reviewArguments("指定设定已删除。")
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
		data["title"] = items[0].Title
	}
	if name == "review_settings" {
		data["open_url"] = base + "/premise"
		if a.SettingUUID != "" {
			data["open_url"] = items[0].OpenURL
			data["title"] = items[0].Title
		}
		if err := reviewPageSlice(data, "items", items, a); err != nil {
			return nil, err
		}
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
	if a.Mode != "" {
		args["mode"] = a.Mode
	}
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

// Chapter summaries and page items have deliberately distinct contracts.
type reviewChapter struct {
	UUID        string `json:"uuid"`
	Revision    int64  `json:"revision"`
	ChapterCode string `json:"chapter_code"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Story       string `json:"story,omitempty"`
	PageCount   int    `json:"page_count"`
	ImageCount  int    `json:"image_count"`
	OpenURL     string `json:"open_url"`
}

func chapterReview(ch story.Chapter, pages []production.ComicSection, base string) reviewChapter {
	result := reviewChapter{UUID: ch.UUID, Revision: ch.Revision, ChapterCode: ch.ChapterCode, Title: ch.Title, PageCount: len(pages), OpenURL: base + "/chapters/" + ch.UUID}
	if ch.CurrentStory != nil {
		excerpt := []rune(ch.CurrentStory.Content)
		if len(excerpt) > 180 {
			excerpt = append(excerpt[:180], '…')
		}
		result.Summary = string(excerpt)
	}
	for _, page := range pages {
		if page.CurrentImage != nil {
			result.ImageCount++
		}
	}
	return result
}
func reviewPageSlice[T any](data map[string]any, field string, items []T, a arguments) error {
	for _, cursor := range []string{a.After, a.Before} {
		if cursor == "" {
			continue
		}
		offset, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil || offset > uint64(len(items)) {
			return reviewArguments("分页游标超出当前审阅范围，请从首组重新读取。")
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return err
	}
	data["snapshot"] = fmt.Sprintf("%x", sha256.Sum256(encoded))
	paged := paginate(items, a)
	data[field], data["cursor_pagination"], data["total"] = paged["items"], paged["cursor_pagination"], len(items)
	return nil
}
