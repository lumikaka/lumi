package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

type capturedEvent struct {
	Topic   string
	Event   string
	Payload any
}

type capturedPublisher struct {
	mu     sync.Mutex
	events []capturedEvent
}

type panickingPublisher struct{}

func (panickingPublisher) Broadcast(string, string, any) { panic("realtime unavailable") }

func (publisher *capturedPublisher) Broadcast(topic, event string, payload any) {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	publisher.events = append(publisher.events, capturedEvent{Topic: topic, Event: event, Payload: payload})
}

func testService(t *testing.T) (*Service, *project.Store, *project.Manager) {
	t.Helper()
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	app, err := appstore.Open(dataDir, config.SQLiteDSN(filepath.Join(dataDir, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(app)
	created, err := manager.Create(ctx, "Assets", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var store *project.Store
	if err := manager.WithCurrentStore(ctx, created.UUID, func(current *project.Store) error { store = current; return nil }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close(); _ = app.Close() })
	return NewService(store, nil), store, manager
}

func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 40), B: 120, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestRepairContentRestoresExactMissingOrDamagedAsset(t *testing.T) {
	service, _, _ := testService(t)
	ctx := context.Background()
	content := pngFixture(t)
	asset, err := service.CommitReader(ctx, CommitInput{Purpose: "comic_section_premise", OriginalFilename: "section-premise.png", SourceType: "generated", Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatal(err)
	}
	row, err := service.assetRowByUUID(ctx, asset.UUID, false)
	if err != nil {
		t.Fatal(err)
	}
	path, err := service.assetPath(row.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	repaired, err := service.RepairContent(ctx, asset.UUID, bytes.NewReader(content))
	if err != nil || repaired.UUID != asset.UUID || repaired.Status != ObjectReady {
		t.Fatalf("missing repair=%+v err=%v", repaired, err)
	}
	opened, err := service.OpenContent(ctx, asset.UUID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := io.ReadAll(opened.File)
	_ = opened.File.Close()
	if err != nil || !bytes.Equal(restored, content) {
		t.Fatalf("restored bytes match=%v err=%v", bytes.Equal(restored, content), err)
	}

	damaged := append([]byte(nil), content...)
	damaged[len(damaged)/2] ^= 0xff
	if err := os.WriteFile(path, damaged, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RepairContent(ctx, asset.UUID, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenContent(ctx, asset.UUID); err != nil {
		t.Fatal(err)
	}
	differentImage := image.NewRGBA(image.Rect(0, 0, 5, 3))
	var different bytes.Buffer
	if err := png.Encode(&different, differentImage); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RepairContent(ctx, asset.UUID, bytes.NewReader(different.Bytes())); errorCode(err) != CodeInvalidContent {
		t.Fatalf("mismatched repair error=%v", err)
	}
}

func TestUploadFinalizeIsIdempotentAndDeduplicatesObjects(t *testing.T) {
	service, store, _ := testService(t)
	publisher := &capturedPublisher{}
	service.events = publisher
	ctx := context.Background()
	content := pngFixture(t)
	firstUpload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "wrong.jpg", DisplayName: "Hero", Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatal(err)
	}
	if firstUpload.State != StateReady || firstUpload.MIMEType != "image/png" || firstUpload.Width == nil || *firstUpload.Width != 4 {
		t.Fatalf("upload = %#v", firstUpload)
	}
	first, err := service.FinalizeUpload(ctx, firstUpload.UUID, "premise_asset")
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.FinalizeUpload(ctx, firstUpload.UUID, "premise_asset")
	if err != nil {
		t.Fatal(err)
	}
	if again.UUID != first.UUID {
		t.Fatalf("idempotent UUID = %s, want %s", again.UUID, first.UUID)
	}
	secondUpload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "premise_setting_image", OriginalFilename: "same.png", Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after claiming an already-ready deduplicated object,
	// before the final File/upload transaction. Reconcile must make the same
	// upload UUID retryable instead of leaving it consuming forever.
	firstRow, err := service.assetRowByUUID(ctx, first.UUID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Model(&uploadRecord{}).Where("uuid = ?", secondUpload.UUID).Updates(map[string]any{"state": StateConsuming, "file_object_id": firstRow.FileObjectID}).Error; err != nil {
		t.Fatal(err)
	}
	reconcile, err := service.Reconcile(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if reconcile.Recovered != 1 {
		t.Fatalf("deduplicated reconcile = %#v", reconcile)
	}
	recoveredUpload, err := service.GetUpload(ctx, secondUpload.UUID)
	if err != nil || recoveredUpload.State != StateReady {
		t.Fatalf("recovered upload = %#v, error=%v", recoveredUpload, err)
	}
	second, err := service.FinalizeUpload(ctx, secondUpload.UUID, "premise_setting_image")
	if err != nil {
		t.Fatal(err)
	}
	if second.UUID == first.UUID {
		t.Fatal("logical files were incorrectly deduplicated")
	}
	var objects, files int64
	if err := store.DB().Table("file_objects").Count(&objects).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Table("files").Count(&files).Error; err != nil {
		t.Fatal(err)
	}
	if objects != 1 || files != 2 {
		t.Fatalf("objects=%d files=%d", objects, files)
	}
	secondPart, err := service.partPath(secondUpload.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secondPart); !os.IsNotExist(err) {
		t.Fatalf("deduplicated part retained: %v", err)
	}
	if err := store.DB().Model(&uploadRecord{}).Where("uuid = ?", secondUpload.UUID).Update("state", "unknown").Error; err == nil {
		t.Fatal("upload state CHECK accepted an unknown state")
	}
	if err := store.DB().Model(&objectRecord{}).Where("id = ?", firstRow.FileObjectID).Update("sha256", strings.Repeat("0", 64)).Error; err == nil {
		t.Fatal("ready object immutability trigger accepted a SHA change")
	}
	if err := store.DB().Model(&fileRecord{}).Where("uuid = ?", second.UUID).Update("kind", "script").Error; err == nil {
		t.Fatal("file kind CHECK accepted an unknown kind")
	}
	caseUUID, _ := newUUIDv7()
	caseCollision := objectRecord{UUID: caseUUID, ProjectID: firstRow.ProjectID, SHA256: strings.Repeat("1", 64), KeyPath: strings.ToUpper(firstRow.KeyPath), MIMEType: firstRow.MIMEType, CanonicalExt: firstRow.CanonicalExt, ByteSize: firstRow.ByteSize, State: ObjectPending, CreatedAt: time.Now().UTC()}
	if err := store.DB().Create(&caseCollision).Error; err == nil {
		t.Fatal("case-insensitive key_path uniqueness accepted a collision")
	}
	opened, err := service.OpenContent(ctx, first.UUID)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.File.Close()
	read, err := io.ReadAll(opened.File)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, content) {
		t.Fatal("stored content changed")
	}
	if first.ContentURL != "/media/projects/"+store.ProjectUUID()+"/assets/"+first.UUID+"/content" {
		t.Fatalf("content_url = %s", first.ContentURL)
	}
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if len(publisher.events) == 0 {
		t.Fatal("Asset lifecycle emitted no realtime refresh events")
	}
	for _, event := range publisher.events {
		encoded, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`"id"`, "file_object_id", "key_path", ".lumi", store.Root()} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("realtime %s leaked %q: %s", event.Event, forbidden, encoded)
			}
		}
		if event.Topic != "project:"+store.ProjectUUID() {
			t.Fatalf("event topic = %s", event.Topic)
		}
	}
}

func TestUploadRejectsDetectedTypeAndPurposeMismatch(t *testing.T) {
	service, store, _ := testService(t)
	ctx := context.Background()
	if _, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "fake.png", Reader: bytes.NewBufferString("not an image")}); errorCode(err) != CodeTypeNotAllowed {
		t.Fatalf("type error = %v", err)
	}
	upload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "story_import", OriginalFilename: "chapter.md", Reader: bytes.NewBufferString("# chapter")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FinalizeUpload(ctx, upload.UUID, "premise_asset"); errorCode(err) != CodePurposeMismatch {
		t.Fatalf("purpose error = %v", err)
	}
	content := pngFixture(t)
	if _, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "truncated.png", Reader: bytes.NewReader(content[:len(content)-12])}); errorCode(err) != CodeInvalidContent {
		t.Fatalf("truncated image error = %v", err)
	}
	if _, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "story_import", OriginalFilename: "large.txt", Reader: bytes.NewReader(make([]byte, (2<<20)+1))}); errorCode(err) != CodeFileTooLarge {
		t.Fatalf("size error = %v", err)
	}
	if _, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "metadata.png", Metadata: map[string]any{"tags": map[string]any{"provider_secret": "must-not-project"}}, Reader: bytes.NewReader(content)}); errorCode(err) != CodeValidationFailed {
		t.Fatalf("metadata schema error = %v", err)
	}
	actorUpload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "story_import", OriginalFilename: "actor.txt", Reader: bytes.NewBufferString("actor")})
	if err != nil {
		t.Fatal(err)
	}
	actorUUID, _ := newUUIDv7()
	now := time.Now().UTC()
	otherActor := project.Actor{UUID: actorUUID, Name: "其他本地创作者", Kind: "local_user", CreatedAt: now, UpdatedAt: now}
	if err := store.DB().Create(&otherActor).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Model(&uploadRecord{}).Where("uuid = ?", actorUpload.UUID).Update("actor_id", otherActor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.FinalizeUpload(ctx, actorUpload.UUID, "story_import"); errorCode(err) != CodeActorMismatch {
		t.Fatalf("actor error = %v", err)
	}
	expiredUpload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "story_import", OriginalFilename: "expired.txt", Reader: bytes.NewBufferString("expired")})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Model(&uploadRecord{}).Where("uuid = ?", expiredUpload.UUID).Update("expires_at", now.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.FinalizeUpload(ctx, expiredUpload.UUID, "story_import"); errorCode(err) != CodeUploadExpired {
		t.Fatalf("expiry error = %v", err)
	}
}

