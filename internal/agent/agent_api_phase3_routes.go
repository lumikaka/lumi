package agent

const (
	RouteProjectGet                      = "project.get"
	RouteProjectUpdate                   = "project.update"
	RouteProjectSetupGet                 = "project_setup.get"
	RouteProjectSetupUpdate              = "project_setup.update"
	RouteProjectSetupFinalize            = "project_setup.finalize"
	RouteYoloWorkflowCreate              = "workflow.yolo.create"
	RouteChapterCreate                   = "chapter.create"
	RouteChapterUpdate                   = "chapter.update"
	RouteChapterStoryList                = "chapter_story.list"
	RouteChapterTrash                    = "chapter.soft_delete"
	RouteChapterRestore                  = "chapter.restore"
	RouteStoryProfileList                = "story_profile.list"
	RouteStoryProfileImport              = "story_profile.import_project_file"
	RouteStoryProfileRegenerate          = "story_profile.regenerate_project_file"
	RouteStoryProfileGenerationCreate    = "generation.story_profile.create"
	RouteStoryProfileRebuildCreate       = "generation.story_profile_from_chapters.create"
	RouteChapterBatchPlanCreate          = "generation.chapter_batch_plan.create"
	RouteComicStoryboardGenerationCreate = "generation.comic_storyboard.create"
	RouteComicImageGenerationBatchCreate = "generation.comic_image_batch.create"
	RoutePremiseUpdate                   = "premise.update"
	RoutePremiseSourceList               = "premise_source.list"
	RoutePremiseSourceCreate             = "premise_source.create"
	RoutePremiseSourceUpdate             = "premise_source.update"
	RouteSettingImageList                = "premise_setting_image.list"
	RouteSettingImageImport              = "premise_setting_image.import"
	RouteSettingImageSelect              = "premise_setting_image.select"
	RoutePremiseAssetRestore             = "premise_asset.restore"
	RoutePremiseAssetVariantList         = "premise_asset_variant.list"
	RoutePremiseAssetVariantCreate       = "premise_asset_variant.create"
	RoutePremiseAssetVariantSelect       = "premise_asset_variant.select"
	RouteComicStateGet                   = "comic.get"
	RouteComicSectionList                = "comic_section.list"
	RouteComicSectionCreate              = "comic_section.create"
	RouteComicSectionUpdate              = "comic_section.update"
	RouteComicSectionReorder             = "comic_section.reorder"
	RouteComicSectionDelete              = "comic_section.delete"
	RouteStoryboardList                  = "storyboard.list"
	RouteStoryboardSelect                = "storyboard.select"
	RouteComicSectionImageImport         = "comic_section_image.import"
	RouteComicImageVariantList           = "comic_image_variant.list"
	RouteComicImageVariantSelect         = "comic_image_variant.select"
	RouteComicSnapshotList               = "comic_snapshot.list"
	RouteComicSnapshotGet                = "comic_snapshot.get"
	RouteComicSnapshotRestore            = "comic_snapshot.restore"
	RouteComicExportReadiness            = "comic_export.readiness"
	RouteComicExportList                 = "comic_export.list"
	RouteComicExportCreate               = "comic_export.create"
	RouteStoryTaskList                   = "task.story.list"
	RouteStoryTaskEventList              = "task.story_event.list"
	RouteStoryTaskCancel                 = "task.story.cancel"
	RouteStoryTaskRetry                  = "task.story.retry"
	RouteProductionTaskList              = "task.production.list"
	RouteProductionTaskEventList         = "task.production_event.list"
	RouteProductionTaskCancel            = "task.production.cancel"
	RouteProductionTaskRetry             = "task.production.retry"
	RouteProjectAssetList                = "project_asset.list"
	RouteProjectAssetGet                 = "project_asset.get"
	RouteProjectAssetUpdate              = "project_asset.update"
	RouteProjectAssetTrash               = "project_asset.soft_delete"
	RouteProjectAssetRestore             = "project_asset.restore"
)

func apiBoolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func apiLimitedString(description string, maxLength int) map[string]any {
	return map[string]any{"type": "string", "description": description, "maxLength": maxLength}
}

func apiBoundedInteger(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
}

