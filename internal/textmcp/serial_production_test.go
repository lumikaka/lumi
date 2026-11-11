package textmcp

import (
	"testing"
)

// Protocol acceptance for a two-chapter/three-scene production. Images and
// visual verdicts are test doubles; this does not validate host orchestration
// or a model's art review. Every business read/write uses the public MCP API.
func TestSerialProductionSettingRepairAndImportedPageResume(t *testing.T) {
	s, root, chapter10, page1 := imagePromptPage(t)
	path := preparedFixtureImage(t)
	createPage := func(chapter, title string) string {
		state := object(t, invoke(t, s, root, "pages", map[string]any{"action": "list", "chapter_uuid": chapter, "summary_only": true}))["state"].(map[string]any)
		return object(t, invoke(t, s, root, "pages", map[string]any{"action": "create", "chapter_uuid": chapter, "expected_revision": state["revision"], "title": title, "description_md": "小悠：钥匙在这里。", "storyboard_md": "小悠右手持铜钥匙，红外套。逐字对白：钥匙在这里。"}))["uuid"].(string)
	}
	page2, page3 := createPage(chapter10, "码头第二页"), createPage(chapter10, "灯塔第一场")
	project := object(t, invoke(t, s, root, "project_overview", map[string]any{}))["project"].(map[string]any)
	chapter11 := object(t, invoke(t, s, root, "chapters", map[string]any{"action": "create", "chapter_code": "vol01.ch11", "title": "海上", "expected_revision": project["revision"]}))["uuid"].(string)
	page4 := createPage(chapter11, "海上第一场")
	get := func(chapter, page string) map[string]any {
		return object(t, invoke(t, s, root, "pages", map[string]any{"action": "get", "chapter_uuid": chapter, "page_uuid": page, "summary_only": true}))
	}
	review := func(chapter, page, verdict, observation string) {
		p := get(chapter, page)
		args := map[string]any{"action": "record_image_review", "chapter_uuid": chapter, "page_uuid": page, "variant_uuid": p["variant_uuid"], "expected_revision": p["revision"], "verdict": verdict, "observation": observation}
		if p["review_revision"] != nil {
			args["expected_review_revision"] = p["review_revision"]
		}
		object(t, invoke(t, s, root, "pages", args))
	}
	setting := createReadinessSetting(t, s, root, "character", "小悠红外套与铜钥匙")
	settingImage := object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": setting["revision"]}))
	selection := []string{setting["uuid"].(string)}
	importPage := func(page string) map[string]any {
		return object(t, invoke(t, s, root, "import_image", preparedImportArgs(preparePageFixture(t, s, root, page, selection, ""), path)))
	}
	importPage(page1)
	review(chapter10, page1, "passed", "Synthetic review before discovering the shared setting defect")
	imported2 := importPage(page2) // Interrupt before review/report, after commit.
	oldReviews := fixtureSettingReviews(t, s, root, page2, selection)
	stalePrepared := preparePageFixture(t, s, root, page2, selection, "")
	resumed := get(chapter10, page2)
	if resumed["review_status"] != "pending" || resumed["variant_uuid"] != imported2["variant_uuid"] {
		t.Fatalf("resume must review the saved image, without another import: %+v", resumed)
	}
	// Shared setting error discovered in the interrupted scene. Its repair
	// invalidates generation evidence but does not silently invalidate page reviews.
	updated := object(t, invoke(t, s, root, "settings", map[string]any{"action": "update", "setting_uuid": setting["uuid"], "expected_revision": settingImage["revision"], "summary": "红外套，铜钥匙始终在右手；修正设定中的左右手错误。"}))
	finalSetting := object(t, invoke(t, s, root, "import_image", map[string]any{"path": path, "target_uuid": setting["uuid"], "slot": "setting_image", "expected_revision": updated["revision"]}))
	if got := code(invoke(t, s, root, "import_image", preparedImportArgs(stalePrepared, path))); got != "page_image_preparation_stale" {
		t.Fatalf("old preparation survived setting repair: %s", got)
	}
	if got := code(invoke(t, s, root, "prepare_page_image", map[string]any{"page_uuid": page2, "setting_uuids": selection, "setting_reviews": oldReviews})); got != "setting_image_review_stale" {
		t.Fatalf("old setting assessment survived repair: %s", got)
	}
	if get(chapter10, page1)["review_status"] != "passed" {
		t.Fatal("fixture expects explicit impact re-review, not automatic setting propagation")
	}
	// Revisit both affected pages, including the previously passed page. The
	// second scene and chapter stay untouched while the first scene is repaired.
	for _, page := range []string{page1, page2} {
		review(chapter10, page, "needs_revision", "Synthetic setting defect: key on the wrong hand")
		importPage(page)
		review(chapter10, page, "passed", "Synthetic repaired right hand, coat, copy and continuity")
	}
	if get(chapter10, page3)["review_status"] != "missing_image" || get(chapter11, page4)["review_status"] != "missing_image" {
		t.Fatal("later scene/chapter was modified during the first scene")
	}
	importPage(page3)
	review(chapter10, page3, "passed", "Synthetic lighthouse continuity checked")
	outline := object(t, invoke(t, s, root, "book_outline", map[string]any{"chapter_uuid": chapter10, "limit": 100}))
	for _, raw := range outline["items"].([]any) {
		if raw.(map[string]any)["review_status"] != "passed" {
			t.Fatal("chapter handoff before all target pages passed")
		}
	}
	// The next chapter must use the final setting, not the initial version.
	prepared11 := preparePageFixture(t, s, root, page4, selection, "接续灯塔场景，钥匙在右手。")
	source := prepared11["sources"].([]any)[0].(map[string]any)
	if source["variant_uuid"] != finalSetting["variant_uuid"] || source["variant_uuid"] == settingImage["variant_uuid"] {
		t.Fatal("chapter handoff used an obsolete setting version")
	}
	object(t, invoke(t, s, root, "import_image", preparedImportArgs(prepared11, path)))
	review(chapter11, page4, "passed", "Synthetic cross-chapter right-hand key continuity checked")
	if get(chapter11, page4)["review_status"] != "passed" {
		t.Fatal("next chapter review not saved")
	}
}