func TestConcurrentGeneratedContentDeduplicatesPhysicalObject(t *testing.T) {
	service, store, _ := testService(t)
	content := pngFixture(t)
	const writers = 6
	errorsCh := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := service.CommitReader(context.Background(), CommitInput{Purpose: "premise_asset", OriginalFilename: fmt.Sprintf("generated-%d.png", index), SourceType: "generated", Reader: bytes.NewReader(content)})
			errorsCh <- err
		}(index)
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var objectCount, fileCount int64
	if err := store.DB().Table("file_objects").Count(&objectCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Table("files").Count(&fileCount).Error; err != nil {
		t.Fatal(err)
	}
	if objectCount != 1 || fileCount != writers {
		t.Fatalf("concurrent objects=%d files=%d", objectCount, fileCount)
	}
}

func TestRealtimeFailureDoesNotRollBackCommittedAsset(t *testing.T) {
	service, _, _ := testService(t)
	service.events = panickingPublisher{}
	upload, err := service.CreateUpload(context.Background(), CreateUploadInput{Purpose: "story_import", OriginalFilename: "event.txt", Reader: bytes.NewBufferString("durable")})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.FinalizeUpload(context.Background(), upload.UUID, "story_import")
	if err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenContent(context.Background(), asset.UUID)
	if err != nil {
		t.Fatal(err)
	}
	_ = opened.File.Close()
}

