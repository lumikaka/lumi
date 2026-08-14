package jobqueue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lumi/internal/production"
	"lumi/internal/project"

	"github.com/riverqueue/river/rivertype"
)

func TestComicExportCleanupPeriodicContract(t *testing.T) {
	if comicExportCleanupPeriod != time.Hour {
		t.Fatalf("cleanup period=%s", comicExportCleanupPeriod)
	}
	periodic := comicExportCleanupPeriodicOptions()
	if periodic.ID != "comic_export_cleanup_v1" || !periodic.RunOnStart {
		t.Fatalf("periodic options=%+v", periodic)
	}
	projectUUID := "01900000-0000-7000-8000-000000000901"
	args, insert := comicExportCleanupInsert(projectUUID)
	typed, ok := args.(exportCleanupArgs)
	if !ok || typed.Version != 1 || typed.ProjectUUID != projectUUID {
		t.Fatalf("cleanup args=%+v", args)
	}
	if insert.Queue != QueueAssetMaintenance || insert.MaxAttempts != 3 || !insert.UniqueOpts.ByArgs {
		t.Fatalf("cleanup insert opts=%+v", insert)
	}
	wanted := map[rivertype.JobState]bool{
		rivertype.JobStateAvailable: true, rivertype.JobStatePending: true,
		rivertype.JobStateRunning: true, rivertype.JobStateRetryable: true,
		rivertype.JobStateScheduled: true,
	}
	if len(insert.UniqueOpts.ByState) != len(wanted) {
		t.Fatalf("unique active states=%v", insert.UniqueOpts.ByState)
	}
	for _, state := range insert.UniqueOpts.ByState {
		if !wanted[state] {
			t.Fatalf("unexpected unique state=%s", state)
		}
	}
	if comicExportCleanupPeriodicJob(projectUUID) == nil {
		t.Fatal("cleanup periodic job was nil")
	}
}

func TestComicExportCleanupRunOnStartCompensatesDowntime(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	if err := harness.queue.StopProject(ctx, harness.project.UUID); err != nil {
		t.Fatal(err)
	}
	const (
		exportUUID   = "01900000-0000-7000-8000-000000000911"
		taskUUID     = "01900000-0000-7000-8000-000000000912"
		snapshotHash = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	)
	now := time.Now().UTC()
	content := []byte("cleanup missed while Lumi was closed")
	digest := sha256.Sum256(content)
	var target string
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		var projectID int64
		if err := store.DB().Table("projects").Where("uuid = ?", harness.project.UUID).Pluck("id", &projectID).Error; err != nil {
			return err
		}
		snapshot := production.ExportSnapshot{Version: 2, ProjectUUID: harness.project.UUID, Scope: "project"}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		relative := production.ExportRelativePath(exportUUID, "project", "", snapshotHash, snapshot)
		target, err = store.ResolvePath(relative)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
		completedAt := now.Add(-8 * 24 * time.Hour)
		if err := store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,progress,attempt,max_attempts,completed_at,created_at,updated_at) VALUES(?,?,'comic_export',?,'{}','completed','cleanup-run-on-start',100,1,3,?,?,?)`, taskUUID, projectID, harness.project.UUID, completedAt, completedAt, completedAt).Error; err != nil {
			return err
		}
		return store.DB().Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,expires_at,byte_size,content_sha256,created_at,completed_at) VALUES(?,? ,?,'project','zip','ready',?,?,?,7,?,?,?,?,?)`, exportUUID, projectID, taskUUID, string(encoded), snapshotHash, relative, now.Add(-time.Second), len(content), fmt.Sprintf("%x", digest[:]), completedAt, completedAt).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
		return harness.queue.StartProject(ctx, store)
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var exports, tasks int64
		err := harness.projects.WithCurrentStore(ctx, harness.project.UUID, func(store *project.Store) error {
			if err := store.DB().Table("comic_exports").Where("uuid = ?", exportUUID).Count(&exports).Error; err != nil {
				return err
			}
			return store.DB().Table("production_task_runs").Where("uuid = ?", taskUUID).Count(&tasks).Error
		})
		_, statErr := os.Stat(target)
		if err == nil && exports == 0 && tasks == 0 && errors.Is(statErr, os.ErrNotExist) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("RunOnStart did not clean export=%s target=%s", exportUUID, target)
}
