package appstore

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Only newly created system drafts opt in. Imported and historical projects
// retain the migration's false default.
func (store *Store) EnableDraftDirectoryNaming(ctx context.Context, projectUUID string) error {
	return store.db.WithContext(ctx).Model(&RecentProject{}).Where("uuid = ?", projectUUID).
		Update("auto_name_directory", true).Error
}

func (store *Store) QueueDraftDirectoryName(ctx context.Context, projectUUID, name string) error {
	return store.db.WithContext(ctx).Model(&RecentProject{}).Where("uuid = ? AND auto_name_directory = ?", projectUUID, true).
		Updates(map[string]any{"auto_name_directory": false, "pending_directory_name": name}).Error
}

func (store *Store) SetPendingDirectoryName(ctx context.Context, projectUUID, name string) error {
	return store.db.WithContext(ctx).Model(&RecentProject{}).Where("uuid = ?", projectUUID).
		Updates(map[string]any{"auto_name_directory": false, "pending_directory_name": name}).Error
}

func (store *Store) PendingDirectoryProjects(ctx context.Context) ([]RecentProject, error) {
	var items []RecentProject
	err := store.db.WithContext(ctx).Where("pending_directory_name <> ''").Find(&items).Error
	return items, err
}

func (store *Store) CompleteDirectoryRename(ctx context.Context, projectUUID, root string, now time.Time) error {
	return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&RecentProject{}).Where("uuid = ?", projectUUID).Updates(map[string]any{
			"root_path": root, "pending_directory_name": "", "auto_name_directory": false, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&ProjectCreationSession{}).Where("planned_project_uuid = ?", projectUUID).
			Update("planned_root_path", root).Error
	})
}
