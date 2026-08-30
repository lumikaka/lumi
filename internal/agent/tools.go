package agent

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"lumi/internal/imagegen"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"

	"gorm.io/gorm"
)

type toolContext struct {
	ProjectUUID                  string
	BootstrapCreationSessionUUID string
	Thread                       threadRecord
	Turn                         turnRecord
	Run                          runRecord
	ToolMode                     string
	ToolProtocol                 string
	RequestUUID                  string
	RequestOrdinal               int
}

type toolExecutionRecord struct {
	ID, ThreadID, RunID, TurnID, ItemID                                            int64
	UUID, ToolCallUUID, ToolName, TargetUUID, ArgumentsJSON, IdempotencyKey, State string
	RouteID, Action, Method, Path                                                  string
	ResultJSON                                                                     *string
	ErrorCode, ErrorMessage                                                        string
	StartedAt, CompletedAt                                                         *time.Time
	CreatedAt, UpdatedAt                                                           time.Time
}

const (
	maxToolValidationRepairs             = 2
	confirmationQuestionIDArgumentRepair = "confirmation.question_id"
)

func toolDefinitions() []map[string]any {
	return append(projectAPISharedToolDefinitions(), projectAPIV4RequestUserInputDefinition())
}

func projectAPISharedToolDefinitions() []map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	stringField := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return []map[string]any{
		{"name": "request_api", "description": "Call any server-registered API route under the current /api/v1/projects/{project_uuid} scope in-process. Reviewed routes retain stricter schemas and optimized domain dispatch; other routes use the application router and its public API contract. Never put confirmation in query or request_body. After a bound request_user_input is confirmed, the runtime executes the original request automatically; do not replay it.", "parameters": object(map[string]any{
			"url":             stringField("Canonical relative /api/v1/projects/{current_project_uuid}/... path"),
			"method":          map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
			"query":           map[string]any{"type": "object", "additionalProperties": true, "description": "Optional route-specific typed query object; never append a query string to url."},
			"request_body":    map[string]any{"type": "object", "additionalProperties": true},
			"response_filter": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048, "description": "Required safe projection beginning with .data. Select only the fields needed for the current step; use .data only when the complete compact response is necessary."},
		}, "url", "method", "response_filter")},
		{"name": "read_agent_doc", "description": "Read a registered Agent Overview, reusable capability Guide, or Project API contract. Start with /api/v1/agent-docs/overview.md to discover capabilities and routes.", "parameters": object(map[string]any{
			"path": stringField("Registered /api/v1/agent-docs/...md path"),
		}, "path")},
		{"name": "image_gen", "description": "Generate a project-scoped image synchronously. Select zero to four image-capable References from the current Turn by their resource_uuid; the backend resolves their frozen images in the supplied order.", "parameters": object(map[string]any{
			"prompt": stringField("Detailed image generation prompt"), "reference_uuids": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "string"}},
			"size": map[string]any{"type": "string", "enum": []string{"512x512", "1024x1024", "1024x1536", "1536x1024"}}, "quality": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}}, "filename": stringField("Optional output filename"),
		}, "prompt", "reference_uuids")},
	}
}

func projectAPIV4RequestUserInputDefinition() map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	stringField := func(description string, minimum, maximum int) map[string]any {
		return map[string]any{"type": "string", "description": description, "minLength": minimum, "maxLength": maximum}
	}
	integerField := func(description string) map[string]any {
		return map[string]any{"type": "integer", "description": description}
	}
	option := object(map[string]any{
		"label":       stringField("User-facing label of 1–5 words. The first option must end with ` (Recommended)`.", 1, 160),
		"description": stringField("One short sentence explaining the impact or tradeoff.", 1, 1000),
	}, "label", "description")
	question := object(map[string]any{
		"header":   stringField("Short UI header of at most 12 characters.", 1, 12),
		"id":       map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^[a-z][a-z0-9_]{0,63}$", "description": "Stable snake_case answer key."},
		"question": stringField("Single-sentence question shown to the user.", 1, 4000),
		"options":  map[string]any{"type": "array", "minItems": 2, "maxItems": 3, "items": option},
	}, "header", "id", "question", "options")
	return map[string]any{"name": "request_user_input", "description": "Pause this run and ask one to three short questions. Each question is mutually exclusive: put the recommended option first and suffix its label with ` (Recommended)`. Do not add an Other option; the client provides free-form Other automatically. Prefer one question and group questions only when they are directly related. For a dangerous Agent API route, include confirmation bound to its exact question and request fingerprint. If the user selects the bound option, the runtime executes the original request automatically; never copy confirmation into request_api or replay the request yourself.", "parameters": object(map[string]any{
		"questions": map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": question},
		"confirmation": object(map[string]any{
			"route": stringField("Dangerous global Agent API route ID.", 1, 160), "project_uuid": stringField("Current public project UUIDv7.", 36, 36), "target_uuid": stringField("Concrete target resource UUIDv7.", 36, 36),
			"expected_revision": integerField("Freshly read target revision."), "request_fingerprint": stringField("Exact sha256 fingerprint returned by request_api.", 71, 71), "question_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^[a-z][a-z0-9_]{0,63}$"}, "confirm_option": integerField("Zero-based confirming option index; index zero is reserved for the safe recommended option."),
		}, "route", "project_uuid", "target_uuid", "expected_revision", "request_fingerprint", "question_id", "confirm_option"),
	}, "questions")}
}

func validateToolArguments(name string, raw string) (map[string]any, error) {
	return validateToolArgumentsForMode(name, raw, ToolModeProjectAPI)
}

func validateToolArgumentsForMode(name string, raw string, mode string) (map[string]any, error) {
	return validateToolArgumentsForProtocol(name, raw, mode, "")
}

