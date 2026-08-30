package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var responseFilterIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)

type responsePathSegment struct {
	Key   string
	Array bool
	Index *int
}

type responseProjection struct {
	Path   []responsePathSegment
	Fields []responseProjectionField
}

type responseProjectionField struct {
	OutputKey string
	SourceKey string
	Value     responseProjection
}

type parsedResponseFilter struct {
	Path       []responsePathSegment
	Projection *responseProjection
}

func runResponseFilter(envelope map[string]any, expression string) (any, error) {
	parsed, err := parseResponseFilter(expression)
	if err != nil {
		return nil, invalidResponseFilter(expression)
	}
	value, err := responsePathValue(envelope, append([]responsePathSegment{{Key: "data"}}, parsed.Path...))
	if err != nil {
		return nil, invalidResponseFilter(expression)
	}
	if parsed.Projection == nil {
		return value, nil
	}
	value, err = applyResponseProjection(value, *parsed.Projection)
	if err != nil {
		return nil, invalidResponseFilter(expression)
	}
	return value, nil
}

func parseResponseFilter(expression string) (parsedResponseFilter, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" || strings.Contains(expression, "..") || strings.ContainsAny(expression, "()$;") {
		return parsedResponseFilter{}, fmt.Errorf("unsupported expression")
	}
	parts := strings.Split(expression, "|")
	if len(parts) > 2 {
		return parsedResponseFilter{}, fmt.Errorf("multiple pipes")
	}
	pathExpression := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(pathExpression, ".data") {
		return parsedResponseFilter{}, fmt.Errorf("filter must begin with .data")
	}
	path, err := parseResponsePath(strings.TrimPrefix(pathExpression, ".data"))
	if err != nil {
		return parsedResponseFilter{}, err
	}
	result := parsedResponseFilter{Path: path}
	if len(parts) == 2 {
		projection, err := parseResponseProjection(strings.TrimSpace(parts[1]))
		if err != nil {
			return parsedResponseFilter{}, err
		}
		result.Projection = &projection
	}
	return result, nil
}

