package textmcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"lumi/internal/httpapi"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

// Old book preview calls keep their resource scope, but launch normal pages.
func normalReviewTarget(name string, a arguments) (string, arguments) {
	if name == "review_book" {
		if a.ChapterUUID != "" {
			name = "review_chapter"
		}
		a.Mode = ""
	}
	return name, a
}

// Review tools launch a workspace. Details belong to the UI resource channel,
// and explicit authoring reads remain available when the model needs prose.
func (s *Service) review(ctx context.Context, store *project.Store, st *story.Service, prod *production.Service, name string, a arguments) (any, error) {
	name, a = normalReviewTarget(name, a)
	p, err := st.GetProject(ctx)
	if err != nil {
		return nil, err
	}
	base := s.Endpoint + "/projects/" + store.ProjectUUID()
	view := strings.TrimPrefix(name, "review_")
	args := aReviewArgs(name, a)
	data := map[string]any{"contract_version": 3, "view": view, "project_uuid": store.ProjectUUID(), "target_uuid": store.ProjectUUID(), "revision": p.Revision, "title": p.Name, "summary": reviewExcerpt(p.Description, 300), "open_url": base + "/", "open_command": "lumi open", "request": map[string]any{"name": name, "arguments": args}, "permissions": map[string]any{"can_write": !project.IsReadOnly(ctx)}, "data_source": "app_resource", "state": map[string]any{"setup_status": p.SetupStatus, "chapter_count": p.ChapterCount}}
	pageTarget := name == "review_page" || (name == "review_candidates" && a.SlotKind == "page")
	chapterTarget := name == "review_chapter" || pageTarget
	if chapterTarget {
		ch, err := st.GetChapter(ctx, a.ChapterUUID)
		if err != nil {
			return nil, err
		}
		if ch.TrashedAt != nil {
			return nil, reviewArguments("指定章节已删除。")
		}
		data["chapter_uuid"], data["title"], data["revision"], data["target_uuid"], data["open_url"] = ch.UUID, ch.Title, ch.Revision, ch.UUID, base+"/chapters/"+ch.UUID
		if ch.CurrentStory != nil {
			data["summary"] = reviewExcerpt(ch.CurrentStory.Content, 180)
		}
	}
	switch {
	case name == "review_chapters":
		data["open_url"] = base + "/chapters"
	case pageTarget:
		page, err := prod.GetSection(ctx, a.ChapterUUID, a.PageUUID)
		if err != nil {
			return nil, err
		}
		if page.DeletedAt != nil {
			return nil, reviewArguments("指定页面已删除。")
		}
		data["state"] = map[string]any{"has_image": page.CurrentImage != nil, "has_text": page.DescriptionMD != "", "has_storyboard": page.HasStoryboard}
		data["page_uuid"], data["target_uuid"], data["revision"], data["title"], data["summary"], data["open_url"] = page.UUID, page.UUID, page.Revision, page.Title, reviewExcerpt(page.DescriptionMD, 180), base+"/chapters/"+a.ChapterUUID+"/sections/"+page.UUID
	case name == "review_settings" || (name == "review_candidates" && a.SlotKind == "setting"):
		data["open_url"] = base + "/premise"
		if a.SettingUUID != "" {
			setting, err := prod.GetPremiseAsset(ctx, a.SettingUUID)
			if err != nil {
				return nil, err
			}
			if setting.DeletedAt != nil {
				return nil, reviewArguments("指定设定已删除。")
			}
			data["target_uuid"], data["revision"], data["title"], data["summary"], data["open_url"] = setting.UUID, setting.Revision, setting.Title, reviewExcerpt(setting.Summary, 180), base+"/premise/assets/"+setting.UUID
		}
	case name == "review_candidates" && a.SlotKind == "text":
		if len(a.Candidates) < 2 {
			return nil, reviewArguments("文本候选需要 2–12 项。")
		}
		delete(args, "candidates")
		data["candidate_count"] = len(a.Candidates)
		// Transport-only field is removed by reviewLaunchToolResult before publish.
		data["_ui_arguments"] = aReviewArgs(name, a)
	}
	return data, nil
}
func reviewExcerpt(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

func reviewLaunchToolResult(result httpapi.Envelope) (*mcp.CallToolResult, error) {
	if !result.Success {
		return boundedResult(result)
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err = json.Unmarshal(encoded, &data); err != nil {
		return nil, err
	}
	ui := data["_ui_arguments"]
	delete(data, "_ui_arguments")
	result.Data = data
	output, err := boundedResult(result)
	if err == nil && ui != nil {
		output.Meta = mcp.Meta{"lumi/launch_arguments": ui}
	}
	return output, err
}
