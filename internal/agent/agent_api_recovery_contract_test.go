package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"lumi/internal/llm"
)

func TestToolValidationViolationKeepsFullPathAndSafeConstraints(t *testing.T) {
	schema := apiObject(map[string]any{
		"generation_language": apiEnum("生成语言。", "zh-Hans", "en"),
		"picture_book": apiObject(map[string]any{
			"format": apiEnum("绘本形式。", "vertical_strip", "comic_story"),
		}, "format"),
	}, "generation_language", "picture_book")

	tests := []struct {
		name         string
		value        map[string]any
		wantPath     string
		wantRule     string
		wantExpected string
		wantAllowed  string
	}{
		{name: "enum", value: map[string]any{"generation_language": "中文", "picture_book": map[string]any{"format": "vertical_strip"}}, wantPath: "request_body.generation_language", wantRule: "enum", wantExpected: "string", wantAllowed: "zh-Hans,en"},
		{name: "unknown", value: map[string]any{"generation_language": "zh-Hans", "picture_book": map[string]any{"format": "vertical_strip", "candidate": true}}, wantPath: "request_body.picture_book.candidate", wantRule: "unknown_field"},
		{name: "missing", value: map[string]any{"generation_language": "zh-Hans", "picture_book": map[string]any{}}, wantPath: "request_body.picture_book.format", wantRule: "required"},
		{name: "type", value: map[string]any{"generation_language": float64(1), "picture_book": map[string]any{"format": "vertical_strip"}}, wantPath: "request_body.generation_language", wantRule: "type", wantExpected: "string"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateArgumentShape("request_body", test.value, schema)
			violation, ok := toolValidationViolationFromError(err)
			if err == nil || !ok {
				t.Fatalf("validation err=%v violation=%+v", err, violation)
			}
			if violation.Path != test.wantPath || violation.Rule != test.wantRule || violation.ExpectedType != test.wantExpected || strings.Join(violation.AllowedValues, ",") != test.wantAllowed {
				t.Fatalf("violation=%+v", violation)
			}
			var agentErr *Error
			if !errors.As(err, &agentErr) {
				t.Fatalf("error type=%T", err)
			}
			if strings.Contains(agentErr.Details, "中文") {
				t.Fatalf("validation details leaked rejected value: %q", agentErr.Details)
			}
		})
	}
}

