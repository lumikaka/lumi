package production

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/project"
	"lumi/internal/story"
)

func websiteChapter(t *testing.T, h *productionHarness) story.Chapter {
	t.Helper()
	c, err := h.stories.CreateChapter(t.Context(), story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Moon & <Stars>", Content: "PRIVATE STORY MUST NOT LEAK", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func websiteSection(t *testing.T, h *productionHarness, chapter string, b []byte) ComicSection {
	t.Helper()
	page, err := h.service.CreateSection(t.Context(), chapter, CreateSectionInput{Title: "A page", StoryboardMD: "PRIVATE PROMPT MUST NOT LEAK"})
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		page, err = h.service.ImportSectionImage(t.Context(), chapter, page.UUID, upload(t, h.service, "comic_section_image", b), page.Revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	return page
}
func freezeWebsite(t *testing.T, s *Service) (string, websiteGeneration) {
	t.Helper()
	snapshot, _, hash, err := s.BuildWebsiteSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(snapshot)
	taskUUID := mustUUID(t)
	now := time.Now().UTC()
	if err = s.store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,created_at,updated_at) SELECT ?,id,'website_generation',?,'{}','running',?,?,? FROM projects WHERE uuid=?`, taskUUID, s.store.ProjectUUID(), taskUUID, now, now, s.store.ProjectUUID()).Error; err != nil {
		t.Fatal(err)
	}
	var taskID, projectID int64
	s.store.DB().Table("production_task_runs").Select("id").Where("uuid=?", taskUUID).Scan(&taskID)
	s.store.DB().Table("projects").Select("id").Where("uuid=?", s.store.ProjectUUID()).Scan(&projectID)
	g := websiteGeneration{UUID: mustUUID(t), ProjectID: projectID, TaskID: taskID, SnapshotJSON: string(encoded), SnapshotHash: hash, ManifestJSON: "{}", CreatedAt: now}
	if err = s.store.DB().Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	return taskUUID, g
}
func runWebsite(t *testing.T, s *Service) websiteGeneration {
	t.Helper()
	uuid, _ := freezeWebsite(t, s)
	if err := s.RenderWebsite(t.Context(), uuid, nil); err != nil {
		t.Fatal(err)
	}
	current, err := s.currentWebsite(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return current
}
func websiteJSON(t *testing.T, s *Service) websitePresentation {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.store.Root(), "website/data/site.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data websitePresentation
	if err = json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWebsiteIncrementalBuildPrivacyAndManagedCleanup(t *testing.T) {
	h := newProductionHarness(t)
	s := h.service
	c := websiteChapter(t, h)
	one := websiteSection(t, h, c.UUID, imageBytes(t, 10))
	two := websiteSection(t, h, c.UUID, imageBytes(t, 20))
	three := websiteSection(t, h, c.UUID, imageBytes(t, 10)) // identical source only produces one JPEG
	first := runWebsite(t, s)
	if first.ProcessedImages != 2 || first.ReusedImages != 0 {
		t.Fatalf("first=%+v", first)
	}
	output := filepath.Join(s.store.Root(), "website")
	data := websiteJSON(t, s)
	if len(data.Chapters) != 1 || len(data.Chapters[0].Pages) != 3 || data.Chapters[0].Pages[0].Source != data.Chapters[0].Pages[2].Source {
		t.Fatalf("presentation=%+v", data)
	}
	if data.Chapters[0].Pages[0].Width != 12 || data.Chapters[0].Pages[0].Height != 8 {
		t.Fatal("small image upscaled")
	}
	if err := filepath.WalkDir(output, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(path, ".jpg") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, forbidden := range []string{"PRIVATE PROMPT", "PRIVATE STORY", "assets/", "project.sqlite", s.store.Root(), `"id":`, `"asset_uuid":`, `"sha256":`, `"snapshot"`} {
			if bytes.Contains(b, []byte(forbidden)) {
				t.Errorf("%s leaks %s", path, forbidden)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	second := runWebsite(t, s)
	if second.ProcessedImages != 0 || second.ReusedImages != 2 {
		t.Fatalf("unchanged=%+v", second)
	}
	if err := os.WriteFile(filepath.Join(output, "notes.txt"), []byte("user-owned"), 0644); err != nil {
		t.Fatal(err)
	}
	// Title and order change the website but not the image cache key.
	if err := s.store.DB().Exec(`UPDATE projects SET name='Changed title'`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReorderSections(t.Context(), c.UUID, []string{two.UUID, one.UUID, three.UUID}); err != nil {
		t.Fatal(err)
	}
	if err := s.store.DB().Exec(`UPDATE chapters SET title='Renamed chapter' WHERE uuid=?`, c.UUID).Error; err != nil {
		t.Fatal(err)
	}
	changed := runWebsite(t, s)
	if changed.ProcessedImages != 0 || changed.ReusedImages != 2 {
		t.Fatalf("title=%+v", changed)
	}
	two, err := s.GetSection(t.Context(), c.UUID, two.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportSectionImage(t.Context(), c.UUID, two.UUID, upload(t, s, "comic_section_image", imageBytes(t, 30)), two.Revision); err != nil {
		t.Fatal(err)
	}
	replacement := runWebsite(t, s)
	if replacement.ProcessedImages != 1 || replacement.ReusedImages != 1 {
		t.Fatalf("replacement=%+v", replacement)
	}
	if _, err := os.Stat(filepath.Join(output, data.Chapters[0].Pages[1].Source)); !os.IsNotExist(err) {
		t.Fatal("unreferenced managed image retained")
	}
	if b, _ := os.ReadFile(filepath.Join(output, "notes.txt")); string(b) != "user-owned" {
		t.Fatal("unrelated file lost")
	}
	entries, _ := os.ReadDir(filepath.Join(s.store.Root(), ".lumi/website-staging"))
	if len(entries) != 0 {
		t.Fatal("staging cache retained")
	}
	if _, err := os.Stat(filepath.Join(s.store.Root(), ".lumi/website-previous")); !os.IsNotExist(err) {
		t.Fatal("second image cache retained")
	}
	for _, mode := range []string{"missing", "corrupt"} {
		current := websiteJSON(t, s)
		target := filepath.Join(output, current.Chapters[0].Pages[0].Source)
		if mode == "missing" {
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
		} else {
			b, _ := os.ReadFile(target)
			b[len(b)/2] ^= 255
			if err := os.WriteFile(target, b, 0644); err != nil {
				t.Fatal(err)
			}
		}
		rebuilt := runWebsite(t, s)
		if rebuilt.ProcessedImages != 1 || rebuilt.ReusedImages != 1 {
			t.Fatalf("%s=%+v", mode, rebuilt)
		}
	}
	status, err := s.WebsiteStatus(t.Context())
	if err != nil || !status.Available || status.HasUpdates {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestWebsiteSnapshotRemainsFrozenAndFailureKeepsPrevious(t *testing.T) {
	h := newProductionHarness(t)
	s := h.service
	c := websiteChapter(t, h)
	section := websiteSection(t, h, c.UUID, imageBytes(t, 41))
	original := runWebsite(t, s)
	originalRSS, err := os.ReadFile(filepath.Join(s.store.Root(), "website/rss.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"failure", "cancelled", "edited"} {
		task, g := freezeWebsite(t, s)
		callback := func(progress int) error {
			if progress != 95 {
				return nil
			}
			switch mode {
			case "failure":
				return errors.New("simulated disk error")
			case "cancelled":
				return s.store.DB().Exec(`UPDATE production_task_runs SET cancel_requested_at=? WHERE id=?`, time.Now(), g.TaskID).Error
			case "edited":
				if err := s.store.DB().Exec(`UPDATE projects SET name='Edited during build'`).Error; err != nil {
					return err
				}
				_, err := s.ImportSectionImage(t.Context(), c.UUID, section.UUID, upload(t, s, "comic_section_image", imageBytes(t, 45)), section.Revision)
				return err
			}
			return nil
		}
		err := s.RenderWebsite(t.Context(), task, callback)
		if mode == "edited" {
			if err != nil {
				t.Fatal(err)
			}
			data := websiteJSON(t, s)
			if data.Title == "Edited during build" {
				t.Fatal("live edits contaminated frozen snapshot")
			}
			status, err := s.WebsiteStatus(t.Context())
			if err != nil || !status.HasUpdates {
				t.Fatalf("must retain update indicator: %+v %v", status, err)
			}
		} else {
			if err == nil {
				t.Fatal("expected render failure")
			}
			current, err := s.currentWebsite(t.Context())
			if err != nil || current.UUID != original.UUID {
				t.Fatal("previous website replaced on failure")
			}
			if rss, err := os.ReadFile(filepath.Join(s.store.Root(), "website/rss.xml")); err != nil || !bytes.Equal(rss, originalRSS) {
				t.Fatal("previous RSS replaced on failure or cancellation", err)
			}
			if err := s.store.DB().Exec(`UPDATE production_task_runs SET status='cancelled',cancel_requested_at=NULL WHERE id=?`, g.TaskID).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestWebsiteMissingPhysicalImagesAndWidthPreset(t *testing.T) {
	h := newProductionHarness(t)
	s := h.service
	c := websiteChapter(t, h)
	websiteSection(t, h, c.UUID, imageBytes(t, 11))
	page := websiteSection(t, h, c.UUID, imageBytes(t, 12))
	websiteSection(t, h, c.UUID, nil)
	var key string
	if err := s.store.DB().Raw(`SELECT o.key_path FROM file_objects o JOIN files f ON f.file_object_id=o.id WHERE f.uuid=?`, page.CurrentImage.Asset.UUID).Scan(&key).Error; err != nil {
		t.Fatal(err)
	}
	// Use the opened file's name; the production reader still goes through Asset Store.
	content, err := s.files.OpenContent(t.Context(), page.CurrentImage.Asset.UUID)
	if err != nil {
		t.Fatal(err)
	}
	source := content.File.Name()
	content.File.Close()
	if err = os.Remove(source); err != nil {
		t.Fatal(err)
	}
	snapshot, readiness, _, err := s.BuildWebsiteSnapshot(t.Context())
	if err != nil || readiness.Complete || readiness.MissingSectionCount != 2 || len(snapshot.Chapters[0].Pages) != 1 {
		t.Fatalf("readiness=%+v err=%v", readiness, err)
	}
	runWebsite(t, s)
	if websiteJSON(t, s).MissingCount != 2 {
		t.Fatal("partial export lost missing count")
	}
	// Tall strips scale by width: 2000x6000 -> 1600x4800, not 533x1600.
	img := image.NewRGBA(image.Rect(0, 0, 2000, 6000))
	img.Set(0, 0, color.White)
	var buffer bytes.Buffer
	if err = png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	tall := websiteSection(t, h, c.UUID, buffer.Bytes())
	snap, _, _, err := s.BuildWebsiteSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var frozen WebsitePage
	for _, p := range snap.Chapters[0].Pages {
		if p.UUID == tall.UUID {
			frozen = p
		}
	}
	b, meta, err := s.compressWebsiteImage(t.Context(), frozen)
	if err != nil || meta.Width != 1600 || meta.Height != 4800 {
		t.Fatalf("meta=%+v err=%v", meta, err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	r, g, blue, _ := decoded.At(100, 100).RGBA()
	if r < 64000 || g < 64000 || blue < 64000 {
		t.Fatal("transparent source did not composite on white")
	}
	if websiteMediaKey(frozen.SHA256, "v-next") == websiteMediaKey(frozen.SHA256, WebsiteProcessor) {
		t.Fatal("processor version not in key")
	}
}

func TestWebsiteRecoveryAcrossDirectorySwitchBoundaries(t *testing.T) {
	for _, phase := range []string{"journal", "old_moved", "new_moved", "committed"} {
		t.Run(phase, func(t *testing.T) {
			h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
			s := h.service
			c := websiteChapter(t, h)
			websiteSection(t, h, c.UUID, imageBytes(t, 50))
			old := runWebsite(t, s)
			_, next := freezeWebsite(t, s)
			root := s.store.Root()
			out := filepath.Join(root, "website")
			backup := filepath.Join(root, ".lumi/website-previous")
			swap, _ := json.Marshal(websiteSwap{GenerationUUID: next.UUID, PreviousUUID: old.UUID})
			if err := websiteWrite(filepath.Join(root, ".lumi/website-swap.json"), swap); err != nil {
				t.Fatal(err)
			}
			if phase != "journal" {
				if err := os.Rename(out, backup); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "new_moved" || phase == "committed" {
				marker, _ := json.Marshal(map[string]string{"generation_uuid": next.UUID})
				if err := websiteWrite(filepath.Join(out, "data/build.json"), marker); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "committed" {
				if err := s.store.DB().Exec(`UPDATE project_websites SET generation_id=?`, next.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := s.RecoverWebsite(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := old.UUID
			if phase == "committed" {
				want = next.UUID
			}
			if websiteMarker(out) != want {
				t.Fatalf("recovered %s want %s", websiteMarker(out), want)
			}
			if phase != "committed" {
				feed, err := os.ReadFile(filepath.Join(out, "rss.xml"))
				if err != nil || len(parseWebsiteRSS(t, feed).Channel.Items) != 1 {
					t.Fatal("interruption lost the previous RSS", err)
				}
			}
			if err := s.RecoverWebsite(context.Background()); err != nil {
				t.Fatal("recovery not idempotent", err)
			}
			if _, err := os.Stat(backup); !os.IsNotExist(err) {
				t.Fatal("backup retained")
			}
		})
	}
}

func TestWebsiteFirstBuildRecoveryAndUnmanagedDirectory(t *testing.T) {
	h := newProductionHarness(t)
	s := h.service
	c := websiteChapter(t, h)
	websiteSection(t, h, c.UUID, imageBytes(t, 10))
	task, generation := freezeWebsite(t, s)
	root := s.store.Root()
	output := filepath.Join(root, "website")
	if err := websiteWrite(filepath.Join(root, ".lumi/website-swap.json.part"), []byte(`{"generation_uuid":`)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverWebsite(t.Context()); err != nil {
		t.Fatal("torn journal blocked recovery", err)
	}
	swap, _ := json.Marshal(websiteSwap{GenerationUUID: generation.UUID})
	if err := websiteWriteJournal(filepath.Join(root, ".lumi/website-swap.json"), swap); err != nil {
		t.Fatal(err)
	}
	marker, _ := json.Marshal(map[string]string{"generation_uuid": generation.UUID})
	if err := websiteWrite(filepath.Join(output, "data/build.json"), marker); err != nil {
		t.Fatal(err)
	}
	// Simulate interruption during rollback's deletion of the unpublished build.
	discard := filepath.Join(root, ".lumi/website-discard-"+generation.UUID)
	if err := os.Rename(output, discard); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(discard, "data/build.json")); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverWebsite(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("uncommitted first website exposed")
	}
	if err := websiteWrite(filepath.Join(output, "manual.txt"), []byte("keep me")); err != nil {
		t.Fatal(err)
	}
	if err := s.RenderWebsite(t.Context(), task, nil); err == nil {
		t.Fatal("took over unmanaged website")
	}
	if b, _ := os.ReadFile(filepath.Join(output, "manual.txt")); string(b) != "keep me" {
		t.Fatal("unmanaged content lost")
	}
}

func TestWebsiteSnapshotUsesActiveOrderAndSelectedVersion(t *testing.T) {
	h := newProductionHarnessWithFormat(t, project.PictureBookClassic)
	s := h.service
	c := websiteChapter(t, h)
	page := websiteSection(t, h, c.UUID, imageBytes(t, 1))
	selected := page.CurrentImage
	changed, err := s.ImportSectionImage(t.Context(), c.UUID, page.UUID, upload(t, s, "comic_section_image", imageBytes(t, 2)), page.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SelectImageVariant(t.Context(), c.UUID, page.UUID, selected.UUID, changed.Revision); err != nil {
		t.Fatal(err)
	}
	front, err := s.CreateSection(t.Context(), c.UUID, CreateSectionInput{Title: "Cover", PageRole: PageRoleFrontCover, StoryboardMD: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportSectionImage(t.Context(), c.UUID, front.UUID, upload(t, s, "comic_section_image", imageBytes(t, 3)), front.Revision); err != nil {
		t.Fatal(err)
	}
	second, err := h.stories.CreateChapter(t.Context(), story.CreateChapterInput{ChapterCode: "vol01.ch02", Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	websiteSection(t, h, second.UUID, imageBytes(t, 4))
	deleted, err := h.stories.CreateChapter(t.Context(), story.CreateChapterInput{ChapterCode: "vol01.ch03", Title: "Deleted"})
	if err != nil {
		t.Fatal(err)
	}
	websiteSection(t, h, deleted.UUID, imageBytes(t, 5))
	if err = s.store.DB().Exec(`UPDATE chapters SET deleted_at=? WHERE uuid=?`, time.Now(), deleted.UUID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = h.stories.ReorderChapters(t.Context(), []string{second.UUID, c.UUID}); err != nil {
		t.Fatal(err)
	}
	snapshot, _, _, err := s.BuildWebsiteSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Chapters) != 2 || snapshot.Chapters[0].UUID != second.UUID || snapshot.Chapters[1].Pages[0].Role != PageRoleFrontCover || snapshot.Chapters[1].Pages[1].AssetUUID != selected.Asset.UUID {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestWebsitePublicationDatabaseFailureRollsBackAndRetries(t *testing.T) {
	h := newProductionHarness(t)
	s := h.service
	c := websiteChapter(t, h)
	websiteSection(t, h, c.UUID, imageBytes(t, 70))
	previous := runWebsite(t, s)
	previousRSS, err := os.ReadFile(filepath.Join(s.store.Root(), "website/rss.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.DB().Exec(`UPDATE chapters SET title='Changed before failed publication' WHERE uuid=?`, c.UUID).Error; err != nil {
		t.Fatal(err)
	}
	task, _ := freezeWebsite(t, s)
	if err := s.store.DB().Exec(`CREATE TRIGGER website_test_reject BEFORE UPDATE OF completed_at ON website_generations BEGIN SELECT RAISE(ABORT,'injected publication failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RenderWebsite(t.Context(), task, nil); err == nil {
		t.Fatal("publication error ignored")
	}
	output := filepath.Join(s.store.Root(), "website")
	if websiteMarker(output) != previous.UUID {
		t.Fatal("database rollback did not restore previous website")
	}
	if rss, err := os.ReadFile(filepath.Join(output, "rss.xml")); err != nil || !bytes.Equal(rss, previousRSS) {
		t.Fatal("database rollback did not restore previous RSS", err)
	}
	current, err := s.currentWebsite(t.Context())
	if err != nil || current.UUID != previous.UUID {
		t.Fatalf("database publication changed: %+v %v", current, err)
	}
	if err = s.store.DB().Exec(`DROP TRIGGER website_test_reject`).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.RenderWebsite(t.Context(), task, nil); err != nil {
		t.Fatal("retry failed", err)
	}
	current, err = s.currentWebsite(t.Context())
	if err != nil || current.UUID == previous.UUID || current.ProcessedImages != 0 || current.ReusedImages != 1 {
		t.Fatalf("retry=%+v %v", current, err)
	}
}
