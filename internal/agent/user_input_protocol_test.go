package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"lumi/internal/llm"
	"lumi/internal/production"
)

const validCodexUserInputArguments = `{"questions":[{"header":"画面风格","id":"art_style","question":"这次画面应采用哪种整体风格？","options":[{"label":"温暖手绘 (Recommended)","description":"延续绘本现有的柔和质感和亲切氛围。"},{"label":"电影写实","description":"强化真实光影、景深和镜头感。"}]}]}`

func TestProjectAPIV4RequestUserInputDefinitionExcludesConfirmation(t *testing.T) {
	definition := projectAPIV4RequestUserInputDefinition()
	parameters := definition["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	questions := properties["questions"].(map[string]any)
	question := questions["items"].(map[string]any)
	questionProperties := question["properties"].(map[string]any)
	if _, topLevel := properties["confirmation"]; topLevel {
		t.Fatalf("model-visible request_user_input still exposes confirmation: definition=%+v", definition)
	}
	if _, nested := questionProperties["confirmation"]; nested || !strings.Contains(definition["description"].(string), "generated, persisted, and resumed by the runtime") {
		t.Fatalf("request_user_input runtime ownership is ambiguous: definition=%+v", definition)
	}
}

func TestProjectAPIV4RequestUserInputSchemaAndRuntimeValidation(t *testing.T) {
	for count := 1; count <= 3; count++ {
		questions := make([]map[string]any, 0, count)
		for index := 0; index < count; index++ {
			questions = append(questions, map[string]any{
				"header":   fmt.Sprintf("问题%d", index+1),
				"id":       fmt.Sprintf("question_%d", index+1),
				"question": fmt.Sprintf("第 %d 个问题怎么选？", index+1),
				"options": []map[string]any{
					{"label": "推荐方案 (Recommended)", "description": "采用适合当前请求的默认方案。"},
					{"label": "备选方案", "description": "采用另一种可行方案。"},
				},
			})
		}
		raw, _ := json.Marshal(map[string]any{"questions": questions})
		if _, err := validateToolArgumentsForProtocol("request_user_input", string(raw), ToolModeProjectAPI, ToolProtocolProjectAPI); err != nil {
			t.Fatalf("valid v4 request with %d questions rejected: %v", count, err)
		}
	}
	invalid := map[string]string{
		"no questions":          `{"questions":[]}`,
		"too many questions":    `{"questions":[{"header":"一","id":"one","question":"一？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]},{"header":"二","id":"two","question":"二？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]},{"header":"三","id":"three","question":"三？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]},{"header":"四","id":"four","question":"四？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}]}`,
		"empty header":          `{"questions":[{"header":" ","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}]}`,
		"long header":           `{"questions":[{"header":"这是一个超过十二个字符的标题","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}]}`,
		"invalid id":            `{"questions":[{"header":"风格","id":"Art-Style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}]}`,
		"duplicate id":          `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]},{"header":"色彩","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}]}`,
		"empty question":        `{"questions":[{"header":"风格","id":"style","question":" ","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}]}`,
		"one option":            `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"}]}]}`,
		"four options":          `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"},{"label":"丙","description":"丙。"},{"label":"丁","description":"丁。"}]}]}`,
		"missing label":         `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"description":"乙。"}]}]}`,
		"missing description":   `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙"}]}]}`,
		"multiline description": `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"第一句。\n第二句。"},{"label":"乙","description":"乙。"}]}]}`,
		"wrong recommendation":  `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲","description":"甲。"},{"label":"乙 (Recommended)","description":"乙。"}]}]}`,
		"model other":           `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"Other","description":"自由填写。"}]}]}`,
		"extra option field":    `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。","value":"spoof"},{"label":"乙","description":"乙。"}]}]}`,
		"extra top-level field": `{"questions":[{"header":"风格","id":"style","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}],"input_type":"single_choice"}`,
		"duplicate json key":    `{"questions":[{"header":"风格","id":"style","id":"other","question":"怎么选？","options":[{"label":"甲 (Recommended)","description":"甲。"},{"label":"乙","description":"乙。"}]}]}`,
	}
	for name, raw := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := validateToolArgumentsForProtocol("request_user_input", raw, ToolModeProjectAPI, ToolProtocolProjectAPI); errorCode(err) != CodeToolValidation {
				t.Fatalf("invalid v4 request accepted: %v", err)
			}
		})
	}
}

