package textmcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"lumi/internal/files"
	"lumi/internal/httpapi"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/story"
)

type Service struct {
	Projects      *project.Manager
	Events        *realtime.Hub
	Endpoint      string
	AppConnection func(string) any
	AppRequest    func(context.Context, string, AppResourceRequest) (any, error)
}

type arguments struct {
	ReviewView              string                       `json:"view"`
	Resource                string                       `json:"resource"`
	Method                  string                       `json:"method"`
	Query                   map[string]string            `json:"query"`
	Body                    json.RawMessage              `json:"body"`
	BoardUUID               string                       `json:"board_uuid"`
	SourceText              string                       `json:"source_text"`
	StyleSnapshot           string                       `json:"style_snapshot"`
	IdempotencyKey          string                       `json:"idempotency_key"`
	ExpectedPremiseRevision int64                        `json:"expected_premise_revision"`
	Crops                   []production.HostSettingCrop `json:"assets"`

	Action           string                    `json:"action"`
	Name             string                    `json:"name"`
	Idea             string                    `json:"idea"`
	Language         *string                   `json:"generation_language"`
	ProjectName      *string                   `json:"project_name"`
	Style            *string                   `json:"overall_style"`
	Brief            *string                   `json:"generation_brief"`
	PictureBook      *project.PictureBookInput `json:"picture_book"`
	ExpectedRevision *int64                    `json:"expected_revision"`
	StoryMD          string                    `json:"story_md"`
	ChapterUUID      string                    `json:"chapter_uuid"`
	ChapterCode      string                    `json:"chapter_code"`
	Title            *string                   `json:"title"`
	Content          string                    `json:"content"`
	ContentFormat    string                    `json:"content_format"`
	SettingUUID      string                    `json:"setting_uuid"`
	AssetType        *string                   `json:"asset_type"`
	Summary          *string                   `json:"summary"`
	Tags             *[]string                 `json:"tags"`
	PageUUID         string                    `json:"page_uuid"`
	Description      *string                   `json:"description_md"`
	Storyboard       *string                   `json:"storyboard_md"`
	PageRole         *string                   `json:"page_role"`
	After            string                    `json:"after"`
	Before           string                    `json:"before"`
	Limit            int                       `json:"limit"`
	IncludeVersions  bool                      `json:"include_versions"`
	PromptGroup      string                    `json:"prompt_group"`
	PromptKey        string                    `json:"prompt_key"`
	Target           string                    `json:"target"`
	Variables        map[string]string         `json:"variables"`
	ContextChapters  *[]string                 `json:"context_chapter_uuids"`
	ContextSettings  *[]string                 `json:"context_setting_uuids"`
	ImageSettings    *[]string                 `json:"setting_uuids"`
	Format           string                    `json:"format"`
	AllowMissing     bool                      `json:"allow_missing_images"`
	Path             string                    `json:"path"`
	TargetUUID       string                    `json:"target_uuid"`
	Slot             string                    `json:"slot"`
	FitMode          string                    `json:"fit_mode"`
	ImageDescription string                    `json:"description"`
	ImagePrompt      string                    `json:"prompt"`
	AssetUUID        string                    `json:"asset_uuid"`
	MaxEdge          int                       `json:"max_edge"`
	VariantUUID      string                    `json:"variant_uuid"`
	SlotKind         string                    `json:"kind"`
	Mode             string                    `json:"mode"`
	Candidates       []reviewCandidate         `json:"candidates"`
}

func value[T any](v *T) (zero T) {
	if v != nil {
		return *v
	}
	return
}