func parseResponsePath(rest string) ([]responsePathSegment, error) {
	segments := []responsePathSegment{}
	for rest != "" {
		if !strings.HasPrefix(rest, ".") {
			return nil, fmt.Errorf("invalid path")
		}
		rest = strings.TrimPrefix(rest, ".")
		identifier := responseFilterIdentifier.FindString(rest)
		if identifier == "" {
			return nil, fmt.Errorf("invalid identifier")
		}
		rest = strings.TrimPrefix(rest, identifier)
		segment := responsePathSegment{Key: identifier}
		if strings.HasPrefix(rest, "[") {
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("unterminated index")
			}
			index := rest[1:end]
			rest = rest[end+1:]
			if index == "" {
				segment.Array = true
			} else {
				value, err := strconv.Atoi(index)
				if err != nil || value < 0 {
					return nil, fmt.Errorf("invalid index")
				}
				segment.Index = &value
			}
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

func parseResponseProjection(expression string) (responseProjection, error) {
	if strings.HasPrefix(expression, ".") {
		path, err := parseResponsePath(expression)
		return responseProjection{Path: path}, err
	}
	if !strings.HasPrefix(expression, "{") || !strings.HasSuffix(expression, "}") {
		return responseProjection{}, fmt.Errorf("invalid projection")
	}
	content := strings.TrimSpace(expression[1 : len(expression)-1])
	parts, err := splitResponseTopLevel(content, ',')
	if err != nil || len(parts) == 0 {
		return responseProjection{}, fmt.Errorf("invalid object projection")
	}
	projection := responseProjection{Fields: make([]responseProjectionField, 0, len(parts))}
	for _, raw := range parts {
		fieldParts, err := splitResponseTopLevel(strings.TrimSpace(raw), ':')
		if err != nil || len(fieldParts) < 1 || len(fieldParts) > 2 {
			return responseProjection{}, fmt.Errorf("invalid object field")
		}
		key := strings.TrimSpace(fieldParts[0])
		if responseFilterIdentifier.FindString(key) != key {
			return responseProjection{}, fmt.Errorf("invalid field key")
		}
		field := responseProjectionField{OutputKey: key, SourceKey: key, Value: responseProjection{Path: []responsePathSegment{{Key: key}}}}
		if len(fieldParts) == 2 {
			nested, err := parseResponseProjection(strings.TrimSpace(fieldParts[1]))
			if err != nil || len(nested.Fields) == 0 {
				return responseProjection{}, fmt.Errorf("nested projection must be an object")
			}
			field.Value = nested
		}
		projection.Fields = append(projection.Fields, field)
	}
	return projection, nil
}

func splitResponseTopLevel(content string, delimiter rune) ([]string, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("empty expression")
	}
	level, start := 0, 0
	parts := []string{}
	for index, char := range content {
		switch char {
		case '{':
			level++
		case '}':
			level--
			if level < 0 {
				return nil, fmt.Errorf("unbalanced object")
			}
		default:
			if char == delimiter && level == 0 {
				parts = append(parts, content[start:index])
				start = index + 1
			}
		}
	}
	if level != 0 {
		return nil, fmt.Errorf("unbalanced object")
	}
	parts = append(parts, content[start:])
	return parts, nil
}

func responsePathValue(value any, path []responsePathSegment) (any, error) {
	if len(path) == 0 {
		return value, nil
	}
	segment, rest := path[0], path[1:]
	object, ok := value.(map[string]any)
	if !ok {
		if value == nil && !segment.Array {
			return nil, nil
		}
		return nil, fmt.Errorf("path is not an object")
	}
	next := object[segment.Key]
	if segment.Index != nil {
		items, ok := next.([]any)
		if !ok || *segment.Index >= len(items) {
			return nil, fmt.Errorf("invalid array index")
		}
		return responsePathValue(items[*segment.Index], rest)
	}
	if segment.Array {
		items, ok := next.([]any)
		if !ok {
			return nil, fmt.Errorf("value is not an array")
		}
		if len(rest) == 0 {
			return items, nil
		}
		projected := make([]any, 0, len(items))
		for _, item := range items {
			value, err := responsePathValue(item, rest)
			if err != nil {
				return nil, err
			}
			projected = append(projected, value)
		}
		return projected, nil
	}
	return responsePathValue(next, rest)
}

func applyResponseProjection(value any, projection responseProjection) (any, error) {
	if items, ok := value.([]any); ok {
		result := make([]any, 0, len(items))
		for _, item := range items {
			projected, err := applyResponseProjection(item, projection)
			if err != nil {
				return nil, err
			}
			result = append(result, projected)
		}
		return result, nil
	}
	if projection.Path != nil {
		return responsePathValue(value, projection.Path)
	}
	result := make(map[string]any, len(projection.Fields))
	for _, field := range projection.Fields {
		if field.Value.Path != nil {
			projected, err := responsePathValue(value, field.Value.Path)
			if err != nil {
				return nil, err
			}
			result[field.OutputKey] = projected
			continue
		}
		source, err := responsePathValue(value, []responsePathSegment{{Key: field.SourceKey}})
		if err != nil {
			return nil, err
		}
		projected, err := applyResponseProjection(source, field.Value)
		if err != nil {
			return nil, err
		}
		result[field.OutputKey] = projected
	}
	return result, nil
}

func invalidResponseFilter(expression string) error {
	return toolValidationError(
		"response_filter 无效",
		"response_filter 只允许从 .data 开始的有限路径、数组和对象投影。",
		toolValidationViolation{Path: "response_filter", Rule: "format", ExpectedType: "safe .data projection"},
	)
}

// validateAgentAPIResponseFilter checks the parsed filter against the compact
// response shape declared by a reviewed route. This is intentionally static:
// it cannot depend on response data and therefore runs before a write handler.
func validateAgentAPIResponseFilter(route agentAPIRoute, filter parsedResponseFilter) error {
	projector, ok := agentAPIProjectorByKey(route.Projector)
	if !ok {
		return incompatibleResponseFilter("当前 Route 没有可验证的 response projector。")
	}
	if responseFilterHasIndex(filter) {
		return incompatibleResponseFilter("response_filter 不得使用依赖返回数据长度的数组下标。")
	}
	if projector.NullData {
		if len(filter.Path) != 0 || filter.Projection != nil {
			return incompatibleResponseFilter("data=null Route 只允许 response_filter `.data`。")
		}
		return nil
	}
	if projector.List {
		return validateListResponseFilter(projector, filter)
	}
	return validateObjectResponseFilter(projector, filter)
}

func validateObjectResponseFilter(projector agentAPIProjector, filter parsedResponseFilter) error {
	fields := responseProjectorFields(projector)
	if len(filter.Path) > 0 {
		target, err := validateKnownResponsePath(filter.Path, fields, "data")
		if err != nil {
			return err
		}
		if filter.Projection != nil {
			if !target.Projectable {
				return incompatibleResponseFilter("response_filter 不能对标量或未知嵌套字段应用对象投影。")
			}
			return validateOpaqueResponseProjection(*filter.Projection, "data."+filter.Path[0].Key)
		}
		return nil
	}
	if filter.Projection == nil {
		return incompatibleResponseFilter("对象 Route 必须选择 projector 声明的窄字段；只有 data=null Route 允许 `.data`。")
	}
	return validateResponseProjectionFields(*filter.Projection, fields, "data")
}

func validateListResponseFilter(projector agentAPIProjector, filter parsedResponseFilter) error {
	itemProjector, ok := agentAPIProjectorByKey(projector.ItemProjector)
	if !ok {
		return incompatibleResponseFilter("列表 Route 没有可验证的 item projector。")
	}
	itemFields := responseProjectorFields(itemProjector)
	if len(filter.Path) == 0 {
		if filter.Projection == nil || filter.Projection.Path != nil {
			return incompatibleResponseFilter("列表 Route 必须安全投影 data.items。")
		}
		seenItems := false
		for _, field := range filter.Projection.Fields {
			switch field.SourceKey {
			case "items":
				if field.Value.Path != nil || len(field.Value.Fields) == 0 {
					return incompatibleResponseFilter("data.items 必须使用窄对象投影。")
				}
				if err := validateResponseProjectionFields(field.Value, itemFields, "data.items[]"); err != nil {
					return err
				}
				seenItems = true
			case "pagination", "cursor_pagination", "filter_groups":
				if field.Value.Path != nil || len(field.Value.Fields) == 0 {
					return incompatibleResponseFilter("列表分页或筛选元数据必须使用窄对象投影。")
				}
				if err := validateOpaqueResponseProjection(field.Value, "data."+field.SourceKey); err != nil {
					return err
				}
			default:
				return incompatibleResponseFilter("列表 Route 只允许投影 items、pagination、cursor_pagination 或 filter_groups。")
			}
		}
		if !seenItems {
			return incompatibleResponseFilter("列表 Route 的 response_filter 必须包含 data.items 安全投影。")
		}
		return nil
	}
	first := filter.Path[0]
	if first.Key != "items" || !first.Array {
		return incompatibleResponseFilter("列表 Route 必须从 `.data.items[]` 开始投影。")
	}
	if len(filter.Path) == 1 {
		if filter.Projection == nil {
			return incompatibleResponseFilter("`.data.items[]` 必须继续选择所需字段。")
		}
		return validateResponseProjectionFields(*filter.Projection, itemFields, "data.items[]")
	}
	target, err := validateKnownResponsePath(filter.Path[1:], itemFields, "data.items[]")
	if err != nil {
		return err
	}
	if filter.Projection != nil {
		if !target.Projectable {
			return incompatibleResponseFilter("response_filter 不能对标量或未知嵌套 item 字段应用对象投影。")
		}
		return validateOpaqueResponseProjection(*filter.Projection, "data.items[]."+filter.Path[1].Key)
	}
	return nil
}

func validateResponseProjectionFields(projection responseProjection, allowed map[string]agentAPIResponseField, path string) error {
	if projection.Path != nil {
		_, err := validateKnownResponsePath(projection.Path, allowed, path)
		return err
	}
	if len(projection.Fields) == 0 {
		return incompatibleResponseFilter(path + " 必须使用非空对象投影。")
	}
	for _, field := range projection.Fields {
		metadata, ok := allowed[field.SourceKey]
		if !ok {
			return incompatibleResponseFilter(path + "." + field.SourceKey + " 不在 Route projector 中。")
		}
		if field.Value.Path != nil {
			if len(field.Value.Path) != 1 || field.Value.Path[0].Key != field.SourceKey || field.Value.Path[0].Array || field.Value.Path[0].Index != nil {
				return incompatibleResponseFilter(path + "." + field.SourceKey + " 必须直接选择 projector 字段。")
			}
			continue
		}
		if !responseFieldProjectsObjects(metadata.Type) {
			return incompatibleResponseFilter(path + "." + field.SourceKey + " 不是可应用嵌套对象投影的字段。")
		}
		if err := validateOpaqueResponseProjection(field.Value, path+"."+field.SourceKey); err != nil {
			return err
		}
	}
	return nil
}

type responsePathTarget struct {
	Projectable bool
}

// validateKnownResponsePath proves that a path cannot hit a runtime shape
// mismatch after a handler has already produced side effects. Projectors
// currently describe nested public objects as opaque shapes, so one terminal
// child may be selected but the filter may not infer a deeper type from data.
func validateKnownResponsePath(path []responsePathSegment, allowed map[string]agentAPIResponseField, base string) (responsePathTarget, error) {
	if len(path) == 0 {
		return responsePathTarget{}, incompatibleResponseFilter(base + " 投影不能为空。")
	}
	first := path[0]
	field, ok := allowed[first.Key]
	if !ok {
		return responsePathTarget{}, incompatibleResponseFilter(base + " 只能投影 projector 声明的字段。")
	}
	isArray := responseFieldIsArray(field.Type)
	if first.Array && !isArray {
		return responsePathTarget{}, incompatibleResponseFilter(base + "." + first.Key + " 的数组展开与 projector 字段类型不匹配。")
	}
	if len(path) == 1 {
		return responsePathTarget{Projectable: responseFieldProjectsObjects(field.Type)}, nil
	}
	if isArray {
		if !first.Array || !responseFieldProjectsObjects(field.Type) {
			return responsePathTarget{}, incompatibleResponseFilter(base + "." + first.Key + " 必须先安全展开对象数组后再读取子字段。")
		}
	} else if !responseFieldProjectsObjects(field.Type) {
		return responsePathTarget{}, incompatibleResponseFilter(base + "." + first.Key + " 是标量字段，不能继续读取子字段。")
	}
	if len(path) != 2 || path[1].Array || path[1].Index != nil {
		return responsePathTarget{}, incompatibleResponseFilter(base + "." + first.Key + " 的嵌套公开结构只能选择一层终端字段。")
	}
	return responsePathTarget{}, nil
}

// validateOpaqueResponseProjection accepts one explicitly selected public
// layer from an object/array<object> field. With no nested schema metadata we
// must not allow another projection level whose runtime type would be guessed.
func validateOpaqueResponseProjection(projection responseProjection, path string) error {
	if projection.Path != nil {
		if len(projection.Path) != 1 || projection.Path[0].Array || projection.Path[0].Index != nil {
			return incompatibleResponseFilter(path + " 的嵌套公开结构只能选择一层终端字段。")
		}
		return nil
	}
	if len(projection.Fields) == 0 {
		return incompatibleResponseFilter(path + " 必须使用非空对象投影。")
	}
	for _, field := range projection.Fields {
		if field.Value.Path == nil || len(field.Value.Path) != 1 || field.Value.Path[0].Key != field.SourceKey || field.Value.Path[0].Array || field.Value.Path[0].Index != nil {
			return incompatibleResponseFilter(path + "." + field.SourceKey + " 不能继续嵌套投影。")
		}
	}
	return nil
}

func responseFieldIsArray(fieldType string) bool {
	return strings.HasPrefix(strings.TrimSpace(fieldType), "array")
}

func responseFieldProjectsObjects(fieldType string) bool {
	return strings.Contains(strings.ToLower(fieldType), "object")
}

func responseProjectorFields(projector agentAPIProjector) map[string]agentAPIResponseField {
	result := make(map[string]agentAPIResponseField, len(projector.Fields))
	for _, field := range projector.Fields {
		result[field.Name] = field
	}
	return result
}

func responseFilterHasIndex(filter parsedResponseFilter) bool {
	if responsePathHasIndex(filter.Path) {
		return true
	}
	return filter.Projection != nil && responseProjectionHasIndex(*filter.Projection)
}

func responseProjectionHasIndex(projection responseProjection) bool {
	if responsePathHasIndex(projection.Path) {
		return true
	}
	for _, field := range projection.Fields {
		if responseProjectionHasIndex(field.Value) {
			return true
		}
	}
	return false
}

func responsePathHasIndex(path []responsePathSegment) bool {
	for _, segment := range path {
		if segment.Index != nil {
			return true
		}
	}
	return false
}

func incompatibleResponseFilter(details string) error {
	return toolValidationError(
		"response_filter 与 Route 返回形状不兼容",
		details,
		toolValidationViolation{Path: "response_filter", Rule: "response_shape", ExpectedType: "route projector compatible projection"},
	)
}
