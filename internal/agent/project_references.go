package agent

import (
	"fmt"
	"regexp"
	"strings"
)

const projectReferencePrefix = "@project"

var canonicalUUIDv7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type agentUIReference struct {
	Href string `json:"href"`
}

type requestAPIToolOutput struct {
	Data  any
	UIRef *agentUIReference
}

func projectReferenceForAgentAPI(request agentAPIRequest, publicValue any) *agentUIReference {
	if request.Method == "GET" {
		return nil
	}

	static := func(path string) *agentUIReference {
		return &agentUIReference{Href: projectReferencePrefix + "/" + path}
	}
	chapter := func(chapterUUID string) *agentUIReference {
		if !isCanonicalUUIDv7(chapterUUID) {
			return nil
		}
		return static("chapters/" + chapterUUID)
	}
	body := func(chapterUUID string) *agentUIReference {
		if !isCanonicalUUIDv7(chapterUUID) {
			return nil
		}
		return static("chapters/" + chapterUUID + "/body")
	}
	section := func(chapterUUID, sectionUUID string) *agentUIReference {
		if !isCanonicalUUIDv7(chapterUUID) || !isCanonicalUUIDv7(sectionUUID) {
			return nil
		}
		return static(fmt.Sprintf("chapters/%s/sections/%s", chapterUUID, sectionUUID))
	}
	asset := func(assetUUID string) *agentUIReference {
		if !isCanonicalUUIDv7(assetUUID) {
			return nil
		}
		return static("premise/assets/" + assetUUID)
	}
	workflow := func(workflowUUID string) *agentUIReference {
		if !isCanonicalUUIDv7(workflowUUID) {
			return nil
		}
		return static("workflows/" + workflowUUID)
	}

	switch request.Route.ID {
	case RouteYoloWorkflowCreate:
		return workflow(publicResponseUUID(publicValue))
	case RouteStoryProfileUpdate,
		RouteStoryProfileImport,
		RouteStoryProfileRegenerate,
		RouteStoryProfileGenerationCreate,
		RouteStoryProfileRebuildCreate:
		return static("story-profile")

	case RoutePremiseUpdate,
		RoutePremiseSourceCreate,
		RoutePremiseSourceUpdate,
		RouteSettingImageImport,
		RouteSettingImageSelect,
		RoutePremiseSettingGenerationCreate,
		RoutePremiseBreakdownCreate:
		return static("premise")

	case RoutePremiseAssetCreate:
		return asset(publicResponseUUID(publicValue))
	case RoutePremiseAssetUpdate,
		RoutePremiseAssetDelete,
		RoutePremiseAssetRestore,
		RoutePremiseAssetVariantCreate,
		RoutePremiseAssetVariantSelect:
		return asset(request.Params["premise_asset_uuid"])

	case RouteChapterCreate:
		chapterUUID := publicResponseUUID(publicValue)
		if strings.TrimSpace(stringArg(request.Body, "content")) != "" {
			return body(chapterUUID)
		}
		return chapter(chapterUUID)
	case RouteChapterUpdate, RouteChapterTrash, RouteChapterRestore:
		return chapter(request.Params["chapter_uuid"])
	case RouteChapterStoryUpdate, RouteChapterGenerationCreate:
		return body(request.Params["chapter_uuid"])
	case RouteComicStoryboardGenerationCreate,
		RouteComicSectionDelete,
		RouteComicSectionReorder,
		RouteComicImageGenerationBatchCreate,
		RouteComicSnapshotRestore:
		return chapter(request.Params["chapter_uuid"])

	case RouteComicSectionCreate:
		return section(request.Params["chapter_uuid"], publicResponseUUID(publicValue))
	case RouteComicSectionUpdate,
		RouteStoryboardUpdate,
		RouteStoryboardSelect,
		RouteComicSectionImageImport,
		RouteComicImageVariantSelect,
		RouteComicImageGenerationCreate:
		return section(request.Params["chapter_uuid"], request.Params["section_uuid"])

	case RouteComicExportCreate:
		return static("exports")
	default:
		return nil
	}
}

func publicResponseUUID(value any) string {
	object, _ := value.(map[string]any)
	uuidValue, _ := object["uuid"].(string)
	return uuidValue
}

func isCanonicalUUIDv7(value string) bool {
	return canonicalUUIDv7Pattern.MatchString(value) && isUUIDv7(value)
}