// Call is invoked exclusively in the CLI instance. root is supplied by the
// stdio process, never exposed as a tool argument. Every operation acquires a
// manager lease; no separate SQLite handle or legacy MCP grant is used.
func (s *Service) Call(ctx context.Context, root, name string, raw json.RawMessage) httpapi.Envelope {
	inputLimit := MaxBytes
	if name == "write_app_resource" {
		inputLimit = MaxAppInputBytes
	}
	if len(raw) > inputLimit {
		return Failure("arguments_too_large", "工具输入过大", "请缩小单次写入范围（最多 256 KiB）。")
	}
	if err := validate(name, raw); err != nil {
		return Failure("invalid_arguments", "工具参数无效", err.Error())
	}
	var a arguments
	if err := json.Unmarshal(raw, &a); err != nil {
		return Failure("invalid_arguments", "工具参数无效", err.Error())
	}
	if a.After != "" && a.Before != "" {
		return Failure("invalid_arguments", "游标冲突", "before 和 after 不能同时提供。")
	}
	if project.IsReadOnly(ctx) && !readOnlyOperation(name, a.Action) {
		return Failure("read_only_context", "只读任务不能执行此操作", "只允许查看当前已打开项目；初始化、定稿、图片选择和导出均需写权限。")
	}
	if !filepath.IsAbs(root) {
		return Failure("invalid_arguments", "启动目录必须是绝对路径", "重新启动 lumi mcp。")
	}
	expectedUUID := ExpectedProjectUUID(ctx)
	if expectedUUID != "" {
		if !publicProjectUUID.MatchString(expectedUUID) {
			return Failure("invalid_arguments", "项目绑定无效", "重新读取当前任务项目。")
		}
	}
	if expectedUUID != "" && !project.IsReadOnly(ctx) {
		// Check before initialization or opening can migrate/write anything.
		info, err := project.InspectDirectory(ctx, root)
		if err != nil || info.UUID != expectedUUID || info.RootPath != filepath.Clean(root) {
			return Failure("project_identity_mismatch", "项目目录与绑定身份不一致", "释放当前任务连接，再从 Cindy 当前目录重新读取；未执行创作操作。")
		}
	}
	var opened project.Summary
	var err error
	if name == "project_init" {
		lang := value(a.Language)
		if lang == "" {
			lang = project.DefaultGenerationLanguage
		}
		if strings.TrimSpace(a.Idea) == "" {
			return Failure("invalid_arguments", "idea 不能为空", "请提供用户原始创意。")
		}
		_, err = s.Projects.InitTextAt(ctx, root, project.CreateInput{Name: a.Name, GenerationLanguage: lang}, a.Idea)
	} else if _, statErr := os.Stat(filepath.Join(root, "project.sqlite")); errors.Is(statErr, os.ErrNotExist) {
		return Failure("project_not_initialized", "当前目录尚未初始化为 Lumi 项目", "先调用 project_init；初始化后同一 MCP 进程可继续使用，无需重启。")
	}
	if err == nil && project.IsReadOnly(ctx) {
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return publicError(err)
		}
		var items []project.Summary
		items, err = s.Projects.OpenProjects(ctx)
		for _, item := range items {
			if item.RootPath == filepath.Clean(root) && (expectedUUID == "" || item.UUID == expectedUUID) {
				opened = item
				break
			}
		}
		if err == nil && opened.UUID == "" {
			return Failure("read_only_project_not_open", "只读任务只能查看 CLI 已打开的项目", "先在可写任务打开相同项目；只读调用不会启动项目、迁移或恢复投影。")
		}
	} else if err == nil {
		opened, err = s.Projects.OpenSelected(ctx, project.ExplicitExistingDirectory(root))
	}
	if err != nil {
		var pe *project.Error
		if errors.As(err, &pe) && pe.Code == project.CodeInvalidProject {
			return Failure(pe.Code, pe.Message, pe.Details+" 可调用 project_init 检查初始化；已有受管路径冲突时需先处理，工具不会覆盖。")
		}
		return publicError(err)
	}
	var result any
	withStore := s.Projects.WithStore
	if project.IsReadOnly(ctx) {
		withStore = func(ctx context.Context, uuid string, callback func(*project.Store) error) error {
			return s.Projects.WithStore(ctx, uuid, func(store *project.Store) error { return store.ReadOnly(ctx, callback) })
		}
	}
	err = withStore(ctx, opened.UUID, func(store *project.Store) error {
		// UUID alone is insufficient: a copied project can have the same UUID.
		// Recheck the leased store so a concurrent close/reopen cannot redirect
		// the operation between OpenSelected and WithStore.
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil || store.Root() != canonical || (expectedUUID != "" && store.ProjectUUID() != expectedUUID) {
			return httpapi.NewError(409, "project_identity_mismatch", "项目目录与绑定身份不一致", "释放当前任务连接后重新读取项目。", nil)
		}
		if err := store.RequireDirectoryIdentity(); err != nil {
			return err
		}
		st := story.NewService(store).WithEvents(s.Events)
		prod := production.NewService(store, s.Events)
		if isMutation(name, a.Action) && name != "project_setup" && name != "project_init" {
			if err := store.RequireReady(); err != nil {
				return err
			}
			if err := store.RequireInitializationAvailable(ctx); err != nil {
				return err
			}
		}
		var callErr error
		switch name {
		case "read_app_review":
			reviewName := "review_" + a.ReviewView
			if a.ReviewView == "reader" {
				reviewName = "review_book"
				a.Mode = "preview"
			}
			result, callErr = s.reviewLegacy(ctx, store, st, prod, reviewName, a)
		case "read_app_resource", "write_app_resource":
			result, callErr = s.appResource(ctx, store.ProjectUUID(), name, a)
		case "project_init", "project_overview":
			result, callErr = overview(ctx, store, st, prod)
		case "project_setup":
			result, callErr = s.setup(ctx, store, st, a)
		case "story_profile":
			if a.Action == "get" {
				result, callErr = st.GetStoryProfile(ctx)
			} else {
				result, callErr = st.UpdateStoryProfile(ctx, a.StoryMD, value(a.ExpectedRevision))
			}
		case "chapters":
			result, callErr = chapters(ctx, st, a)
		case "settings":
			result, callErr = settings(ctx, prod, a)
		case "pages":
			result, callErr = pages(ctx, prod, a)
		case "book_outline":
			result, callErr = outline(ctx, st, prod, a)
		case "prompt_catalog":
			var items []story.PromptCatalogItem
			items, callErr = st.ListPromptCatalog(ctx, a.PromptGroup)
			metadata := []map[string]any{}
			for _, item := range items {
				metadata = append(metadata, map[string]any{"prompt_group": item.PromptGroup, "prompt_key": item.PromptKey, "title": item.Title, "description": item.Description, "prompt_type": item.PromptType, "placeholders": item.Placeholders, "is_custom": item.IsCustom})
			}
			result = paginate(metadata, a)
		case "render_prompt":
			result, callErr = render(ctx, store, st, prod, a)
		case "setting_boards":
			switch a.Action {
			case "list":
				var items []production.HostSettingBoardSummary
				items, callErr = prod.ListHostSettingBoards(ctx)
				result = paginate(items, a)
			case "get":
				result, callErr = prod.GetHostSettingBoard(ctx, a.BoardUUID)
			case "import":
				result, callErr = prod.ImportHostSettingBoard(ctx, production.ImportHostSettingBoardInput{Path: a.Path, SourceText: a.SourceText, StyleSnapshot: a.StyleSnapshot, Prompt: a.ImagePrompt, IdempotencyKey: a.IdempotencyKey, ExpectedRevision: value(a.ExpectedRevision)})
			}
		case "apply_setting_breakdown":
			result, callErr = prod.ApplyHostSettingBreakdown(ctx, production.ApplyHostSettingBreakdownInput{BoardUUID: a.BoardUUID, IdempotencyKey: a.IdempotencyKey, ExpectedRevision: value(a.ExpectedRevision), ExpectedPremiseRevision: a.ExpectedPremiseRevision, Assets: a.Crops})

		case "image_target":
			result, callErr = imageTarget(ctx, prod, a)
		case "import_image":
			result, callErr = prod.ImportHostImage(ctx, production.ImportHostImageInput{Path: a.Path, TargetUUID: a.TargetUUID, Slot: a.Slot, FitMode: a.FitMode, Description: a.ImageDescription, Prompt: a.ImagePrompt, ExpectedRevision: value(a.ExpectedRevision)})
		case "read_image", "read_app_image":
			result, callErr = readImage(ctx, prod.Files(), a.AssetUUID, a.MaxEdge)
		case "read_app_connection":
			if s.AppConnection == nil {
				return httpapi.NewError(503, "app_connection_unavailable", "实时连接不可用", "在 Lumi 中打开以查看最新状态。", nil)
			}
			result = s.AppConnection(store.ProjectUUID())
		case "watch_app_changes":
			return httpapi.NewError(400, "app_watch_transport_required", "实时提示需要 MCP 会话", "请通过 MCP Apps 的 watch_app_changes 调用。", nil)
		case "review_book", "review_chapters", "review_chapter", "review_page", "review_candidates", "review_settings":
			result, callErr = s.review(ctx, store, st, prod, name, a)
		case "export_book":
			var book production.TextBook
			book, callErr = readBook(ctx, store, st, prod)
			if callErr == nil {
				result, callErr = prod.ExportTextBook(ctx, book, a.Format, a.AllowMissing)
			}
		}
		return callErr
	})
	if err != nil {
		return publicError(err)
	}
	if isMutation(name, a.Action) && name != "write_app_resource" {
		result = mutationSummary(result)
	}
	b, err := json.Marshal(result)
	if err != nil {
		return publicError(err)
	}
	resultLimit := MaxBytes
	if name == "read_app_resource" || name == "write_app_resource" || name == "read_app_review" {
		resultLimit = MaxAppBytes
	}
	if len(b) > resultLimit-1024 {
		return Failure("result_too_large", "工具结果超过 256 KiB", "减小 limit、按单个 UUID 读取，或用 context_chapter_uuids / context_setting_uuids 缩小提示词上下文；大文本可用 export_book 导出。")
	}
	return httpapi.Envelope{Success: true, Data: result}
}