func validateToolArgumentsForProtocol(name string, raw string, mode, protocol string) (map[string]any, error) {
	if !json.Valid([]byte(raw)) {
		return nil, domainError(CodeToolValidation, "工具参数不是有效 JSON", "arguments 必须是 JSON object。", nil)
	}
	if err := rejectDuplicateJSONFields(raw); err != nil {
		return nil, err
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return nil, domainError(CodeToolValidation, "工具参数不是 JSON object", "arguments 必须是 JSON object。", err)
	}
	var parameters map[string]any
	definitions := toolDefinitionsForProtocol(mode, protocol)
	for _, definition := range definitions {
		if definition["name"] == name {
			parameters, _ = definition["parameters"].(map[string]any)
			break
		}
	}
	if parameters == nil {
		return nil, domainError(CodeToolNotAllowed, "工具不在 allowlist", "Agent 只能调用当前项目注册的受控工具。", nil)
	}
	if name == "request_api" {
		if err := rejectRequestAPIConfirmationPlacement(args); err != nil {
			return nil, err
		}
	}
	if err := validatePublicToolArguments(name, protocol, args); err != nil {
		return nil, err
	}
	if name == "request_user_input" && protocol != ToolProtocolProjectV2 && protocol != ToolProtocolProjectV3 && normalizedToolMode(mode) == ToolModeProjectAPI {
		normalizeProjectAPIV4ConfirmationQuestionID(args)
	}
	properties, _ := parameters["properties"].(map[string]any)
	for key, value := range args {
		rawSchema, ok := properties[key]
		if !ok {
			return nil, toolValidationError("工具参数包含未知字段", key+" 不在该工具的参数 schema 中。", toolValidationViolation{Path: key, Rule: "unknown_field"})
		}
		schema, _ := rawSchema.(map[string]any)
		if err := validateArgumentShape(key, value, schema); err != nil {
			return nil, err
		}
	}
	if required, ok := parameters["required"].([]string); ok {
		for _, key := range required {
			if _, exists := args[key]; !exists {
				return nil, toolValidationError("工具参数缺少必填字段", key+" 是必填字段。", toolValidationViolation{Path: key, Rule: "required"})
			}
		}
	}
	if name == "request_user_input" && protocol != ToolProtocolProjectV2 && protocol != ToolProtocolProjectV3 && normalizedToolMode(mode) == ToolModeProjectAPI {
		if err := validateProjectAPIV4UserInputArguments(args); err != nil {
			return nil, err
		}
	}
	return args, nil
}

func rejectRequestAPIConfirmationPlacement(args map[string]any) error {
	if _, exists := args["confirmation"]; exists {
		return requestAPIConfirmationPlacementError()
	}
	for _, key := range []string{"query", "request_body"} {
		if containsJSONField(args[key], "confirmation") {
			return requestAPIConfirmationPlacementError()
		}
	}
	return nil
}

func containsJSONField(value any, field string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == field || containsJSONField(child, field) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsJSONField(child, field) {
				return true
			}
		}
	}
	return false
}

func requestAPIConfirmationPlacementError() error {
	return domainError(
		CodeToolValidation,
		"request_api 不接受 confirmation",
		"confirmation 只能传给 request_user_input；用户确认后运行时会自动执行原请求，不要在 request_api、query 或 request_body 中携带 confirmation，也不要自行重放 request_api。",
		nil,
	)
}

func rejectDuplicateJSONFields(raw string) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return domainError(CodeToolValidation, "工具参数对象无效", "JSON object key 必须是字符串。", nil)
				}
				if _, exists := seen[key]; exists {
					return domainError(CodeToolValidation, "工具参数包含重复字段", key+" 在同一个 JSON object 中重复出现。", nil)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return domainError(CodeToolValidation, "工具参数 JSON 无效", "arguments 包含无效 JSON delimiter。", nil)
		}
	}
	if err := walk(); err != nil {
		return err
	}
	if decoder.More() {
		return domainError(CodeToolValidation, "工具参数 JSON 无效", "arguments 必须只包含一个 JSON object。", nil)
	}
	return nil
}

func validateArgumentShape(path string, value any, schema map[string]any) error {
	want, _ := schema["type"].(string)
	valid := false
	switch want {
	case "string":
		text, ok := value.(string)
		valid = ok
		if ok {
			if minimum, exists := schema["minLength"].(int); exists && len([]rune(text)) < minimum {
				valid = false
			}
			if maximum, exists := schema["maxLength"].(int); exists && len([]rune(text)) > maximum {
				valid = false
			}
			if pattern, exists := schema["pattern"].(string); exists {
				matched, err := regexp.MatchString(pattern, text)
				valid = valid && err == nil && matched
			}
		}
	case "integer":
		number, ok := value.(float64)
		valid = ok && number == float64(int64(number))
		if valid {
			if minimum, exists := schema["minimum"].(int); exists && int64(number) < int64(minimum) {
				valid = false
			}
			if maximum, exists := schema["maximum"].(int); exists && int64(number) > int64(maximum) {
				valid = false
			}
		}
	case "boolean":
		_, valid = value.(bool)
	case "array":
		values, ok := value.([]any)
		valid = ok
		if ok {
			if minimum, exists := schema["minItems"].(int); exists && len(values) < minimum {
				valid = false
			}
			if maximum, exists := schema["maxItems"].(int); exists && len(values) > maximum {
				valid = false
			}
			itemSchema, _ := schema["items"].(map[string]any)
			for index, item := range values {
				if err := validateArgumentShape(fmt.Sprintf("%s[%d]", path, index), item, itemSchema); err != nil {
					return err
				}
			}
		}
	case "object":
		object, ok := value.(map[string]any)
		valid = ok
		if ok {
			properties, _ := schema["properties"].(map[string]any)
			allowAdditional, _ := schema["additionalProperties"].(bool)
			for childKey, childValue := range object {
				rawChildSchema, exists := properties[childKey]
				childPath := argumentChildPath(path, childKey)
				if !exists {
					if allowAdditional {
						continue
					}
					return toolValidationError("工具参数包含未知字段", childPath+" 不在该工具的参数 schema 中。", toolValidationViolation{Path: childPath, Rule: "unknown_field"})
				}
				childSchema, _ := rawChildSchema.(map[string]any)
				if err := validateArgumentShape(childPath, childValue, childSchema); err != nil {
					return err
				}
			}
			if required, exists := schema["required"].([]string); exists {
				for _, childKey := range required {
					if _, present := object[childKey]; !present {
						childPath := argumentChildPath(path, childKey)
						return toolValidationError("工具参数缺少必填字段", childPath+" 是必填字段。", toolValidationViolation{Path: childPath, Rule: "required"})
					}
				}
			}
		}
	default:
		valid = true
	}
	if !valid {
		return toolValidationError("工具参数类型无效", path+" 不符合工具参数 schema。", toolValidationViolation{Path: path, Rule: "type", ExpectedType: want})
	}
	if enum, ok := schema["enum"].([]string); ok {
		text, _ := value.(string)
		matched := false
		for _, candidate := range enum {
			matched = matched || text == candidate
		}
		if !matched {
			encodedAllowed, _ := json.Marshal(enum)
			return toolValidationError(
				"工具参数枚举值无效",
				path+" 不在允许值中；允许值："+string(encodedAllowed)+"。",
				toolValidationViolation{Path: path, Rule: "enum", ExpectedType: "string", AllowedValues: enum},
			)
		}
	}
	return nil
}

func argumentChildPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func validatePublicArguments(value any, key string) error {
	return validatePublicArgumentsAt(value, key, nil, false)
}

func validatePublicToolArguments(name, protocol string, value any) error {
	allowQuestionID := name == "request_user_input" && protocol != ToolProtocolProjectV2 && protocol != ToolProtocolProjectV3
	return validatePublicArgumentsAt(value, "", nil, allowQuestionID)
}