func TestProjectSetupRecoveryContractUsesRegisteredSchemasAndProjector(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	raw := requestAPITestArguments(t, map[string]any{
		"method": "PATCH", "url": "/api/v1/projects/" + projectUUID + "/project-setup",
		"request_body": map[string]any{
			"expected_revision": float64(1), "project_name": "我的刀盾", "generation_language": "中文",
			"picture_book": map[string]any{"format": "vertical_strip"},
		},
		"response_filter": ".data | {uuid,revision,draft_values}",
	})
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Fatal(err)
	}
	_, cause := parseAgentAPIRequest(tc, args)
	service := &Service{}
	recovery, ok := service.buildRejectedAgentAPICallRecovery(tc, "request_api", raw, cause)
	if cause == nil || !ok {
		t.Fatalf("cause=%v recovery=%+v", cause, recovery)
	}
	contract := recovery.Contract
	if contract.RouteID != RouteProjectSetupUpdate || contract.Method != "PATCH" || contract.DocPath != projectSetupDocPath || contract.PathTemplate != "/api/v1/projects/{project_uuid}/project-setup" {
		t.Fatalf("route contract=%+v", contract)
	}
	properties, _ := contract.Input.RequestBodySchema["properties"].(map[string]any)
	language, _ := properties["generation_language"].(map[string]any)
	allowed, _ := language["enum"].([]string)
	if strings.Join(allowed, ",") != "zh-Hans,en" || contract.Violation.Path != "request_body.generation_language" || contract.Violation.Rule != "enum" {
		t.Fatalf("input contract=%+v violation=%+v", contract.Input, contract.Violation)
	}
	if contract.Output.DataShape != "object" || !containsString(contract.Output.AllowedFields, "draft_values") || !strings.Contains(contract.Output.RecommendedResponseFilter, "draft_values") {
		t.Fatalf("output contract=%+v", contract.Output)
	}
	if contract.Policy.Risk != RiskWrite || !contract.Policy.ExpectedRevision || contract.Policy.ReadOnly || contract.Policy.RequiresConfirmation {
		t.Fatalf("policy=%+v", contract.Policy)
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"id":`) || strings.Contains(string(encoded), `"project_id"`) || strings.Contains(string(encoded), "中文") {
		t.Fatalf("contract leaked internal or rejected values: %s", encoded)
	}
}

func TestRecoveryContractFallsBackWithoutBreakingToolEnvelope(t *testing.T) {
	cause := toolValidationError(
		"工具参数枚举值无效",
		"request_body.generation_language 不在允许值中；允许值：[\"zh-Hans\",\"en\"]。",
		toolValidationViolation{Path: "request_body.generation_language", Rule: "enum", ExpectedType: "string", AllowedValues: []string{"zh-Hans", "en"}},
	)
	recovery := &rejectedAgentAPICallRecovery{Contract: agentAPIRecoveryContract{
		RouteID: RouteProjectSetupUpdate, Method: "PATCH", PathTemplate: "/api/v1/projects/{project_uuid}/project-setup", DocPath: projectSetupDocPath,
		Input:     agentAPIRecoveryInput{RequestBodySchema: map[string]any{"type": "object", "description": strings.Repeat("x", MaxToolResult)}},
		Output:    agentAPIRecoveryOutput{DataShape: "object", RecommendedResponseFilter: ".data | {uuid,revision}"},
		Violation: toolValidationViolation{Path: "request_body.generation_language", Rule: "enum", AllowedValues: []string{"zh-Hans", "en"}},
	}}
	result, included := toolErrorResultWithRecovery(cause, recovery)
	if !included || len(result) > MaxToolResult || !json.Valid(result) {
		t.Fatalf("fallback included=%v bytes=%d result=%s", included, len(result), result)
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
		Error   struct {
			Code    string `json:"code"`
			Details string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result, &envelope); err != nil {
		t.Fatal(err)
	}
	contractJSON := recoveryContractJSON(t, envelope.Error.Details)
	var fallback map[string]any
	if err := json.Unmarshal([]byte(contractJSON), &fallback); err != nil {
		t.Fatal(err)
	}
	if envelope.Success || envelope.Data != nil || envelope.Error.Code != CodeToolValidation || fallback["route_id"] != RouteProjectSetupUpdate || fallback["input"] != nil {
		t.Fatalf("envelope=%+v fallback=%+v", envelope, fallback)
	}
}

func TestRecoveryContractRequiresMatchedRequestAPIRoute(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	cause := toolValidationError("工具参数包含未知字段", "request_body.candidate 不在该工具的参数 schema 中。", toolValidationViolation{Path: "request_body.candidate", Rule: "unknown_field"})
	service := &Service{}
	tests := []struct {
		name string
		tool string
		raw  string
	}{
		{name: "other tool", tool: "request_user_input", raw: `{}`},
		{name: "unknown route", tool: "request_api", raw: requestAPITestArguments(t, map[string]any{"method": "PATCH", "url": "/api/v1/projects/" + projectUUID + "/not-registered", "response_filter": ".data"})},
		{name: "cross project", tool: "request_api", raw: requestAPITestArguments(t, map[string]any{"method": "PATCH", "url": "/api/v1/projects/" + mustAgentUUID(t) + "/project-setup", "response_filter": ".data"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if recovery, ok := service.buildRejectedAgentAPICallRecovery(tc, test.tool, test.raw, cause); ok || recovery != nil {
				t.Fatalf("unexpected recovery=%+v", recovery)
			}
			result, included := toolErrorResultWithRecovery(cause, nil)
			if included || strings.Contains(string(result), agentAPIRecoveryContractMarker) || !json.Valid(result) {
				t.Fatalf("generic result included=%v result=%s", included, result)
			}
		})
	}
}

func TestProjectSetupValidationRecoveryExecutesOnlyCorrectedRequest(t *testing.T) {
	harness := newAgentHarness(t)
	installProjectSetupDispatcherForTest(t, harness)
	makeHarnessProjectDraft(t, harness, mustAgentUUID(t), "参考图一是我的刀盾，以此作为主角创作一个条漫。")

	invalid := requestAPICallResponse(t, "invalid-project-setup", map[string]any{
		"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID + "/project-setup",
		"request_body": map[string]any{"expected_revision": float64(1), "candidate": map[string]any{
			"project_name": "我的刀盾", "generation_language": "中文", "overall_style": "温暖卡通插画",
			"picture_book": map[string]any{"format": "vertical_strip"},
		}},
		"response_filter": ".data | {uuid,revision,draft_values,missing_information}",
	})
	corrected := requestAPICallResponse(t, "corrected-project-setup", map[string]any{
		"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID + "/project-setup",
		"request_body": map[string]any{
			"expected_revision": float64(1), "project_name": "我的刀盾", "generation_language": "zh-Hans",
			"overall_style": "温暖卡通插画", "picture_book": map[string]any{"format": "vertical_strip"},
		},
		"response_filter": ".data | {uuid,revision,draft_values,missing_information}",
	})
	harness.model.responses = []llm.ChatResponse{invalid, corrected, finalResponse("项目初始化草稿已整理。")}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "创建条漫"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}

	state, err := harness.store.ProjectSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 2 || state.DraftValues.ProjectName != "我的刀盾" || state.DraftValues.GenerationLanguage != "zh-Hans" || state.DraftValues.PictureBook == nil {
		t.Fatalf("setup state=%+v", state)
	}
	var repairs, executions int64
	if err := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_result' AND json_extract(metadata_json,'$.validation_repair')=1", turn.UUID).Count(&repairs).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND tool_name='request_api'", turn.UUID).Count(&executions).Error; err != nil {
		t.Fatal(err)
	}
	if repairs != 1 || executions != 1 {
		t.Fatalf("repairs=%d executions=%d", repairs, executions)
	}

	var rejectedContent, metadataJSON string
	if err := harness.store.DB().Table("chat_items").Select("content,metadata_json").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_result' AND json_extract(metadata_json,'$.validation_repair')=1", turn.UUID).Row().Scan(&rejectedContent, &metadataJSON); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rejectedContent, "中文") {
		t.Fatalf("recovery result leaked rejected value: %s", rejectedContent)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["route_id"] != RouteProjectSetupUpdate || metadata["doc_path"] != projectSetupDocPath || metadata["validation_path"] != "request_body.candidate" || metadata["recovery_contract_included"] != true {
		t.Fatalf("rejected metadata=%+v", metadata)
	}
	var eventPayload string
	if err := harness.store.DB().Table("chat_events").Select("payload_json").Where("event_type='tool_result' AND json_extract(payload_json,'$.recovery_contract_included')=1").Row().Scan(&eventPayload); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(eventPayload), &event); err != nil {
		t.Fatal(err)
	}
	if event["route_id"] != RouteProjectSetupUpdate || event["doc_path"] != projectSetupDocPath || event["validation_path"] != "request_body.candidate" {
		t.Fatalf("rejected event=%+v", event)
	}
	contract := recoveryContractFromToolResult(t, rejectedContent)
	properties, _ := contract.Input.RequestBodySchema["properties"].(map[string]any)
	language, _ := properties["generation_language"].(map[string]any)
	if allowed := stringSliceArg(language, "enum"); strings.Join(allowed, ",") != "zh-Hans,en" || contract.Violation.Path != "request_body.candidate" {
		t.Fatalf("recovery contract=%+v", contract)
	}

	harness.model.mu.Lock()
	requests := append([]llm.ChatRequest(nil), harness.model.requests...)
	harness.model.mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("model requests=%d", len(requests))
	}
	foundRecovery := false
	for _, message := range requests[1].Messages {
		if message.Role == "tool" && message.ToolCallID == "invalid-project-setup" && strings.Contains(message.Content, agentAPIRecoveryContractMarker) {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatal("corrective model request did not receive the recovery contract")
	}
}

func TestProjectSetupRecoveryContractDoesNotResetRepairLimit(t *testing.T) {
	harness := newAgentHarness(t)
	harness.service.turnBudget.MaxNoProgressRounds = 10
	invalid := func(callID string) llm.ChatResponse {
		return requestAPICallResponse(t, callID, map[string]any{
			"method": "PATCH", "url": "/api/v1/projects/" + harness.project.UUID + "/project-setup",
			"request_body":    map[string]any{"expected_revision": float64(1), "candidate": map[string]any{"generation_language": "中文"}},
			"response_filter": ".data | {uuid,revision}",
		})
	}
	harness.model.responses = []llm.ChatResponse{invalid("invalid-1"), invalid("invalid-2"), invalid("invalid-3"), finalResponse("should not be reached")}
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(context.Background(), harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "更新项目初始化草稿"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.execute(t, thread.UUID, turn.UUID, JobChatTurn); err != nil {
		t.Fatal(err)
	}
	harness.model.mu.Lock()
	calls := harness.model.calls
	harness.model.mu.Unlock()
	turns, err := harness.service.ListTurns(context.Background(), harness.project.UUID, thread.UUID)
	if err != nil || len(turns) != 1 || turns[0].Status != TurnFailed || turns[0].ErrorCode != CodeToolValidation {
		t.Fatalf("turns=%+v err=%v", turns, err)
	}
	var repairs, contracts, executions int64
	items := harness.store.DB().Table("chat_items").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?) AND item_type='tool_result' AND json_extract(metadata_json,'$.validation_repair')=1", turn.UUID)
	if err := items.Count(&repairs).Error; err != nil {
		t.Fatal(err)
	}
	if err := items.Where("json_extract(metadata_json,'$.recovery_contract_included')=1").Count(&contracts).Error; err != nil {
		t.Fatal(err)
	}
	if err := harness.store.DB().Table("agent_tool_executions").Where("turn_id=(SELECT id FROM chat_turns WHERE uuid=?)", turn.UUID).Count(&executions).Error; err != nil {
		t.Fatal(err)
	}
	if calls != 3 || repairs != maxToolValidationRepairs || contracts != maxToolValidationRepairs || executions != 0 {
		t.Fatalf("calls=%d repairs=%d contracts=%d executions=%d", calls, repairs, contracts, executions)
	}
}

func requestAPITestArguments(t *testing.T, args map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func requestAPICallResponse(t *testing.T, callID string, args map[string]any) llm.ChatResponse {
	t.Helper()
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: callID, Name: "request_api", Arguments: requestAPITestArguments(t, args)}}}, FinishReason: "tool_calls"}
}

func recoveryContractFromToolResult(t *testing.T, raw string) agentAPIRecoveryContract {
	t.Helper()
	var envelope struct {
		Error struct {
			Details string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	var contract agentAPIRecoveryContract
	if err := json.Unmarshal([]byte(recoveryContractJSON(t, envelope.Error.Details)), &contract); err != nil {
		t.Fatal(err)
	}
	return contract
}

func recoveryContractJSON(t *testing.T, details string) string {
	t.Helper()
	index := strings.Index(details, agentAPIRecoveryContractMarker)
	if index < 0 {
		t.Fatalf("missing recovery contract in details: %q", details)
	}
	return details[index+len(agentAPIRecoveryContractMarker):]
}