func isMutation(name, action string) bool {
	return name == "write_app_resource" || name == "project_init" || name == "import_image" || name == "apply_setting_breakdown" || (name == "setting_boards" && action == "import") || ((name == "project_setup" || name == "story_profile" || name == "chapters" || name == "settings" || name == "pages") && action != "get" && action != "list")
}

func mutationSummary(result any) any {
	b, _ := json.Marshal(result)
	var data any
	_ = json.Unmarshal(b, &data)
	var trim func(any)
	trim = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				if k == "content" || k == "story_md" || k == "content_md" || k == "description_md" || k == "summary" || k == "original_input" {
					delete(x, k)
				} else {
					trim(child)
				}
			}
		case []any:
			for _, child := range x {
				trim(child)
			}
		}
	}
	trim(data)
	return data
}

func publicError(err error) httpapi.Envelope {
	var pe *project.Error
	var se *story.Error
	var de *production.Error
	var ae *httpapi.APIError
	var fe *files.Error
	switch {
	case errors.As(err, &pe):
		if pe.Code == project.CodeProjectSetupIncomplete {
			return Failure(pe.Code, pe.Message, pe.Details+" 可通过 project_setup update/finalize 完成设置，无需服务商。")
		}
		return Failure(pe.Code, pe.Message, pe.Details)
	case errors.As(err, &se):
		return Failure(se.Code, se.Message, se.Details)
	case errors.As(err, &de):
		return Failure(de.Code, de.Message, de.Details)
	case errors.As(err, &fe):
		return Failure(fe.Code, fe.Message, fe.Details)
	case errors.As(err, &ae):
		return Failure(ae.Code, ae.Message, ae.Details)
	default:
		slog.Error("text authoring operation failed", "error", err)
		return Failure("internal_error", "本地创作操作失败", "请查看 CLI 实例日志后重试。")
	}
}