func apiEnum(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func projectSetupDraftUpdateBodySchema() map[string]any {
	setupPictureBook := apiObject(map[string]any{
		"format":                   apiEnum("绘本形式。", "classic_picture_book", "wordless_picture_book", "interactive_picture_book", "comic_story", "vertical_strip"),
		"aspect_ratio":             apiObject(map[string]any{"mode": apiEnum("画面比例模式。", "landscape", "square", "portrait", "custom"), "width": apiBoundedInteger("custom 模式的宽。", 1, 100), "height": apiBoundedInteger("custom 模式的高。", 1, 100)}, "mode"),
		"large_image_minimal_text": apiBoolean("经典图文是否采用大图少字。"),
		"interaction_mode":         apiEnum("互动绘本互动形式。", "find_it", "make_a_choice", "guess", "follow_along"),
		"comic_layout":             apiEnum("漫画版式。", "four_panel", "page_comic"),
	}, "format")
	return apiObject(map[string]any{
		"expected_revision":   apiBoundedInteger("刚读取到的最新设置 revision。", 1, 1<<31-1),
		"project_name":        apiLimitedString("初始化草稿中的项目名称。", 120),
		"generation_language": apiEnum("生成语言。", "zh-Hans", "en"),
		"overall_style":       apiLimitedString("初始化草稿中的整体画风。", 12000),
		"picture_book":        setupPictureBook,
	}, "expected_revision")
}

func phase3AgentAPIProjectors() []agentAPIProjector {
	fields := func(names ...string) []agentAPIResponseField {
		result := make([]agentAPIResponseField, 0, len(names))
		for _, name := range names {
			result = append(result, agentAPIResponseField{Name: name, Type: "public", Description: "经审查的公开字段。"})
		}
		return result
	}
	return []agentAPIProjector{
		{Key: "project", Fields: fields("uuid", "name", "description", "generation_language", "revision", "chapter_count", "trash_count", "updated_at"), RecommendedFields: []string{"uuid", "name", "description", "generation_language", "revision", "chapter_count", "trash_count", "updated_at"}},
		{Key: "project_setup", Fields: fields("uuid", "project_uuid", "setup_status", "status", "revision", "original_input", "draft_values", "field_sources", "missing_information", "final_picture_book", "error_code", "error_message", "created_at", "updated_at", "finalized_at"), RecommendedFields: []string{"uuid", "project_uuid", "setup_status", "status", "revision", "draft_values", "field_sources", "missing_information", "final_picture_book", "updated_at"}},
		{Key: "workflow", Fields: fields("uuid", "thread_uuid", "presentation_mode", "kind", "title", "status", "current_step_key", "steps"), RecommendedFields: []string{"uuid", "thread_uuid", "presentation_mode", "kind", "title", "status", "current_step_key", "steps"}},
		{Key: "chapter_story", Fields: fields("uuid", "version_no", "source_type", "source_uuid", "source_item_uuid", "content", "content_format", "char_count", "created_at"), RecommendedFields: []string{"uuid", "version_no", "source_type", "source_uuid", "source_item_uuid", "content_format", "char_count", "created_at"}},
		{Key: "chapter_story_list", List: true, ItemProjector: "chapter_story"},
		{Key: "story_profile_list", List: true, ItemProjector: "story_profile"},
		{Key: "premise_source", Fields: fields("uuid", "source_type", "source_text", "style_snapshot", "ignored_at", "revision", "created_at"), RecommendedFields: []string{"uuid", "source_type", "ignored_at", "revision", "created_at"}},
		{Key: "premise_source_list", List: true, ItemProjector: "premise_source"},
		{Key: "setting_image", Fields: fields("uuid", "source_uuid", "origin", "prompt", "asset", "created_at"), RecommendedFields: []string{"uuid", "source_uuid", "origin", "created_at"}},
		{Key: "setting_image_list", List: true, ItemProjector: "setting_image"},
		{Key: "premise_asset_variant", Fields: fields("uuid", "version_no", "source_type", "source_setting_image_uuid", "crop", "asset", "created_at"), RecommendedFields: []string{"uuid", "version_no", "source_type", "source_setting_image_uuid", "created_at"}},
		{Key: "premise_asset_variant_list", List: true, ItemProjector: "premise_asset_variant"},
		{Key: "comic_state", Fields: fields("uuid", "chapter_uuid", "status", "has_premise_assets", "premise_asset_count", "revision", "updated_at"), RecommendedFields: []string{"uuid", "chapter_uuid", "status", "has_premise_assets", "premise_asset_count", "revision", "updated_at"}},
		{Key: "comic_section_list", List: true, ItemProjector: "comic_section"},
		{Key: "storyboard", Fields: fields("uuid", "version_no", "content_md", "source_type", "created_at"), RecommendedFields: []string{"uuid", "version_no", "source_type", "created_at"}},
		{Key: "storyboard_list", List: true, ItemProjector: "storyboard"},
		{Key: "comic_image_variant", Fields: fields("uuid", "version_no", "source_type", "generation_uuid", "asset", "created_at"), RecommendedFields: []string{"uuid", "version_no", "source_type", "generation_uuid", "created_at"}},
		{Key: "comic_image_variant_list", List: true, ItemProjector: "comic_image_variant"},
		{Key: "comic_image_generation_batch", Fields: fields("chapter_uuid", "requested_count", "accepted_count", "tasks"), RecommendedFields: []string{"chapter_uuid", "requested_count", "accepted_count", "tasks"}},
		{Key: "comic_snapshot", Fields: fields("uuid", "version_no", "reason", "source", "section_count", "created_at"), RecommendedFields: []string{"uuid", "version_no", "reason", "source", "section_count", "created_at"}},
		{Key: "comic_snapshot_list", List: true, ItemProjector: "comic_snapshot"},
		{Key: "comic_snapshot_detail", Fields: fields("uuid", "version_no", "reason", "source", "section_count", "schema_version", "chapter", "sections", "created_at"), RecommendedFields: []string{"uuid", "version_no", "reason", "source", "section_count", "schema_version", "created_at"}},
		{Key: "comic_export_readiness", Fields: fields("scope", "chapter_uuid", "active_chapter_count", "active_section_count", "image_section_count", "missing_section_count", "can_export", "complete", "missing_sections"), RecommendedFields: []string{"scope", "chapter_uuid", "active_chapter_count", "active_section_count", "image_section_count", "missing_section_count", "can_export", "complete"}},
		{Key: "comic_export", Fields: fields("uuid", "task_uuid", "scope", "chapter_uuid", "format", "filename", "status", "snapshot_hash", "expires_at", "retention_days", "byte_size", "content_sha256", "error_code", "created_at", "completed_at"), RecommendedFields: []string{"uuid", "task_uuid", "scope", "chapter_uuid", "format", "filename", "status", "expires_at", "byte_size", "error_code", "completed_at"}},
		{Key: "comic_export_list", List: true, ItemProjector: "comic_export"},
		{Key: "task_list", List: true, ItemProjector: "task"},
		{Key: "task_event", Fields: fields("uuid", "sequence", "event_type", "created_at"), RecommendedFields: []string{"uuid", "sequence", "event_type", "created_at"}},
		{Key: "task_event_list", List: true, ItemProjector: "task_event"},
		{Key: "project_asset", Fields: fields("uuid", "kind", "purpose", "original_filename", "display_name", "source_type", "source_asset_uuid", "mime_type", "byte_size", "width", "height", "duration_ms", "status", "deleted_at", "created_at"), RecommendedFields: []string{"uuid", "kind", "purpose", "original_filename", "display_name", "mime_type", "byte_size", "width", "height", "duration_ms", "status", "deleted_at", "created_at"}},
		{Key: "project_asset_list", List: true, ItemProjector: "project_asset"},
	}
}

func phase3AgentAPIRoutes() []agentAPIRoute {
	project := "/api/v1/projects/{project_uuid}"
	revisionBody := apiObject(map[string]any{"expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "expected_revision")
	pageQuery := apiObject(map[string]any{"page": apiBoundedInteger("页码。", 1, 1000000), "per_page": apiBoundedInteger("每页数量。", 1, 100)})
	listQuery := apiObject(map[string]any{"status": apiEnum("可选公开状态过滤。", "queued", "running", "waiting_for_input", "completed", "failed", "cancelled", "interrupted"), "limit": apiBoundedInteger("返回数量。", 1, 100)})
	eventQuery := apiObject(map[string]any{"before": apiLimitedString("上一页 cursor。", 32), "after": apiLimitedString("下一页 cursor。", 32), "limit": apiBoundedInteger("返回数量。", 1, 100)})
	generationBody := apiObject(map[string]any{
		"prompt": apiLimitedString("生成指令。", 262144), "model": apiLimitedString("可选模型覆盖。", 512),
		"chapter_count": apiBoundedInteger("计划章节数。", 1, 20), "max_section_count": apiBoundedInteger("最大 Section 数。", 1, 48),
	}, "prompt")
	return []agentAPIRoute{
		{ID: RouteYoloWorkflowCreate, Action: "启动受控 YOLO 项目初始化", Method: "POST", PathTemplate: project + "/workflows", Handler: RouteYoloWorkflowCreate, Projector: "workflow", DocPath: workflowDocPath, BodySchema: apiObject(map[string]any{"story_prompt": apiLimitedString("基于原始需求、用户补充和已展示建议整理的 YOLO 故事 Brief。", 4000), "model": apiLimitedString("可选文本模型覆盖。", 512)}, "story_prompt"), RecommendedResponseFilter: ".data | {uuid,thread_uuid,presentation_mode,kind,title,status,current_step_key,steps}", Async: true, StrictSchema: true, Risk: RiskWrite},
		{ID: RouteProjectGet, Action: "读取项目公开信息", Method: "GET", PathTemplate: project, Handler: RouteProjectGet, Projector: "project", DocPath: projectDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteProjectUpdate, Action: "更新项目元数据", Method: "PATCH", PathTemplate: project, Handler: RouteProjectUpdate, Projector: "project", DocPath: projectDocPath, BodySchema: apiObject(map[string]any{"name": apiLimitedString("项目名称。", 120), "description": apiLimitedString("项目简介。", 2000), "generation_language": apiEnum("生成语言。", "zh-Hans", "en"), "expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "name", "description", "expected_revision"), ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteChapterCreate, Action: "创建章节", Method: "POST", PathTemplate: project + "/chapters", Handler: RouteChapterCreate, Projector: "chapter", DocPath: chapterDocPath, BodySchema: apiObject(map[string]any{"chapter_code": apiLimitedString("章节业务编号。", 64), "title": apiLimitedString("章节标题。", 255), "content": apiLimitedString("可选初始正文。", 3000000), "content_format": apiEnum("正文格式。", "txt", "md")}, "chapter_code", "title"), Risk: RiskWrite},
		{ID: RouteChapterUpdate, Action: "更新章节元数据", Method: "PATCH", PathTemplate: project + "/chapters/{chapter_uuid}", Handler: RouteChapterUpdate, Projector: "chapter", DocPath: chapterDocPath, BodySchema: apiObject(map[string]any{"title": apiLimitedString("章节标题。", 255), "expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "title", "expected_revision"), ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteChapterStoryList, Action: "读取章节正文历史", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/stories", Handler: RouteChapterStoryList, Projector: "chapter_story_list", DocPath: chapterDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteChapterTrash, Action: "将章节移入回收站", Method: "DELETE", PathTemplate: project + "/chapters/{chapter_uuid}", Handler: RouteChapterTrash, Projector: "chapter", DocPath: chapterDocPath, BodySchema: revisionBody, ExpectedRevision: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteChapterRestore, Action: "从回收站恢复章节", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/restorations", Handler: RouteChapterRestore, Projector: "chapter", DocPath: chapterDocPath, BodySchema: revisionBody, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteStoryProfileList, Action: "读取 Story Profile 历史", Method: "GET", PathTemplate: project + "/story-profile/versions", Handler: RouteStoryProfileList, Projector: "story_profile_list", DocPath: storyDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteStoryProfileImport, Action: "从项目 STORY.md 导入", Method: "POST", PathTemplate: project + "/story-profile/imports", Handler: RouteStoryProfileImport, Projector: "story_profile", DocPath: storyDocPath, BodySchema: revisionBody, ExpectedRevision: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteStoryProfileRegenerate, Action: "重新生成项目 STORY.md", Method: "POST", PathTemplate: project + "/story-profile/projection", Handler: RouteStoryProfileRegenerate, Projector: "story_profile", DocPath: storyDocPath, BodySchema: revisionBody, ExpectedRevision: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteStoryProfileGenerationCreate, Action: "创建 Story Profile 生成任务", Method: "POST", PathTemplate: project + "/story-profile/generations", Handler: RouteStoryProfileGenerationCreate, Projector: "task", DocPath: generationDocPath, BodySchema: generationBody, Async: true, Risk: RiskWrite},
		{ID: RouteStoryProfileRebuildCreate, Action: "从章节重建 Story Profile", Method: "POST", PathTemplate: project + "/story-profile/reconstructions", Handler: RouteStoryProfileRebuildCreate, Projector: "task", DocPath: generationDocPath, BodySchema: apiObject(map[string]any{"model": apiLimitedString("可选模型覆盖。", 512)}), Async: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteChapterBatchPlanCreate, Action: "创建章节批量规划任务", Method: "POST", PathTemplate: project + "/chapter-batches", Handler: RouteChapterBatchPlanCreate, Projector: "task", DocPath: generationDocPath, BodySchema: generationBody, Async: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteComicStoryboardGenerationCreate, Action: "创建漫画分镜规划任务", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-storyboard-generations", Handler: RouteComicStoryboardGenerationCreate, Projector: "task", DocPath: generationDocPath, BodySchema: generationBody, Async: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RoutePremiseUpdate, Action: "更新 Premise", Method: "PATCH", PathTemplate: project + "/premise", Handler: RoutePremiseUpdate, Projector: "premise", DocPath: premiseDocPath, BodySchema: apiObject(map[string]any{"default_style": apiLimitedString("项目整体画风。", 8192), "expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "default_style", "expected_revision"), ExpectedRevision: true, Risk: RiskWrite},
		{ID: RoutePremiseSourceList, Action: "列出 Premise Source", Method: "GET", PathTemplate: project + "/premise-sources", Handler: RoutePremiseSourceList, Projector: "premise_source_list", DocPath: premiseDocPath, QuerySchema: pageQuery, ReadOnly: true, Risk: RiskLow},
		{ID: RoutePremiseSourceCreate, Action: "创建 Premise Source", Method: "POST", PathTemplate: project + "/premise-sources", Handler: RoutePremiseSourceCreate, Projector: "premise_source", DocPath: premiseDocPath, BodySchema: apiObject(map[string]any{"source_text": apiLimitedString("Premise 来源文本。", 262144), "style_snapshot": apiLimitedString("画风快照。", 8192), "source_type": apiEnum("来源类型。", "manual", "generated"), "model": apiLimitedString("可选模型记录。", 512), "parameters": map[string]any{"type": "object", "additionalProperties": true}}, "source_text", "style_snapshot", "source_type"), Risk: RiskWrite},
		{ID: RoutePremiseSourceUpdate, Action: "更新 Premise Source 状态", Method: "PATCH", PathTemplate: project + "/premise-sources/{source_uuid}", Handler: RoutePremiseSourceUpdate, Projector: "premise_source", DocPath: premiseDocPath, BodySchema: apiObject(map[string]any{"ignored": apiBoolean("是否忽略该来源。"), "expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "ignored", "expected_revision"), ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteSettingImageList, Action: "列出 Premise Setting Image", Method: "GET", PathTemplate: project + "/premise-setting-images", Handler: RouteSettingImageList, Projector: "setting_image_list", DocPath: premiseDocPath, QuerySchema: apiObject(map[string]any{"source_uuids": map[string]any{"type": "array", "maxItems": 100, "items": apiString("Source UUIDv7。")}}), ReadOnly: true, Risk: RiskLow},
		{ID: RouteSettingImageImport, Action: "导入 Premise Setting Image", Method: "POST", PathTemplate: project + "/premise-setting-images", Handler: RouteSettingImageImport, Projector: "setting_image", DocPath: premiseDocPath, BodySchema: apiObject(map[string]any{"upload_uuid": apiString("当前项目 ready upload UUIDv7。"), "source_uuid": apiString("可选 Source UUIDv7。"), "prompt": apiLimitedString("图片说明。", 262144)}, "upload_uuid"), Risk: RiskWrite},
		{ID: RouteSettingImageSelect, Action: "选择 Premise Setting Image", Method: "POST", PathTemplate: project + "/premise-setting-images/{setting_image_uuid}/selections", Handler: RouteSettingImageSelect, Projector: "premise", DocPath: premiseDocPath, BodySchema: apiObject(map[string]any{}), Risk: RiskWrite},
		{ID: RoutePremiseAssetRestore, Action: "从回收站恢复设定项", Method: "POST", PathTemplate: project + "/premise-assets/{premise_asset_uuid}/restorations", Handler: RoutePremiseAssetRestore, Projector: "premise_asset", DocPath: premiseAssetDocPath, BodySchema: revisionBody, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RoutePremiseAssetVariantList, Action: "列出设定项图片 variants", Method: "GET", PathTemplate: project + "/premise-assets/{premise_asset_uuid}/variants", Handler: RoutePremiseAssetVariantList, Projector: "premise_asset_variant_list", DocPath: premiseAssetDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RoutePremiseAssetVariantCreate, Action: "创建设定项图片 variant", Method: "POST", PathTemplate: project + "/premise-assets/{premise_asset_uuid}/variants", Handler: RoutePremiseAssetVariantCreate, Projector: "premise_asset", DocPath: premiseAssetDocPath, BodySchema: apiObject(map[string]any{"upload_uuid": apiString("当前项目 ready upload UUIDv7。"), "expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "upload_uuid", "expected_revision"), ExpectedRevision: true, Risk: RiskWrite},
		{ID: RoutePremiseAssetVariantSelect, Action: "选择设定项图片 variant", Method: "POST", PathTemplate: project + "/premise-assets/{premise_asset_uuid}/variants/{variant_uuid}/selections", Handler: RoutePremiseAssetVariantSelect, Projector: "premise_asset", DocPath: premiseAssetDocPath, BodySchema: revisionBody, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteComicStateGet, Action: "读取 Comic 状态", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/comic", Handler: RouteComicStateGet, Projector: "comic_state", DocPath: comicDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteComicSectionList, Action: "列出 Comic Section", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections", Handler: RouteComicSectionList, Projector: "comic_section_list", DocPath: comicDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteComicSectionCreate, Action: "创建 Comic Section", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections", Handler: RouteComicSectionCreate, Projector: "comic_section", DocPath: comicDocPath, BodySchema: apiObject(map[string]any{"title": apiLimitedString("Section 标题。", 160), "description_md": apiLimitedString("Section 描述。", 262144), "storyboard_md": apiLimitedString("可选 Storyboard。", 262144)}, "title"), Risk: RiskWrite},
		{ID: RouteComicSectionUpdate, Action: "更新 Comic Section", Method: "PATCH", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}", Handler: RouteComicSectionUpdate, Projector: "comic_section", DocPath: comicDocPath, BodySchema: apiObject(map[string]any{"title": apiLimitedString("Section 标题。", 160), "description_md": apiLimitedString("Section 描述。", 262144), "expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "expected_revision"), ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteComicSectionReorder, Action: "重排 Comic Section", Method: "PUT", PathTemplate: project + "/chapters/{chapter_uuid}/comic-section-order", Handler: RouteComicSectionReorder, Projector: "comic_section_list", DocPath: comicDocPath, BodySchema: apiObject(map[string]any{"section_uuids": map[string]any{"type": "array", "minItems": 1, "maxItems": 200, "items": apiString("Section UUIDv7。")}}, "section_uuids"), RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteComicSectionDelete, Action: "删除 Comic Section", Method: "DELETE", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}", Handler: RouteComicSectionDelete, Projector: "comic_section", DocPath: comicDocPath, BodySchema: revisionBody, ExpectedRevision: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteStoryboardList, Action: "列出 Storyboard variants", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants", Handler: RouteStoryboardList, Projector: "storyboard_list", DocPath: storyboardDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteStoryboardSelect, Action: "选择 Storyboard variant", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}/storyboard-variants/{variant_uuid}/selections", Handler: RouteStoryboardSelect, Projector: "comic_section", DocPath: storyboardDocPath, BodySchema: revisionBody, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteComicSectionImageImport, Action: "导入 Section Image", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}/images", Handler: RouteComicSectionImageImport, Projector: "comic_section", DocPath: comicDocPath, BodySchema: apiObject(map[string]any{"upload_uuid": apiString("当前项目 ready upload UUIDv7。"), "expected_revision": apiBoundedInteger("最新 revision。", 0, 1<<31-1)}, "upload_uuid", "expected_revision"), ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteComicImageGenerationBatchCreate, Action: "批量创建漫画图片任务", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-image-generation-batches", Handler: RouteComicImageGenerationBatchCreate, Projector: "comic_image_generation_batch", DocPath: generationDocPath, BodySchema: apiObject(map[string]any{"section_uuids": map[string]any{"type": "array", "minItems": 1, "maxItems": 48, "items": apiString("Section UUIDv7。")}}, "section_uuids"), RecommendedResponseFilter: ".data | {chapter_uuid,requested_count,accepted_count,tasks:{uuid,kind,resource_uuid,status,error_code,error_message}}", Async: true, Risk: RiskWrite},
		{ID: RouteComicImageVariantList, Action: "列出 Section Image variants", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants", Handler: RouteComicImageVariantList, Projector: "comic_image_variant_list", DocPath: comicDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteComicImageVariantSelect, Action: "选择 Section Image variant", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-sections/{section_uuid}/image-variants/{variant_uuid}/selections", Handler: RouteComicImageVariantSelect, Projector: "comic_section", DocPath: comicDocPath, BodySchema: revisionBody, ExpectedRevision: true, Risk: RiskWrite},
		{ID: RouteComicSnapshotList, Action: "列出 Comic Snapshot", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/comic-snapshots", Handler: RouteComicSnapshotList, Projector: "comic_snapshot_list", DocPath: comicSnapshotDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteComicSnapshotGet, Action: "读取 Comic Snapshot", Method: "GET", PathTemplate: project + "/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}", Handler: RouteComicSnapshotGet, Projector: "comic_snapshot_detail", DocPath: comicSnapshotDocPath, ReadOnly: true, Risk: RiskLow},
		{ID: RouteComicSnapshotRestore, Action: "恢复 Comic Snapshot", Method: "POST", PathTemplate: project + "/chapters/{chapter_uuid}/comic-snapshots/{snapshot_uuid}/restorations", Handler: RouteComicSnapshotRestore, Projector: "comic_section_list", DocPath: comicSnapshotDocPath, BodySchema: apiObject(map[string]any{}), RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteComicExportReadiness, Action: "查询 Comic Export readiness", Method: "GET", PathTemplate: project + "/comic-exports/readiness", Handler: RouteComicExportReadiness, Projector: "comic_export_readiness", DocPath: comicExportDocPath, QuerySchema: apiObject(map[string]any{"scope": apiEnum("导出范围。", "project", "chapter"), "chapter_uuid": apiString("chapter scope 时的 Chapter UUIDv7。")}, "scope"), ReadOnly: true, Risk: RiskLow},
		{ID: RouteComicExportList, Action: "列出 Comic Export", Method: "GET", PathTemplate: project + "/comic-exports", Handler: RouteComicExportList, Projector: "comic_export_list", DocPath: comicExportDocPath, QuerySchema: apiObject(map[string]any{"page": apiBoundedInteger("页码。", 1, 1000000), "per_page": apiBoundedInteger("每页数量。", 1, 100), "scope": apiEnum("导出范围。", "project", "chapter"), "chapter_uuid": apiString("Chapter UUIDv7。"), "task_uuid": apiString("Task UUIDv7。"), "snapshot_hash": apiLimitedString("Snapshot hash。", 128), "format": apiEnum("导出格式。", "zip", "pdf"), "status": apiLimitedString("导出状态。", 64)}), ReadOnly: true, Risk: RiskLow},
		{ID: RouteComicExportCreate, Action: "创建 Comic Export", Method: "POST", PathTemplate: project + "/comic-exports", Handler: RouteComicExportCreate, Projector: "task", DocPath: comicExportDocPath, BodySchema: apiObject(map[string]any{"scope": apiEnum("导出范围。", "project", "chapter"), "chapter_uuid": apiString("chapter scope 时的 Chapter UUIDv7。"), "format": apiEnum("导出格式。", "zip", "pdf"), "allow_missing_images": apiBoolean("是否允许缺图导出。")}, "scope", "format"), Async: true, Risk: RiskWrite},
		{ID: RouteStoryTaskList, Action: "列出 Story Task", Method: "GET", PathTemplate: project + "/tasks", Handler: RouteStoryTaskList, Projector: "task_list", DocPath: taskDocPath, QuerySchema: listQuery, ReadOnly: true, Async: true, Risk: RiskLow},
		{ID: RouteStoryTaskEventList, Action: "读取 Story Task 事件", Method: "GET", PathTemplate: project + "/tasks/{task_uuid}/events", Handler: RouteStoryTaskEventList, Projector: "task_event_list", DocPath: taskDocPath, QuerySchema: eventQuery, ReadOnly: true, Async: true, Risk: RiskLow},
		{ID: RouteStoryTaskCancel, Action: "取消 Story Task", Method: "POST", PathTemplate: project + "/tasks/{task_uuid}/cancellations", Handler: RouteStoryTaskCancel, Projector: "task", DocPath: taskDocPath, BodySchema: apiObject(map[string]any{}), Async: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteStoryTaskRetry, Action: "重试 Story Task", Method: "POST", PathTemplate: project + "/tasks/{task_uuid}/retries", Handler: RouteStoryTaskRetry, Projector: "task", DocPath: taskDocPath, BodySchema: apiObject(map[string]any{}), Async: true, Risk: RiskWrite},
		{ID: RouteProductionTaskList, Action: "列出 Production Task", Method: "GET", PathTemplate: project + "/production-tasks", Handler: RouteProductionTaskList, Projector: "task_list", DocPath: taskDocPath, QuerySchema: listQuery, ReadOnly: true, Async: true, Risk: RiskLow},
		{ID: RouteProductionTaskEventList, Action: "读取 Production Task 事件", Method: "GET", PathTemplate: project + "/production-tasks/{task_uuid}/events", Handler: RouteProductionTaskEventList, Projector: "task_event_list", DocPath: taskDocPath, QuerySchema: eventQuery, ReadOnly: true, Async: true, Risk: RiskLow},
		{ID: RouteProductionTaskCancel, Action: "取消 Production Task", Method: "POST", PathTemplate: project + "/production-tasks/{task_uuid}/cancellations", Handler: RouteProductionTaskCancel, Projector: "task", DocPath: taskDocPath, BodySchema: apiObject(map[string]any{}), Async: true, RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteProductionTaskRetry, Action: "重试 Production Task", Method: "POST", PathTemplate: project + "/production-tasks/{task_uuid}/retries", Handler: RouteProductionTaskRetry, Projector: "task", DocPath: taskDocPath, BodySchema: apiObject(map[string]any{}), Async: true, Risk: RiskWrite},
		{ID: RouteProjectAssetList, Action: "列出 Project Asset", Method: "GET", PathTemplate: project + "/assets", Handler: RouteProjectAssetList, Projector: "project_asset_list", DocPath: projectAssetDocPath, QuerySchema: apiObject(map[string]any{"purpose": apiLimitedString("用途过滤。", 120), "kind": apiLimitedString("类型过滤。", 120), "deleted": apiBoolean("只列出回收站资源。"), "limit": apiBoundedInteger("返回数量。", 1, 100)}), ReadOnly: true, Risk: RiskLow},
		{ID: RouteProjectAssetGet, Action: "读取 Project Asset", Method: "GET", PathTemplate: project + "/assets/{asset_uuid}", Handler: RouteProjectAssetGet, Projector: "project_asset", DocPath: projectAssetDocPath, QuerySchema: apiObject(map[string]any{"include_trashed": apiBoolean("允许读取回收站资源。")}), ReadOnly: true, Risk: RiskLow},
		{ID: RouteProjectAssetUpdate, Action: "更新 Project Asset 公开元数据", Method: "PATCH", PathTemplate: project + "/assets/{asset_uuid}", Handler: RouteProjectAssetUpdate, Projector: "project_asset", DocPath: projectAssetDocPath, BodySchema: apiObject(map[string]any{"display_name": apiLimitedString("显示名称。", 255), "metadata": map[string]any{"type": "object", "additionalProperties": true}}), Risk: RiskWrite},
		{ID: RouteProjectAssetTrash, Action: "将 Project Asset 移入回收站", Method: "DELETE", PathTemplate: project + "/assets/{asset_uuid}", Handler: RouteProjectAssetTrash, Projector: "project_asset", DocPath: projectAssetDocPath, BodySchema: apiObject(map[string]any{}), RequiresConfirmation: true, Risk: RiskDangerous},
		{ID: RouteProjectAssetRestore, Action: "从回收站恢复 Project Asset", Method: "POST", PathTemplate: project + "/assets/{asset_uuid}/restorations", Handler: RouteProjectAssetRestore, Projector: "project_asset", DocPath: projectAssetDocPath, BodySchema: apiObject(map[string]any{}), Risk: RiskWrite},
	}
}
