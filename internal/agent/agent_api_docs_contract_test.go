package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"lumi/internal/files"
)

const maxAgentAPIDocSoftBytes = 24 << 10

const agentAPIDocPublicAssetShape = "public_asset_v1"

var (
	agentAPIDocOperationHeading = regexp.MustCompile(`(?m)^## \x60(GET|POST|PUT|PATCH|DELETE) (/api/v1/[^\x60]+)\x60\s*$`)
	agentAPIDocJSONFence        = regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	agentAPIDocPlaceholder      = regexp.MustCompile(`<[^>]+>`)
	agentAPIDocUUID             = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
)

type agentAPIDocSection struct {
	DocPath string
	Method  string
	Path    string
	Body    string
}

type agentAPIDocTableRow struct {
	Field       string
	Location    string
	Type        string
	Required    string
	Description string
}

type agentAPIDocSchemaField struct {
	Path     string
	Location string
	Schema   map[string]any
	Required bool
}

func TestAgentAPIDocContractsMatchExactlyReviewedRoutes(t *testing.T) {
	routes := agentAPIRoutes()
	if len(routes) != 83 {
		t.Fatalf("reviewed routes=%d want=83", len(routes))
	}
	sections := readAgentAPIDocSections(t)
	if len(sections) != len(routes) {
		t.Fatalf("documented unique operations=%d reviewed routes=%d", len(sections), len(routes))
	}

	for _, route := range routes {
		route := route
		t.Run(route.ID, func(t *testing.T) {
			key := agentAPIRouteKey(route.Method, route.PathTemplate)
			section, ok := sections[key]
			if !ok {
				t.Fatalf("reviewed route missing detailed Contract section: %s", key)
			}
			if section.DocPath != route.DocPath {
				t.Fatalf("%s documented in %s, registry owns %s", key, section.DocPath, route.DocPath)
			}
			assertAgentAPIDocSectionStructure(t, route, section)
			assertAgentAPIDocRequestFields(t, route, section)
			assertAgentAPIDocResponseFields(t, route, section)
			assertAgentAPIDocExample(t, route, section)
		})
	}
}

func TestAgentAPIOverviewOwnsSharedResponseAndErrorContract(t *testing.T) {
	source, err := readAgentDocTemplate(agentDocOverviewPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`{ "success": true, "data": {} }`,
		`"success": false`,
		`"data": null`,
		`"error"`,
		`"code"`,
		`"message"`,
		`"details"`,
		`snake_case`,
		`UUIDv7`,
		`agent_tool_confirmation_required`,
		`request_fingerprint`,
		`expected_revision`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Overview omits shared Contract rule %q", required)
		}
	}
	if !strings.Contains(source, "内部自增 `id`") || !strings.Contains(source, "不得出现在公开 Contract") {
		t.Fatal("Overview must prohibit exposing internal id values")
	}
	assertAgentAPIDocPublicAssetShape(t, source)
}

func TestPhase3ResponseFieldMetadataDistinguishesOmittedFromNull(t *testing.T) {
	omittedFields := []string{
		"chapter_uuid",
		"source_uuid",
		"source_setting_image_uuid",
		"generation_uuid",
		"source_asset_uuid",
		"original_input",
		"final_picture_book",
		"error_code",
		"error_message",
		"current_step_key",
		"ignored_at",
		"completed_at",
		"original_filename",
		"display_name",
		"width",
		"height",
		"duration_ms",
		"deleted_at",
		"finalized_at",
	}
	for _, name := range omittedFields {
		field, ok := phase3AgentAPIResponseFields[name]
		if !ok {
			t.Fatalf("missing phase3 response field metadata %s", name)
		}
		if strings.Contains(strings.ToLower(field.Type), "null") {
			t.Fatalf("omitempty field %s must not claim a JSON null value: %q", name, field.Type)
		}
		if !strings.Contains(field.Description, "省略") {
			t.Fatalf("omitempty field %s must document omission: %q", name, field.Description)
		}
	}
	if field := phase3AgentAPIResponseFields["source_item_uuid"]; strings.Contains(strings.ToLower(field.Type), "null") {
		t.Fatalf("always-present source_item_uuid must not claim nullability: %q", field.Type)
	}
}

