package textmcp

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// modelVisibleSpec projects only our root discriminator -> required conditions.
// The affected host declarations currently degrade with these conditions; this
// is a publication workaround, not a change to JSON Schema validation. Specs
// remains authoritative in Service.Call and appWatches.call.
func modelVisibleSpec(spec Spec) (Spec, error) {
	// Detach every map and slice, including shared property definitions in Specs.
	b, err := json.Marshal(spec.Schema)
	if err != nil {
		return Spec{}, err
	}
	var schema map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		return Spec{}, err
	}
	if schema["type"] != "object" {
		return Spec{}, fmt.Errorf("expected an object input schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return Spec{}, fmt.Errorf("expected object properties")
	}
	baseRequired, ok := schemaStrings(schema["required"])
	if !ok {
		return Spec{}, fmt.Errorf("expected base required fields")
	}
	// Group overlapping conditions (e.g. target=page) so the description lists
	// all of that action's requirements together, without making them global.
	requirements := map[string][]string{}
	if raw, present := schema["allOf"]; present {
		conditions, ok := raw.([]any)
		if !ok || len(conditions) == 0 {
			return Spec{}, fmt.Errorf("expected nonempty conditional allOf")
		}
		for index, raw := range conditions {
			selector, values, fields, err := conditionalRequired(raw, properties, baseRequired)
			if err != nil {
				return Spec{}, fmt.Errorf("allOf[%d]: %w", index, err)
			}
			for _, value := range values {
				key := selector + "=" + value
				requirements[key] = append(requirements[key], fields...)
			}
		}
		// Delete only after every condition has been understood and represented.
		delete(schema, "allOf")
	}
	if err := checkModelSchema(schema); err != nil {
		return Spec{}, err
	}
	description := spec.Description
	if len(requirements) > 0 {
		clauses := make([]string, 0, len(requirements))
		for _, key := range slices.Sorted(maps.Keys(requirements)) {
			fields := requirements[key]
			slices.Sort(fields)
			clauses = append(clauses, key+" requires "+strings.Join(slices.Compact(fields), ", "))
		}
		description += " Required fields by action/target/kind (enforced by server): " + strings.Join(clauses, "; ") + "."
	}
	return Spec{Name: spec.Name, Description: description, Schema: schema}, nil
}

// Accept exactly {if:{properties:{selector:{const:string|enum:[]string}}},
// then:{required:[]string}}. Additional predicates, then constraints, else
// branches, or nested combinators need an explicit projection design first.
func conditionalRequired(raw any, properties map[string]any, baseRequired []string) (string, []string, []string, error) {
	fail := func() (string, []string, []string, error) {
		return "", nil, nil, fmt.Errorf("unsupported conditional required rule")
	}
	rule, ok := raw.(map[string]any)
	if !ok || len(rule) != 2 {
		return fail()
	}
	predicate, ok := rule["if"].(map[string]any)
	if !ok || len(predicate) != 1 {
		return fail()
	}
	selectors, ok := predicate["properties"].(map[string]any)
	if !ok || len(selectors) != 1 {
		return fail()
	}
	consequence, ok := rule["then"].(map[string]any)
	if !ok || len(consequence) != 1 {
		return fail()
	}
	fields, ok := schemaStrings(consequence["required"])
	if !ok || len(fields) == 0 {
		return fail()
	}
	for _, field := range fields {
		if _, exists := properties[field]; !exists {
			return fail()
		}
	}
	for selector, raw := range selectors {
		if !slices.Contains([]string{"action", "target", "kind"}, selector) || !slices.Contains(baseRequired, selector) {
			return fail()
		}
		property, ok := properties[selector].(map[string]any)
		if !ok || property["type"] != "string" {
			return fail()
		}
		allowed, ok := schemaStrings(property["enum"])
		if !ok {
			return fail()
		}
		match, ok := raw.(map[string]any)
		if !ok {
			return fail()
		}
		var values []string
		if value, ok := match["const"].(string); ok && len(match) == 1 {
			values = []string{value}
		} else {
			// enum() also emits type:string; accept that exact redundant type.
			if len(match) != 1 && !(len(match) == 2 && match["type"] == "string") {
				return fail()
			}
			if values, ok = schemaStrings(match["enum"]); !ok {
				return fail()
			}
		}
		if len(values) == 0 {
			return fail()
		}
		for _, value := range values {
			if !slices.Contains(allowed, value) {
				return fail()
			}
		}
		return selector, values, fields, nil
	}
	return fail()
}

func schemaStrings(raw any) ([]string, bool) {
	values, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value, ok := raw.(string)
		if !ok || value == "" {
			return nil, false
		}
		result = append(result, value)
	}
	return result, true
}

// Check schema positions, not arbitrary JSON keys (a property can be named
// "if"). Never recursively remove constraints. Other field constraints pass
// through unchanged; unsupported composition fails registration explicitly.
func checkModelSchema(schema map[string]any) error {
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "if", "then", "else", "not", "$ref", "$dynamicRef", "dependentSchemas"} {
		if _, present := schema[keyword]; present {
			return fmt.Errorf("unsupported model schema keyword %s", keyword)
		}
	}
	for _, keyword := range []string{"properties", "patternProperties", "$defs", "definitions"} {
		children, _ := schema[keyword].(map[string]any)
		for _, key := range slices.Sorted(maps.Keys(children)) {
			if child, ok := children[key].(map[string]any); ok {
				if err := checkModelSchema(child); err != nil {
					return fmt.Errorf("%s.%s: %w", keyword, key, err)
				}
			}
		}
	}
	for _, keyword := range []string{"items", "additionalProperties"} {
		if child, ok := schema[keyword].(map[string]any); ok {
			if err := checkModelSchema(child); err != nil {
				return fmt.Errorf("%s: %w", keyword, err)
			}
		}
	}
	return nil
}
