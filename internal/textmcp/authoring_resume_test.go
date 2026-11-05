package textmcp

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// Model a reconnect with seven saved pages and five missing, including a
// corrected candidate. Use the public tool contract for every mutation/read.
func TestChapterResumeTracksAdoptedPageReviews(t *testing.T) {
	s, root, ch, first := imagePromptPage(t)
	imagePath := preparedFixtureImage(t)
	pages := []string{first}
	for n := 2; n <= 12; n++ {
		list := object(t, invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": ch, "summary_only": true}))
		page := object(t, invoke(t, s, root, "pages", map[string]any{"action": "create", "chapter_uuid": ch, "expected_revision": list["state"].(map[string]any)["revision"], "title": fmt.Sprint(n), "description_md": strings.Repeat("正文", 4600), "storyboard_md": "The speaker holds a single flower, using both hands."}))
		pages = append(pages, page["uuid"].(string))
	}
	for _, page := range pages[:7] {
		prepared := preparePageFixture(t, s, root, page, []string{}, "")
		imported := object(t, invoke(t, s, root, "import_image", preparedImportArgs(prepared, imagePath)))
		lastReview := map[string]any{"action": "record_image_review", "chapter_uuid": ch, "page_uuid": page, "variant_uuid": imported["variant_uuid"], "expected_revision": imported["revision"], "verdict": "passed", "observation": "Two hands and one flower; the name in the bubble matches the text."}
		saved := object(t, invoke(t, s, root, "pages", lastReview))
		retry := object(t, invoke(t, s, root, "pages", lastReview))
		if retry["review_revision"] != saved["review_revision"] || saved["revision"] != imported["revision"] {
			t.Fatal("retry changed page revision")
		}
	}
	if bad := invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": ch, "limit": 20}); code(bad) != "result_too_large" {
		t.Fatalf("fixture should overflow full results: %s", code(bad))
	}
	outline := object(t, invoke(t, s, root, "book_outline", map[string]any{"chapter_uuid": ch, "limit": 20}))
	items := outline["items"].([]any)
	if len(items) != 12 {
		t.Fatal(len(items))
	}
	for i, item := range items {
		p := item.(map[string]any)
		want := "passed"
		if i >= 7 {
			want = "missing_image"
		}
		if p["uuid"] != pages[i] || p["review_status"] != want || p["has_image"] != (i < 7) {
			t.Fatal(p)
		}
	}
	// The eighth image contains a defect noted by the visual reviewer.
	page := pages[7]
	imported := object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, page, []string{}, ""), imagePath)))
	get := map[string]any{"action": "get", "chapter_uuid": ch, "page_uuid": page, "summary_only": true}
	if current := object(t, invoke(t, s, root, "pages", get)); current["review_status"] != "pending" {
		t.Fatal(current)
	}
	defect := map[string]any{"action": "record_image_review", "chapter_uuid": ch, "page_uuid": page, "variant_uuid": imported["variant_uuid"], "expected_revision": imported["revision"], "verdict": "needs_revision", "observation": "Middle panel shows a third hand."}
	object(t, invoke(t, s, root, "pages", defect))
	current := object(t, invoke(t, s, root, "pages", get))
	if current["review_status"] != "needs_revision" || current["observation"] != defect["observation"] {
		t.Fatal(current)
	}
	object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, page, []string{}, "Correct the hand"), imagePath)))
	current = object(t, invoke(t, s, root, "pages", get))
	if current["review_status"] != "pending" || current["variant_uuid"] == imported["variant_uuid"] {
		t.Fatal(current)
	}
	defect["expected_revision"] = current["revision"]
	if r := invoke(t, s, root, "pages", defect); r.Success {
		t.Fatal("old review attached to new image")
	}
}

