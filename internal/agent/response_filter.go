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
