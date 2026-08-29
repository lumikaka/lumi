package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentprompts "lumi/internal/agent/prompts"
	"lumi/internal/llm"
	"lumi/internal/story"
)

func TestProjectAPIToolsAndGuidesAreGloballyRegistered(t *testing.T) {
	thread := threadRecord{}
	registeredGuides := map[string]bool{}
	guides := agentGuideDefinitions()
	for _, guide := range guides {
		registeredGuides[guide.Path] = true
	}
	got := definitionNames(llmToolDefinitionsForMode(thread, ToolModeProjectAPI))
	if strings.Join(got, ",") != strings.Join(projectAPIToolNames, ",") {
		t.Fatalf("project API tools=%v want=%v", got, projectAPIToolNames)
	}
	if containsString(got, currentProjectAPIToolName) || containsString(got, "get_premise") || containsString(got, "get_comic_section") {
		t.Fatalf("project API mode exposed a duplicate legacy route: %v", got)
	}
	expectedGuidePaths := []string{
		agentDocBasePath + "/guides/初始化新项目.md",
		agentDocBasePath + "/guides/管理故事总纲.md",
		agentDocBasePath + "/guides/创建章节.md",
		agentDocBasePath + "/guides/修改章节.md",
		agentDocBasePath + "/guides/管理章节回收站.md",
		agentDocBasePath + "/guides/生成项目设定.md",
		agentDocBasePath + "/guides/创建设定资产.md",
		agentDocBasePath + "/guides/维护设定资产.md",
		agentDocBasePath + "/guides/管理设定资产回收站.md",
		agentDocBasePath + "/guides/生成漫画分镜.md",
		agentDocBasePath + "/guides/管理漫画段落.md",
		agentDocBasePath + "/guides/编辑与选择漫画分镜.md",
		agentDocBasePath + "/guides/生成导入与选择漫画图片.md",
		agentDocBasePath + "/guides/恢复漫画快照.md",
		agentDocBasePath + "/guides/导出漫画.md",
	}
	if len(guides) != len(expectedGuidePaths) || len(registeredGuides) != len(expectedGuidePaths) {
		t.Fatalf("global guide count=%d unique=%d want=%d", len(guides), len(registeredGuides), len(expectedGuidePaths))
	}
	for _, path := range expectedGuidePaths {
		if !registeredGuides[path] {
			t.Fatalf("global guide registry is missing %s", path)
		}
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
	projectThread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Project cost baseline", ProviderUUID: harness.provider.UUID})
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
	}{{scene: "project_chat", thread: record(projectThread), toolCount: 4}}
	for _, test := range cases {
		definitions := llmToolDefinitionsForMode(test.thread, ToolModeProjectAPI)
		if len(definitions) != test.toolCount {
			t.Fatalf("scene=%s tools=%d want=%d", test.scene, len(definitions), test.toolCount)
		}
		prompts, err := loadContextPromptsForMode(ctx, harness.store, test.thread, ToolModeProjectAPI)
		if err != nil {
			t.Fatal(err)
		}
		for _, guide := range agentGuideDefinitions() {
			if strings.Contains(prompts.Scene, guide.Path) {
				t.Fatalf("project chat system prompt contains scene guide %s", guide.Path)
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
				t.Fatalf("thread rejected global route %v: %v", input, err)
			}
		}
		if _, err := parseAgentAPIRequest(tc, map[string]any{"method": "GET", "url": base + "/not-registered", "response_filter": ".data | {uuid}"}); err == nil || errorCode(err) != CodeToolNotAllowed {
			t.Fatalf("thread accepted unregistered route: %v", err)
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
	for _, path := range []string{
		agentDocBasePath + "/missing.md",
		agentDocBasePath + "/scenes/asset_reference.md",
		agentDocBasePath + "/../../AGENTS.md",
		premiseAssetDocPath + "?raw=1",
		premiseAssetDocPath + "#x",
		agentDocBasePath + "/%70remise-asset.md",
		agentDocBasePath + "/guides/%E5%88%9B%E5%BB%BA%E8%AE%BE%E5%AE%9A%E8%B5%84%E4%BA%A7.md",
		agentDocBasePath + "/guides/premise-asset-create.md",
		agentDocBasePath + "/guides/premise-asset-maintain.md",
		agentDocBasePath + "/guides/storyboard-update.md",
		"/tmp/premise-asset.md",
	} {
		if _, err := readAgentDoc(tc, map[string]any{"path": path}); err == nil {
			t.Errorf("unauthorized doc accepted: %q", path)
		}
	}
}

func TestContextMessagesKeepAllReadAgentDocResults(t *testing.T) {
	first := `{"success":true,"data":{"path":"/api/v1/agent-docs/guides/生成项目设定.md","content":"FIRST DOC IMPORTANT RULE"}}`
	second := `{"success":true,"data":{"path":"/api/v1/agent-docs/api/generation.md","content":"SECOND DOC API CONTRACT"}}`
	items := []contextItem{
		{itemRecord: itemRecord{Sequence: 1, ItemType: "tool_result", Role: "tool", ToolName: "read_agent_doc", Content: first, MetadataJSON: `{"provider_call_id":"first-doc"}`}},
		{itemRecord: itemRecord{Sequence: 2, ItemType: "tool_result", Role: "tool", ToolName: "read_agent_doc", Content: second, MetadataJSON: `{"provider_call_id":"second-doc"}`}},
	}
	messages := contextMessages(items, "", int64(0), contextPromptSet{Assistant: "BASE", APIOverview: "OVERVIEW", ProjectUUID: mustAgentUUID(t), ToolProtocol: ToolProtocolProjectAPI})
	if len(messages) != 3 || messages[1].Content != first || messages[2].Content != second {
		t.Fatalf("read_agent_doc results were not preserved: %+v", messages)
	}
	if strings.Contains(messages[1].Content, `"compacted":true`) || strings.Contains(messages[2].Content, `"compacted":true`) {
		t.Fatalf("read_agent_doc result was replaced by a compact reference: %+v", messages)
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

func TestAPIDocsAreMaintainedAsConciseStaticMarkdown(t *testing.T) {
	for _, definition := range agentAPIDocDefinitions() {
		source, err := readAgentDocTemplate(definition.Path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(source, "{{") {
			t.Fatalf("API Contract %s still contains a template token", definition.Path)
		}
		if len(source) > 6000 {
			t.Fatalf("API Contract %s is not concise: %d bytes", definition.Path, len(source))
		}
		for _, unnecessary := range []string{"## Query 字段", "## 响应字段", "## 权限", "route_id"} {
			if strings.Contains(source, unnecessary) {
				t.Fatalf("API Contract %s retains generated detail %q", definition.Path, unnecessary)
			}
		}
		for _, route := range routesForAgentDoc(definition.Path) {
			operation := "`" + route.Method + " " + route.PathTemplate + "`"
			if !strings.Contains(source, operation) {
				t.Fatalf("static API Contract %s missing operation %s", definition.Path, operation)
			}
			for _, schema := range []map[string]any{route.QuerySchema, route.BodySchema} {
				requiredFields, _ := schema["required"].([]string)
				for _, field := range requiredFields {
					if !strings.Contains(source, field) {
						t.Fatalf("static API Contract %s missing required field %s for %s", definition.Path, field, operation)
					}
				}
			}
			if route.RequiresConfirmation && !strings.Contains(source, "确认") {
				t.Fatalf("static API Contract %s omits confirmation for %s", definition.Path, operation)
			}
		}

		rendered, err := renderAgentDocWithRoutes(definition.Path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if rendered != strings.TrimSpace(source)+"\n" {
			t.Fatalf("API Contract %s was changed by runtime route rendering", definition.Path)
		}
	}

	chapterSource, err := readAgentDocTemplate(chapterDocPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"`POST /api/v1/projects/{project_uuid}/chapters`",
		"`POST /api/v1/projects/{project_uuid}/chapters/{chapter_uuid}/generations`",
		"`prompt_key`",
		"`story_chapter`",
		"`next_story_chapter`",
		"`chapter_code`",
		"`title`",
		`{"chapter_code":"vol01.ch01","title":"第一章","content":"...","content_format":"md"}`,
		`{"expected_revision":3}`,
		`{"prompt_key":"story_chapter","prompt":"生成本章正文"}`,
	} {
		if !strings.Contains(chapterSource, required) {
			t.Fatalf("static Chapter Contract missing %q", required)
		}
	}
	if strings.Contains(chapterSource, `"model":"可选"`) {
		t.Fatal("Chapter Contract contains a placeholder that could be sent literally")
	}
	chapterProjector, _ := agentAPIProjectorByKey("chapter")
	if !containsString(agentAPIProjectorFieldNames(chapterProjector), "trashed_at") {
		t.Fatal("Chapter Contract documents trashed_at but the compact response omits it")
	}
}

func TestOverviewIndexesMatchGuideAndAPIDocRegistries(t *testing.T) {
	content, err := renderAgentDoc(agentDocOverviewPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(agentGuideDefinitions()) != 15 {
		t.Fatalf("guide count=%d want=15", len(agentGuideDefinitions()))
	}
	for _, guide := range agentGuideDefinitions() {
		if strings.Count(content, "`"+guide.Path+"`") != 1 {
			t.Fatalf("capability index missing Guide %s", guide.Path)
		}
	}
	for _, doc := range agentAPIDocDefinitions() {
		if strings.Count(content, "`"+doc.Path+"`") != 1 {
			t.Fatalf("API Contract index does not contain exactly one document: path=%s", doc.Path)
		}
	}
	if strings.Contains(content, "`"+agentDocOverviewPath+"`") {
		t.Fatal("overview indexes itself as an API Contract")
	}
	for _, route := range agentAPIRoutes() {
		if strings.Contains(content, "`"+route.ID+"`") || strings.Contains(content, "`"+route.PathTemplate+"`") {
			t.Fatalf("overview leaked concrete route %s", route.ID)
		}
	}
	for _, heading := range []string{"Guide", "说明", "所需工具", "上下文/输入前提", "API Contract 路径", "领域与用途"} {
		if !strings.Contains(content, heading) {
			t.Fatalf("overview missing index column %q", heading)
		}
	}
	if strings.Contains(content, "capability_id") {
		t.Fatal("overview still exposes redundant capability IDs")
	}
	if strings.Contains(content, "{{") || strings.Contains(content, "/scenes/") {
		t.Fatalf("overview contains unresolved or Scene-doc content: %s", content)
	}
}

func TestGuidesAreConciseAndReferenceRegisteredAPIContracts(t *testing.T) {
	registeredAPIDocs := map[string]bool{}
	for _, doc := range agentAPIDocDefinitions() {
		registeredAPIDocs[doc.Path] = true
	}
	seenPaths := map[string]bool{}
	for _, guide := range agentGuideDefinitions() {
		if seenPaths[guide.Path] {
			t.Fatalf("duplicate Guide path: %s", guide.Path)
		}
		seenPaths[guide.Path] = true
		content, err := renderAgentDoc(guide.Path)
		if err != nil {
			t.Fatalf("render Guide %s: %v", guide.Path, err)
		}
		maximumBytes := 2048
		if guide.Path == agentDocBasePath+"/guides/初始化新项目.md" {
			maximumBytes = 4096
		}
		if len(content) > maximumBytes {
			t.Fatalf("Guide %s is not concise: %d bytes (maximum %d)", guide.Path, len(content), maximumBytes)
		}
		if strings.Count(content, "## API 调用顺序和说明") != 1 {
			t.Fatalf("Guide %s missing fixed API order section: %s", guide.Path, content)
		}
		readIndex, requestIndex := strings.Index(content, "read_agent_doc"), strings.Index(content, "request_api")
		if readIndex < 0 || requestIndex < 0 || readIndex > requestIndex {
			t.Fatalf("Guide %s does not put API Contract reading before request_api", guide.Path)
		}
		contractCount := 0
		for index, token := range strings.Split(content, "`") {
			if index%2 == 0 || !strings.HasPrefix(token, agentDocAPIBasePath+"/") || !strings.HasSuffix(token, ".md") {
				continue
			}
			contractCount++
			if !registeredAPIDocs[token] {
				t.Fatalf("Guide %s references unregistered API Contract %s", guide.Path, token)
			}
		}
		if contractCount == 0 {
			t.Fatalf("Guide %s does not reference an API Contract", guide.Path)
		}
		if strings.Contains(content, "## Workflow API") {
			t.Fatalf("Guide %s added a Workflow API section", guide.Path)
		}
		if strings.Contains(content, "能力 ID") {
			t.Fatalf("Guide %s still exposes a redundant capability ID", guide.Path)
		}
	}
	for _, path := range []string{
		agentDocBasePath + "/guides/运行快速创作工作流.md",
		agentDocBasePath + "/guides/premise-asset-create.md",
		agentDocBasePath + "/guides/premise-asset-maintain.md",
		agentDocBasePath + "/guides/storyboard-update.md",
	} {
		if seenPaths[path] {
			t.Fatalf("removed Guide path remains registered: %s", path)
		}
	}
}

func TestBootstrapInitializationGuideDefinesControlledYoloBoundary(t *testing.T) {
	guide, err := renderAgentDoc(agentDocBasePath + "/guides/初始化新项目.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		workflowDocPath,
		bootstrapYoloConfirmationQuestionID,
		"Setup Draft",
		"不得创建、选择或切换 Candidate",
		"1～3 个相互关联的问题",
		"vol01.ch01",
		"默认生成封面和第一个正文页的成品图",
		"vertical_strip",
		"不得退化为手工生产",
		"立即结束当前 Turn",
	} {
		if !strings.Contains(guide, required) {
			t.Fatalf("bootstrap Guide missing %q: %s", required, guide)
		}
	}
	setupDoc, err := renderAgentDoc(projectSetupDocPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Setup Draft", "`draft_values`", "`expected_revision`"} {
		if !strings.Contains(setupDoc, required) {
			t.Fatalf("Project Setup Contract missing %q: %s", required, setupDoc)
		}
	}
	if strings.Contains(setupDoc, "`candidate`") || strings.Contains(setupDoc, "候选项目设置") {
		t.Fatalf("Project Setup Contract retained candidate modeling: %s", setupDoc)
	}
	workflowDoc, err := renderAgentDoc(workflowDocPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"`POST /api/v1/projects/{project_uuid}/workflows`", "creation_session_uuid", "dedicated Thread", "不得轮询状态"} {
		if !strings.Contains(workflowDoc, required) {
			t.Fatalf("Workflow Contract missing %q: %s", required, workflowDoc)
		}
	}
}

func TestPremiseAssetGuidesRouteBatchCreationThroughSettingWorkflow(t *testing.T) {
	createGuide, err := renderAgentDoc(agentDocBasePath + "/guides/创建设定资产.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"立即停止本 Guide",
		"禁止调用 `image_gen`",
		"禁止循环调用设定资产 API",
		agentDocBasePath + "/guides/生成项目设定.md",
	} {
		if !strings.Contains(createGuide, required) {
			t.Fatalf("create asset Guide missing batch guard %q: %s", required, createGuide)
		}
	}

	batchGuide, err := renderAgentDoc(agentDocBasePath + "/guides/生成项目设定.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		premiseDocPath,
		generationDocPath,
		taskDocPath,
		"只创建一个 Premise Source",
		"只创建一次 Premise 设定图任务",
		"等待用户确认",
		"再创建一次 Premise 拆解任务",
	} {
		if !strings.Contains(batchGuide, required) {
			t.Fatalf("batch premise Guide missing workflow instruction %q: %s", required, batchGuide)
		}
	}
	if readIndex, requestIndex := strings.Index(batchGuide, "read_agent_doc"), strings.Index(batchGuide, "request_api"); readIndex < 0 || requestIndex < 0 || readIndex > requestIndex {
		t.Fatalf("batch premise Guide requests API before reading Contracts: %s", batchGuide)
	}
}

func TestComicImageGuideRequiresOneBatchRequestForMultipleSections(t *testing.T) {
	guide, err := renderAgentDoc(agentDocBasePath + "/guides/生成导入与选择漫画图片.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"只允许调用一次 Section 列表接口",
		"comic-image-generation-batches",
		"禁止循环调用单图接口",
		"禁止使用通用 `image_gen`",
		"冻结目标的 `page_role`",
		"角色漂移",
		"只表示图片任务已创建",
	} {
		if !strings.Contains(guide, required) {
			t.Fatalf("comic image Guide missing batch rule %q: %s", required, guide)
		}
	}
}

func TestComicSectionPageRoleIsExposedByReviewedAgentContract(t *testing.T) {
	projector, ok := agentAPIProjectorByKey("comic_section")
	if !ok || !containsString(agentAPIProjectorFieldNames(projector), "page_role") || !containsString(projector.RecommendedFields, "page_role") {
		t.Fatalf("comic section projector does not expose page_role: %+v", projector)
	}

	routes := map[string]agentAPIRoute{}
	for _, route := range agentAPIRoutes() {
		routes[route.ID] = route
	}
	for _, routeID := range []string{RouteComicSectionGet, RouteComicSectionList, RouteComicSectionCreate, RouteComicSectionUpdate, RouteStoryboardUpdate, RouteStoryboardSelect, RouteComicSnapshotRestore} {
		route := routes[routeID]
		if route.ID == "" || !strings.Contains(recommendedAgentAPIResponseFilter(route), "page_role") {
			t.Fatalf("route %s does not recommend page_role: %+v filter=%q", routeID, route, recommendedAgentAPIResponseFilter(route))
		}
	}

	wantRoles := "front_cover,body,back_cover"
	for _, routeID := range []string{RouteComicSectionCreate, RouteComicSectionUpdate} {
		properties, _ := routes[routeID].BodySchema["properties"].(map[string]any)
		roleSchema, _ := properties["page_role"].(map[string]any)
		roles, _ := roleSchema["enum"].([]string)
		if strings.Join(roles, ",") != wantRoles {
			t.Fatalf("route %s page_role enum=%v", routeID, roles)
		}
	}
	createProperties, _ := routes[RouteComicSectionCreate].BodySchema["properties"].(map[string]any)
	createRoleSchema, _ := createProperties["page_role"].(map[string]any)
	if description, _ := createRoleSchema["description"].(string); !strings.Contains(description, "空页面序列首项必须为 body") {
		t.Fatalf("comic section create schema lost body-first invariant: %q", description)
	}

	projectUUID, chapterUUID, sectionUUID := mustAgentUUID(t), mustAgentUUID(t), mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	base := "/api/v1/projects/" + projectUUID + "/chapters/" + chapterUUID + "/comic-sections"
	validRequests := []map[string]any{
		{"method": "POST", "url": base, "request_body": map[string]any{"title": "封面", "page_role": "front_cover"}, "response_filter": recommendedAgentAPIResponseFilter(routes[RouteComicSectionCreate])},
		{"method": "PATCH", "url": base + "/" + sectionUUID, "request_body": map[string]any{"page_role": "back_cover", "expected_revision": float64(1)}, "response_filter": recommendedAgentAPIResponseFilter(routes[RouteComicSectionUpdate])},
	}
	for _, request := range validRequests {
		if _, err := parseAgentAPIRequest(tc, request); err != nil {
			t.Fatalf("valid page_role request rejected: request=%+v err=%v", request, err)
		}
	}
	invalid := cloneToolArguments(validRequests[0])
	invalid["request_body"].(map[string]any)["page_role"] = "cover"
	if _, err := parseAgentAPIRequest(tc, invalid); err == nil || errorCode(err) != CodeToolValidation {
		t.Fatalf("invalid page_role accepted: %v", err)
	}

	comicDoc, err := renderAgentDoc(comicDocPath)
	if err != nil {
		t.Fatal(err)
	}
	guide, err := renderAgentDoc(agentDocBasePath + "/guides/管理漫画段落.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"front_cover", "body", "back_cover", "绝对装订顺序", "vertical_strip", "全部 active `body`", "空页面序列必须先创建 `body`", "条漫可删除最后一个", "冻结目标 Section 的 `page_role`"} {
		if !strings.Contains(comicDoc+guide, required) {
			t.Fatalf("comic page-role docs missing %q", required)
		}
	}
	snapshotDoc, err := renderAgentDoc(comicSnapshotDocPath)
	if err != nil {
		t.Fatal(err)
	}
	restoreGuide, err := renderAgentDoc(agentDocBasePath + "/guides/恢复漫画快照.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"至少一个 active `body`", "空快照", "只有封面/封底", "条漫可恢复 empty"} {
		if !strings.Contains(snapshotDoc+restoreGuide, required) {
			t.Fatalf("comic snapshot page-role docs missing %q", required)
		}
	}
	zhPrompt := agentprompts.MustRead("base", "zh-Hans")
	enPrompt := agentprompts.MustRead("base", "en")
	for _, required := range []string{"front_cover", "正文页", "封底", "绝对装订顺序", "空页面序列必须先创建 `body`"} {
		if !strings.Contains(zhPrompt, required) {
			t.Fatalf("Chinese base prompt missing page-role term %q", required)
		}
	}
	for _, required := range []string{"front cover", "body page", "back cover", "absolute binding order", "empty page sequence must start with body"} {
		if !strings.Contains(enPrompt, required) {
			t.Fatalf("English base prompt missing page-role term %q", required)
		}
	}
}

func TestAgentAPIProjectorsAreComplete(t *testing.T) {
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
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Project API", ProviderUUID: harness.provider.UUID})
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
	for _, expected := range []string{RouteStoryProfileGet, "读取故事档案", `"method":"GET"`} {
		if !strings.Contains(metadata, expected) {
			t.Fatalf("tool metadata missing %q: %s", expected, metadata)
		}
	}
	if strings.Contains(metadata, `"scene"`) {
		t.Fatalf("tool metadata retained public scene: %s", metadata)
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
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Protocol v2", ProviderUUID: harness.provider.UUID})
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
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Recover API intent", ProviderUUID: harness.provider.UUID})
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
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "API execution", ProviderUUID: harness.provider.UUID})
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

func TestProjectSetupFinalizationRequiresFingerprintBoundConfirmation(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Setup confirmation", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "请展示摘要后让我确认定稿"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	_, err = executeRequestAPITool(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "setup-finalize-confirmation"}, map[string]any{
		"method": "POST", "url": "/api/v1/projects/" + harness.project.UUID + "/project-setup-finalizations",
		"request_body": map[string]any{"expected_revision": float64(2)}, "response_filter": ".data | {project_uuid,setup_status,status,revision,final_picture_book}",
	})
	var domainErr *Error
	if !errors.As(err, &domainErr) || domainErr.Code != CodeToolConfirmation || !strings.Contains(domainErr.Details, "request_fingerprint") || !strings.Contains(domainErr.Details, RouteProjectSetupFinalize) {
		t.Fatalf("confirmation error=%+v", err)
	}
}
