package agent

import (
	"context"
	"embed"
	"encoding/json"
	"strings"
	"testing"

	"lumi/internal/llm"
	"lumi/internal/story"
)

//go:embed testdata/api-docs/*.md
var agentAPIDocGoldenFiles embed.FS

func TestSceneDefinitionsShareToolsAndRecommendRegisteredGuides(t *testing.T) {
	subject := mustAgentUUID(t)
	cases := []struct {
		name   string
		thread threadRecord
		key    string
		guides []string
	}{
		{name: "project assistant", thread: threadRecord{Scope: ThreadScopeProject}, key: SceneProjectAssistant, guides: []string{GuidePremiseAssetCreate, GuidePremiseAssetMaintain, GuideStoryboardUpdate}},
		{name: "premise asset generation", thread: threadRecord{Scope: ThreadScopePremise, Scene: ScenePremiseAsset}, key: ScenePremiseAsset, guides: []string{GuidePremiseAssetCreate}},
		{name: "asset reference", thread: threadRecord{Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: subject}, key: SceneAssetReference, guides: []string{GuidePremiseAssetCreate, GuidePremiseAssetMaintain}},
		{name: "storyboard reference", thread: threadRecord{Scope: ThreadScopeProject, Scene: SceneAssetReference, SubjectUUID: subject}, key: SceneStoryboardReference, guides: []string{GuideStoryboardUpdate}},
	}
	registeredGuides := map[string]bool{}
	for _, guide := range agentGuideDefinitions() {
		registeredGuides[guide.ID] = true
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition, ok := sceneDefinitionForThread(tc.thread)
			if !ok || definition.Key != tc.key || definition.BasePromptKey == "" || definition.ScenePromptKey == "" {
				t.Fatalf("definition=%+v ok=%v", definition, ok)
			}
			got := definitionNames(llmToolDefinitionsForMode(tc.thread, ToolModeProjectAPI))
			if strings.Join(got, ",") != strings.Join(projectAPIToolNames, ",") {
				t.Fatalf("project API tools=%v want=%v", got, projectAPIToolNames)
			}
			legacy := definitionNames(llmToolDefinitionsForMode(tc.thread, ToolModeLegacyTyped))
			if containsString(legacy, "request_api") || containsString(legacy, "read_agent_doc") {
				t.Fatalf("legacy mode exposed project API tools: %v", legacy)
			}
			if containsString(got, currentProjectAPIToolName) || containsString(got, "get_premise") || containsString(got, "get_comic_section") {
				t.Fatalf("project API mode exposed a duplicate legacy route: %v", got)
			}
			if strings.Join(definition.RecommendedGuideIDs, ",") != strings.Join(tc.guides, ",") {
				t.Fatalf("guide recommendations=%v want=%v", definition.RecommendedGuideIDs, tc.guides)
			}
			for _, guideID := range definition.RecommendedGuideIDs {
				if !registeredGuides[guideID] {
					t.Fatalf("Scene recommends unregistered Guide %s", guideID)
				}
			}
		})
	}
}

func definitionNames(definitions []llm.ToolDefinition) []string {
	result := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, definition.Name)
	}
	return result
}

