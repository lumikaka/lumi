package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"lumi/internal/files"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

func executePhase3AgentAPIRoute(ctx context.Context, service *Service, store *project.Store, tc toolContext, execution toolExecutionRecord, request agentAPIRequest) (any, bool, error) {
	storyService := story.NewService(store)
	productionService := production.NewService(store, service.hub)
	fileService := productionService.Files()
	args := cloneToolArguments(request.Body)
	chapterUUID, sectionUUID := request.Params["chapter_uuid"], request.Params["section_uuid"]
	switch request.Route.Handler {
	case RouteYoloWorkflowCreate:
		if !isBootstrapToolContext(tc) {
			return nil, true, bootstrapYoloNotAuthorizedError()
		}
		authorized, err := bootstrapYoloAuthorized(ctx, store, tc)
		if err != nil {
			return nil, true, err
		}
		if !authorized {
			return nil, true, bootstrapYoloNotAuthorizedError()
		}
		projectDetail, err := storyService.GetProject(ctx)
		if err != nil {
			return nil, true, err
		}
		workflow, err := service.CreateYoloWorkflow(ctx, tc.ProjectUUID, CreateYoloInput{
			Title: projectDetail.Name, StoryPrompt: stringArg(args, "story_prompt"), ProviderUUID: tc.Run.ProviderUUID,
			Model: stringArg(args, "model"), IdempotencyKey: bootstrapYoloIdempotencyPrefix + tc.BootstrapCreationSessionUUID,
			Invocation: chatToolInvocationContext(tc, execution),
		})
		if err != nil {
			return nil, true, err
		}
		return agentYoloWorkflowValue(workflow), true, nil
	case RouteProjectGet:
		value, err := storyService.GetProject(ctx)
		return value, true, err
	case RouteProjectUpdate:
		language, hasLanguage := args["generation_language"].(string)
		var languagePointer *string
		if hasLanguage {
			languagePointer = &language
		}
		value, err := storyService.UpdateProject(ctx, story.UpdateProjectInput{
			Name: stringArg(args, "name"), Description: stringArg(args, "description"),
			GenerationLanguage: languagePointer, ExpectedRevision: intArg(args, "expected_revision"),
		})
		if err == nil && service.projects != nil {
			err = service.projects.SyncProjectName(ctx, tc.ProjectUUID)
		}
		return value, true, err
	case RouteChapterCreate:
		format := stringArg(args, "content_format")
		if stringArg(args, "content") != "" && format == "" {
			format = "txt"
		}
		value, err := storyService.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: stringArg(args, "chapter_code"), Title: stringArg(args, "title"), Content: stringArg(args, "content"), ContentFormat: format})
		return value, true, err
	case RouteChapterUpdate:
		value, err := storyService.UpdateChapter(ctx, chapterUUID, story.UpdateChapterInput{Title: stringArg(args, "title"), ExpectedRevision: intArg(args, "expected_revision")})
		return value, true, err
	case RouteChapterStoryList:
		items, err := storyService.ListChapterStories(ctx, chapterUUID)
		return map[string]any{"items": items}, true, err
	case RouteChapterTrash:
		value, err := storyService.TrashChapter(ctx, chapterUUID, intArg(args, "expected_revision"))
		return value, true, err
	case RouteChapterRestore:
		value, err := storyService.RestoreChapter(ctx, chapterUUID, intArg(args, "expected_revision"))
		return value, true, err
	case RouteStoryProfileList:
		items, err := storyService.ListStoryProfiles(ctx)
		return map[string]any{"items": items}, true, err
	case RouteStoryProfileImport:
		value, err := storyService.ImportExternalStoryMD(ctx, intArg(args, "expected_revision"))
		return value, true, err
	case RouteStoryProfileRegenerate:
		value, err := storyService.RegenerateStoryMD(ctx, intArg(args, "expected_revision"))
		return value, true, err
	case RouteStoryProfileGenerationCreate:
		value, err := phase3Generation(ctx, service, tc, execution, args, "story_profile_generation", tc.ProjectUUID, "")
		return value, true, err
	case RouteStoryProfileRebuildCreate:
		value, err := phase3Generation(ctx, service, tc, execution, args, "story_profile_from_chapters", tc.ProjectUUID, "")
		return value, true, err
	case RouteChapterBatchPlanCreate:
		value, err := phase3Generation(ctx, service, tc, execution, args, "story_chapter_batch_plan", tc.ProjectUUID, "")
		return value, true, err
	case RouteComicStoryboardGenerationCreate:
		value, err := phase3Generation(ctx, service, tc, execution, args, "comic_storyboard_generation", chapterUUID, chapterUUID)
		return value, true, err
	case RoutePremiseUpdate:
		value, err := productionService.UpdatePremise(ctx, production.UpdatePremiseInput{DefaultStyle: stringArg(args, "default_style"), ExpectedRevision: intArg(args, "expected_revision")})
		return value, true, err
	case RoutePremiseSourceList:
		items, pagination, err := productionService.ListPremiseSourcesPage(ctx, queryInt(request.Query, "page", 1), queryInt(request.Query, "per_page", 20))
		return map[string]any{"items": items, "pagination": pagination}, true, err
	case RoutePremiseSourceCreate:
		value, err := productionService.CreatePremiseSource(ctx, production.CreateSourceInput{SourceText: stringArg(args, "source_text"), StyleSnapshot: stringArg(args, "style_snapshot"), SourceType: stringArg(args, "source_type"), Model: stringArg(args, "model"), Parameters: args["parameters"]})
		return value, true, err
	case RoutePremiseSourceUpdate:
		ignored, _ := args["ignored"].(bool)
		value, err := productionService.SetPremiseSourceIgnored(ctx, request.Params["source_uuid"], ignored, intArg(args, "expected_revision"))
		return value, true, err
	case RouteSettingImageList:
		sources := stringSliceArg(request.Query, "source_uuids")
		var items []production.SettingImage
		var err error
		if len(sources) == 0 {
			items, err = productionService.ListSettingImages(ctx)
		} else {
			items, err = productionService.ListSettingImagesForSources(ctx, sources)
		}
		return map[string]any{"items": items}, true, err
	case RouteSettingImageImport:
		value, err := productionService.ImportSettingImage(ctx, stringArg(args, "upload_uuid"), stringArg(args, "source_uuid"), stringArg(args, "prompt"))
		return value, true, err
	case RouteSettingImageSelect:
		value, err := productionService.SelectSettingImage(ctx, request.Params["setting_image_uuid"])
		return value, true, err
	case RoutePremiseAssetRestore:
		value, err := productionService.SetPremiseAssetTrashedFromTool(ctx, request.Params["premise_asset_uuid"], false, intArg(args, "expected_revision"), execution.UUID)
		return value, true, err
	case RoutePremiseAssetVariantList:
		items, err := productionService.ListAssetVariants(ctx, request.Params["premise_asset_uuid"])
		return map[string]any{"items": items}, true, err
	case RoutePremiseAssetVariantCreate:
		value, err := productionService.ImportPremiseAssetVariant(ctx, request.Params["premise_asset_uuid"], stringArg(args, "upload_uuid"), nil, intArg(args, "expected_revision"))
		return value, true, err
	case RoutePremiseAssetVariantSelect:
		value, err := productionService.SelectAssetVariant(ctx, request.Params["premise_asset_uuid"], request.Params["variant_uuid"], intArg(args, "expected_revision"))
		return value, true, err
	case RouteComicStateGet:
		value, err := productionService.GetComicState(ctx, chapterUUID)
		return value, true, err
	case RouteComicSectionList:
		items, err := productionService.ListSections(ctx, chapterUUID)
		return map[string]any{"items": items}, true, err
	case RouteComicSectionCreate:
		value, err := productionService.CreateSection(ctx, chapterUUID, production.CreateSectionInput{Title: stringArg(args, "title"), DescriptionMD: stringArg(args, "description_md"), StoryboardMD: stringArg(args, "storyboard_md"), PageRole: stringArg(args, "page_role")})
		return value, true, err
	case RouteComicSectionUpdate:
		input := production.UpdateSectionInput{ExpectedRevision: intArg(args, "expected_revision")}
		if value, ok := args["title"].(string); ok {
			input.Title = &value
		}
		if value, ok := args["description_md"].(string); ok {
			input.DescriptionMD = &value
		}
		if value, ok := args["page_role"].(string); ok {
			input.PageRole = &value
		}
		value, err := productionService.UpdateSection(ctx, chapterUUID, sectionUUID, input)
		return value, true, err
	case RouteComicSectionReorder:
		items, err := productionService.ReorderSections(ctx, chapterUUID, stringSliceArg(args, "section_uuids"))
		return map[string]any{"items": items}, true, err
	case RouteComicSectionDelete:
		err := productionService.DeleteSection(ctx, chapterUUID, sectionUUID, intArg(args, "expected_revision"))
		return map[string]any{"uuid": sectionUUID, "deleted": err == nil}, true, err
	case RouteStoryboardList:
		items, err := productionService.ListStoryboards(ctx, chapterUUID, sectionUUID)
		return map[string]any{"items": items}, true, err
	case RouteStoryboardSelect:
		value, err := productionService.SelectStoryboard(ctx, chapterUUID, sectionUUID, request.Params["variant_uuid"], intArg(args, "expected_revision"))
		return value, true, err
	case RouteComicSectionImageImport:
		value, err := productionService.ImportSectionImage(ctx, chapterUUID, sectionUUID, stringArg(args, "upload_uuid"), intArg(args, "expected_revision"))
		return value, true, err
	case RouteComicImageGenerationBatchCreate:
		key := strings.TrimSpace(execution.IdempotencyKey)
		if key == "" {
			key = execution.UUID
		}
		value, err := service.queue.StartDomainTaskBatch(ctx, tc.ProjectUUID, DomainTaskBatchRequest{
			Kind: "comic_image_generation", ResourceUUIDs: stringSliceArg(args, "section_uuids"),
			ChapterUUID: chapterUUID, IdempotencyKey: key, Invocation: chatToolInvocationContext(tc, execution),
		})
		return value, true, err
	case RouteComicImageVariantList:
		items, err := productionService.ListImageVariants(ctx, chapterUUID, sectionUUID)
		return map[string]any{"items": items}, true, err
	case RouteComicImageVariantSelect:
		value, err := productionService.SelectImageVariant(ctx, chapterUUID, sectionUUID, request.Params["variant_uuid"], intArg(args, "expected_revision"))
		return value, true, err
	case RouteComicSnapshotList:
		items, err := productionService.ListChapterSnapshots(ctx, chapterUUID)
		return map[string]any{"items": items}, true, err
	case RouteComicSnapshotGet:
		value, err := productionService.GetChapterSnapshot(ctx, chapterUUID, request.Params["snapshot_uuid"])
		return value, true, err
	case RouteComicSnapshotRestore:
		items, err := productionService.RestoreChapterSnapshot(ctx, chapterUUID, request.Params["snapshot_uuid"])
		return map[string]any{"items": items}, true, err
	case RouteComicExportReadiness:
		value, err := productionService.ExportReadiness(ctx, stringArg(request.Query, "scope"), stringArg(request.Query, "chapter_uuid"))
		return value, true, err
	case RouteComicExportList:
		items, pagination, err := productionService.ListExportsPage(ctx, production.ExportFilter{Scope: stringArg(request.Query, "scope"), ChapterUUID: stringArg(request.Query, "chapter_uuid"), TaskUUID: stringArg(request.Query, "task_uuid"), SnapshotHash: stringArg(request.Query, "snapshot_hash"), Format: stringArg(request.Query, "format"), Status: stringArg(request.Query, "status")}, queryInt(request.Query, "page", 1), queryInt(request.Query, "per_page", 20))
		return map[string]any{"items": items, "pagination": pagination}, true, err
	case RouteComicExportCreate:
		request := DomainTaskRequest{Kind: "comic_export", Scope: stringArg(args, "scope"), ChapterUUID: stringArg(args, "chapter_uuid"), Format: stringArg(args, "format"), AllowMissingImages: boolArg(args, "allow_missing_images"), IdempotencyKey: execution.IdempotencyKey}
		value, err := service.queue.StartDomainTask(ctx, tc.ProjectUUID, request)
		return value, true, err
	case RouteStoryTaskList, RouteProductionTaskList:
		domain := taskDomain(request.Route.ID)
		items, err := service.queue.ListDomainTasks(ctx, tc.ProjectUUID, domain, stringArg(request.Query, "status"), queryInt(request.Query, "limit", 50))
		return map[string]any{"items": items}, true, err
	case RouteStoryTaskEventList, RouteProductionTaskEventList:
		domain := taskDomain(request.Route.ID)
		before, after, err := eventCursors(request.Query)
		if err != nil {
			return nil, true, err
		}
		items, pagination, err := service.queue.ListDomainTaskEvents(ctx, tc.ProjectUUID, domain, request.Params["task_uuid"], before, after, queryInt(request.Query, "limit", 50))
		return map[string]any{"items": items, "cursor_pagination": pagination}, true, err
	case RouteStoryTaskCancel, RouteProductionTaskCancel:
		domain := taskDomain(request.Route.ID)
		if err := service.queue.CancelDomainTask(ctx, tc.ProjectUUID, domain, request.Params["task_uuid"]); err != nil {
			return nil, true, err
		}
		value, err := service.queue.GetDomainTask(ctx, tc.ProjectUUID, domain, request.Params["task_uuid"])
		return value, true, err
	case RouteStoryTaskRetry, RouteProductionTaskRetry:
		value, err := service.queue.RetryDomainTask(ctx, tc.ProjectUUID, taskDomain(request.Route.ID), request.Params["task_uuid"])
		return value, true, err
	case RouteProjectAssetList:
		trashed := boolArg(request.Query, "deleted")
		items, err := fileService.ListAssets(ctx, files.AssetFilter{Purpose: stringArg(request.Query, "purpose"), Kind: stringArg(request.Query, "kind"), IncludeTrashed: trashed, TrashedOnly: trashed, Limit: queryInt(request.Query, "limit", 100)})
		return map[string]any{"items": items}, true, err
	case RouteProjectAssetGet:
		value, err := fileService.GetAsset(ctx, request.Params["asset_uuid"], boolArg(request.Query, "include_trashed"))
		return value, true, err
	case RouteProjectAssetUpdate:
		input := files.UpdateAssetInput{Metadata: mapArg(args, "metadata")}
		if value, ok := args["display_name"].(string); ok {
			input.DisplayName = &value
		}
		value, err := fileService.UpdateAsset(ctx, request.Params["asset_uuid"], input)
		return value, true, err
	case RouteProjectAssetTrash:
		value, err := fileService.SoftDelete(ctx, request.Params["asset_uuid"])
		return value, true, err
	case RouteProjectAssetRestore:
		value, err := fileService.Restore(ctx, request.Params["asset_uuid"])
		return value, true, err
	default:
		return nil, false, nil
	}
}

