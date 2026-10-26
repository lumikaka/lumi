package agentevals

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type Score struct {
	Name     string  `json:"name"`
	Value    float64 `json:"value"`
	Basis    string  `json:"basis"`
	Observed any     `json:"observed"`
	Expected any     `json:"expected"`
}

func normalized(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}
func equal(a, b any) bool { return reflect.DeepEqual(normalized(a), normalized(b)) }
func subset(actual, expected map[string]any) bool {
	for k, v := range expected {
		if !equal(actual[k], v) {
			return false
		}
	}
	return true
}
func grade(c Case, e *Environment, final string) []Score {
	out := make([]Score, 0, len(c.Checks))
	for _, ch := range c.Checks {
		s := Score{Name: ch.Name, Basis: ch.Basis, Expected: ch.Expected}
		ok := false
		switch ch.Kind {
		case "final_equals":
			s.Observed = strings.TrimSpace(final)
			ok = equal(s.Observed, ch.Expected)
		case "final_contains":
			s.Observed = final
			expected, isString := ch.Expected.(string)
			ok = isString && strings.Contains(final, expected)
		case "file_equals":
			v, exists := e.files[ch.Path]
			s.Observed = v
			ok = exists && equal(v, ch.Expected)
		case "file_json":
			v, exists := e.files[ch.Path]
			var parsed any
			err := json.Unmarshal([]byte(v), &parsed)
			s.Observed = parsed
			ok = exists && err == nil && equal(parsed, ch.Expected)
		case "tool_call":
			count := 0
			for _, ev := range e.events {
				if ev.Name == ch.Tool && subset(ev.Arguments, ch.Arguments) {
					count++
				}
			}
			s.Observed = count
			if ch.Count != nil {
				s.Expected = *ch.Count
				ok = count == *ch.Count
			} else {
				s.Expected = "at least one matching call"
				ok = count > 0
			}
		case "audit_evidence":
			texts := []string{}
			for _, ev := range e.events {
				if ev.Name == "eval_submit" {
					txt, _ := ev.Arguments["explanation"].(string)
					texts = append(texts, txt)
				}
			}
			s.Observed = texts
			s.Expected = "one explanation citing src/names.py and format_name, submitted as the final tool action"
			ok = len(texts) == 1 && strings.Contains(texts[0], "src/names.py") && strings.Contains(texts[0], "format_name") && len(e.events) > 0 && e.events[len(e.events)-1].Name == "eval_submit"
		default:
			panic(fmt.Sprintf("unvalidated check: %s", ch.Kind))
		}
		if ok {
			s.Value = 1
		}
		out = append(out, s)
	}
	return out
}
