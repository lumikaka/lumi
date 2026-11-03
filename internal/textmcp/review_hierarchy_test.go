package textmcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"lumi/internal/project"
)

func TestReviewHierarchyContractsPaginationIsolationAndDeletion(t *testing.T) {
	s, root, projectUUID := harness(t)
	s.Endpoint = "http://127.0.0.1:49237"
	read := func(name string, args map[string]any) map[string]any {
		t.Helper()
		if strings.HasPrefix(name, "review_") {
			return object(t, invokeReviewDetails(t, s, root, name, args))
		}
		return object(t, invoke(t, s, root, name, args))
	}
	overview := read("review_book", map[string]any{})
	if overview["view"] != "book" || overview["target_uuid"] != projectUUID || overview["open_url"] != s.Endpoint+"/projects/"+projectUUID+"/" || len(overview["items"].([]any)) != 0 {
		t.Fatal(overview)
	}
	if overview["book"].(map[string]any)["page_count"] != float64(0) {
		t.Fatal(overview)
	}
	var chapters []string
	for i := 1; i <= 22; i++ {
		p := read("project_overview", map[string]any{})["project"].(map[string]any)
		ch := read("chapters", map[string]any{"action": "create", "chapter_code": fmt.Sprintf("vol01.ch%02d", i), "title": fmt.Sprintf("Chapter %d", i), "expected_revision": p["revision"]})
		chapters = append(chapters, ch["uuid"].(string))
	}
	countStates := func() int64 {
		t.Helper()
		var count int64
		if err := s.Projects.WithStore(t.Context(), projectUUID, func(store *project.Store) error { return store.DB().Table("chapter_comic_states").Count(&count).Error }); err != nil {
			t.Fatal(err)
		}
		return count
	}
	before := countStates()
	empty := read("review_chapter", map[string]any{"chapter_uuid": chapters[0]})
	if len(empty["items"].([]any)) != 0 || empty["chapter"].(map[string]any)["page_count"] != float64(0) {
		t.Fatal(empty)
	}
	list := read("review_chapters", map[string]any{})
	if list["view"] != "chapters" || len(list["chapters"].([]any)) != 20 || list["total"] != float64(22) {
		t.Fatal(list)
	}
	last := read("review_chapters", map[string]any{"after": "20"})
	if last["chapters"].([]any)[0].(map[string]any)["uuid"] != chapters[20] || len(last["chapters"].([]any)) != 2 || last["snapshot"] != list["snapshot"] {
		t.Fatal(last)
	}
	back := read("review_chapters", map[string]any{"before": "20"})
	if len(back["chapters"].([]any)) != 20 {
		t.Fatal(back)
	}
	if countStates() != before {
		t.Fatal("review mutated empty chapters")
	}
	ch := read("chapters", map[string]any{"action": "get", "chapter_uuid": chapters[0]})
	read("chapters", map[string]any{"action": "write", "chapter_uuid": chapters[0], "content": "完整章节故事，第一页到最后一页。", "content_format": "txt", "expected_revision": ch["revision"]})
	var pages []string
	for i := 0; i < 23; i++ {
		state := read("pages", map[string]any{"action": "list", "chapter_uuid": chapters[0]})["state"].(map[string]any)
		page := read("pages", map[string]any{"action": "create", "chapter_uuid": chapters[0], "title": fmt.Sprintf("Page %d", i), "description_md": "完整正文", "storyboard_md": "完整分镜", "expected_revision": state["revision"]})
		pages = append(pages, page["uuid"].(string))
	}
	chapter := read("review_chapter", map[string]any{"chapter_uuid": chapters[0], "limit": 2})
	if chapter["view"] != "chapter" || chapter["target_uuid"] != chapters[0] || chapter["total"] != float64(23) || chapter["chapter"].(map[string]any)["story"] != "完整章节故事，第一页到最后一页。" {
		t.Fatal(chapter)
	}
	more := read("review_chapter", map[string]any{"chapter_uuid": chapters[0], "limit": 2, "after": "2"})
	if more["items"].([]any)[0].(map[string]any)["uuid"] != pages[2] || more["snapshot"] != chapter["snapshot"] {
		t.Fatal(more)
	}
	page := read("review_page", map[string]any{"chapter_uuid": chapters[0], "page_uuid": pages[1]})
	item := page["items"].([]any)[0].(map[string]any)
	if page["view"] != "page" || page["target_uuid"] != pages[1] || page["revision"] != item["revision"] || item["text"] != "完整正文" || item["scene"] != "完整分镜" || page["previous_page_uuid"] != pages[0] || page["next_page_uuid"] != pages[2] || page["position"] != float64(2) {
		t.Fatal(page)
	}
	first := read("review_page", map[string]any{"chapter_uuid": chapters[0], "page_uuid": pages[0]})
	final := read("review_page", map[string]any{"chapter_uuid": chapters[0], "page_uuid": pages[22]})
	if first["previous_page_uuid"] != nil || final["next_page_uuid"] != nil {
		t.Fatal(first, final)
	}
	for _, args := range []map[string]any{{"chapter_uuid": chapters[0]}, {"mode": "preview", "chapter_uuid": chapters[0]}} {
		preview := read("review_book", args)
		if preview["view"] != "chapter" || preview["target_uuid"] != chapters[0] || preview["total"] != float64(23) {
			t.Fatal(preview)
		}
	}
	for _, bad := range []struct {
		name string
		args map[string]any
	}{
		{"review_chapter", map[string]any{"chapter_uuid": projectUUID}},
		{"review_page", map[string]any{"chapter_uuid": chapters[1], "page_uuid": pages[0]}},

		{"review_chapters", map[string]any{"after": "23"}},
		{"review_chapter", map[string]any{"chapter_uuid": chapters[0], "before": "24"}},
		{"review_chapters", map[string]any{"after": "1", "before": "2"}},
		{"review_chapters", map[string]any{"project_uuid": projectUUID}},
		{"review_chapter", map[string]any{"chapter_uuid": "1"}},
	} {
		if invokeReviewDetails(t, s, root, bad.name, bad.args).Success {
			t.Fatalf("accepted %s %v", bad.name, bad.args)
		}
	}
	otherRoot := t.TempDir()
	object(t, invoke(t, s, otherRoot, "project_init", map[string]any{"idea": "isolated", "name": "Other"}))
	// The chapter exists but the connection is bound to another project directory.
	if invokeReviewDetails(t, s, otherRoot, "review_chapter", map[string]any{"chapter_uuid": chapters[0]}).Success {
		t.Fatal("cross-project chapter accepted")
	}
	raw, _ := json.Marshal(map[string]any{"chapter_uuid": chapters[0]})
	object(t, s.CallReadOnly(WithExpectedProjectUUID(t.Context(), projectUUID), root, "review_chapter", raw))
	read("pages", map[string]any{"action": "delete", "chapter_uuid": chapters[0], "page_uuid": pages[1], "expected_revision": item["revision"]})
	if invokeReviewDetails(t, s, root, "review_page", map[string]any{"chapter_uuid": chapters[0], "page_uuid": pages[1]}).Success {
		t.Fatal("deleted page accepted")
	}
	updated := read("review_chapter", map[string]any{"chapter_uuid": chapters[0]})
	if updated["snapshot"] == chapter["snapshot"] {
		t.Fatal("snapshot did not detect deletion")
	}
	encoded, _ := json.Marshal(updated)
	if strings.Contains(string(encoded), `"id":`) || strings.Contains(string(encoded), `_id":`) {
		t.Fatal("internal ID exposed")
	}
}
