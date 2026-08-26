package agent

import "strings"

const (
	legacySceneProjectAssistant = "project_assistant"
	ToolModeLegacyTyped         = "legacy_typed_tools"
	ToolModeProjectAPI          = "project_api_tools"
	ToolProtocolProjectAPI      = "project_api_v4"
	ToolProtocolProjectV3       = "project_api_v3"
	ToolProtocolProjectV2       = "project_api_v2"
)

var projectAPIToolNames = []string{"request_api", "read_agent_doc", "image_gen", "request_user_input"}

// legacyLogicalSceneKey exists only to resume frozen Scene-era runs. New
// threads and runs do not select a Scene or consult this discriminator.
func legacyLogicalSceneKey(thread threadRecord) string {
	if legacyStoryboardReferenceThread(thread) {
		return SceneStoryboardReference
	}
	if thread.Scene == "" {
		return legacySceneProjectAssistant
	}
	return thread.Scene
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
	return ToolModeProjectAPI
}
