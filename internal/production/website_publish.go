package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/gorm"
	"lumi/internal/durablefs"
	"lumi/internal/project"
	"lumi/internal/taskexecution"
)

type websiteSwap struct {
	GenerationUUID string `json:"generation_uuid"`
	PreviousUUID   string `json:"previous_uuid"`
}

func (s *Service) publishWebsite(ctx context.Context, taskUUID string, generation websiteGeneration, stage string, previous, manifest websiteManifest, processed, reused int) error {
	return s.store.WithFileCommit(func() (result error) {
		output, err := s.store.ResolvePath("website")
		if err != nil {
			return err
		}
		backup, err := s.store.ResolvePath(".lumi/website-previous")
		if err != nil {
			return err
		}
		journal, err := s.store.ResolvePath(".lumi/website-swap.json")
		if err != nil {
			return err
		}
		old, err := s.currentWebsite(ctx)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Preserve untracked files. Refuse collisions and symlinks instead of
		// deleting user content or accidentally publishing an outside path.
		if err = websitePreserveExtras(output, stage, previous, manifest); err != nil {
			return err
		}
		if err = websiteSyncTree(stage); err != nil {
			return err
		}
		if err = durablefs.SyncDirectory(filepath.Dir(stage)); err != nil {
			return err
		}
		db, err := s.store.DB().DB()
		if err != nil {
			return err
		}
		tx, err := taskexecution.Begin(ctx, db)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var allowed bool
		if err = tx.QueryRowContext(ctx, `SELECT status='running' AND cancel_requested_at IS NULL FROM production_task_runs WHERE id=? AND uuid=?`, generation.TaskID, taskUUID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return context.Canceled
		}
		committed := false
		swap := websiteSwap{GenerationUUID: generation.UUID, PreviousUUID: old.UUID}
		encoded, _ := json.Marshal(swap)
		if err = websiteWriteJournal(journal, encoded); err != nil {
			return err
		}
		// Once the journal exists, every exit (including failed fsync/COMMIT)
		// uses SQLite as the source of truth to finish or undo the switch.
		defer func() {
			_ = tx.Rollback()
			recoveryErr := s.recoverWebsiteSwap(context.WithoutCancel(ctx))
			// Cleanup can be retried on open; a committed publication stays successful.
			if !committed {
				result = errors.Join(result, recoveryErr)
			}
		}()
		if err = durablefs.SyncDirectory(filepath.Dir(journal)); err != nil {
			return err
		}
		if _, err = os.Lstat(output); err == nil {
			if old.ID == 0 {
				return domainError(CodeConflict, "website 目录已存在", "请先移走未受管目录。", nil)
			}
			if err = os.Rename(output, backup); err != nil {
				return err
			}
			if err = durablefs.SyncDirectory(s.store.Root()); err != nil {
				return err
			}
			if err = durablefs.SyncDirectory(filepath.Dir(backup)); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err = os.Rename(stage, output); err != nil {
			return err
		}
		if err = durablefs.SyncDirectory(s.store.Root()); err != nil {
			return err
		}
		if err = durablefs.SyncDirectory(filepath.Dir(stage)); err != nil {
			return err
		}
		encoded, err = json.Marshal(manifest)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if _, err = tx.ExecContext(ctx, `UPDATE website_generations SET manifest_json=?,processed_images=?,reused_images=?,completed_at=? WHERE id=?`, string(encoded), processed, reused, now, generation.ID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO project_websites(project_id,generation_id) VALUES(?,?) ON CONFLICT(project_id) DO UPDATE SET generation_id=excluded.generation_id`, generation.ProjectID, generation.ID); err != nil {
			return err
		}
		// Completion and publication commit together, so cancellation can only
		// win before this transaction, never after replacing the good website.
		if _, err = tx.ExecContext(ctx, `UPDATE production_task_runs SET status='completed',progress=100,completed_at=?,updated_at=?,error_code='',error_message='' WHERE id=?`, now, now, generation.TaskID); err != nil {
			return err
		}
		eventUUID, err := newUUIDv7()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"project_uuid": s.store.ProjectUUID(), "task_uuid": taskUUID, "resource_uuid": s.store.ProjectUUID(), "status": "completed", "progress": 100})
		if _, err = tx.ExecContext(ctx, `INSERT INTO production_task_events(uuid,production_task_run_id,sequence,event_type,payload,created_at) SELECT ?,?,COALESCE(MAX(sequence),0)+1,'task_completed',?,? FROM production_task_events WHERE production_task_run_id=?`, eventUUID, generation.TaskID, string(payload), now, generation.TaskID); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		committed = true
		s.emit("production_task:completed", map[string]any{"task_uuid": taskUUID})
		s.emit("website:changed", map[string]any{"generation_uuid": generation.UUID, "task_uuid": taskUUID})
		return nil
	})
}

func websitePreserveExtras(output, stage string, previous, next websiteManifest) error {
	_, err := os.Lstat(output)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return filepath.WalkDir(output, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == output {
			return nil
		}
		relative, err := filepath.Rel(output, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if d.Type()&os.ModeSymlink != 0 {
			return domainError(CodeConflict, "网站目录包含符号链接", "请将自定义文件移至独立副本后重试。", nil)
		}
		if _, managed := previous.Files[relative]; managed {
			return nil
		}
		target, err := project.ResolveRelativePath(stage, relative)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if _, collision := next.Files[relative]; collision {
			return domainError(CodeConflict, "网站文件与自定义内容冲突", "请将自定义文件移至独立副本后重试。", nil)
		}
		b, err := websiteRead(output, relative)
		if err != nil {
			return err
		}
		return websiteWrite(target, b)
	})
}

// RecoverWebsite runs before queue workers start, and before each retry. No
// timer or frontend polling participates in filesystem recovery.
func (s *Service) RecoverWebsite(ctx context.Context) error {
	return s.store.WithFileCommit(func() error {
		if err := s.recoverWebsiteSwap(ctx); err != nil {
			return err
		}
		partial, err := s.store.ResolvePath(".lumi/website-swap.json.part")
		if err != nil {
			return err
		}
		if err = os.Remove(partial); err != nil && !os.IsNotExist(err) {
			return err
		}
		var generations []websiteGeneration
		if err := s.store.DB().WithContext(ctx).Select("uuid").Find(&generations).Error; err != nil {
			return err
		}
		for _, generation := range generations {
			stage, err := s.store.ResolvePath(".lumi/website-staging/" + generation.UUID)
			if err != nil {
				return err
			}
			if err = os.RemoveAll(stage); err != nil {
				return err
			}
		}
		return nil
	})
}

func websiteMarker(root string) string {
	b, err := websiteRead(root, "data/build.json")
	if err != nil {
		return ""
	}
	var marker struct {
		GenerationUUID string `json:"generation_uuid"`
	}
	if json.Unmarshal(b, &marker) != nil {
		return ""
	}
	return marker.GenerationUUID
}

func (s *Service) recoverWebsiteSwap(ctx context.Context) error {
	journal, err := s.store.ResolvePath(".lumi/website-swap.json")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(journal)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var swap websiteSwap
	if json.Unmarshal(b, &swap) != nil || !isUUIDv7(swap.GenerationUUID) || (swap.PreviousUUID != "" && !isUUIDv7(swap.PreviousUUID)) {
		return fmt.Errorf("invalid website recovery journal")
	}
	output, err := s.store.ResolvePath("website")
	if err != nil {
		return err
	}
	backup, err := s.store.ResolvePath(".lumi/website-previous")
	if err != nil {
		return err
	}
	current, err := s.currentWebsite(ctx)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if current.UUID == swap.GenerationUUID {
		if websiteMarker(output) != swap.GenerationUUID {
			return fmt.Errorf("published website marker mismatch")
		}
		if err = os.RemoveAll(backup); err != nil {
			return err
		}
	} else {
		discard, err := s.store.ResolvePath(".lumi/website-discard-" + swap.GenerationUUID)
		if err != nil {
			return err
		}
		if websiteMarker(output) == swap.GenerationUUID {
			// Rename before deleting: interruption during RemoveAll must never
			// leave a half-removed directory blocking restoration of backup.
			if err = os.Rename(output, discard); err != nil {
				return err
			}
			if err = durablefs.SyncDirectory(s.store.Root()); err != nil {
				return err
			}
			if err = durablefs.SyncDirectory(filepath.Dir(discard)); err != nil {
				return err
			}
		}
		if _, err = os.Lstat(backup); err == nil {
			if _, existsErr := os.Lstat(output); !os.IsNotExist(existsErr) {
				return fmt.Errorf("website recovery destination already exists")
			}
			if err = os.Rename(backup, output); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err = os.RemoveAll(discard); err != nil {
			return err
		}
	}
	if err = durablefs.SyncDirectory(s.store.Root()); err != nil {
		return err
	}
	if err = durablefs.SyncDirectory(filepath.Dir(backup)); err != nil {
		return err
	}
	if err = os.Remove(journal); err != nil {
		return err
	}
	return durablefs.SyncDirectory(filepath.Dir(journal))
}

// A torn journal write must not strand a previously good website on next open.
func websiteWriteJournal(path string, data []byte) error {
	partial := path + ".part"
	if err := websiteWrite(partial, data); err != nil {
		return err
	}
	if err := os.Rename(partial, path); err != nil {
		return err
	}
	return durablefs.SyncDirectory(filepath.Dir(path))
}
