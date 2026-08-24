package agent

import (
	"strings"
)

const (
	SceneProjectAssistant = "project_assistant"

	ToolModeLegacyTyped    = "legacy_typed_tools"
	ToolModeProjectAPI     = "project_api_tools"
	ToolProtocolProjectAPI = "project_api_v2"

	SubjectTypeNone         = ""
	SubjectTypePremiseAsset = "premise_asset"
	SubjectTypeComicSection = "comic_section"

	ImageReferenceNone            = "none"
	ImageReferenceMessage         = "message_attachments"
	ImageReferenceBoundAssetFirst = "bound_asset_first"
)

var projectAPIToolNames = []string{"request_api", "read_agent_doc", "image_gen", "request_user_input"}

// SceneDefinition contains only a logical Scene's runtime identity, prompt,
// subject requirements, image-reference policy, and Guide recommendations.
// Every valid project_api_tools Scene receives the same top-level tool set.
type SceneDefinition struct {
	Key                  string
	Scopes               []string
	BasePromptKey        string
	ScenePromptKey       string
	RecommendedGuideIDs  []string
	RequiresSubject      bool
	SubjectType          string
	ImageReferencePolicy string
}

func rawSceneDefinitions() []SceneDefinition {
	return []SceneDefinition{
		{
			Key: SceneProjectAssistant, Scopes: []string{ThreadScopeProject, ThreadScopePremise},
			BasePromptKey: "base", ScenePromptKey: "scene_project_assistant",
			RecommendedGuideIDs:  []string{GuidePremiseAssetCreate, GuidePremiseAssetMaintain, GuideStoryboardUpdate},
			ImageReferencePolicy: ImageReferenceNone,
		},
		{
			Key: ScenePremiseAsset, Scopes: []string{ThreadScopePremise},
			BasePromptKey: "base", ScenePromptKey: "scene_premise_asset",
			RecommendedGuideIDs:  []string{GuidePremiseAssetCreate},
			ImageReferencePolicy: ImageReferenceMessage,
		},
		{
			Key: SceneAssetReference, Scopes: []string{ThreadScopePremise},
			BasePromptKey: "base", ScenePromptKey: "scene_asset_reference",
			RecommendedGuideIDs: []string{GuidePremiseAssetCreate, GuidePremiseAssetMaintain},
			RequiresSubject:     true, SubjectType: SubjectTypePremiseAsset,
			ImageReferencePolicy: ImageReferenceBoundAssetFirst,
		},
		{
			Key: SceneStoryboardReference, Scopes: []string{ThreadScopeProject},
			BasePromptKey: "base", ScenePromptKey: "scene_storyboard_reference",
			RecommendedGuideIDs: []string{GuideStoryboardUpdate},
			RequiresSubject:     true, SubjectType: SubjectTypeComicSection, ImageReferencePolicy: ImageReferenceNone,
		},
	}
}

func sceneDefinitions() []SceneDefinition {
	return rawSceneDefinitions()
}

func logicalSceneKey(thread threadRecord) string {
	if isStoryboardReferenceThread(thread) {
		return SceneStoryboardReference
	}
	if thread.Scene == "" {
		return SceneProjectAssistant
	}
	return thread.Scene
}

func sceneDefinitionForThread(thread threadRecord) (SceneDefinition, bool) {
	key := logicalSceneKey(thread)
	for _, definition := range sceneDefinitions() {
		if definition.Key != key || !containsString(definition.Scopes, thread.Scope) {
			continue
		}
		if definition.RequiresSubject && !isUUIDv7(thread.SubjectUUID) {
			return SceneDefinition{}, false
		}
		if !definition.RequiresSubject && strings.TrimSpace(thread.SubjectUUID) != "" {
			return SceneDefinition{}, false
		}
		return definition, true
	}
	return SceneDefinition{}, false
}

func normalizedToolMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ToolModeProjectAPI:
		return ToolModeProjectAPI
	case ToolModeLegacyTyped:
		return ToolModeLegacyTyped
	default:
		return ""
	}
}

func toolAllowedForThreadMode(name string, thread threadRecord, mode string) bool {
	_, ok := sceneDefinitionForThread(thread)
	if !ok {
		return false
	}
	if normalizedToolMode(mode) == ToolModeLegacyTyped {
		return legacyRecoveryToolAllowed(name, thread)
	}
	return normalizedToolMode(mode) == ToolModeProjectAPI && containsString(projectAPIToolNames, name)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (service *Service) configuredToolMode(thread threadRecord) string {
	if _, ok := sceneDefinitionForThread(thread); !ok {
		return ""
	}
	return ToolModeProjectAPI
}