func validatePublicArgumentsAt(value any, key string, path []string, allowQuestionID bool) error {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			lower := strings.ToLower(childKey)
			logicalQuestionID := allowQuestionID && ((lower == "id" && len(path) == 2 && path[0] == "questions" && path[1] == "[]") || (lower == "question_id" && len(path) == 1 && path[0] == "confirmation"))
			if (!logicalQuestionID && lower == "id") || (!logicalQuestionID && strings.HasSuffix(lower, "_id")) || strings.HasSuffix(lower, "_path") ||
				lower == "authorization" || lower == "cookie" || lower == "password" || strings.HasSuffix(lower, "_password") ||
				lower == "secret" || strings.HasSuffix(lower, "_secret") || lower == "credential" || strings.HasSuffix(lower, "_credential") ||
				lower == "token" || strings.HasSuffix(lower, "_token") || lower == "api_key" || strings.HasSuffix(lower, "_api_key") {
				return domainError(CodeToolValidation, "工具参数包含内部字段", "只允许公开业务字段和 UUID，不允许 id、磁盘路径或凭据字段。", nil)
			}
			if err := validatePublicArgumentsAt(child, childKey, append(path, childKey), allowQuestionID); err != nil {
				return err
			}
		}
	case []any:
		if strings.HasSuffix(strings.ToLower(key), "_uuids") {
			for _, child := range typed {
				text, ok := child.(string)
				if !ok || !isUUIDv7(text) {
					return domainError(CodeToolValidation, "工具 UUID 列表无效", key+" 必须是 UUIDv7 数组。", nil)
				}
			}
			return nil
		}
		for _, child := range typed {
			if err := validatePublicArgumentsAt(child, key, append(path, "[]"), allowQuestionID); err != nil {
				return err
			}
		}
	case string:
		lower := strings.ToLower(key)
		if strings.HasSuffix(lower, "_uuid") && typed != "" && !isUUIDv7(typed) {
			return domainError(CodeToolValidation, "工具 UUID 参数无效", key+" 必须是 UUIDv7。", nil)
		}
		if strings.HasSuffix(lower, "_uuids") && typed != "" {
			return domainError(CodeToolValidation, "工具 UUID 列表无效", key+" 必须是 UUIDv7 数组。", nil)
		}
	}
	return nil
}

var requestUserInputQuestionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// A dangerous v4 confirmation can only bind one question, so question_id is a
// redundant model-authored key rather than part of the security identity. Keep
// every security-bearing field strict, but canonicalize a non-empty mismatched
// question_id to the sole valid question key before schema validation.
func normalizeProjectAPIV4ConfirmationQuestionID(args map[string]any) bool {
	values, ok := args["questions"].([]any)
	if !ok || len(values) != 1 {
		return false
	}
	question, ok := values[0].(map[string]any)
	if !ok {
		return false
	}
	questionID := strings.TrimSpace(stringArg(question, "id"))
	if !requestUserInputQuestionIDPattern.MatchString(questionID) {
		return false
	}
	confirmation, ok := args["confirmation"].(map[string]any)
	if !ok {
		return false
	}
	providedID, ok := confirmation["question_id"].(string)
	if !ok || strings.TrimSpace(providedID) == "" || providedID == questionID {
		return false
	}
	confirmation["question_id"] = questionID
	return true
}

func projectAPIV4ConfirmationQuestionIDWasNormalized(name, protocol, mode, raw string, normalized map[string]any) bool {
	if name != "request_user_input" || protocol == ToolProtocolProjectV2 || protocol == ToolProtocolProjectV3 || normalizedToolMode(mode) != ToolModeProjectAPI {
		return false
	}
	var original map[string]any
	if json.Unmarshal([]byte(raw), &original) != nil {
		return false
	}
	originalConfirmation, originalOK := original["confirmation"].(map[string]any)
	normalizedConfirmation, normalizedOK := normalized["confirmation"].(map[string]any)
	if !originalOK || !normalizedOK {
		return false
	}
	originalID, originalOK := originalConfirmation["question_id"].(string)
	normalizedID, normalizedOK := normalizedConfirmation["question_id"].(string)
	return originalOK && normalizedOK && originalID != normalizedID
}

func validateProjectAPIV4UserInputArguments(args map[string]any) error {
	values, _ := args["questions"].([]any)
	seen := map[string]struct{}{}
	for _, value := range values {
		question, _ := value.(map[string]any)
		header := strings.TrimSpace(stringArg(question, "header"))
		questionID := strings.TrimSpace(stringArg(question, "id"))
		prompt := strings.TrimSpace(stringArg(question, "question"))
		options, _ := question["options"].([]any)
		if header == "" || len([]rune(header)) > 12 || prompt == "" || len([]rune(prompt)) > 4000 || !requestUserInputQuestionIDPattern.MatchString(questionID) {
			return domainError(CodeToolValidation, "用户输入问题无效", "header、id 或 question 不符合 request_user_input v4 限制。", nil)
		}
		if _, duplicate := seen[questionID]; duplicate {
			return domainError(CodeToolValidation, "用户输入问题 ID 重复", "questions 中的 id 必须唯一。", nil)
		}
		seen[questionID] = struct{}{}
		for index, rawOption := range options {
			option, _ := rawOption.(map[string]any)
			label := strings.TrimSpace(stringArg(option, "label"))
			description := strings.TrimSpace(stringArg(option, "description"))
			recommended := strings.HasSuffix(label, " (Recommended)")
			if label == "" || len([]rune(label)) > 160 || description == "" || len([]rune(description)) > 1000 || strings.ContainsAny(description, "\r\n") || (index == 0) != recommended || strings.EqualFold(label, "Other") || strings.EqualFold(label, "Other (Recommended)") {
				return domainError(CodeToolValidation, "用户输入选项无效", "选项必须包含非空 label/description；仅第一项 label 必须以 ` (Recommended)` 结尾，且模型不得创建 Other。", nil)
			}
		}
	}
	if raw, exists := args["confirmation"]; exists {
		confirmation, _ := raw.(map[string]any)
		questionID := strings.TrimSpace(stringArg(confirmation, "question_id"))
		if len(values) != 1 || questionID == "" {
			return domainError(CodeToolValidation, "危险操作确认问题无效", "confirmation 只允许绑定单问题请求，并且必须提供 question_id。", nil)
		}
		question, _ := values[0].(map[string]any)
		if questionID != strings.TrimSpace(stringArg(question, "id")) {
			return domainError(CodeToolValidation, "危险操作确认问题不匹配", "confirmation.question_id 必须匹配唯一问题的 id。", nil)
		}
		options, _ := question["options"].([]any)
		confirmOption := int(intArg(confirmation, "confirm_option"))
		if confirmOption <= 0 || confirmOption >= len(options) {
			return domainError(CodeToolValidation, "危险操作确认选项无效", "confirmation.confirm_option 必须绑定非首项的有效选项；首项保留为安全推荐项。", nil)
		}
	}
	return nil
}