func TestSceneToolContextCostBaseline(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	_, assetThread := createAssetReferenceMigrationFixture(t, harness)
	_, _, storyboardThread := createStoryboardMigrationFixture(t, harness)
	projectThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Project cost baseline", Scope: ThreadScopeProject, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	premiseThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Premise cost baseline", Scope: ThreadScopePremise, Scene: ScenePremiseAsset, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	record := func(thread Thread) threadRecord {
		t.Helper()
		var value threadRecord
		if err := harness.store.DB().WithContext(ctx).Where("uuid=?", thread.UUID).First(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	cases := []struct {
		scene     string
		thread    threadRecord
		toolCount int
	}{
		{scene: SceneProjectAssistant, thread: record(projectThread), toolCount: 4},
		{scene: ScenePremiseAsset, thread: record(premiseThread), toolCount: 4},
		{scene: SceneAssetReference, thread: record(assetThread), toolCount: 4},
		{scene: SceneStoryboardReference, thread: record(storyboardThread), toolCount: 4},
	}
	for _, test := range cases {
		definitions := llmToolDefinitionsForMode(test.thread, ToolModeProjectAPI)
		if len(definitions) != test.toolCount {
			t.Fatalf("scene=%s tools=%d want=%d", test.scene, len(definitions), test.toolCount)
		}
		prompts, err := loadContextPromptsForMode(ctx, harness.store, test.thread, ToolModeProjectAPI)
		if err != nil {
			t.Fatal(err)
		}
		definition, ok := sceneDefinitionForThread(test.thread)
		if !ok {
			t.Fatalf("scene=%s has no definition", test.scene)
		}
		recommended := make(map[string]bool, len(definition.RecommendedGuideIDs))
		for _, guideID := range definition.RecommendedGuideIDs {
			recommended[guideID] = true
		}
		for _, guide := range agentGuideDefinitions() {
			if strings.Contains(prompts.Scene, guide.Path) != recommended[guide.ID] {
				t.Fatalf("scene=%s guide=%s presence=%v want=%v", test.scene, guide.ID, strings.Contains(prompts.Scene, guide.Path), recommended[guide.ID])
			}
		}
		for _, forbidden := range []string{
			"默认没有 image_gen",
			"不开放 image_gen",
			"image_gen is unavailable",
			"image_gen is not available",
			"完整的新 Storyboard Markdown",
			"纯白、无纹理背景",
		} {
			if strings.Contains(prompts.Scene, forbidden) {
				t.Fatalf("scene=%s prompt contains legacy workflow or tool restriction %q", test.scene, forbidden)
			}
		}
		toolJSON, err := json.Marshal(definitions)
		if err != nil {
			t.Fatal(err)
		}
		promptJSON, err := json.Marshal(prompts)
		if err != nil {
			t.Fatal(err)
		}
		initialJSON, err := json.Marshal(map[string]any{"prompts": prompts, "tools": definitions})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("COST_BASELINE scene=%s mode=%s tools=%d tool_schema_bytes=%d prompt_bytes=%d initial_context_bytes=%d", test.scene, ToolModeProjectAPI, len(definitions), len(toolJSON), len(promptJSON), len(initialJSON))
	}
	for _, path := range registeredAgentDocPaths() {
		content, err := renderAgentDoc(path)
		if err != nil {
			t.Fatalf("render cost baseline doc %s: %v", path, err)
		}
		t.Logf("DOC_BASELINE path=%s bytes=%d", path, len(content))
	}
}

func TestRequestAPIRejectsUnregisteredCrossProjectAndNonCanonicalPaths(t *testing.T) {
	projectUUID, otherProjectUUID := mustAgentUUID(t), mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	validPath := "/api/v1/projects/" + projectUUID + "/story-profile"
	filter := ".data | {uuid,revision}"
	valid, err := parseAgentAPIRequest(tc, map[string]any{"method": "GET", "url": validPath, "response_filter": filter})
	if err != nil || valid.Route.ID != RouteStoryProfileGet {
		t.Fatalf("valid request=%+v err=%v", valid, err)
	}
	invalid := []string{
		"https://example.test" + validPath,
		"//api/v1/projects/" + projectUUID + "/story-profile",
		validPath + "?all=true",
		validPath + "#fragment",
		validPath + "/",
		"/api/v1/projects/" + projectUUID + "/%73tory-profile",
		"/api/v1/projects/" + projectUUID + "/../story-profile",
		"/api/v1/projects/" + projectUUID + "//story-profile",
		"/api/v1/projects/" + projectUUID + "\\story-profile",
		"/api/v1/projects/" + otherProjectUUID + "/story-profile",
		"/api/v1/projects/" + projectUUID + "/chat_threads",
	}
	for _, path := range invalid {
		if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "GET", "url": path, "response_filter": filter}); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
	if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "get", "url": validPath, "response_filter": filter}); err == nil {
		t.Fatal("lowercase method was accepted")
	}
	if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "GET", "url": validPath, "request_body": map[string]any{}, "response_filter": filter}); err == nil {
		t.Fatal("GET request body was accepted")
	}
	if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "PUT", "url": validPath, "request_body": map[string]any{"story_md": "# Story", "expected_revision": float64(0), "project_uuid": otherProjectUUID}, "response_filter": filter}); err == nil {
		t.Fatal("project_uuid in request body was accepted")
	}
	if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "PUT", "url": validPath, "request_body": map[string]any{"story_md": strings.Repeat("x", (256<<10)+1), "expected_revision": float64(0)}, "response_filter": filter}); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("oversized request body was accepted: %v", err)
	}
	raw := `{"url":"` + validPath + `","method":"PUT","request_body":{"story_md":"# Story","expected_revision":0,"internal_id":9},"response_filter":".data | {uuid,revision}"}`
	if _, err := validateToolArguments("request_api", raw); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("internal id was accepted: %v", err)
	}
	if err := validateAgentAPIResponse(map[string]any{"uuid": projectUUID, "internal_id": int64(9)}); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("internal response id was accepted: %v", err)
	}
}