func TestAgentAPIDocToolResultsAreCompleteAndWithinHardEnvelopeLimit(t *testing.T) {
	tc := toolContext{ProjectUUID: mustAgentUUID(t), ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	apiDocPaths := make(map[string]bool, len(agentAPIDocDefinitions()))
	for _, definition := range agentAPIDocDefinitions() {
		apiDocPaths[definition.Path] = true
		source, err := readAgentDocTemplate(definition.Path)
		if err != nil {
			t.Fatal(err)
		}
		if len(source) > maxAgentAPIDocSoftBytes {
			t.Fatalf("API Contract %s exceeds 24 KiB soft budget: %d", definition.Path, len(source))
		}
	}
	for _, path := range registeredAgentDocPaths() {
		expected, err := renderAgentDocWithRoutes(path, agentAPIRoutes())
		if err != nil {
			t.Fatalf("render %s: %v", path, err)
		}
		value, err := readAgentDocWithRoutes(tc, map[string]any{"path": path}, agentAPIRoutes())
		if err != nil {
			t.Fatalf("read_agent_doc %s: %v", path, err)
		}
		content, _ := value["content"].(string)
		if content != expected || strings.Contains(content, `"compacted":true`) || strings.Contains(content, `"preview"`) {
			t.Fatalf("read_agent_doc %s did not return the complete rendered source", path)
		}
		encoded, err := json.Marshal(map[string]any{"success": true, "data": value})
		if err != nil || len(encoded) > MaxToolResult {
			t.Fatalf("read_agent_doc %s envelope bytes=%d err=%v", path, len(encoded), err)
		}
		if apiDocPaths[path] && len(content) > maxAgentAPIDocSoftBytes {
			t.Fatalf("API Contract %s rendered above 24 KiB soft budget: %d", path, len(content))
		}
	}
}

func readAgentAPIDocSections(t *testing.T) map[string]agentAPIDocSection {
	t.Helper()
	sections := make(map[string]agentAPIDocSection)
	for _, definition := range agentAPIDocDefinitions() {
		source, err := readAgentDocTemplate(definition.Path)
		if err != nil {
			t.Fatal(err)
		}
		matches := agentAPIDocOperationHeading.FindAllStringSubmatchIndex(source, -1)
		for index, match := range matches {
			end := len(source)
			if index+1 < len(matches) {
				end = matches[index+1][0]
			}
			method, path := source[match[2]:match[3]], source[match[4]:match[5]]
			key := agentAPIRouteKey(method, path)
			if previous, duplicate := sections[key]; duplicate {
				t.Fatalf("duplicate detailed operation %s in %s and %s", key, previous.DocPath, definition.Path)
			}
			sections[key] = agentAPIDocSection{DocPath: definition.Path, Method: method, Path: path, Body: source[match[1]:end]}
		}
	}
	return sections
}

func assertAgentAPIDocSectionStructure(t *testing.T, route agentAPIRoute, section agentAPIDocSection) {
	t.Helper()
	for _, heading := range []string{"### 请求字段", "### 返回字段", "### request_api 示例", "### 接口约束"} {
		if strings.Count(section.Body, heading) != 1 {
			t.Fatalf("%s must contain exactly one %s", route.ID, heading)
		}
	}
	purpose := strings.TrimSpace(strings.SplitN(section.Body, "### 请求字段", 2)[0])
	if purpose == "" {
		t.Fatalf("%s has no one-line purpose", route.ID)
	}
	constraints := agentAPIDocSubsection(section.Body, "### 接口约束")
	if route.ExpectedRevision && !strings.Contains(strings.ToLower(constraints), "revision") {
		t.Fatalf("%s omits revision constraint", route.ID)
	}
	if route.RequiresConfirmation && !strings.Contains(constraints, "确认") {
		t.Fatalf("%s omits dangerous confirmation constraint", route.ID)
	}
	if route.Async && !strings.Contains(constraints, "任务") && !strings.Contains(constraints, "异步") {
		t.Fatalf("%s omits asynchronous/task constraint", route.ID)
	}
}

func assertAgentAPIDocRequestFields(t *testing.T, route agentAPIRoute, section agentAPIDocSection) {
	t.Helper()
	requestFields := agentAPIDocSubsection(section.Body, "### 请求字段")
	rows := agentAPIDocTableRows(t, route.ID, requestFields, 5)
	for _, match := range regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`).FindAllStringSubmatch(route.PathTemplate, -1) {
		row, ok := rows[match[1]]
		if !ok || row.Location != "path" {
			t.Fatalf("%s omits path field %s", route.ID, match[1])
		}
		assertAgentAPIDocFieldRowComplete(t, route.ID, row)
		if row.Required != "是" {
			t.Fatalf("%s path field %s must be required: %q", route.ID, match[1], row.Required)
		}
	}
	for _, input := range []struct {
		location string
		schema   map[string]any
	}{{"query", route.QuerySchema}, {"body", route.BodySchema}} {
		properties, _ := input.schema["properties"].(map[string]any)
		if input.location == "body" && input.schema != nil && len(properties) == 0 {
			row, ok := rows["request_body"]
			if !ok {
				t.Fatalf("%s requires an explicit empty request_body row", route.ID)
			}
			assertAgentAPIDocFieldRowComplete(t, route.ID, row)
			if row.Location != "body" || row.Required != "是" || !agentAPIDocTypeMatches("object", row.Type) || !strings.Contains(row.Description, "{}") {
				t.Fatalf("%s empty request_body row is incomplete: %+v", route.ID, row)
			}
		}
		for _, field := range agentAPISchemaFields(input.schema, input.location, "") {
			row, ok := rows[field.Path]
			if !ok {
				t.Fatalf("%s omits %s schema field %s", route.ID, input.location, field.Path)
			}
			if row.Location != input.location {
				t.Fatalf("%s documents %s at %q, want %q", route.ID, field.Path, row.Location, input.location)
			}
			assertAgentAPIDocFieldRowComplete(t, route.ID, row)
			if field.Required {
				if row.Required != "是" && row.Required != "条件必填" {
					t.Fatalf("%s does not mark required %s.%s: %q", route.ID, input.location, field.Path, row.Required)
				}
			} else if row.Required != "否" && row.Required != "条件必填" {
				t.Fatalf("%s does not mark optional %s.%s: %q", route.ID, input.location, field.Path, row.Required)
			}
			assertAgentAPIDocSchemaDetails(t, route.ID, field, row)
		}
	}
}

func assertAgentAPIDocResponseFields(t *testing.T, route agentAPIRoute, section agentAPIDocSection) {
	t.Helper()
	returns := agentAPIDocSubsection(section.Body, "### 返回字段")
	projector, ok := agentAPIProjectorByKey(route.Projector)
	if !ok {
		t.Fatalf("%s has no projector %s", route.ID, route.Projector)
	}
	if projector.NullData {
		rows := agentAPIDocTableRows(t, route.ID, returns, 3)
		row, ok := rows["data"]
		if !ok || !strings.Contains(strings.ToLower(row.Type+" "+row.Description), "null") {
			t.Fatalf("%s does not document data=null", route.ID)
		}
		assertAgentAPIDocFieldRowComplete(t, route.ID, row)
		return
	}
	rows := agentAPIDocTableRows(t, route.ID, returns, 3)
	fieldProjector := projector
	prefix := "data."
	if projector.List {
		prefix = "data.items[]."
		var found bool
		fieldProjector, found = agentAPIProjectorByKey(projector.ItemProjector)
		if !found {
			t.Fatalf("%s has unknown list item projector %s", route.ID, projector.ItemProjector)
		}
		itemsRow, hasItems := rows["data.items"]
		if !hasItems {
			t.Fatalf("%s does not document list items", route.ID)
		}
		assertAgentAPIDocFieldRowComplete(t, route.ID, itemsRow)
	}
	for _, field := range fieldProjector.Fields {
		if strings.TrimSpace(field.Type) == "" || strings.TrimSpace(field.Description) == "" {
			t.Fatalf("%s projector field %s has incomplete reviewed metadata", route.ID, field.Name)
		}
		row, ok := rows[prefix+field.Name]
		if !ok {
			t.Fatalf("%s omits projected response field %s%s", route.ID, prefix, field.Name)
		}
		assertAgentAPIDocFieldRowComplete(t, route.ID, row)
		if !agentAPIDocTypeMatches(field.Type, row.Type) {
			t.Fatalf("%s response field %s type=%q does not reflect projector type=%q", route.ID, prefix+field.Name, row.Type, field.Type)
		}
		if strings.Contains(field.Description, "省略") && strings.Contains(strings.ToLower(row.Type), "null") {
			t.Fatalf("%s response field %s is omitted when absent, not emitted as null: %q", route.ID, prefix+field.Name, row.Type)
		}
	}
	for _, assetPath := range agentAPIDocPublicAssetPaths(route.Projector) {
		row, ok := rows[assetPath]
		if !ok {
			t.Fatalf("%s omits nested public Asset field %s", route.ID, assetPath)
		}
		assertAgentAPIDocFieldRowComplete(t, route.ID, row)
		if !agentAPIDocTypeMatches("object", row.Type) || !strings.Contains(row.Description, agentAPIDocPublicAssetShape) {
			t.Fatalf("%s nested Asset %s must reference complete %s shape: %+v", route.ID, assetPath, agentAPIDocPublicAssetShape, row)
		}
	}
}

func assertAgentAPIDocExample(t *testing.T, route agentAPIRoute, section agentAPIDocSection) {
	t.Helper()
	exampleSection := agentAPIDocSubsection(section.Body, "### request_api 示例")
	match := agentAPIDocJSONFence.FindStringSubmatch(exampleSection)
	if len(match) != 2 {
		t.Fatalf("%s has no single JSON request_api example", route.ID)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(match[1]), &args); err != nil {
		t.Fatalf("%s example JSON: %v", route.ID, err)
	}
	projectUUID := mustAgentUUID(t)
	args = normalizeAgentAPIDocExampleUUIDs(args, projectUUID).(map[string]any)
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	request, err := parseAgentAPIRequest(tc, args)
	if err != nil {
		t.Fatalf("%s example is not a legal reviewed request: %v\n%s", route.ID, err, match[1])
	}
	if request.Route.ID != route.ID || request.Method != route.Method {
		t.Fatalf("%s example resolved to %s", route.ID, request.Route.ID)
	}
	filter := strings.TrimSpace(stringArg(args, "response_filter"))
	projector, _ := agentAPIProjectorByKey(route.Projector)
	if filter == ".data" && !projector.NullData {
		t.Fatalf("%s uses broad .data filter", route.ID)
	}
	if !projector.NullData && !strings.Contains(filter, "{") {
		t.Fatalf("%s response_filter does not project narrow fields: %s", route.ID, filter)
	}
}

func normalizeAgentAPIDocExampleUUIDs(value any, replacement string) any {
	switch typed := value.(type) {
	case string:
		typed = agentAPIDocPlaceholder.ReplaceAllString(typed, replacement)
		return agentAPIDocUUID.ReplaceAllString(typed, replacement)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = normalizeAgentAPIDocExampleUUIDs(item, replacement)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = normalizeAgentAPIDocExampleUUIDs(item, replacement)
		}
		return result
	default:
		return value
	}
}

func agentAPIDocSubsection(section, heading string) string {
	start := strings.Index(section, heading)
	if start < 0 {
		return ""
	}
	value := section[start+len(heading):]
	if next := strings.Index(value, "\n### "); next >= 0 {
		value = value[:next]
	}
	return strings.TrimSpace(value)
}

func agentAPIDocLevelTwoSection(source, heading string) string {
	start := strings.Index(source, heading)
	if start < 0 {
		return ""
	}
	value := source[start+len(heading):]
	if next := strings.Index(value, "\n## "); next >= 0 {
		value = value[:next]
	}
	return strings.TrimSpace(value)
}

func assertAgentAPIDocPublicAssetShape(t *testing.T, overview string) {
	t.Helper()
	const heading = "## 公开 `" + agentAPIDocPublicAssetShape + "` 返回结构"
	section := agentAPIDocLevelTwoSection(overview, heading)
	if section == "" {
		t.Fatalf("Overview omits %s", agentAPIDocPublicAssetShape)
	}
	expected := map[string]struct {
		fieldType string
		presence  string
	}{
		"uuid":              {"string", "是"},
		"kind":              {"string", "是"},
		"purpose":           {"string", "是"},
		"original_filename": {"string", "可省略"},
		"display_name":      {"string", "可省略"},
		"source_type":       {"string", "是"},
		"source_asset_uuid": {"string", "可省略"},
		"mime_type":         {"string", "是"},
		"byte_size":         {"integer", "是"},
		"width":             {"integer", "可省略"},
		"height":            {"integer", "可省略"},
		"duration_ms":       {"integer", "可省略"},
		"status":            {"string", "是"},
		"deleted_at":        {"string", "可省略"},
		"created_at":        {"string", "是"},
	}
	rows := agentAPIDocTableRows(t, agentAPIDocPublicAssetShape, section, 4)
	if len(rows) != len(expected) {
		t.Fatalf("%s fields=%d want=%d", agentAPIDocPublicAssetShape, len(rows), len(expected))
	}
	for name, contract := range expected {
		row, ok := rows[name]
		if !ok {
			t.Fatalf("%s omits field %s", agentAPIDocPublicAssetShape, name)
		}
		assertAgentAPIDocFieldRowComplete(t, agentAPIDocPublicAssetShape, row)
		if !agentAPIDocTypeMatches(contract.fieldType, row.Type) || row.Required != contract.presence {
			t.Fatalf("%s field %s row=%+v want type=%s presence=%s", agentAPIDocPublicAssetShape, name, row, contract.fieldType, contract.presence)
		}
	}
	for _, forbidden := range []string{"id", "metadata", "content_url", "download_url", "relative_path"} {
		if _, present := rows[forbidden]; present {
			t.Fatalf("%s exposes forbidden field %s", agentAPIDocPublicAssetShape, forbidden)
		}
	}
	assetType := reflect.TypeOf(files.Asset{})
	rawAsset := make(map[string]any, assetType.NumField())
	omitEmpty := make(map[string]bool, assetType.NumField())
	for index := 0; index < assetType.NumField(); index++ {
		tag := assetType.Field(index).Tag.Get("json")
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "" || name == "-" {
			continue
		}
		rawAsset[name] = "test"
		omitEmpty[name] = containsString(parts[1:], "omitempty")
	}
	sanitized, _ := sanitizeAgentAPIValue(rawAsset).(map[string]any)
	if len(sanitized) != len(expected) {
		t.Fatalf("%s fields=%v do not match sanitized files.Asset=%v", agentAPIDocPublicAssetShape, sortedAgentAPIDocKeys(expected), sortedAgentAPIDocKeys(sanitized))
	}
	for name, contract := range expected {
		if _, present := sanitized[name]; !present {
			t.Fatalf("%s documents field %s absent from sanitized files.Asset", agentAPIDocPublicAssetShape, name)
		}
		wantOptional := contract.presence == "可省略"
		if omitEmpty[name] != wantOptional {
			t.Fatalf("%s field %s presence=%s disagrees with files.Asset omitempty=%t", agentAPIDocPublicAssetShape, name, contract.presence, omitEmpty[name])
		}
	}
	projector, ok := agentAPIProjectorByKey("project_asset")
	if !ok {
		t.Fatal("missing project_asset projector")
	}
	projected := agentAPIProjectorFieldNames(projector)
	sort.Strings(projected)
	wanted := make([]string, 0, len(expected))
	for name := range expected {
		wanted = append(wanted, name)
	}
	sort.Strings(wanted)
	if strings.Join(projected, ",") != strings.Join(wanted, ",") {
		t.Fatalf("%s fields drifted from project_asset projector: docs=%v projector=%v", agentAPIDocPublicAssetShape, wanted, projected)
	}
}

func sortedAgentAPIDocKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func agentAPIDocPublicAssetPaths(projector string) []string {
	switch projector {
	case "premise":
		return []string{"data.current_setting_image.asset"}
	case "setting_image":
		return []string{"data.asset"}
	case "setting_image_list":
		return []string{"data.items[].asset"}
	case "premise_asset":
		return []string{"data.current_variant.asset"}
	case "premise_asset_list":
		return []string{"data.items[].current_variant.asset"}
	case "premise_asset_variant_list", "comic_image_variant_list":
		return []string{"data.items[].asset"}
	case "comic_snapshot_detail":
		return []string{"data.sections[].current_image.asset", "data.sections[].premise_reference.asset"}
	default:
		return nil
	}
}

func agentAPIDocTableRows(t *testing.T, routeID, table string, expectedColumns int) map[string]agentAPIDocTableRow {
	t.Helper()
	rows := make(map[string]agentAPIDocTableRow)
	for _, line := range strings.Split(table, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}
		cells := splitAgentAPIMarkdownTableRow(line)
		if len(cells) != expectedColumns {
			t.Fatalf("%s has malformed %d-column Contract table row: %s", routeID, expectedColumns, line)
		}
		if cells[0] == "字段" || strings.Trim(cells[0], "-: ") == "" {
			continue
		}
		field := strings.TrimSpace(strings.Trim(cells[0], "`"))
		row := agentAPIDocTableRow{Field: field}
		switch expectedColumns {
		case 5:
			row.Location = cells[1]
			row.Type = cells[2]
			row.Required = cells[3]
			row.Description = cells[4]
		case 4:
			row.Type = cells[1]
			row.Required = cells[2]
			row.Description = cells[3]
		case 3:
			row.Type = cells[1]
			row.Description = cells[2]
		default:
			t.Fatalf("unsupported Contract table width %d", expectedColumns)
		}
		if _, duplicate := rows[field]; duplicate {
			t.Fatalf("%s documents field %s more than once in one table", routeID, field)
		}
		rows[field] = row
	}
	return rows
}

func splitAgentAPIMarkdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	result := make([]string, 0, 5)
	var current strings.Builder
	escaped := false
	inCode := false
	for _, char := range line {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == '`' {
			inCode = !inCode
			current.WriteRune(char)
			continue
		}
		if char == '|' && !inCode {
			result = append(result, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteRune(char)
	}
	if escaped {
		current.WriteRune('\\')
	}
	result = append(result, strings.TrimSpace(current.String()))
	return result
}

func assertAgentAPIDocFieldRowComplete(t *testing.T, routeID string, row agentAPIDocTableRow) {
	t.Helper()
	if strings.TrimSpace(row.Type) == "" || strings.Trim(strings.TrimSpace(row.Type), "-`) ") == "" {
		t.Fatalf("%s field %s has no concrete type", routeID, row.Field)
	}
	if strings.TrimSpace(row.Description) == "" || strings.Trim(strings.TrimSpace(row.Description), "-。 ") == "" {
		t.Fatalf("%s field %s has no description", routeID, row.Field)
	}
	if strings.Contains(row.Type, "经审查的公开字段") || strings.EqualFold(strings.TrimSpace(row.Type), "public") {
		t.Fatalf("%s field %s still uses placeholder type %q", routeID, row.Field, row.Type)
	}
}

func agentAPISchemaFields(schema map[string]any, location, prefix string) []agentAPIDocSchemaField {
	if schema == nil {
		return nil
	}
	properties, _ := schema["properties"].(map[string]any)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	required := make(map[string]bool)
	for _, name := range agentAPISchemaRequired(schema) {
		required[name] = true
	}
	result := make([]agentAPIDocSchemaField, 0, len(names))
	for _, name := range names {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		child, _ := properties[name].(map[string]any)
		result = append(result, agentAPIDocSchemaField{Path: path, Location: location, Schema: child, Required: required[name]})
		result = append(result, agentAPISchemaFields(child, location, path)...)
		if items, _ := child["items"].(map[string]any); items != nil {
			result = append(result, agentAPISchemaFields(items, location, path+"[]")...)
		}
	}
	return result
}

func assertAgentAPIDocSchemaDetails(t *testing.T, routeID string, field agentAPIDocSchemaField, row agentAPIDocTableRow) {
	t.Helper()
	expectedType, _ := field.Schema["type"].(string)
	if expectedType != "" && !agentAPIDocTypeMatches(expectedType, row.Type) {
		t.Fatalf("%s request field %s type=%q does not reflect schema type=%q", routeID, field.Path, row.Type, expectedType)
	}
	combined := row.Type + " " + row.Description
	if values, ok := field.Schema["enum"].([]string); ok {
		for _, value := range values {
			if !strings.Contains(combined, value) {
				t.Fatalf("%s request field %s omits enum value %q", routeID, field.Path, value)
			}
		}
	}
	for _, constraint := range []string{"minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems"} {
		value, exists := field.Schema[constraint]
		if !exists {
			continue
		}
		if !strings.Contains(agentAPIDocNormalizeNumber(combined), agentAPIDocNormalizeNumber(fmt.Sprint(value))) {
			t.Fatalf("%s request field %s omits %s=%v", routeID, field.Path, constraint, value)
		}
	}
	if value, exists := field.Schema["default"]; exists && !strings.Contains(combined, fmt.Sprint(value)) {
		t.Fatalf("%s request field %s omits default=%v", routeID, field.Path, value)
	}
	if nullable, _ := field.Schema["nullable"].(bool); nullable && !agentAPIDocMentionsNullability(combined) {
		t.Fatalf("%s request field %s omits nullability", routeID, field.Path)
	}
}

func agentAPIDocNormalizeNumber(value string) string {
	replacer := strings.NewReplacer(",", "", "，", "", "_", "", " ", "", "\u00a0", "")
	return replacer.Replace(value)
}

func agentAPIDocTypeMatches(expected, documented string) bool {
	expected = strings.ToLower(strings.TrimSpace(expected))
	documented = strings.ToLower(strings.TrimSpace(documented))
	if strings.Contains(expected, "|") {
		parts := strings.Split(expected, "|")
		if strings.Contains(documented, "null") && !strings.Contains(documented, strings.TrimSpace(parts[0])) {
			return true
		}
		expected = strings.TrimSpace(parts[0])
	}
	hasAny := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(documented, value) {
				return true
			}
		}
		return false
	}
	switch {
	case strings.HasPrefix(expected, "array"):
		return hasAny("array", "数组")
	case strings.HasPrefix(expected, "object"):
		return hasAny("object", "对象", "映射", "map")
	case strings.HasPrefix(expected, "string"):
		return hasAny("string", "字符串", "文本")
	case strings.HasPrefix(expected, "integer"):
		return hasAny("integer", "整数")
	case strings.HasPrefix(expected, "number"):
		return hasAny("number", "数字", "数值", "浮点")
	case strings.HasPrefix(expected, "boolean"):
		return hasAny("boolean", "bool", "布尔")
	case strings.HasPrefix(expected, "json value"):
		return hasAny("json", "任意值")
	case expected == "null":
		return hasAny("null")
	default:
		return documented != ""
	}
}

func agentAPIDocMentionsNullability(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{"null", "可空", "为空", "可省略", "省略", "可缺省", "缺省"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func agentAPISchemaRequired(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	required, _ := schema["required"].([]string)
	return required
}