func paginate[T any](items []T, a arguments) map[string]any {
	limit := a.Limit
	if limit == 0 {
		limit = 20
	}
	offset, _ := strconv.Atoi(a.After)
	if a.Before != "" {
		offset, _ = strconv.Atoi(a.Before)
		offset = max(0, offset-limit)
	}
	start := min(offset, len(items))
	end := min(start+limit, len(items))
	return map[string]any{"items": items[start:end], "cursor_pagination": map[string]any{"per_page": limit, "next_cursor": strconv.Itoa(end), "prev_cursor": strconv.Itoa(start), "has_more": end < len(items)}}
}

func overview(ctx context.Context, store *project.Store, st *story.Service, prod *production.Service) (any, error) {
	p, err := st.GetProject(ctx)
	if err != nil {
		return nil, err
	}
	setup, err := store.ProjectSetup(ctx)
	if err != nil {
		return nil, err
	}
	var settingCount, pageCount int64
	if err := store.DB().WithContext(ctx).Table("premise_assets").Where("deleted_at IS NULL").Count(&settingCount).Error; err != nil {
		return nil, err
	}
	if err := store.DB().WithContext(ctx).Table("comic_sections s").Joins("JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id JOIN chapters c ON c.id=cs.chapter_id").Where("s.deleted_at IS NULL AND c.deleted_at IS NULL").Count(&pageCount).Error; err != nil {
		return nil, err
	}
	return map[string]any{"project": p, "setup": setup, "setting_count": settingCount, "page_count": pageCount}, nil
}

