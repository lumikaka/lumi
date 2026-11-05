package textmcp

import (
	"slices"
	"strings"
	"testing"
)

func TestStripMomentPlanVariesByChapter(t *testing.T) {
	first := stripMomentPlan("01989abc-def0-7000-8000-000000000001", 12)
	second := stripMomentPlan("01989abc-def0-7000-8000-000000000002", 12)
	if slices.Equal(first, second) {
		t.Fatalf("different chapter seeds returned the same plan: %v", first)
	}
}

func TestRenderStripStoryboardAutoPlan(t *testing.T) {
	s, _, _ := harness(t)
	root := t.TempDir()
	init := object(t, invoke(t, s, root, "project_init", map[string]any{"idea": "A reunion", "name": "Strip"}))
	setup := object(t, invoke(t, s, root, "project_setup", map[string]any{
		"action": "update", "project_name": "Strip", "overall_style": "ink", "picture_book": map[string]string{"format": "vertical_strip"},
		"expected_revision": init["setup"].(map[string]any)["revision"],
	}))
	object(t, invoke(t, s, root, "project_setup", map[string]any{"action": "finalize", "expected_revision": setup["revision"]}))
	p := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	ch := object(t, invoke(t, s, root, "chapters", map[string]any{
		"action": "create", "chapter_code": "vol01.ch01", "title": "Reunion", "content": "The door opens.", "expected_revision": p["revision"],
	}))
	chapter := ch["uuid"].(string)
	vars := map[string]string{"moment_count_plan": "auto", "max_section_count": "12"}
	request := map[string]any{"target": "pages", "chapter_uuid": chapter, "prompt_group": "chapter", "prompt_key": "comic_storyboard", "variables": vars}
	rendered := func() string {
		return object(t, invoke(t, s, root, "render_prompt", request))["prompt"].(string)
	}
	plan := stripMomentPlan(chapter, 12)
	for i := 0; i < len(plan); i += 3 {
		group := slices.Clone(plan[i : i+3])
		slices.Sort(group)
		if !slices.Equal(group, []int{1, 2, 3}) {
			t.Fatalf("unexpected three-page group in %v", plan)
		}
	}
	for _, count := range []string{"12", "5", "12"} {
		vars["max_section_count"] = count
		want := plan
		if count == "5" {
			want = plan[:5]
		}
		if !strings.Contains(rendered(), "\n"+jsonText(want)+"\n") {
			t.Fatalf("missing stable rendered plan %v", want)
		}
	}
	vars["max_moments_per_section"] = "5"
	if got := code(invoke(t, s, root, "render_prompt", request)); got != "invalid_arguments" {
		t.Fatalf("conflicting maximum: got %s", got)
	}
	vars["max_moments_per_section"], vars["moment_count_plan"] = "2", "[2,2,2]"
	vars["max_section_count"] = "3"
	if !strings.Contains(rendered(), "\n[2,2,2]\n") {
		t.Fatal("explicit storyboard plan was replaced")
	}
}