func TestRequestAPIRoutesAndSubjectsAreSceneIndependent(t *testing.T) {
	projectUUID, subjectUUID, otherUUID := mustAgentUUID(t), mustAgentUUID(t), mustAgentUUID(t)
	base := "/api/v1/projects/" + projectUUID
	contexts := []toolContext{
		{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}},
		{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopePremise, Scene: ScenePremiseAsset}},
		{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: subjectUUID}},
		{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject, Scene: SceneAssetReference, SubjectUUID: subjectUUID}},
	}
	for _, tc := range contexts {
		for _, input := range []map[string]any{
			{"method": "GET", "url": base + "/story-profile", "response_filter": ".data | {uuid,revision}"},
			{"method": "GET", "url": base + "/premise-assets/" + otherUUID, "response_filter": ".data | {uuid,revision}"},
		} {
			if _, err := parseAgentAPIRequest(tc, input); err != nil {
				t.Fatalf("scene=%s rejected global route %v: %v", logicalSceneKey(tc.Thread), input, err)
			}
		}
		if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "GET", "url": base + "/not-registered", "response_filter": ".data | {uuid}"}); err == nil || errorCode(err) != CodeToolNotAllowed {
			t.Fatalf("scene=%s accepted unregistered route: %v", logicalSceneKey(tc.Thread), err)
		}
	}
}

func TestReadAgentDocUsesGlobalRegistryAcrossScenes(t *testing.T) {
	projectUUID, subjectUUID := mustAgentUUID(t), mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: subjectUUID}}
	for _, path := range registeredAgentDocPaths() {
		value, err := readAgentDoc(tc, map[string]any{"path": path})
		if err != nil || value["path"] != path || !strings.Contains(value["content"].(string), "#") || strings.Contains(value["content"].(string), "{{") {
			t.Fatalf("read %s value=%+v err=%v", path, value, err)
		}
	}
	for _, path := range []string{agentDocBasePath + "/missing.md", agentDocBasePath + "/scenes/asset_reference.md", agentDocBasePath + "/../../AGENTS.md", premiseAssetDocPath + "?raw=1", premiseAssetDocPath + "#x", agentDocBasePath + "/%70remise-asset.md", "/tmp/premise-asset.md"} {
		if _, err := readAgentDoc(tc, map[string]any{"path": path}); err == nil {
			t.Errorf("unauthorized doc accepted: %q", path)
		}
	}
	content := compactAgentDocContextResult(`{"success":true,"data":{"path":"` + premiseAssetDocPath + `","doc_ref":"` + premiseAssetDocPath + `","content":"large"}}`)
	if strings.Contains(content, "large") || !strings.Contains(content, `"compacted":true`) || !strings.Contains(content, premiseAssetDocPath) {
		t.Fatalf("doc context was not compacted: %s", content)
	}
}