func TestFinalizeRejectsSymlinkEscape(t *testing.T) {
	for _, unsafe := range []string{"../escape.png", "/absolute.png", `C:/windows.png`, `\\server\share.png`, "nul\x00.png"} {
		if errorCode(validateKeyPath(unsafe)) != CodeUnsafePath {
			t.Fatalf("unsafe key accepted: %q", unsafe)
		}
	}
	service, store, _ := testService(t)
	upload, err := service.CreateUpload(context.Background(), CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "escape.png", Reader: bytes.NewReader(pngFixture(t))})
	if err != nil {
		t.Fatal(err)
	}
	premiseDir, err := store.ResolvePath("assets/premise")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), premiseDir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := service.FinalizeUpload(context.Background(), upload.UUID, "premise_asset"); errorCode(err) != CodeUnsafePath {
		t.Fatalf("symlink error=%v", err)
	}
}

func TestReconcileRecoversPendingRenameWindowAndReportsMissing(t *testing.T) {
	service, store, _ := testService(t)
	ctx := context.Background()
	upload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "recover.png", Reader: bytes.NewReader(pngFixture(t))})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.uploadRecord(ctx, upload.UUID)
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := policyFor(record.Purpose)
	objectUUID, _ := newUUIDv7()
	object := objectRecord{UUID: objectUUID, ProjectID: record.ProjectID, SHA256: *record.SHA256, KeyPath: policy.Namespace + "/recover--" + record.ReservedFileUUID + "." + *record.CanonicalExt, MIMEType: *record.MIMEType, CanonicalExt: *record.CanonicalExt, ByteSize: *record.ByteSize, Width: record.Width, Height: record.Height, State: ObjectPending, CreatedAt: time.Now().UTC()}
	if err := store.DB().Create(&object).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Model(&uploadRecord{}).Where("id = ?", record.ID).Updates(map[string]any{"state": StateConsuming, "file_object_id": object.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.publishObject(record, object); err != nil {
		t.Fatal(err)
	}
	summary, err := service.Reconcile(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovered != 1 {
		t.Fatalf("summary=%#v", summary)
	}
	recovered, err := service.GetUpload(ctx, upload.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != StateReady {
		t.Fatalf("state=%s", recovered.State)
	}
	asset, err := service.FinalizeUpload(ctx, upload.UUID, "premise_asset")
	if err != nil {
		t.Fatal(err)
	}
	row, err := service.assetRowByUUID(ctx, asset.UUID, false)
	if err != nil {
		t.Fatal(err)
	}
	path, err := service.assetPath(row.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	summary, err = service.Reconcile(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Missing < 1 {
		t.Fatalf("missing summary=%#v", summary)
	}
	if _, err := service.OpenContent(ctx, asset.UUID); errorCode(err) != CodeObjectUnavailable {
		t.Fatalf("open missing error=%v", err)
	}
}

func TestReadyAssetSurvivesProjectMove(t *testing.T) {
	service, store, manager := testService(t)
	ctx := context.Background()
	content := pngFixture(t)
	asset, err := service.CommitReader(ctx, CommitInput{Purpose: "premise_asset", OriginalFilename: "move.png", SourceType: "generated", Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatal(err)
	}
	projectUUID := store.ProjectUUID()
	oldRoot := store.Root()
	newRoot := filepath.Join(filepath.Dir(oldRoot), "moved-project.lumi")
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	opened, err := manager.OpenSelected(ctx, project.ExplicitExistingDirectory(newRoot))
	if err != nil {
		t.Fatal(err)
	}
	if opened.UUID != projectUUID {
		t.Fatalf("moved project UUID = %s, want %s", opened.UUID, projectUUID)
	}
	if err := manager.WithCurrentStore(ctx, projectUUID, func(current *project.Store) error {
		result, openErr := NewService(current, nil).OpenContent(ctx, asset.UUID)
		if openErr != nil {
			return openErr
		}
		defer result.File.Close()
		actual, readErr := io.ReadAll(result.File)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(actual, content) {
			t.Fatal("moved Asset content changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrityScanPersistsOrphansAndGCDryRunIsNonDestructive(t *testing.T) {
	service, store, _ := testService(t)
	ctx := context.Background()
	upload, err := service.CreateUpload(ctx, CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "scan.png", Reader: bytes.NewReader(pngFixture(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.FinalizeUpload(ctx, upload.UUID, "premise_asset")
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := store.ResolvePath("assets/premise/assets/orphan.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	corruptAsset, err := service.CommitReader(ctx, CommitInput{Purpose: "story_import", OriginalFilename: "corrupt.txt", SourceType: "generated", Reader: bytes.NewBufferString("source")})
	if err != nil {
		t.Fatal(err)
	}
	corruptRow, err := service.assetRowByUUID(ctx, corruptAsset.UUID, false)
	if err != nil {
		t.Fatal(err)
	}
	corruptPath, err := service.assetPath(corruptRow.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corruptPath, []byte("tamper"), 0o600); err != nil {
		t.Fatal(err)
	}
	scan, err := service.RunIntegrityScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range scan.Findings {
		if finding.Kind == "orphan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings=%#v", scan.Findings)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("scan changed orphan: %v", err)
	}
	var quarantined objectRecord
	if err := store.DB().First(&quarantined, corruptRow.FileObjectID).Error; err != nil {
		t.Fatal(err)
	}
	if quarantined.State != ObjectQuarantined {
		t.Fatalf("corrupt object state = %s", quarantined.State)
	}
	quarantinePath, err := service.quarantinePath(quarantined.UUID, quarantined.CanonicalExt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(quarantinePath); err != nil {
		t.Fatalf("quarantined content missing: %v", err)
	}
	if _, err := service.OpenContent(ctx, corruptAsset.UUID); errorCode(err) != CodeObjectUnavailable {
		t.Fatalf("quarantined content opened: %v", err)
	}
	if _, err := service.SoftDelete(ctx, asset.UUID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	if err := store.DB().Table("files").Where("uuid = ?", asset.UUID).Update("deleted_at", old).Error; err != nil {
		t.Fatal(err)
	}
	plan, err := service.GCDryRun(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("plan=%#v", plan)
	}
	assetUUIDs, ok := plan.Entries[0].ReferenceSummary["asset_uuids"].([]any)
	if !ok || len(assetUUIDs) != 1 || assetUUIDs[0] != asset.UUID {
		t.Fatalf("GC reference summary=%#v", plan.Entries[0].ReferenceSummary)
	}
	row, err := service.assetRowByUUID(ctx, asset.UUID, true)
	if err != nil {
		t.Fatal(err)
	}
	path, err := service.assetPath(row.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dry-run removed file: %v", err)
	}
	if _, err := service.Restore(ctx, asset.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GCApply(ctx, plan.UUID, 7*24*time.Hour); errorCode(err) != CodeGCPlanStale {
		t.Fatalf("changed snapshot error=%v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stale apply changed file: %v", err)
	}
	if _, err := service.SoftDelete(ctx, asset.UUID); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Table("files").Where("uuid = ?", asset.UUID).Update("deleted_at", old).Error; err != nil {
		t.Fatal(err)
	}
	plan, err = service.GCDryRun(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := service.GCApply(ctx, plan.UUID, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != "applied" {
		t.Fatalf("applied=%#v", applied)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("GC path error=%v", err)
	}
	if _, err := service.SoftDelete(ctx, corruptAsset.UUID); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Table("files").Where("uuid = ?", corruptAsset.UUID).Update("deleted_at", old).Error; err != nil {
		t.Fatal(err)
	}
	quarantinePlan, err := service.GCDryRun(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantinePlan.Entries) != 1 {
		t.Fatalf("quarantine GC plan=%#v", quarantinePlan)
	}
	if _, err := service.GCApply(ctx, quarantinePlan.UUID, 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(quarantinePath); !os.IsNotExist(err) {
		t.Fatalf("GC retained quarantined content: %v", err)
	}
	assertProjectSQLiteHealthy(t, store)
}

func assertProjectSQLiteHealthy(t *testing.T, store *project.Store) {
	t.Helper()
	var integrity string
	if err := store.DB().Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil || integrity != "ok" {
		t.Fatalf("integrity_check=%q error=%v", integrity, err)
	}
	rows, err := store.DB().Raw("PRAGMA foreign_key_check").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check returned a violation")
	}
}

func errorCode(err error) string {
	var domain *Error
	if errors.As(err, &domain) {
		return domain.Code
	}
	return ""
}