func toolCallKey(runUUID, providerCallID, toolName string) string {
	digest := sha256.Sum256([]byte(runUUID + "\x00" + providerCallID + "\x00" + toolName))
	return "agent-tool-v1:" + hex.EncodeToString(digest[:])
}

func (service *Service) persistToolIntent(ctx context.Context, store *project.Store, tc toolContext, providerCallID, name, raw string) (toolExecutionRecord, json.RawMessage, bool, error) {
	key := toolCallKey(tc.Run.UUID, providerCallID, name)
	var existing toolExecutionRecord
	query := store.DB().WithContext(ctx).Where("idempotency_key=?", key).First(&existing)
	if query.Error == nil {
		if existing.State == "completed" && existing.ResultJSON != nil {
			return existing, json.RawMessage(*existing.ResultJSON), true, nil
		}
		return existing, nil, false, nil
	}
	if !errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return existing, nil, false, query.Error
	}
	args, err := validateToolArgumentsForProtocol(name, raw, tc.ToolMode, tc.ToolProtocol)
	if err != nil {
		return existing, nil, false, err
	}
	questionIDRepaired := projectAPIV4ConfirmationQuestionIDWasNormalized(name, tc.ToolProtocol, tc.ToolMode, raw, args)
	if !toolAllowedForThreadMode(name, tc.Thread, tc.ToolMode) {
		return existing, nil, false, domainError(CodeToolNotAllowed, "工具不适用于当前 Run", "当前冻结的 Tool Mode 无法使用该工具。", nil)
	}
	var apiRequest agentAPIRequest
	if normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped {
		if err := validateLegacyRecoveryIntent(tc, name, args); err != nil {
			return existing, nil, false, err
		}
	}
	if name == "request_api" {
		apiRequest, err = service.parseAgentAPIRequest(tc, args)
		if err != nil {
			return existing, nil, false, err
		}
	} else if name == "read_agent_doc" {
		if _, err := service.readAgentDoc(tc, args); err != nil {
			return existing, nil, false, err
		}
	}
	if normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped && !legacyRecoveryToolTargetAllowed(name, args, tc.Thread) {
		return existing, nil, false, domainError(CodeToolNotAllowed, "旧协议工具目标越界", "恢复中的旧设定项引用只能读写其冻结 subject_uuid。", nil)
	}
	publicCallUUID, err := newUUIDv7()
	if err != nil {
		return existing, nil, false, err
	}
	executionUUID, err := newUUIDv7()
	if err != nil {
		return existing, nil, false, err
	}
	targetUUID := ""
	if name == "image_gen" {
		targetUUID = tc.Thread.UUID
	} else if name == "request_api" {
		targetUUID = apiRequest.TargetUUID
	} else if name == "read_agent_doc" {
		targetUUID = tc.Thread.UUID
	} else if normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped {
		targetUUID = legacyRecoveryTargetUUID(name, args, tc.Thread)
	}
	routeID, action, method, path := "", name, "", ""
	if name == "request_api" {
		routeID, action, method, path = apiRequest.Route.ID, apiRequest.Route.Action, apiRequest.Method, apiRequest.Path
	} else if name == "read_agent_doc" {
		routeID, action, method, path = "agent_doc.read", "读取 Agent 文档", "READ", stringArg(args, "path")
	}
	storedArgs := make(map[string]any, len(args)+1)
	for key, value := range args {
		storedArgs[key] = value
	}
	storedArgs["__provider_call_id"] = providerCallID
	if isUUIDv7(tc.RequestUUID) {
		storedArgs["__request_uuid"] = tc.RequestUUID
		storedArgs["__request_ordinal"] = tc.RequestOrdinal
	}
	if routeID != "" {
		storedArgs["__route_id"], storedArgs["__action"] = routeID, action
		storedArgs["__method"], storedArgs["__path"] = method, path
		storedArgs["__target_uuid"] = targetUUID
	}
	encodedArgs, _ := json.Marshal(storedArgs)
	sqlDB, err := store.DB().DB()
	if err != nil {
		return existing, nil, false, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return existing, nil, false, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return existing, nil, false, err
	}
	now := service.now().UTC()
	metadata := map[string]any{"purpose": name, "action": action, "target_uuid": targetUUID, "provider_call_id": providerCallID}
	if questionIDRepaired {
		metadata["argument_repaired"] = confirmationQuestionIDArgumentRepair
	}
	if isUUIDv7(tc.RequestUUID) {
		metadata["request_uuid"] = tc.RequestUUID
		metadata["request_ordinal"] = tc.RequestOrdinal
	}
	if routeID != "" {
		metadata["route_id"], metadata["method"], metadata["path"] = routeID, method, path
	}
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", raw, "json", "in_progress", publicCallUUID, name, targetUUID, metadata, now)
	if err != nil {
		return existing, nil, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'intent',?,?)`, executionUUID, tc.Thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, publicCallUUID, name, targetUUID, string(encodedArgs), key, now, now)
	if err != nil {
		return existing, nil, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return existing, nil, false, err
	}
	toolEvent := map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": publicCallUUID, "tool_name": name, "route_id": routeID, "action": action, "method": method, "path": path, "target_uuid": targetUUID}
	if questionIDRepaired {
		toolEvent["argument_repaired"] = confirmationQuestionIDArgumentRepair
	}
	if isUUIDv7(tc.RequestUUID) {
		toolEvent["request_uuid"] = tc.RequestUUID
		toolEvent["request_ordinal"] = tc.RequestOrdinal
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "tool_intent", toolEvent, now); err != nil {
		return existing, nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return existing, nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return existing, nil, false, err
	}
	existing = toolExecutionRecord{ID: id, ThreadID: tc.Thread.ID, RunID: tc.Run.ID, TurnID: tc.Turn.ID, ItemID: item.ID, UUID: executionUUID, ToolCallUUID: publicCallUUID, ToolName: name, TargetUUID: targetUUID, ArgumentsJSON: string(encodedArgs), IdempotencyKey: key, RouteID: routeID, Action: action, Method: method, Path: path, State: "intent", CreatedAt: now, UpdatedAt: now}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_call", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": publicCallUUID, "tool_name": name, "route_id": routeID, "action": action, "method": method, "path": path, "target_uuid": targetUUID, "status": "in_progress"})
	return existing, nil, false, nil
}

// persistRejectedToolCall records a model-produced tool call that was safe to
// inspect but failed argument validation. The paired tool result gives the
// model a bounded opportunity to repair its own call without executing it.
func (service *Service) persistRejectedToolCall(ctx context.Context, store *project.Store, tc toolContext, providerCallID, name, raw string, cause error) (bool, json.RawMessage, error) {
	sqlDB, err := store.DB().DB()
	if err != nil {
		return false, nil, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return false, nil, err
	}
	var repairs int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_items WHERE run_id=? AND item_type='tool_result' AND json_extract(metadata_json,'$.validation_repair')=1`, tc.Run.ID).Scan(&repairs); err != nil {
		return false, nil, err
	}
	if repairs >= maxToolValidationRepairs {
		return false, nil, nil
	}
	publicCallUUID, err := newUUIDv7()
	if err != nil {
		return false, nil, err
	}
	now := service.now().UTC()
	code := errorCode(cause)
	recovery, _ := service.buildRejectedAgentAPICallRecovery(tc, name, raw, cause)
	result, recoveryIncluded := toolErrorResultWithRecovery(cause, recovery)
	routeID, action, method, path, targetUUID, docPath := "", name, "", "", "", ""
	if recovery != nil {
		routeID, action, method, path = recovery.Route.ID, recovery.Route.Action, recovery.Route.Method, recovery.Path
		targetUUID, docPath = recovery.TargetUUID, recovery.Route.DocPath
	}
	violation, hasViolation := toolValidationViolationFromError(cause)
	metadata := map[string]any{
		"purpose": name, "action": action, "target_uuid": targetUUID, "provider_call_id": providerCallID,
		"validation_repair": true, "error_code": code,
	}
	if routeID != "" {
		metadata["route_id"], metadata["method"], metadata["path"], metadata["doc_path"] = routeID, method, path, docPath
	}
	if hasViolation && violation.Path != "" {
		metadata["validation_path"] = violation.Path
	}
	if recoveryIncluded {
		metadata["recovery_contract_included"] = true
	}
	if isUUIDv7(tc.RequestUUID) {
		metadata["request_uuid"] = tc.RequestUUID
		metadata["request_ordinal"] = tc.RequestOrdinal
	}
	format := "text"
	if json.Valid([]byte(raw)) {
		format = "json"
	}
	if _, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_call", "assistant", raw, format, "failed", publicCallUUID, name, targetUUID, metadata, now); err != nil {
		return false, nil, err
	}
	resultItem, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_result", "tool", string(result), "json", "completed", publicCallUUID, name, targetUUID, metadata, now)
	if err != nil {
		return false, nil, err
	}
	eventBase := map[string]any{
		"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID,
		"run_uuid": tc.Run.UUID, "tool_call_uuid": publicCallUUID, "tool_name": name,
		"route_id": routeID, "action": action, "method": method, "path": path,
		"target_uuid": targetUUID, "status": "failed", "error_code": code,
	}
	if docPath != "" {
		eventBase["doc_path"] = docPath
	}
	if hasViolation && violation.Path != "" {
		eventBase["validation_path"] = violation.Path
	}
	if recoveryIncluded {
		eventBase["recovery_contract_included"] = true
	}
	if isUUIDv7(tc.RequestUUID) {
		eventBase["request_uuid"] = tc.RequestUUID
		eventBase["request_ordinal"] = tc.RequestOrdinal
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "tool_intent", eventBase, now); err != nil {
		return false, nil, err
	}
	resultEvent := make(map[string]any, len(eventBase)+1)
	for key, value := range eventBase {
		resultEvent[key] = value
	}
	resultEvent["item_uuid"] = resultItem.UUID
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "tool_result", resultEvent, now); err != nil {
		return false, nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return false, nil, err
	}
	if err := tx.Commit(); err != nil {
		return false, nil, err
	}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_call", eventBase)
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_result", resultEvent)
	return true, result, nil
}