func TestReadAgentDocEntryPointAndEmbeddedSourcesAreDiscoverable(t *testing.T) {
	subjectUUID := mustAgentUUID(t)
	thread := threadRecord{Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: subjectUUID}
	definitions := llmToolDefinitionsForMode(thread, ToolModeProjectAPI)
	var description string
	for _, definition := range definitions {
		if definition.Name == "read_agent_doc" {
			description = definition.Description
			break
		}
	}
	if !strings.Contains(description, agentDocOverviewPath) || strings.Contains(description, "/scenes/") {
		t.Fatalf("read_agent_doc entry point is not discoverable: %q", description)
	}

	for _, path := range registeredAgentDocPaths() {
		template, err := readAgentDocTemplate(path)
		if err != nil || strings.TrimSpace(template) == "" {
			t.Fatalf("embedded doc source missing for path=%s: %v", path, err)
		}
		content, err := renderAgentDoc(path)
		if err != nil || strings.Contains(content, "{{") || !strings.Contains(content, "#") || len(content) > maxAgentDocBytes {
			t.Fatalf("rendered doc invalid for path=%s: content=%q err=%v", path, content, err)
		}
	}
}

func TestOverviewIndexesMatchGuideAndAPIDocRegistries(t *testing.T) {
	content, err := renderAgentDoc(agentDocOverviewPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, guide := range agentGuideDefinitions() {
		if strings.Count(content, "`"+guide.ID+"`") != 1 || !strings.Contains(content, "`"+guide.Path+"`") {
			t.Fatalf("capability index missing Guide %s", guide.ID)
		}
	}
	for _, doc := range agentAPIDocDefinitions() {
		if strings.Count(content, "`"+doc.Path+"`") != 1 {
			t.Fatalf("API Contract index does not contain exactly one document: path=%s", doc.Path)
		}
	}
	for _, route := range agentAPIRoutes() {
		if strings.Contains(content, "`"+route.ID+"`") || strings.Contains(content, "`"+route.PathTemplate+"`") {
			t.Fatalf("overview leaked concrete route %s", route.ID)
		}
	}
	for _, heading := range []string{"capability_id", "说明", "所需工具", "上下文/输入前提", "Guide 路径", "API Contract 路径", "领域与用途"} {
		if !strings.Contains(content, heading) {
			t.Fatalf("overview missing index column %q", heading)
		}
	}
	if strings.Contains(content, "{{") || strings.Contains(content, "/scenes/") {
		t.Fatalf("overview contains unresolved or Scene-doc content: %s", content)
	}
}

func TestDetailedAPIDocsUseVACSStyleTablesAndProjectorFields(t *testing.T) {
	for _, route := range agentAPIRoutes() {
		projector, ok := agentAPIProjectorByKey(route.Projector)
		if !ok {
			t.Fatalf("route %s has no registered response projector", route.ID)
		}
		if projector.List {
			if _, ok := agentAPIProjectorByKey(projector.ItemProjector); !ok {
				t.Fatalf("list projector %s has no item projector %s", projector.Key, projector.ItemProjector)
			}
		}
	}

	content, err := renderAgentDoc(comicSectionDocPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, heading := range []string{"## 方法与路径", "## 路径参数", "## 请求体字段", "## 响应字段", "## 权限", "## 调用约束", "## 错误与调用示例"} {
		if !strings.Contains(content, heading) {
			t.Fatalf("detailed API doc missing %q: %s", heading, content)
		}
	}
	for _, field := range []string{"data.uuid", "data.chapter_uuid", "data.section_no", "data.current_storyboard", "data.revision"} {
		if !strings.Contains(content, "`"+field+"`") {
			t.Fatalf("detailed API doc missing projector field %s: %s", field, content)
		}
	}
	if strings.Contains(content, "Scene 范围") || strings.Contains(content, "当前 Scene") || strings.Contains(content, "Agent Scene 约束") {
		t.Fatalf("generic API detail doc contains Scene authorization text: %s", content)
	}
}

func TestComicSectionDocsMatchGoldenFiles(t *testing.T) {
	cases := []struct {
		scene      SceneDefinition
		goldenPath string
	}{
		{scene: sceneDefinitions()[0], goldenPath: "testdata/api-docs/project_assistant-comic-section.md"},
		{scene: sceneDefinitions()[3], goldenPath: "testdata/api-docs/storyboard_reference-comic-section.md"},
	}
	for _, tc := range cases {
		got, err := renderAgentDoc(comicSectionDocPath)
		if err != nil {
			t.Fatal(err)
		}
		want, err := agentAPIDocGoldenFiles.ReadFile(tc.goldenPath)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Fatalf("rendered doc differs from %s\n--- got ---\n%s\n--- want ---\n%s", tc.goldenPath, got, want)
		}
	}
}

func TestResponseFilterAllowsOnlyFiniteDataProjection(t *testing.T) {
	envelope := map[string]any{"success": true, "data": map[string]any{"items": []any{map[string]any{"uuid": "u1", "title": "one"}, map[string]any{"uuid": "u2", "title": "two"}}, "pagination": map[string]any{"total": float64(2)}}}
	value, err := runResponseFilter(envelope, ".data.items[] | {uuid,title}")
	items, ok := value.([]any)
	if err != nil || !ok || len(items) != 2 || items[1].(map[string]any)["uuid"] != "u2" {
		t.Fatalf("projection value=%+v err=%v", value, err)
	}
	value, err = runResponseFilter(envelope, ".data.items[0].title")
	if err != nil || value != "one" {
		t.Fatalf("indexed path value=%+v err=%v", value, err)
	}
	for _, expression := range []string{".items", ".data..items", ".data | map(.)", ".data | $x", ".data; system", ".data | {uuid", ".data.items[-1]"} {
		if _, err := runResponseFilter(envelope, expression); err == nil || errorCode(err) != CodeToolValidation {
			t.Errorf("unsafe filter accepted: %q err=%v", expression, err)
		}
	}
}

func TestRequestAPIRequiresNarrowResponseFilterForNewCalls(t *testing.T) {
	requiredFields := func(definitions []map[string]any) []string {
		t.Helper()
		for _, definition := range definitions {
			if definition["name"] != "request_api" {
				continue
			}
			parameters, _ := definition["parameters"].(map[string]any)
			required, _ := parameters["required"].([]string)
			return required
		}
		t.Fatal("request_api definition missing")
		return nil
	}
	if !containsString(requiredFields(toolDefinitions()), "response_filter") {
		t.Fatal("active request_api schema does not require response_filter")
	}
	if containsString(requiredFields(legacyRecoveryToolDefinitions()), "response_filter") {
		t.Fatal("legacy recovery unexpectedly requires response_filter")
	}
	projectUUID := mustAgentUUID(t)
	raw := `{"method":"GET","url":"/api/v1/projects/` + projectUUID + `/story-profile"}`
	if _, err := validateToolArguments("request_api", raw); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("missing response_filter was accepted: %v", err)
	}
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	for _, filter := range []string{"", ".data | map(.)"} {
		if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "GET", "url": "/api/v1/projects/" + projectUUID + "/story-profile", "response_filter": filter}); err == nil || errorCode(err) != CodeToolValidation {
			t.Fatalf("invalid response_filter %q was accepted: %v", filter, err)
		}
	}
	for _, route := range agentAPIRoutes() {
		filter := recommendedAgentAPIResponseFilter(route)
		if filter == "" || filter == ".data" {
			t.Errorf("route %s has broad recommended response_filter %q", route.ID, filter)
			continue
		}
		parsed, err := parseResponseFilter(filter)
		if err != nil {
			t.Errorf("route %s has invalid recommended response_filter %q: %v", route.ID, filter, err)
			continue
		}
		projector, ok := agentAPIProjectorByKey(route.Projector)
		if projector.List {
			projector, ok = agentAPIProjectorByKey(projector.ItemProjector)
		}
		if !ok || parsed.Projection == nil || len(parsed.Projection.Fields) == 0 {
			t.Errorf("route %s recommendation is not an object projection: %q", route.ID, filter)
			continue
		}
		allowed := map[string]bool{}
		for _, field := range projector.Fields {
			allowed[field.Name] = true
		}
		for _, field := range parsed.Projection.Fields {
			if !allowed[field.SourceKey] {
				t.Errorf("route %s recommends undocumented field %s in %q", route.ID, field.SourceKey, filter)
			}
		}
	}
}