func TestProjectAPIV4RejectsEveryModelAuthoredConfirmationPlacement(t *testing.T) {
	confirmation := `{"route":"comic.storyboard_generation.create","project_uuid":"01990000-0000-7000-8000-000000000321","target_uuid":"01990000-0000-7000-8000-000000000322","expected_revision":0,"request_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","question_id":"confirm_storyboard","confirm_option":1}`
	question := `{"header":"生成确认","id":"confirm_storyboard","question":"是否生成漫画分镜？","options":[{"label":"暂不生成 (Recommended)","description":"保留当前状态。"},{"label":"确认生成","description":"创建生成任务。"}]`
	cases := map[string]string{
		"top-level":            `{"questions":[` + question + `}],"confirmation":` + confirmation + `}`,
		"single nested":        `{"questions":[` + question + `,"confirmation":` + confirmation + `}]}`,
		"top-level and nested": `{"questions":[` + question + `,"confirmation":` + confirmation + `}],"confirmation":` + confirmation + `}`,
		"multiple questions":   `{"questions":[` + question + `,"confirmation":` + confirmation + `},` + question + `}]}`,
		"non-object":           `{"questions":[` + question + `,"confirmation":"invalid"}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := validateToolArgumentsForProtocol("request_user_input", raw, ToolModeProjectAPI, ToolProtocolProjectAPI)
			var agentErr *Error
			if !errors.As(err, &agentErr) || agentErr.Code != CodeToolValidation || agentErr.Message != "危险操作确认由运行时管理" || !strings.Contains(agentErr.Details, "模型不得构造") {
				t.Fatalf("placement error=%v", err)
			}
		})
	}
}

func TestNestedModelConfirmationNeverPersistsToolIntent(t *testing.T) {
	nested := `{"questions":[{"header":"创建确认","id":"confirm_setup","question":"是否定稿并开始生成？","options":[{"label":"继续修改 (Recommended)","description":"保留当前草稿。"},{"label":"定稿并开始生成","description":"定稿并开始自动生成流程。"}],"confirmation":{"route":"project_setup.finalize","project_uuid":"01990000-0000-7000-8000-000000000321","target_uuid":"01990000-0000-7000-8000-000000000322","expected_revision":1,"request_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","question_id":"confirm_setup","confirm_option":1}}]}`
	harness := newAgentHarness(t,
		llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "nested-confirmation", Name: "request_user_input", Arguments: nested}}}, FinishReason: "tool_calls"},
		finalResponse("已停止等待无效的模型确认。"),
	)
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "继续"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	var executions int64
	if err := harness.store.DB().Table("agent_tool_executions").Where("run_id=(SELECT id FROM chat_runs WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?)) AND tool_name='request_user_input'", turn.UUID).Count(&executions).Error; err != nil || executions != 0 {
		t.Fatalf("nested confirmation created %d execution intents: %v", executions, err)
	}
	requests, err := harness.service.ListUserInputRequests(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(requests) != 0 {
		t.Fatalf("nested confirmation created user input requests=%+v err=%v", requests, err)
	}
}

func TestPersistedNestedConfirmationIsReadOnlyCompatibilityState(t *testing.T) {
	raw := `{"questions":[{"header":"创建确认","id":"confirm_setup","question":"是否定稿并开始生成？","options":[{"label":"继续修改 (Recommended)","description":"保留当前草稿。"},{"label":"定稿并开始生成","description":"定稿并开始自动生成流程。"}],"confirmation":{"route":"project_setup.finalize","project_uuid":"01990000-0000-7000-8000-000000000321","target_uuid":"01990000-0000-7000-8000-000000000322","expected_revision":1,"request_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","question_id":"confirm_setup","confirm_option":1}}]}`
	binding, err := dangerousConfirmationFromArguments(raw)
	if err != nil {
		t.Fatal(err)
	}
	if binding == nil || binding.Route != RouteProjectSetupFinalize || binding.QuestionID != "confirm_setup" || binding.ConfirmOption != 1 {
		t.Fatalf("historical nested confirmation was not recovered: %+v", binding)
	}

	var publicValue map[string]any
	if err := json.Unmarshal([]byte(raw), &publicValue); err != nil {
		t.Fatal(err)
	}
	stripJSONField(publicValue, "confirmation")
	if encoded, _ := json.Marshal(publicValue); strings.Contains(string(encoded), `"confirmation"`) {
		t.Fatalf("historical confirmation leaked into public arguments: %s", encoded)
	}
	if _, err := validateToolArgumentsForProtocol("request_user_input", raw, ToolModeProjectAPI, ToolProtocolProjectAPI); errorCode(err) != CodeToolValidation {
		t.Fatalf("historical repair path accepted live model arguments: %v", err)
	}
}

func TestProjectAPIRequestUserInputDefinitionsAreFrozenByProtocol(t *testing.T) {
	legacy := `{"input_type":"single_choice","question":"继续吗？","options":[{"label":"继续"},{"label":"取消"}]}`
	legacyConfirmation := `{"input_type":"single_choice","question":"继续吗？","options":[{"label":"取消"},{"label":"继续"}],"confirmation":{"route":"project_setup.finalize","project_uuid":"01990000-0000-7000-8000-000000000321","target_uuid":"01990000-0000-7000-8000-000000000322","expected_revision":1,"request_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","confirm_option":1}}`
	for _, protocol := range []string{ToolProtocolProjectV2, ToolProtocolProjectV3} {
		definitions := toolDefinitionsForProtocol(ToolModeProjectAPI, protocol)
		var definition map[string]any
		for _, candidate := range definitions {
			if candidate["name"] == "request_user_input" {
				definition = candidate
				break
			}
		}
		if definition == nil {
			t.Fatalf("%s lost request_user_input", protocol)
		}
		properties := definition["parameters"].(map[string]any)["properties"].(map[string]any)
		if _, exposed := properties["confirmation"]; exposed || !strings.Contains(definition["description"].(string), "generated and replayed internally by the runtime") {
			t.Fatalf("%s still exposes a model-authored confirmation contract: %+v", protocol, definition)
		}
		if _, err := validateToolArgumentsForProtocol("request_user_input", legacy, ToolModeProjectAPI, protocol); err != nil {
			t.Fatalf("%s rejected frozen request: %v", protocol, err)
		}
		if _, err := validateToolArgumentsForProtocol("request_user_input", validCodexUserInputArguments, ToolModeProjectAPI, protocol); errorCode(err) != CodeToolValidation {
			t.Fatalf("%s accepted v4 request: %v", protocol, err)
		}
		if _, err := validateToolArgumentsForProtocol("request_user_input", legacyConfirmation, ToolModeProjectAPI, protocol); errorCode(err) != CodeToolValidation {
			t.Fatalf("%s accepted model-authored confirmation: %v", protocol, err)
		}
	}
	legacyDefinition := legacyToolDefinitionByName("request_user_input")
	if legacyDefinition == nil {
		t.Fatal("legacy recovery protocol lost request_user_input")
	}
	legacyProperties := legacyDefinition["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, exposed := legacyProperties["confirmation"]; exposed || !strings.Contains(legacyDefinition["description"].(string), "generated and replayed internally by the runtime") {
		t.Fatalf("legacy recovery still exposes a model-authored confirmation contract: %+v", legacyDefinition)
	}
	if _, err := validateToolArgumentsForProtocol("request_user_input", legacy, ToolModeProjectAPI, ToolProtocolProjectAPI); errorCode(err) != CodeToolValidation {
		t.Fatalf("v4 accepted legacy request: %v", err)
	}
	v3Image := `{"prompt":"恢复 v3 图片","reference_uuids":[]}`
	v2Image := `{"prompt":"恢复 v2 图片","reference_file_uuids":[]}`
	if _, err := validateToolArgumentsForProtocol("image_gen", v3Image, ToolModeProjectAPI, ToolProtocolProjectV3); err != nil {
		t.Fatalf("v3 rejected frozen image schema: %v", err)
	}
	if _, err := validateToolArgumentsForProtocol("image_gen", v2Image, ToolModeProjectAPI, ToolProtocolProjectV3); errorCode(err) != CodeToolValidation {
		t.Fatalf("v3 accepted v2 image schema: %v", err)
	}
	if _, err := validateToolArgumentsForProtocol("image_gen", v2Image, ToolModeProjectAPI, ToolProtocolProjectV2); err != nil {
		t.Fatalf("v2 rejected frozen image schema: %v", err)
	}
	if _, err := validateToolArgumentsForProtocol("image_gen", v3Image, ToolModeProjectAPI, ToolProtocolProjectV2); errorCode(err) != CodeToolValidation {
		t.Fatalf("v2 accepted v3 image schema: %v", err)
	}
}

func TestRequestAPIRejectsConfirmationPlacement(t *testing.T) {
	cases := map[string]map[string]any{
		"top_level":   {"confirmation": map[string]any{}},
		"query":       {"query": map[string]any{"confirmation": map[string]any{}}},
		"nested_body": {"request_body": map[string]any{"parameters": map[string]any{"confirmation": map[string]any{}}}},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			arguments := map[string]any{
				"url":    "/api/v1/projects/01990000-0000-7000-8000-000000000321/chapters",
				"method": "POST", "response_filter": ".data | {uuid}",
			}
			for key, value := range extra {
				arguments[key] = value
			}
			raw, _ := json.Marshal(arguments)
			_, err := validateToolArgumentsForProtocol("request_api", string(raw), ToolModeProjectAPI, ToolProtocolProjectAPI)
			var agentErr *Error
			if !errors.As(err, &agentErr) || agentErr.Code != CodeToolValidation || agentErr.Message != "request_api 不接受 confirmation" || !strings.Contains(agentErr.Details, "运行时根据已持久化的原请求生成") {
				t.Fatalf("%s confirmation placement error=%v", name, err)
			}
		})
	}
}

func TestPersistedV2AndV3UserInputRunsResumeWithFrozenContract(t *testing.T) {
	for _, protocol := range []string{ToolProtocolProjectV2, ToolProtocolProjectV3} {
		t.Run(protocol, func(t *testing.T) {
			call := llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
				ID: "legacy-input", Name: "request_user_input", Arguments: `{"input_type":"single_choice","question":"继续吗？","options":[{"label":"继续"},{"label":"取消"}]}`,
			}}}, FinishReason: "tool_calls"}
			harness := newAgentHarness(t, call, finalResponse("已恢复旧协议。"))
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复旧请求"})
			if err != nil {
				t.Fatal(err)
			}
			if err := harness.store.DB().Exec(`UPDATE chat_items SET metadata_json=json_set(metadata_json,'$.prompt_snapshot.tool_protocol',?) WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='user_message'`, protocol, turn.UUID).Error; err != nil {
				t.Fatal(err)
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
				t.Fatalf("frozen run did not wait: %v", err)
			}
			requests, err := harness.service.ListUserInputRequests(context.Background(), harness.project.UUID, thread.UUID)
			if err != nil || len(requests) != 1 || requests[0].SchemaVersion != userInputSchemaLegacyChoice || len(requests[0].Options) != 2 || len(requests[0].Questions) != 0 {
				t.Fatalf("frozen request=%+v err=%v", requests, err)
			}
			if _, err := harness.service.RespondUserInput(context.Background(), harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{SelectedOptionUUIDs: []string{requests[0].Options[0].UUID}}); err != nil {
				t.Fatal(err)
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPersistedV2AndV3DangerousConfirmationsAutoReplay(t *testing.T) {
	for _, protocol := range []string{ToolProtocolProjectV2, ToolProtocolProjectV3} {
		t.Run(protocol, func(t *testing.T) {
			harness := newAgentHarness(t)
			harness.service.turnBudget.MaxModelRequests = 2
			ctx := context.Background()
			asset, thread := createAssetReferenceMigrationFixture(t, harness)
			turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "这个设定项似乎没用了，应该怎么处理？", MaxSteps: 2})
			if err != nil {
				t.Fatal(err)
			}
			if err := harness.store.DB().Exec(`UPDATE chat_items SET metadata_json=json_set(metadata_json,'$.prompt_snapshot.tool_protocol',?) WHERE turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='user_message'`, protocol, turn.UUID).Error; err != nil {
				t.Fatal(err)
			}
			assetURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID
			requestArguments := map[string]any{
				"method": "DELETE", "url": assetURL,
				"request_body":    map[string]any{"expected_revision": float64(asset.Revision)},
				"response_filter": ".data | {uuid,deleted_at}",
			}
			requestJSON, _ := json.Marshal(requestArguments)
			harness.model.respond = func(call int, modelRequest llm.ChatRequest) (llm.ChatResponse, error) {
				switch call {
				case 1:
					return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "legacy-danger", Name: "request_api", Arguments: string(requestJSON)}}}, FinishReason: "tool_calls"}, nil
				case 2:
					if len(modelRequest.Tools) != 0 {
						t.Fatalf("%s finalization exposed tools: %v", protocol, definitionNames(modelRequest.Tools))
					}
					return finalResponse("已按确认移入回收站。"), nil
				default:
					return llm.ChatResponse{}, fmt.Errorf("unexpected %s model call %d", protocol, call)
				}
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); !errors.Is(err, ErrWaitingInput) {
				t.Fatalf("%s did not wait for confirmation: %v", protocol, err)
			}
			requests, err := harness.service.ListUserInputRequests(ctx, harness.project.UUID, thread.UUID)
			if err != nil || len(requests) != 1 || requests[0].SchemaVersion != userInputSchemaLegacyChoice {
				t.Fatalf("%s requests=%+v err=%v", protocol, requests, err)
			}
			var requestItemContentBefore string
			if err := harness.store.DB().Table("chat_items").Select("content").Where("uuid=?", requests[0].ItemUUID).Scan(&requestItemContentBefore).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := harness.service.RespondUserInput(ctx, harness.project.UUID, thread.UUID, requests[0].UUID, UserInputResponse{SelectedOptionUUIDs: []string{requests[0].Options[1].UUID}}); err != nil {
				t.Fatal(err)
			}
			var requestItemContentAfter string
			if err := harness.store.DB().Table("chat_items").Select("content").Where("uuid=?", requests[0].ItemUUID).Scan(&requestItemContentAfter).Error; err != nil {
				t.Fatal(err)
			}
			if requestItemContentAfter != requestItemContentBefore {
				t.Fatalf("%s confirmation response rewrote the user-input card: before=%s after=%s", protocol, requestItemContentBefore, requestItemContentAfter)
			}
			if err := harness.execute(t, thread.UUID, turn.UUID, JobChatResume); err != nil {
				t.Fatal(err)
			}
			deleted, err := production.NewService(harness.store, nil).GetPremiseAsset(ctx, asset.UUID)
			if err != nil || deleted.DeletedAt == nil {
				t.Fatalf("%s did not execute automatic replay: asset=%+v err=%v", protocol, deleted, err)
			}
			var replayCount int64
			if err := harness.store.DB().Table("agent_tool_executions").Where("json_extract(arguments_json,'$.__confirmation_auto_replay')=1").Count(&replayCount).Error; err != nil || replayCount != 1 {
				t.Fatalf("%s replay count=%d err=%v", protocol, replayCount, err)
			}
			if harness.model.calls != 2 {
				t.Fatalf("%s model calls=%d want exactly 2 (request + post-replay finalization)", protocol, harness.model.calls)
			}
		})
	}
}

func TestConfirmationRequiredResultRecoveryBuildsOneRuntimeIntentForEverySupportedProtocol(t *testing.T) {
	cases := []struct {
		name, mode, protocol string
	}{
		{name: ToolProtocolProjectAPI, mode: ToolModeProjectAPI, protocol: ToolProtocolProjectAPI},
		{name: ToolProtocolProjectV3, mode: ToolModeProjectAPI, protocol: ToolProtocolProjectV3},
		{name: ToolProtocolProjectV2, mode: ToolModeProjectAPI, protocol: ToolProtocolProjectV2},
		{name: "legacy_typed_recovery", mode: ToolModeLegacyTyped},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newAgentHarness(t)
			ctx := context.Background()
			thread := harness.createThread(t)
			turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "恢复升级前的危险操作确认"})
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
			sourceContext := tc
			sourceContext.ToolMode, sourceContext.ToolProtocol = ToolModeProjectAPI, ToolProtocolProjectAPI
			tc.ToolMode, tc.ToolProtocol = testCase.mode, testCase.protocol
			arguments, _ := json.Marshal(map[string]any{
				"method": "POST",
				"url":    "/api/v1/projects/" + harness.project.UUID + "/project-setup-finalizations",
				"request_body": map[string]any{
					"expected_revision": float64(1),
				},
				"response_filter": ".data | {project_uuid,setup_status,status,revision}",
			})
			source, _, completed, err := harness.service.persistToolIntent(ctx, harness.store, sourceContext, "pre-upgrade-confirmation:"+testCase.name, "request_api", string(arguments))
			if err != nil || completed {
				t.Fatalf("persist source=%+v completed=%v err=%v", source, completed, err)
			}
			result, err := harness.service.executeTool(ctx, harness.store, sourceContext, source)
			if err != nil || !confirmationRequiredToolResult(string(result)) {
				t.Fatalf("confirmation result=%s err=%v", result, err)
			}

			// Simulate the exact upgrade gap: the request_api result is durable,
			// but the old process stopped before it could create a confirmation.
			if err := harness.store.DB().Model(&toolExecutionRecord{}).Where("id=?", source.ID).Updates(map[string]any{
				"state": "completed", "result_json": string(result),
			}).Error; err != nil {
				t.Fatal(err)
			}
			intent, recovered, err := harness.service.recoverRuntimeDangerousConfirmation(ctx, harness.store, tc)
			if err != nil || !recovered || intent.ToolName != "request_user_input" {
				t.Fatalf("recovered=%v intent=%+v err=%v", recovered, intent, err)
			}
			request, err := harness.service.createUserInputRequest(ctx, harness.store, tc, intent)
			if err != nil {
				t.Fatal(err)
			}
			wantSchema := userInputSchemaLegacyChoice
			if testCase.protocol == ToolProtocolProjectAPI {
				wantSchema = userInputSchemaCodexQuestions
			}
			if request.SchemaVersion != wantSchema {
				t.Fatalf("schema=%s want=%s request=%+v", request.SchemaVersion, wantSchema, request)
			}
			if duplicate, recoveredAgain, err := harness.service.recoverRuntimeDangerousConfirmation(ctx, harness.store, tc); err != nil || recoveredAgain || duplicate.ID != 0 {
				t.Fatalf("duplicate recovery=%v intent=%+v err=%v", recoveredAgain, duplicate, err)
			}
			var intentCount int64
			if err := harness.store.DB().Table("agent_tool_executions").Where(
				"run_id=? AND tool_name='request_user_input' AND json_extract(arguments_json,'$.__runtime_generated_confirmation')=1 AND json_extract(arguments_json,'$.__confirmation_source_execution_uuid')=?",
				tc.Run.ID, source.UUID,
			).Count(&intentCount).Error; err != nil || intentCount != 1 {
				t.Fatalf("runtime confirmation count=%d err=%v", intentCount, err)
			}
		})
	}
}

func TestCodexUserInputResponseValidatesEveryQuestionAndBuildsLabelResult(t *testing.T) {
	firstUUID := "01990000-0000-7000-8000-000000000901"
	secondUUID := "01990000-0000-7000-8000-000000000902"
	requestJSON, _ := json.Marshal(map[string]any{"questions": []UserInputQuestion{
		{Header: "风格", ID: "style", Question: "选择风格？", Options: []UserInputOption{{UUID: firstUUID, Label: "手绘 (Recommended)", Description: "柔和。"}, {UUID: secondUUID, Label: "写实", Description: "真实。"}}},
		{Header: "页数", ID: "pages", Question: "选择页数？", Options: []UserInputOption{{UUID: "01990000-0000-7000-8000-000000000903", Label: "八页 (Recommended)", Description: "简洁。"}, {UUID: "01990000-0000-7000-8000-000000000904", Label: "十六页", Description: "完整。"}}},
	}})
	row := userInputRow{SchemaVersion: userInputSchemaCodexQuestions, RequestJSON: string(requestJSON)}
	persisted, toolResult, err := validateCodexUserInputResponse(row, UserInputResponse{Answers: map[string]UserInputAnswer{
		"style": {SelectedOptionUUID: secondUUID},
		"pages": {OtherText: "12 页"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	persistedJSON, _ := json.Marshal(persisted)
	toolJSON, _ := json.Marshal(toolResult)
	if !strings.Contains(string(persistedJSON), secondUUID) || !strings.Contains(string(toolJSON), `"style":{"answers":["写实"]}`) || !strings.Contains(string(toolJSON), `"pages":{"answers":["12 页"]}`) {
		t.Fatalf("persisted=%s tool=%s", persistedJSON, toolJSON)
	}

	invalid := []UserInputResponse{
		{Answers: map[string]UserInputAnswer{"style": {SelectedOptionUUID: secondUUID}}},
		{Answers: map[string]UserInputAnswer{"style": {SelectedOptionUUID: secondUUID}, "pages": {OtherText: "12"}, "unknown": {OtherText: "x"}}},
		{Answers: map[string]UserInputAnswer{"style": {SelectedOptionUUID: "01990000-0000-7000-8000-000000000903"}, "pages": {OtherText: "12"}}},
		{Answers: map[string]UserInputAnswer{"style": {SelectedOptionUUID: secondUUID, OtherText: "both"}, "pages": {OtherText: "12"}}},
		{Answers: map[string]UserInputAnswer{"style": {SelectedOptionUUID: secondUUID}, "pages": {OtherText: " "}}},
		{Answers: map[string]UserInputAnswer{"style": {SelectedOptionUUID: secondUUID}, "pages": {OtherText: strings.Repeat("页", 4001)}}},
		{Answers: map[string]UserInputAnswer{"style": {SelectedOptionUUID: "01990000-0000-7000-8000-000000000999"}, "pages": {OtherText: "12"}}},
		{Answers: map[string]UserInputAnswer{"style": {}, "pages": {OtherText: "12"}}},
	}
	for index, response := range invalid {
		if _, _, err := validateCodexUserInputResponse(row, response); errorCode(err) != CodeValidation {
			t.Fatalf("invalid response %d accepted: %v", index, err)
		}
	}
}

func TestCodexDangerousConfirmationOnlyAcceptsExactBoundOption(t *testing.T) {
	requestJSON := `{"questions":[{"header":"删除确认","id":"delete_asset","question":"删除？","options":[{"uuid":"01990000-0000-7000-8000-000000000911","label":"保留 (Recommended)"},{"uuid":"01990000-0000-7000-8000-000000000912","label":"删除"}]}]}`
	binding := dangerousConfirmationBinding{QuestionID: "delete_asset", ConfirmOption: 1}
	responses := []struct {
		response string
		want     bool
	}{
		{`{"answers":{"delete_asset":{"selected_option_uuid":"01990000-0000-7000-8000-000000000912","other_text":""}}}`, true},
		{`{"answers":{"delete_asset":{"selected_option_uuid":"01990000-0000-7000-8000-000000000911","other_text":""}}}`, false},
		{`{"answers":{"delete_asset":{"selected_option_uuid":"","other_text":"确认删除"}}}`, false},
	}
	for _, test := range responses {
		got, err := dangerousConfirmationSelected(userInputSchemaCodexQuestions, requestJSON, test.response, binding)
		if err != nil || got != test.want {
			t.Fatalf("confirmation=%s got=%v want=%v err=%v", test.response, got, test.want, err)
		}
	}
}