func (service *Service) executeTool(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord) (json.RawMessage, error) {
	if execution.ID > 0 && execution.State == "completed" {
		if execution.ResultJSON == nil || len(*execution.ResultJSON) > MaxToolResult || !json.Valid([]byte(*execution.ResultJSON)) {
			return nil, domainError(CodeStateConflict, "已完成工具结果损坏", "completed 工具调用必须保留有效且未超限的结果信封。", nil)
		}
		var envelope struct {
			Success *bool `json:"success"`
		}
		if json.Unmarshal([]byte(*execution.ResultJSON), &envelope) != nil || envelope.Success == nil {
			return nil, domainError(CodeStateConflict, "已完成工具结果损坏", "completed 工具调用缺少统一结果信封，不能安全重放。", nil)
		}
		return append(json.RawMessage(nil), (*execution.ResultJSON)...), nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(execution.ArgumentsJSON), &args); err != nil {
		return nil, domainError(CodeToolValidation, "持久化工具参数损坏", "无法安全恢复工具调用。", err)
	}
	delete(args, "__provider_call_id")
	delete(args, "__request_uuid")
	delete(args, "__request_ordinal")
	// Active request_api calls are schema-required to provide a narrow filter.
	// A pre-upgrade intent may already be persisted without one, so recovery
	// selects a projector-compatible filter without weakening new-call checks.
	if execution.ToolName == "request_api" && strings.TrimSpace(stringArg(args, "response_filter")) == "" {
		args["response_filter"] = service.recoveryAgentAPIResponseFilter(args)
	}
	service.hydrateToolExecutionMetadata(tc, &execution, args)
	if execution.ID > 0 && execution.State != "executing" {
		now := service.now().UTC()
		updated := store.DB().WithContext(ctx).Model(&toolExecutionRecord{}).Table("agent_tool_executions").Where("id=? AND state='intent'", execution.ID).Updates(map[string]any{"state": "executing", "started_at": now, "updated_at": now})
		if updated.Error != nil {
			return nil, updated.Error
		}
		if updated.RowsAffected != 1 {
			var state string
			if err := store.DB().WithContext(ctx).Table("agent_tool_executions").Select("state").Where("id=?", execution.ID).Scan(&state).Error; err != nil {
				return nil, err
			}
			if state != "executing" {
				return nil, domainError(CodeStateConflict, "工具执行状态无法领取", "只有 intent 或已领取的 executing 工具调用可以执行。", nil)
			}
		}
		execution.State = "executing"
	}
	var value any
	var uiRef *agentUIReference
	var err error
	switch execution.ToolName {
	case "request_api":
		var output requestAPIToolOutput
		output, err = executeRequestAPIToolWithUIRef(ctx, service, store, tc, execution, args)
		value, uiRef = output.Data, output.UIRef
	case "read_agent_doc":
		value, err = service.readAgentDoc(tc, args)
	case "image_gen":
		value, err = service.executeImageGenTool(ctx, store, tc, execution, args)
	default:
		if normalizedToolMode(tc.ToolMode) == ToolModeLegacyTyped {
			value, err = service.executeLegacyToolRecovery(ctx, store, tc, execution, args)
		} else {
			err = domainError(CodeToolNotAllowed, "工具不在 allowlist", "工具未注册。", nil)
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, ctx.Err()) || errors.Is(err, ErrWaitingWorkflow) {
			return nil, err
		}
		return toolErrorResult(err), nil
	}
	envelope := map[string]any{"success": true, "data": value}
	if uiRef != nil {
		envelope["ui_ref"] = uiRef
	}
	return compactToolResult(envelope, execution.TargetUUID), nil
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func intArg(args map[string]any, key string) int64 {
	switch value := args[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		result, _ := value.Int64()
		return result
	}
	return 0
}

func stringSliceArg(args map[string]any, key string) []string {
	values, _ := args[key].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func updateStoryProfileTool(ctx context.Context, service *story.Service, args map[string]any) (story.StoryProfile, error) {
	desired := stringArg(args, "story_md")
	current, err := service.GetStoryProfile(ctx)
	if err != nil {
		return story.StoryProfile{}, err
	}
	if strings.TrimSpace(current.StoryMD) == strings.TrimSpace(desired) {
		return current, nil
	}
	return service.UpdateStoryProfile(ctx, desired, intArg(args, "expected_revision"))
}

func updateChapterStoryTool(ctx context.Context, service *story.Service, args map[string]any) (story.Chapter, error) {
	chapterUUID := stringArg(args, "chapter_uuid")
	desired := stringArg(args, "content")
	current, err := service.GetChapter(ctx, chapterUUID)
	if err != nil {
		return story.Chapter{}, err
	}
	if current.CurrentStory != nil && strings.TrimSpace(current.CurrentStory.Content) == strings.TrimSpace(desired) && current.CurrentStory.ContentFormat == stringArg(args, "content_format") {
		return current, nil
	}
	return service.UpdateStory(ctx, chapterUUID, story.UpdateStoryInput{Content: desired, ContentFormat: stringArg(args, "content_format"), ExpectedRevision: intArg(args, "expected_revision")})
}

func createPremiseAssetTool(ctx context.Context, service *production.Service, tc toolContext, execution toolExecutionRecord, args map[string]any) (production.PremiseAsset, error) {
	uploadUUID, fileUUID := stringArg(args, "upload_uuid"), stringArg(args, "file_uuid")
	if (uploadUUID == "") == (fileUUID == "") {
		return production.PremiseAsset{}, domainError(CodeToolValidation, "设定项图片来源无效", "file_uuid 与 upload_uuid 必须且只能提供一个。", nil)
	}
	input := production.CreateAssetInput{UploadUUID: uploadUUID, FileUUID: fileUUID, ToolExecutionUUID: execution.UUID, ChatThreadUUID: tc.Thread.UUID, AssetType: stringArg(args, "asset_type"), Title: stringArg(args, "title"), Summary: stringArg(args, "summary"), Tags: stringSliceArg(args, "tags"), SourceType: "manual"}
	if fileUUID != "" {
		return service.CreatePremiseAssetFromFile(ctx, input)
	}
	return service.ImportPremiseAsset(ctx, input)
}

func updatePremiseAssetTool(ctx context.Context, service *production.Service, tc toolContext, execution toolExecutionRecord, args map[string]any) (production.PremiseAsset, error) {
	uuid := stringArg(args, "premise_asset_uuid")
	current, err := service.GetPremiseAsset(ctx, uuid)
	if err != nil {
		return production.PremiseAsset{}, err
	}
	input := production.UpdateAssetInput{ExpectedRevision: intArg(args, "expected_revision"), FileUUID: stringArg(args, "file_uuid"), ToolExecutionUUID: execution.UUID, ChatThreadUUID: tc.Thread.UUID}
	if value, ok := args["asset_type"].(string); ok {
		input.AssetType = &value
	}
	if value, ok := args["title"].(string); ok {
		input.Title = &value
	}
	if value, ok := args["summary"].(string); ok {
		input.Summary = &value
	}
	if _, ok := args["tags"]; ok {
		value := stringSliceArg(args, "tags")
		input.Tags = &value
	}
	if input.FileUUID != "" {
		return service.UpdatePremiseAssetFromFile(ctx, uuid, input)
	}
	if current.Revision == input.ExpectedRevision && premiseAssetMatches(current, input) {
		return current, nil
	}
	return service.UpdatePremiseAsset(ctx, uuid, input)
}

func premiseAssetMatches(current production.PremiseAsset, input production.UpdateAssetInput) bool {
	if input.AssetType != nil && current.AssetType != strings.TrimSpace(*input.AssetType) {
		return false
	}
	if input.Title != nil && current.Title != strings.TrimSpace(*input.Title) {
		return false
	}
	if input.Summary != nil && current.Summary != strings.TrimSpace(*input.Summary) {
		return false
	}
	if input.Tags != nil {
		currentTags, wanted := append([]string(nil), current.Tags...), append([]string(nil), (*input.Tags)...)
		if strings.Join(currentTags, "\x00") != strings.Join(wanted, "\x00") {
			return false
		}
	}
	return true
}

func updateStoryboardTool(ctx context.Context, service *production.Service, args map[string]any) (production.ComicSection, error) {
	chapterUUID, sectionUUID, desired := stringArg(args, "chapter_uuid"), stringArg(args, "section_uuid"), stringArg(args, "content_md")
	current, err := service.GetSection(ctx, chapterUUID, sectionUUID)
	if err != nil {
		return production.ComicSection{}, err
	}
	if current.CurrentStoryboard != nil && strings.TrimSpace(current.CurrentStoryboard.ContentMD) == strings.TrimSpace(desired) {
		return current, nil
	}
	return service.CreateStoryboard(ctx, chapterUUID, sectionUUID, desired, "generated", intArg(args, "expected_revision"))
}

func (service *Service) startGenerationTool(ctx context.Context, tc toolContext, execution toolExecutionRecord, args map[string]any) (DomainTask, error) {
	request := DomainTaskRequest{Kind: stringArg(args, "kind"), ResourceUUID: stringArg(args, "resource_uuid"), ChapterUUID: stringArg(args, "chapter_uuid"), ProviderUUID: tc.Run.ProviderUUID, Model: stringArg(args, "model"), Prompt: stringArg(args, "prompt"), PremiseAssetUUIDs: stringSliceArg(args, "premise_asset_uuids"), IdempotencyKey: execution.IdempotencyKey, Invocation: chatToolInvocationContext(tc, execution)}
	return service.queue.StartDomainTask(ctx, tc.ProjectUUID, request)
}

func toolErrorResult(err error) json.RawMessage {
	code, message, details := CodeToolValidation, "工具调用失败。", ""
	var agentErr *Error
	if errors.As(err, &agentErr) {
		code, message = agentErr.Code, agentErr.Message
		if code == CodeToolConfirmation || code == CodeToolValidation {
			details = agentErr.Details
		}
	} else {
		var storyErr *story.Error
		var productionErr *production.Error
		var imageErr *imagegen.Error
		var projectErr *project.Error
		switch {
		case errors.As(err, &projectErr):
			code, message, details = projectErr.Code, projectErr.Message, projectErr.Details
		case errors.As(err, &storyErr):
			code, message = storyErr.Code, storyErr.Message
		case errors.As(err, &productionErr):
			code, message = productionErr.Code, productionErr.Message
		case errors.As(err, &imageErr):
			code, message = imageErr.Code, imageErr.SafeMessage
		}
	}
	encoded, _ := json.Marshal(map[string]any{"success": false, "data": nil, "error": map[string]any{"code": code, "message": message, "details": details}})
	return encoded
}

func compactToolResult(value any, targetUUID string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return toolErrorResult(domainError(CodeToolValidation, "工具结果无法编码", "result 不是有效 JSON。", err))
	}
	if len(encoded) <= MaxToolResult {
		return encoded
	}
	previewBytes := encoded
	if len(previewBytes) > 8<<10 {
		previewBytes = previewBytes[:8<<10]
	}
	result := map[string]any{"success": true, "data": map[string]any{"compacted": true, "target_uuid": targetUUID, "byte_size": len(encoded), "preview": string(previewBytes)}}
	if envelope, ok := value.(map[string]any); ok {
		if uiRef, exists := envelope["ui_ref"]; exists {
			result["ui_ref"] = uiRef
		}
	}
	compacted, _ := json.Marshal(result)
	return compacted
}

func (service *Service) persistToolResult(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord, result json.RawMessage) error {
	if len(result) > MaxToolResult || !json.Valid(result) {
		return domainError(CodeResultTooLarge, "工具结果过大或无效", "结果必须是不超过限制的有效 JSON。", nil)
	}
	var executionArgs map[string]any
	_ = json.Unmarshal([]byte(execution.ArgumentsJSON), &executionArgs)
	delete(executionArgs, "__provider_call_id")
	service.hydrateToolExecutionMetadata(tc, &execution, executionArgs)
	sqlDB, err := store.DB().DB()
	if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return err
	}
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM agent_tool_executions WHERE id=?`, execution.ID).Scan(&state); err != nil {
		return err
	}
	if state == "completed" {
		return tx.Commit()
	}
	now := service.now().UTC()
	metadata := map[string]any{"purpose": execution.ToolName, "action": execution.Action, "route_id": execution.RouteID, "method": execution.Method, "path": execution.Path, "target_uuid": execution.TargetUUID}
	var args map[string]any
	_ = json.Unmarshal([]byte(execution.ArgumentsJSON), &args)
	if value, ok := args["__provider_call_id"].(string); ok {
		metadata["provider_call_id"] = value
	}
	requestUUID, _ := args["__request_uuid"].(string)
	if isUUIDv7(requestUUID) {
		metadata["request_uuid"] = requestUUID
		metadata["request_ordinal"] = intArg(args, "__request_ordinal")
	}
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "tool_result", "tool", string(result), "json", "completed", execution.ToolCallUUID, execution.ToolName, execution.TargetUUID, metadata, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_tool_executions SET state='completed',result_json=?,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=?`, string(result), now, now, execution.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_items SET status='completed' WHERE id=? AND status='in_progress'`, execution.ItemID); err != nil {
		return err
	}
	toolResultEvent := map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": execution.ToolCallUUID, "tool_name": execution.ToolName, "route_id": execution.RouteID, "action": execution.Action, "method": execution.Method, "path": execution.Path, "target_uuid": execution.TargetUUID, "item_uuid": item.UUID}
	if isUUIDv7(requestUUID) {
		toolResultEvent["request_uuid"] = requestUUID
		toolResultEvent["request_ordinal"] = intArg(args, "__request_ordinal")
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "tool_result", toolResultEvent, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:tool_result", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "tool_call_uuid": execution.ToolCallUUID, "tool_name": execution.ToolName, "route_id": execution.RouteID, "action": execution.Action, "method": execution.Method, "path": execution.Path, "target_uuid": execution.TargetUUID, "status": "completed"})
	return nil
}

func (service *Service) createUserInputRequest(ctx context.Context, store *project.Store, tc toolContext, execution toolExecutionRecord) (UserInputRequest, error) {
	prepared, err := service.prepareUserInputRequest(tc, execution.ArgumentsJSON)
	if err != nil {
		return UserInputRequest{}, err
	}
	requestUUID, err := newUUIDv7()
	if err != nil {
		return UserInputRequest{}, err
	}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return UserInputRequest{}, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return UserInputRequest{}, err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return UserInputRequest{}, err
	}
	var existing userInputRow
	err = tx.QueryRowContext(ctx, `SELECT q.id,q.thread_id,q.run_id,q.turn_id,q.item_id,q.uuid,q.tool_call_uuid,q.schema_version,q.request_json,q.response_json,q.status,q.answered_at,q.resumed_at,q.cancelled_at,q.created_at,q.updated_at,r.uuid,t.uuid,i.uuid,i.metadata_json FROM chat_user_input_requests q JOIN chat_runs r ON r.id=q.run_id JOIN chat_turns t ON t.id=q.turn_id JOIN chat_items i ON i.id=q.item_id WHERE q.run_id=? AND q.tool_call_uuid=?`, tc.Run.ID, execution.ToolCallUUID).Scan(&existing.ID, &existing.ThreadID, &existing.RunID, &existing.TurnID, &existing.ItemID, &existing.UUID, &existing.ToolCallUUID, &existing.SchemaVersion, &existing.RequestJSON, &existing.ResponseJSON, &existing.Status, &existing.AnsweredAt, &existing.ResumedAt, &existing.CancelledAt, &existing.CreatedAt, &existing.UpdatedAt, &existing.RunUUID, &existing.TurnUUID, &existing.ItemUUID, &existing.ItemMetadataJSON)
	if err == nil {
		existing.ThreadUUID = tc.Thread.UUID
		return existing.DTO(), tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return UserInputRequest{}, err
	}
	var confirmationSource dangerousConfirmationSourceExecution
	if prepared.Confirmation != nil {
		confirmationSource, err = service.findDangerousConfirmationSourceTx(ctx, tx, tc.Run.ID, execution.ID, tc.ProjectUUID, "", *prepared.Confirmation)
		if err != nil {
			return UserInputRequest{}, err
		}
	}
	now := service.now().UTC()
	contentObject := map[string]any{"request_uuid": requestUUID, "schema_version": prepared.SchemaVersion}
	if prepared.SchemaVersion == userInputSchemaCodexQuestions {
		contentObject["questions"] = prepared.Questions
	} else {
		contentObject["input_type"] = prepared.InputType
		contentObject["question"] = prepared.Question
		contentObject["options"] = prepared.Options
	}
	content, _ := json.Marshal(contentObject)
	itemMetadata := map[string]any{"purpose": "request_user_input"}
	if confirmationSource.UUID != "" {
		itemMetadata["confirmation_source_execution_uuid"] = confirmationSource.UUID
	}
	item, err := appendItemTx(ctx, tx, &thread, &tc.Turn.ID, &tc.Run.ID, "user_input_request", "assistant", string(content), "json", "completed", execution.ToolCallUUID, "request_user_input", requestUUID, itemMetadata, now)
	if err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO chat_user_input_requests(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,schema_version,request_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,'pending',?,?)`, requestUUID, thread.ID, tc.Run.ID, tc.Turn.ID, item.ID, execution.ToolCallUUID, prepared.SchemaVersion, prepared.RequestJSON, now, now); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_tool_executions SET state='executing',started_at=COALESCE(started_at,?),updated_at=? WHERE id=?`, now, now, execution.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_runs SET status='waiting_for_input',updated_at=? WHERE id=? AND status='in_progress'`, now, tc.Run.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_turns SET status='waiting_for_input',updated_at=? WHERE id=? AND status='in_progress'`, now, tc.Turn.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_item_sequence=?,next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextItemSequence, thread.NextEventSequence, now, thread.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "user_input_requested", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "request_uuid": requestUUID, "tool_call_uuid": execution.ToolCallUUID}, now); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=? WHERE id=?`, thread.NextEventSequence, thread.ID); err != nil {
		return UserInputRequest{}, err
	}
	if _, err := RecomputeThreadStatusTx(ctx, tx, thread.ID, now); err != nil {
		return UserInputRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return UserInputRequest{}, err
	}
	request := UserInputRequest{UUID: requestUUID, ThreadUUID: tc.Thread.UUID, RunUUID: tc.Run.UUID, TurnUUID: tc.Turn.UUID, ItemUUID: item.UUID, ToolCallUUID: execution.ToolCallUUID, SchemaVersion: prepared.SchemaVersion, Questions: prepared.Questions, InputType: prepared.InputType, Question: prepared.Question, Options: prepared.Options, Status: "pending", CreatedAt: now, UpdatedAt: now}
	service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:user_input_requested", map[string]any{"project_uuid": tc.ProjectUUID, "thread_uuid": tc.Thread.UUID, "turn_uuid": tc.Turn.UUID, "run_uuid": tc.Run.UUID, "request_uuid": requestUUID, "status": "pending"})
	return request, nil
}