func agentYoloWorkflowValue(workflow Workflow) map[string]any {
	steps := make([]map[string]any, 0, len(workflow.Steps))
	for _, step := range workflow.Steps {
		steps = append(steps, map[string]any{
			"uuid": step.UUID, "step_key": step.StepKey, "position": step.Position,
			"status": step.Status, "progress": step.Progress, "resource_uuid": step.ResourceUUID,
			"error_code": step.ErrorCode,
		})
	}
	return map[string]any{
		"uuid": workflow.UUID, "thread_uuid": workflow.ThreadUUID,
		"presentation_mode": workflow.PresentationMode, "kind": workflow.Kind,
		"title": workflow.Title, "status": workflow.Status,
		"current_step_key": workflow.CurrentStepKey, "steps": steps,
	}
}

func phase3Generation(ctx context.Context, service *Service, tc toolContext, execution toolExecutionRecord, args map[string]any, kind, resourceUUID, chapterUUID string) (DomainTask, error) {
	key := strings.TrimSpace(execution.IdempotencyKey)
	if key == "" {
		key = execution.UUID
	}
	return service.queue.StartDomainTask(ctx, tc.ProjectUUID, DomainTaskRequest{
		Kind: kind, ResourceUUID: resourceUUID, ChapterUUID: chapterUUID,
		ProviderUUID: tc.Run.ProviderUUID, Model: stringArg(args, "model"), Prompt: stringArg(args, "prompt"),
		ChapterCount: queryInt(args, "chapter_count", 1), MaxSectionCount: queryInt(args, "max_section_count", 0), IdempotencyKey: key,
		Invocation: chatToolInvocationContext(tc, execution),
	})
}