func TestInvalidResponseFilterIsRejectedBeforeWriteRouteExecutes(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	tc := toolContext{
		ProjectUUID: harness.project.UUID,
		ToolMode:    ToolModeProjectAPI,
		Thread:      threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject},
	}
	_, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "invalid-filter-no-side-effect"}, map[string]any{
		"method": "POST",
		"url":    "/api/v1/projects/" + harness.project.UUID + "/chapters",
		"request_body": map[string]any{
			"chapter_code": "invalid-filter",
			"title":        "Must not be created",
		},
		"response_filter": ".data | map(.)",
	})
	if err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("invalid response_filter was not rejected: %v", err)
	}
	chapters, listErr := story.NewService(harness.store).ListChapters(ctx, "active")
	if listErr != nil || len(chapters) != 0 {
		t.Fatalf("write route ran before response_filter validation: chapters=%+v err=%v", chapters, listErr)
	}
}

func TestPersistedPreUpgradeRequestAPIIntentFallsBackToCompleteCompactResponse(t *testing.T) {
	harness := newAgentHarness(t)
	tc := toolContext{
		ProjectUUID: harness.project.UUID,
		ToolMode:    ToolModeProjectAPI,
		Thread:      threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject},
	}
	arguments, _ := json.Marshal(map[string]any{
		"method": "GET",
		"url":    "/api/v1/projects/" + harness.project.UUID + "/story-profile",
	})
	result, err := harness.service.executeTool(context.Background(), harness.store, tc, toolExecutionRecord{
		UUID:           mustAgentUUID(t),
		ToolName:       "request_api",
		ArgumentsJSON:  string(arguments),
		IdempotencyKey: "pre-response-filter-upgrade",
	})
	if err != nil || !strings.Contains(string(result), `"success":true`) {
		t.Fatalf("pre-upgrade intent was not recovered: result=%s err=%v", result, err)
	}
}

