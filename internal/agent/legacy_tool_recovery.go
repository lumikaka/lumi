package agent

import (
	"context"

	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

// legacyRecoveryToolAllowed and executeLegacyToolRecovery are the only entry
// points for frozen legacy_typed_tools Run snapshots. New Run assembly never
// selects this protocol and active Scene definitions cannot reference it.
func legacyRecoveryToolAllowed(name string, thread threadRecord) bool {
	tools := map[string][]string{
		SceneProjectAssistant: {
			"get_story_profile", "update_story_profile", "list_chapters", "get_chapter", "update_chapter_story",
			"get_premise", "list_premise_assets", "get_premise_asset", "create_premise_asset", "update_premise_asset",
			"get_comic_section", "update_comic_storyboard", "start_generation", "request_user_input",
		},
		ScenePremiseAsset:        {"get_premise", "list_premise_assets", "get_premise_asset", "create_premise_asset", "image_gen", "request_user_input"},
		SceneAssetReference:      {currentProjectAPIToolName, "image_gen", "request_user_input"},
		SceneStoryboardReference: {"get_comic_section", "update_comic_storyboard", "request_user_input"},
	}
	return containsString(tools[logicalSceneKey(thread)], name)
}

func validateLegacyRecoveryIntent(tc toolContext, name string, args map[string]any) error {
	if name == currentProjectAPIToolName {
		_, err := parseCurrentProjectAPIRequest(tc, args)
		return err
	}
	return nil
}

func legacyRecoveryTargetUUID(name string, args map[string]any, thread threadRecord) string {
	if name == currentProjectAPIToolName {
		return thread.SubjectUUID
	}
	for _, key := range []string{"premise_asset_uuid", "section_uuid", "chapter_uuid", "resource_uuid", "upload_uuid"} {
		if value, ok := args[key].(string); ok && isUUIDv7(value) {
			return value
		}
	}
	return ""
}

func legacyRecoveryToolTargetAllowed(name string, args map[string]any, thread threadRecord) bool {
	if isStoryboardReferenceThread(thread) {
		switch name {
		case "get_comic_section", "update_comic_storyboard":
			return stringArg(args, "section_uuid") == thread.SubjectUUID
		default:
			return true
		}
	}
	if thread.Scene != SceneAssetReference {
		return true
	}
	switch name {
	case "get_premise_asset", "update_premise_asset":
		return stringArg(args, "premise_asset_uuid") == thread.SubjectUUID
	default:
		return true
	}
}

func (service *Service) executeLegacyToolRecovery(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord, args map[string]any) (any, error) {
	if normalizedToolMode(tc.ToolMode) != ToolModeLegacyTyped || !legacyRecoveryToolAllowed(execution.ToolName, tc.Thread) {
		return nil, domainError(CodeToolNotAllowed, "Legacy Tool recovery 越界", "只有冻结的 legacy Run snapshot 可以恢复旧 Tool。", nil)
	}
	storyService := story.NewService(store)
	productionService := production.NewService(store, service.hub)
	switch execution.ToolName {
	case "get_story_profile":
		return storyService.GetStoryProfile(ctx)
	case "update_story_profile":
		return updateStoryProfileTool(ctx, storyService, args)
	case "list_chapters":
		items, err := storyService.ListChapters(ctx, "active")
		return map[string]any{"items": items}, err
	case "get_chapter":
		return storyService.GetChapter(ctx, stringArg(args, "chapter_uuid"))
	case "update_chapter_story":
		return updateChapterStoryTool(ctx, storyService, args)
	case "get_premise":
		return productionService.GetPremise(ctx)
	case "list_premise_assets":
		items, err := productionService.ListPremiseAssets(ctx, "", "active")
		return map[string]any{"items": items}, err
	case "get_premise_asset":
		return productionService.GetPremiseAsset(ctx, stringArg(args, "premise_asset_uuid"))
	case "request_current_project_api":
		return executeCurrentProjectAPITool(ctx, productionService, tc, execution, args)
	case "create_premise_asset":
		return createPremiseAssetTool(ctx, productionService, tc, execution, args)
	case "update_premise_asset":
		return updatePremiseAssetTool(ctx, productionService, tc, execution, args)
	case "get_comic_section":
		return productionService.GetSection(ctx, stringArg(args, "chapter_uuid"), stringArg(args, "section_uuid"))
	case "update_comic_storyboard":
		return updateStoryboardTool(ctx, productionService, args)
	case "start_generation":
		return service.startGenerationTool(ctx, tc, execution, args)
	default:
		return nil, domainError(CodeToolNotAllowed, "Legacy Tool recovery 未注册", "持久化 Tool 不在 recovery allowlist。", nil)
	}
}