func queryInt(values map[string]any, key string, fallback int) int {
	if value := intArg(values, key); value > 0 {
		return int(value)
	}
	return fallback
}

func boolArg(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func mapArg(values map[string]any, key string) map[string]any {
	value, _ := values[key].(map[string]any)
	return value
}

func eventCursors(query map[string]any) (int64, int64, error) {
	_, hasBefore := query["before"]
	_, hasAfter := query["after"]
	if hasBefore && hasAfter {
		return 0, 0, domainError(CodeToolValidation, "事件 cursor 冲突", "before 与 after 不能同时使用。", nil)
	}
	parse := func(key string) (int64, error) {
		raw, present := query[key]
		if !present {
			return 0, nil
		}
		value, _ := raw.(string)
		value = strings.TrimSpace(value)
		if value == "" {
			return 0, domainError(CodeToolValidation, "事件 cursor 无效", key+" 必须是非负事件 sequence cursor。", nil)
		}
		for _, digit := range value {
			if digit < '0' || digit > '9' {
				return 0, domainError(CodeToolValidation, "事件 cursor 无效", key+" 必须是非负事件 sequence cursor。", nil)
			}
		}
		result, err := strconv.ParseInt(value, 10, 64)
		if err != nil || result < 0 {
			return 0, domainError(CodeToolValidation, "事件 cursor 无效", key+" 必须是非负事件 sequence cursor。", err)
		}
		return result, nil
	}
	before, err := parse("before")
	if err != nil {
		return 0, 0, err
	}
	after, err := parse("after")
	if err != nil {
		return 0, 0, err
	}
	return before, after, nil
}

func taskDomain(routeID string) string {
	if strings.Contains(routeID, ".production") {
		return "production"
	}
	return "story"
}

func phase3HandlerError(route agentAPIRoute) error {
	return domainError(CodeToolNotAllowed, "Route handler 未注册", fmt.Sprintf("%s 没有进程内 handler。", route.ID), nil)
}