func TestProjectAPIToolModeRunsInProcessAndRecordsRouteAction(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Project API", Scope: ThreadScopeProject, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取故事档案"})
	if err != nil {
		t.Fatal(err)
	}
	harness.model.respond = func(call int, request llm.ChatRequest) (llm.ChatResponse, error) {
		if call == 1 {
			got := definitionNames(request.Tools)
			want := []string{"request_api", "read_agent_doc", "image_gen", "request_user_input"}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("runtime tools=%v want=%v", got, want)
			}
			arguments, _ := json.Marshal(map[string]any{"url": "/api/v1/projects/" + harness.project.UUID + "/story-profile", "method": "GET", "response_filter": ".data | {uuid,revision}"})
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "request-story", Name: "request_api", Arguments: string(arguments)}}}, FinishReason: "tool_calls"}, nil
		}
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "已读取。"}, FinishReason: "stop"}, nil
	}
	if err := harness.service.ExecuteJob(ctx, harness.store, JobSpec{Version: 1, ProjectUUID: harness.project.UUID, JobKind: JobChatTurn, ResourceUUID: turn.UUID, ThreadUUID: thread.UUID}); err != nil {
		t.Fatal(err)
	}
	var row struct {
		ToolName      string
		ArgumentsJSON string
		ResultJSON    string
	}
	if err := harness.store.DB().Table("agent_tool_executions").Select("tool_name,arguments_json,result_json").Where("tool_name='request_api'").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(row.ArgumentsJSON, harness.project.UUID) || !strings.Contains(row.ArgumentsJSON, RouteStoryProfileGet) || !strings.Contains(row.ArgumentsJSON, "读取故事档案") || !strings.Contains(row.ResultJSON, `"success":true`) || strings.Contains(row.ResultJSON, "story_md") {
		t.Fatalf("execution row=%+v", row)
	}
	var metadata string
	if err := harness.store.DB().Table("chat_items").Select("metadata_json").Where("item_type='tool_call' AND tool_name='request_api'").Take(&metadata).Error; err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{RouteStoryProfileGet, "读取故事档案", `"method":"GET"`, `"scene":"project_assistant"`} {
		if !strings.Contains(metadata, expected) {
			t.Fatalf("tool metadata missing %q: %s", expected, metadata)
		}
	}
	var logResponse string
	if err := harness.store.DB().Table("llm_logs").Select("response").Where("chat_run_id=(SELECT runs.id FROM chat_runs runs JOIN chat_turns turns ON turns.id=runs.turn_id WHERE turns.uuid=?)", turn.UUID).Order("id").Limit(1).Scan(&logResponse).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logResponse, `"agent_tool_routes"`) || !strings.Contains(logResponse, RouteStoryProfileGet) {
		t.Fatalf("LLM log lacks parsed route metadata: %s", logResponse)
	}
	var promptSnapshot string
	if err := harness.store.DB().Table("chat_items").Select("metadata_json").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='user_message'", turn.UUID).Take(&promptSnapshot).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(promptSnapshot, `"tool_mode":"project_api_tools"`) || !strings.Contains(promptSnapshot, "request_api") {
		t.Fatalf("Run did not freeze the new prompt/mode: %s", promptSnapshot)
	}
}