func (s *Service) setup(ctx context.Context, store *project.Store, st *story.Service, a arguments) (any, error) {
	if a.Action == "get" {
		return store.ProjectSetup(ctx)
	}
	var state project.SetupState
	var err error
	if a.Action == "update" {
		if a.ProjectName == nil && a.Language == nil && a.Style == nil && a.Brief == nil && a.PictureBook == nil {
			return nil, httpapi.NewError(400, "invalid_arguments", "没有设置变更", "至少提供一个 setup 字段。", nil)
		}
		scope := project.GenerationScopeProjectOnly
		state, err = store.UpdateProjectSetupDraft(ctx, project.SetupDraftPatchInput{ExpectedRevision: value(a.ExpectedRevision), ProjectName: a.ProjectName, GenerationLanguage: a.Language, OverallStyle: a.Style, GenerationBrief: a.Brief, GenerationScope: &scope, PictureBook: a.PictureBook})
	} else {
		state, err = store.FinalizeProjectSetup(ctx, value(a.ExpectedRevision))
		if err == nil {
			err = st.EnsurePromptCatalogVersions(ctx, "project_created")
		}
		if err == nil {
			err = s.Projects.SyncProjectName(ctx, store.ProjectUUID())
		}
	}
	if err == nil {
		s.Events.Broadcast(realtime.ProjectTopic(store.ProjectUUID()), "project:setup_changed", map[string]any{"project_uuid": store.ProjectUUID(), "setup_uuid": state.UUID, "setup_status": state.SetupStatus, "status": state.Status, "revision": state.Revision})
	}
	return state, err
}

func chapters(ctx context.Context, st *story.Service, a arguments) (any, error) {
	switch a.Action {
	case "list":
		items, err := st.ListChapters(ctx, "active")
		return paginate(items, a), err
	case "get":
		return st.GetChapter(ctx, a.ChapterUUID)
	case "create":
		format := a.ContentFormat
		if format == "" {
			format = "txt"
		}
		return st.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: a.ChapterCode, Title: value(a.Title), Content: a.Content, ContentFormat: format, ExpectedProjectRevision: a.ExpectedRevision})
	case "update":
		return st.UpdateChapter(ctx, a.ChapterUUID, story.UpdateChapterInput{Title: value(a.Title), ExpectedRevision: value(a.ExpectedRevision)})
	case "write":
		format := a.ContentFormat
		if format == "" {
			format = "txt"
		}
		return st.UpdateStory(ctx, a.ChapterUUID, story.UpdateStoryInput{Content: a.Content, ContentFormat: format, ExpectedRevision: value(a.ExpectedRevision)})
	default:
		return st.TrashChapter(ctx, a.ChapterUUID, value(a.ExpectedRevision))
	}
}

func settings(ctx context.Context, prod *production.Service, a arguments) (any, error) {
	switch a.Action {
	case "list":
		items, err := prod.ListPremiseAssets(ctx, "", "active")
		if err != nil {
			return nil, err
		}
		p, err := prod.GetPremise(ctx)
		result := paginate(items, a)
		result["premise"] = map[string]any{"uuid": p.UUID, "revision": p.Revision, "default_style": p.DefaultStyle}
		return result, err
	case "get":
		item, err := prod.GetPremiseAsset(ctx, a.SettingUUID)
		if err != nil || !a.IncludeVersions {
			return item, err
		}
		versions, err := prod.ListAssetVariants(ctx, a.SettingUUID)
		if err != nil {
			return nil, err
		}
		identities := []imageVersionIdentity{}
		for _, v := range versions {
			identities = append(identities, imageVersionIdentity{UUID: v.UUID, VersionNo: v.VersionNo, AssetUUID: v.Asset.UUID, IsCurrent: item.CurrentVariant != nil && item.CurrentVariant.UUID == v.UUID})
		}
		listing := map[string]any{}
		if err := reviewPageSlice(listing, "items", identities, a); err != nil {
			return nil, err
		}
		return struct {
			production.PremiseAsset
			ImageVersions map[string]any `json:"image_versions"`
		}{item, listing}, nil
	case "select_image":
		return prod.SelectAssetVariant(ctx, a.SettingUUID, a.VariantUUID, value(a.ExpectedRevision))
	case "create":
		return prod.CreateTextPremiseAsset(ctx, production.CreateAssetInput{AssetType: value(a.AssetType), Title: value(a.Title), Summary: value(a.Summary), Tags: value(a.Tags)}, value(a.ExpectedRevision))
	case "update":
		if a.AssetType == nil && a.Title == nil && a.Summary == nil && a.Tags == nil {
			return nil, httpapi.NewError(400, "invalid_arguments", "没有设定变更", "至少提供一个设定字段。", nil)
		}
		return prod.UpdatePremiseAsset(ctx, a.SettingUUID, production.UpdateAssetInput{AssetType: a.AssetType, Title: a.Title, Summary: a.Summary, Tags: a.Tags, ExpectedRevision: value(a.ExpectedRevision)})
	default:
		return prod.SetPremiseAssetTrashed(ctx, a.SettingUUID, true, value(a.ExpectedRevision))
	}
}