type preparedUserInputRequest struct {
	SchemaVersion string
	RequestJSON   string
	Questions     []UserInputQuestion
	InputType     string
	Question      string
	Options       []UserInputOption
	Confirmation  *dangerousConfirmationBinding
}

func (service *Service) prepareUserInputRequest(tc toolContext, raw string) (preparedUserInputRequest, error) {
	if usesCodexUserInputProtocol(tc) {
		return service.prepareCodexUserInputRequest(tc, raw)
	}
	return service.prepareLegacyUserInputRequest(tc, raw)
}

func (service *Service) prepareCodexUserInputRequest(tc toolContext, raw string) (preparedUserInputRequest, error) {
	var args struct {
		Questions []struct {
			Header   string `json:"header"`
			ID       string `json:"id"`
			Question string `json:"question"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
		Confirmation *dangerousConfirmationBinding `json:"confirmation"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return preparedUserInputRequest{}, domainError(CodeToolValidation, "用户输入参数无效", "无法解析 request_user_input。", err)
	}
	questions := make([]UserInputQuestion, 0, len(args.Questions))
	for _, candidate := range args.Questions {
		question := UserInputQuestion{Header: strings.TrimSpace(candidate.Header), ID: strings.TrimSpace(candidate.ID), Question: strings.TrimSpace(candidate.Question)}
		for _, rawOption := range candidate.Options {
			uuid, err := newUUIDv7()
			if err != nil {
				return preparedUserInputRequest{}, err
			}
			question.Options = append(question.Options, UserInputOption{UUID: uuid, Label: strings.TrimSpace(rawOption.Label), Description: strings.TrimSpace(rawOption.Description)})
		}
		questions = append(questions, question)
	}
	if args.Confirmation != nil {
		route, err := service.validateCodexDangerousConfirmationBinding(tc, *args.Confirmation, questions)
		if err != nil {
			return preparedUserInputRequest{}, err
		}
		questions[0].Question = fmt.Sprintf("%s\n\n确认操作：%s（%s）；目标 UUID：%s；expected_revision：%d。", questions[0].Question, route.Action, route.ID, args.Confirmation.TargetUUID, args.Confirmation.ExpectedRevision)
	}
	requestJSON, _ := json.Marshal(map[string]any{"questions": questions})
	return preparedUserInputRequest{SchemaVersion: userInputSchemaCodexQuestions, RequestJSON: string(requestJSON), Questions: questions, Confirmation: args.Confirmation}, nil
}

func (service *Service) prepareLegacyUserInputRequest(tc toolContext, raw string) (preparedUserInputRequest, error) {
	var args struct {
		InputType string `json:"input_type"`
		Question  string `json:"question"`
		Options   []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
		Confirmation *dangerousConfirmationBinding `json:"confirmation"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return preparedUserInputRequest{}, domainError(CodeToolValidation, "用户输入参数无效", "无法解析 request_user_input。", err)
	}
	question := strings.TrimSpace(args.Question)
	if (args.InputType != "single_choice" && args.InputType != "multiple_choice") || question == "" || len([]rune(question)) > 4000 || len(args.Options) < 2 || len(args.Options) > 8 {
		return preparedUserInputRequest{}, domainError(CodeToolValidation, "用户输入请求无效", "问题和选项不符合限制。", nil)
	}
	if args.Confirmation != nil {
		route, err := service.validateDangerousConfirmationBinding(tc, *args.Confirmation, args.InputType, len(args.Options))
		if err != nil {
			return preparedUserInputRequest{}, err
		}
		question = fmt.Sprintf("%s\n\n确认操作：%s（%s）；目标 UUID：%s；expected_revision：%d。", question, route.Action, route.ID, args.Confirmation.TargetUUID, args.Confirmation.ExpectedRevision)
	}
	options := make([]UserInputOption, 0, len(args.Options))
	for _, candidate := range args.Options {
		label := strings.TrimSpace(candidate.Label)
		description := strings.TrimSpace(candidate.Description)
		if label == "" || len([]rune(label)) > 160 || len([]rune(description)) > 1000 {
			return preparedUserInputRequest{}, domainError(CodeToolValidation, "用户输入选项无效", "选项标签不能为空且最多 160 字符。", nil)
		}
		uuid, err := newUUIDv7()
		if err != nil {
			return preparedUserInputRequest{}, err
		}
		options = append(options, UserInputOption{UUID: uuid, Label: label, Description: description})
	}
	requestJSON, _ := json.Marshal(map[string]any{"input_type": args.InputType, "question": question, "options": options})
	return preparedUserInputRequest{SchemaVersion: userInputSchemaLegacyChoice, RequestJSON: string(requestJSON), InputType: args.InputType, Question: question, Options: options, Confirmation: args.Confirmation}, nil
}

// Assert the compiler keeps the SQL transaction boundary used by tool intent.
var _ *sql.Tx
var _ = fmt.Sprintf