func TestProjectAPIV2RejectsOldProtocolAndToolName(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Protocol v2", Scope: ThreadScopeProject, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "检查协议"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	oldProtocol := "project_api_" + "v1"
	if err := harness.store.DB().Exec(`UPDATE chat_items SET metadata_json=json_set(metadata_json,'$.prompt_snapshot.tool_protocol',?) WHERE run_id=? AND turn_id=? AND item_type='user_message'`, oldProtocol, tc.Run.ID, tc.Turn.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.loadRunToolMode(ctx, harness.store, tc); err == nil || errorCode(err) != CodeToolNotAllowed {
		t.Fatalf("old Project API protocol was restored: %v", err)
	}
	if err := harness.store.DB().Exec(`UPDATE chat_items SET metadata_json=json_set(metadata_json,'$.prompt_snapshot.tool_protocol',?) WHERE run_id=? AND turn_id=? AND item_type='user_message'`, ToolProtocolProjectAPI, tc.Run.ID, tc.Turn.ID).Error; err != nil {
		t.Fatal(err)
	}
	if mode, err := harness.service.loadRunToolMode(ctx, harness.store, tc); err != nil || mode != ToolModeProjectAPI {
		t.Fatalf("v2 protocol mode=%q err=%v", mode, err)
	}
	oldToolName := "read_" + "api_doc"
	if _, err := validateToolArgumentsForMode(oldToolName, `{"path":"`+agentDocOverviewPath+`"}`, ToolModeProjectAPI); err == nil || errorCode(err) != CodeToolNotAllowed {
		t.Fatalf("old documentation tool name was accepted: %v", err)
	}
}

func TestProjectAPIToolIntentRecoversModeAndRouteAfterRestart(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Recover API intent", Scope: ThreadScopeProject, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复读取"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, tc)
	if err != nil || tc.ToolMode != ToolModeProjectAPI {
		t.Fatalf("initial tool mode=%q err=%v", tc.ToolMode, err)
	}
	arguments, _ := json.Marshal(map[string]any{"url": "/api/v1/projects/" + harness.project.UUID + "/story-profile", "method": "GET", "response_filter": ".data | {uuid,revision}"})
	execution, _, completed, err := harness.service.persistToolIntent(ctx, harness.store, tc, "recover-call", "request_api", string(arguments))
	if err != nil || completed || execution.RouteID != RouteStoryProfileGet {
		t.Fatalf("persist execution=%+v completed=%v err=%v", execution, completed, err)
	}

	// Simulate a process restart: only the frozen Run snapshot and persisted
	// intent remain; the current Service configuration no longer selects a mode.
	recovered, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	recovered.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, recovered)
	if err != nil || recovered.ToolMode != ToolModeProjectAPI {
		t.Fatalf("recovered tool mode=%q err=%v", recovered.ToolMode, err)
	}
	pending, ok, err := harness.service.pendingTool(ctx, harness.store, recovered.Run.ID)
	if err != nil || !ok || pending.RouteID != "" {
		t.Fatalf("pending=%+v ok=%v err=%v", pending, ok, err)
	}
	result, err := harness.service.executeTool(ctx, harness.store, recovered, pending)
	if err != nil || !strings.Contains(string(result), `"success":true`) {
		t.Fatalf("recovered execution result=%s err=%v", result, err)
	}
	if err := harness.service.persistToolResult(ctx, harness.store, recovered, pending, result); err != nil {
		t.Fatal(err)
	}
	var metadata string
	if err := harness.store.DB().Table("chat_items").Select("metadata_json").Where("item_type='tool_result' AND tool_name='request_api'").Take(&metadata).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadata, RouteStoryProfileGet) || !strings.Contains(metadata, "读取故事档案") {
		t.Fatalf("recovered route metadata missing: %s", metadata)
	}
}