func pages(ctx context.Context, prod *production.Service, a arguments) (any, error) {
	switch a.Action {
	case "list":
		items, err := prod.ListSections(ctx, a.ChapterUUID)
		if err != nil {
			return nil, err
		}
		state, err := prod.GetComicState(ctx, a.ChapterUUID)
		result := paginate(items, a)
		result["state"] = state
		return result, err
	case "get":
		item, err := prod.GetSection(ctx, a.ChapterUUID, a.PageUUID)
		if err != nil || !a.IncludeVersions {
			return item, err
		}
		versions, err := prod.ListImageVariants(ctx, a.ChapterUUID, a.PageUUID)
		if err != nil {
			return nil, err
		}
		identities := []imageVersionIdentity{}
		for _, v := range versions {
			identities = append(identities, imageVersionIdentity{UUID: v.UUID, VersionNo: v.VersionNo, AssetUUID: v.Asset.UUID, IsCurrent: item.CurrentImage != nil && item.CurrentImage.UUID == v.UUID})
		}
		listing := map[string]any{}
		if err := reviewPageSlice(listing, "items", identities, a); err != nil {
			return nil, err
		}
		return struct {
			production.ComicSection
			ImageVersions map[string]any `json:"image_versions"`
		}{item, listing}, nil
	case "select_image":
		return prod.SelectImageVariant(ctx, a.ChapterUUID, a.PageUUID, a.VariantUUID, value(a.ExpectedRevision))
	case "create":
		return prod.CreateSection(ctx, a.ChapterUUID, production.CreateSectionInput{Title: value(a.Title), DescriptionMD: value(a.Description), StoryboardMD: value(a.Storyboard), PageRole: value(a.PageRole), ExpectedStateRevision: a.ExpectedRevision})
	case "update":
		if a.Title == nil && a.Description == nil && a.Storyboard == nil && a.PageRole == nil {
			return nil, httpapi.NewError(400, "invalid_arguments", "没有页面变更", "至少提供一个页面字段。", nil)
		}
		return prod.UpdateSection(ctx, a.ChapterUUID, a.PageUUID, production.UpdateSectionInput{Title: a.Title, DescriptionMD: a.Description, StoryboardMD: a.Storyboard, PageRole: a.PageRole, ExpectedRevision: value(a.ExpectedRevision)})
	default:
		err := prod.DeleteSection(ctx, a.ChapterUUID, a.PageUUID, value(a.ExpectedRevision))
		return map[string]any{"uuid": a.PageUUID, "deleted": err == nil}, err
	}
}

func outline(ctx context.Context, st *story.Service, prod *production.Service, a arguments) (any, error) {
	chapters, err := st.ListChapters(ctx, "active")
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for _, ch := range chapters {
		items = append(items, map[string]any{"kind": "chapter", "uuid": ch.UUID, "chapter_code": ch.ChapterCode, "title": ch.Title, "revision": ch.Revision, "has_text": ch.CurrentStory != nil})
		pages, err := prod.ListSections(ctx, ch.UUID)
		if err != nil {
			return nil, err
		}
		for _, p := range pages {
			items = append(items, map[string]any{"kind": "page", "uuid": p.UUID, "chapter_uuid": ch.UUID, "section_no": p.SectionNo, "page_role": p.PageRole, "title": p.Title, "revision": p.Revision, "has_text": p.DescriptionMD != "", "has_storyboard": p.HasStoryboard, "has_image": p.CurrentImage != nil})
		}
	}
	return paginate(items, a), nil
}

// Explicit model reads can resolve a conversation candidate number without
// relying on UI-only results, defaulting every launch to full history, or
// duplicating image bytes/prose in the version manifest.
type imageVersionIdentity struct {
	UUID      string `json:"uuid"`
	VersionNo int    `json:"version_no"`
	AssetUUID string `json:"asset_uuid"`
	IsCurrent bool   `json:"is_current"`
}