func TestChapterScopedExportsExcludeOtherChapters(t *testing.T) {
	s, root, ch, page := imagePromptPage(t)
	path := preparedFixtureImage(t)
	object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, page, []string{}, ""), path)))
	project := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	other := object(t, invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch02", "title": "OTHER_CHAPTER", "expected_revision": project["revision"]}))
	state := object(t, invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": other["uuid"], "summary_only": true}))["state"].(map[string]any)
	otherPage := object(t, invoke(t, s, root, "pages", map[string]any{"action": "create", "chapter_uuid": other["uuid"], "expected_revision": state["revision"], "title": "Other page", "description_md": "Another chapter", "storyboard_md": "Moonrise."}))
	if r := invoke(t, s, root, "export_book", map[string]any{"format": "pdf", "allow_missing_images": false}); r.Success {
		t.Fatal("whole book should have a missing image")
	}
	for _, format := range []string{"markdown", "zip", "pdf"} {
		result := object(t, invoke(t, s, root, "export_book", map[string]any{"format": format, "allow_missing_images": false, "chapter_uuid": ch}))
		if result["chapter_uuid"] != ch || result["scope"] != "chapter" || result["missing_image_count"] != float64(0) || result["page_count"] != float64(1) {
			t.Fatal(result)
		}
		file := result["path"].(string)
		switch format {
		case "markdown":
			b, err := os.ReadFile(file)
			if err != nil || strings.Contains(string(b), "OTHER_CHAPTER") || !strings.Contains(string(b), "A light") {
				t.Fatal(err, string(b))
			}
		case "zip":
			z, err := zip.OpenReader(file)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			f, err := z.Open("book.json")
			if err != nil {
				t.Fatal(err)
			}
			var data struct {
				Chapters []map[string]any `json:"chapters"`
			}
			err = json.NewDecoder(f).Decode(&data)
			f.Close()
			if err != nil || len(data.Chapters) != 1 {
				t.Fatal(data, err)
			}
			if len(z.File) != 3 {
				t.Fatal("expected two text files and one image", len(z.File))
			}
		case "pdf":
			f, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			var header [5]byte
			_, err = io.ReadFull(f, header[:])
			f.Close()
			if err != nil || string(header[:]) != "%PDF-" {
				t.Fatal(err, header)
			}
		}
	}
	object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, otherPage["uuid"].(string), []string{}, ""), path)))
	for _, scope := range []string{ch, ""} {
		args := map[string]any{"format": "pdf", "allow_missing_images": false}
		want := 2
		if scope != "" {
			args["chapter_uuid"] = scope
			want = 1
		}
		res := object(t, invoke(t, s, root, "export_book", args))
		if res["page_count"] != float64(want) {
			t.Fatal(res)
		}
	}
}

func TestReviewEditsUseOwnRevisionAndPageEditsRequireAnotherReview(t *testing.T) {
	s, root, ch, page := imagePromptPage(t)
	imported := object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, page, []string{}, ""), preparedFixtureImage(t))))
	args := map[string]any{"action": "record_image_review", "chapter_uuid": ch, "page_uuid": page, "variant_uuid": imported["variant_uuid"], "expected_revision": imported["revision"], "verdict": "passed", "observation": "Both hands visible and speaker matches."}
	first := object(t, invoke(t, s, root, "pages", args))
	args["observation"] = "There are three hands in the middle panel."
	args["verdict"] = "needs_revision"
	if r := invoke(t, s, root, "pages", args); r.Success {
		t.Fatal("blind review overwrite")
	}
	args["expected_review_revision"] = first["review_revision"]
	second := object(t, invoke(t, s, root, "pages", args))
	if second["review_revision"] != float64(2) || second["revision"] != imported["revision"] {
		t.Fatal(second)
	}
	object(t, invoke(t, s, root, "pages", map[string]any{"action": "update", "chapter_uuid": ch, "page_uuid": page, "expected_revision": imported["revision"], "storyboard_md": "The corrected panel now has one hand on the door."}))
	current := object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": ch, "page_uuid": page, "summary_only": true}))
	if current["review_status"] != "pending" || current["has_image"] != true {
		t.Fatal(current)
	}
	if r := invoke(t, s, root, "pages", args); r.Success {
		t.Fatal("stale review was accepted after page edit")
	}
}