func TestRequestUserInputMixedWithAnotherToolIsRejectedBeforeIntent(t *testing.T) {
	harness := newAgentHarness(t, llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
		{ID: "ask", Name: "request_user_input", Arguments: `{"input_type":"single_choice","question":"继续吗？","options":[{"label":"继续"},{"label":"取消"}]}`},
		{ID: "write", Name: "update_story_profile", Arguments: `{"story_md":"# 不应写入","expected_revision":0}`},
	}}, FinishReason: "tool_calls"})
	ctx := context.Background()
	before, err := story.NewService(harness.store).GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Mixed tools", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "测试混合工具"})
	if err != nil {
		t.Fatal(err)
	}
	err = harness.service.ExecuteJob(ctx, harness.store, JobSpec{Version: 1, ProjectUUID: harness.project.UUID, JobKind: JobChatTurn, ResourceUUID: turn.UUID, ThreadUUID: thread.UUID})
	if err != nil {
		t.Fatal(err)
	}
	var runStatus, runError string
	if dbErr := harness.store.DB().Table("chat_runs AS runs").Select("runs.status,runs.error_code").Joins("JOIN chat_turns turns ON turns.id=runs.turn_id").Where("turns.uuid=?", turn.UUID).Row().Scan(&runStatus, &runError); dbErr != nil {
		t.Fatal(dbErr)
	}
	if runStatus != TurnFailed || runError != CodeToolValidation {
		t.Fatalf("mixed tool response status=%s error=%s", runStatus, runError)
	}
	var executions, requests int64
	if dbErr := harness.store.DB().Table("agent_tool_executions").Where("run_id=(SELECT runs.id FROM chat_runs runs JOIN chat_turns turns ON turns.id=runs.turn_id WHERE turns.uuid=?)", turn.UUID).Count(&executions).Error; dbErr != nil {
		t.Fatal(dbErr)
	}
	if dbErr := harness.store.DB().Table("chat_user_input_requests").Where("run_id=(SELECT runs.id FROM chat_runs runs JOIN chat_turns turns ON turns.id=runs.turn_id WHERE turns.uuid=?)", turn.UUID).Count(&requests).Error; dbErr != nil {
		t.Fatal(dbErr)
	}
	after, err := story.NewService(harness.store).GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if executions != 0 || requests != 0 || after.UUID != before.UUID || after.Revision != before.Revision {
		t.Fatalf("side effect occurred: executions=%d requests=%d before=%+v after=%+v", executions, requests, before, after)
	}
}

func TestAgentProjectAPIRouteExecutionKeepsRevisionAndIdempotencySemantics(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "API execution", Scope: ThreadScopeProject, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "更新故事"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	profile, err := story.NewService(harness.store).GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"url": "/api/v1/projects/" + harness.project.UUID + "/story-profile", "method": "PUT", "request_body": map[string]any{"story_md": "# 新故事", "expected_revision": float64(profile.Revision)}, "response_filter": ".data | {uuid,revision}"}
	execution := toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "request_api", IdempotencyKey: "agent-api-revision"}
	value, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, execution, args)
	if err != nil || value.(map[string]any)["revision"] == nil {
		t.Fatalf("update value=%+v err=%v", value, err)
	}
	replayed, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, execution, args)
	if err != nil || replayed.(map[string]any)["uuid"] != value.(map[string]any)["uuid"] {
		t.Fatalf("idempotent replay=%+v err=%v", replayed, err)
	}
	stale := map[string]any{"url": args["url"], "method": "PUT", "request_body": map[string]any{"story_md": "# 冲突内容", "expected_revision": float64(profile.Revision)}, "response_filter": args["response_filter"]}
	if _, err := executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), ToolName: "request_api", IdempotencyKey: "stale"}, stale); err == nil {
		t.Fatal("stale expected_revision was accepted")
	}
}
