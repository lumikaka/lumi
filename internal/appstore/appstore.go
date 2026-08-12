package appstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"lumi/internal/database"
	"lumi/internal/dbmigrate"

	"gorm.io/gorm"
)

var ErrRecentProjectNotFound = errors.New("recent project not found")

type RecentProject struct {
	ID           int64 `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID         string
	Name         string
	RootPath     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastOpenedAt time.Time
}

func (RecentProject) TableName() string { return "recent_projects" }

type Store struct {
	db      *gorm.DB
	dataDir string
}

func Open(dataDir, dsn string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "cache"), 0o700); err != nil {
		return nil, fmt.Errorf("create application data directory: %w", err)
	}
	runner, err := dbmigrate.OpenApp(dsn)
	if err != nil {
		return nil, fmt.Errorf("open app migrations: %w", err)
	}
	if err := runner.Up(); err != nil && !dbmigrate.IsNoChange(err) {
		_ = runner.Close()
		return nil, fmt.Errorf("migrate app database: %w", err)
	}
	if err := runner.Close(); err != nil {
		return nil, fmt.Errorf("close app migrations: %w", err)
	}
	db, err := database.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, dataDir: dataDir}, nil
}

func (store *Store) DB() *gorm.DB { return store.db }

func (store *Store) DataDir() string { return store.dataDir }

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	db, err := store.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

func (store *Store) RecentProjects(ctx context.Context) ([]RecentProject, error) {
	var projects []RecentProject
	if err := store.db.WithContext(ctx).Order("last_opened_at DESC, id DESC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("list recent projects: %w", err)
	}
	return projects, nil
}

func (store *Store) RecentProject(ctx context.Context, projectUUID string) (RecentProject, error) {
	var project RecentProject
	result := store.db.WithContext(ctx).Where("uuid = ?", projectUUID).Limit(1).Find(&project)
	if result.Error != nil {
		return RecentProject{}, fmt.Errorf("read recent project: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return RecentProject{}, ErrRecentProjectNotFound
	}
	return project, nil
}

func (store *Store) RecordProject(ctx context.Context, projectUUID, name, rootPath string, openedAt time.Time) error {
	return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing RecentProject
		result := tx.Where("uuid = ?", projectUUID).Limit(1).Find(&existing)
		if result.Error != nil {
			return fmt.Errorf("read recent project before update: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			project := RecentProject{
				UUID: projectUUID, Name: name, RootPath: rootPath,
				CreatedAt: openedAt, UpdatedAt: openedAt, LastOpenedAt: openedAt,
			}
			if err := tx.Create(&project).Error; err != nil {
				return fmt.Errorf("create recent project: %w", err)
			}
			return nil
		}
		if err := tx.Model(&existing).Updates(map[string]any{
			"name": name, "root_path": rootPath, "updated_at": openedAt, "last_opened_at": openedAt,
		}).Error; err != nil {
			return fmt.Errorf("update recent project: %w", err)
		}
		return nil
	})
}

func (store *Store) RelocateProject(ctx context.Context, projectUUID, rootPath string, updatedAt time.Time) error {
	result := store.db.WithContext(ctx).Model(&RecentProject{}).Where("uuid = ?", projectUUID).Updates(map[string]any{
		"root_path": rootPath, "updated_at": updatedAt, "last_opened_at": updatedAt,
	})
	if result.Error != nil {
		return fmt.Errorf("relocate recent project: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecentProjectNotFound
	}
	return nil
}

func (store *Store) UpdateProjectName(ctx context.Context, projectUUID, name string, updatedAt time.Time) error {
	result := store.db.WithContext(ctx).Model(&RecentProject{}).Where("uuid = ?", projectUUID).Updates(map[string]any{
		"name": name, "updated_at": updatedAt,
	})
	if result.Error != nil {
		return fmt.Errorf("update recent project name: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecentProjectNotFound
	}
	return nil
}

func (store *Store) ForgetProject(ctx context.Context, projectUUID string) error {
	result := store.db.WithContext(ctx).Where("uuid = ?", projectUUID).Delete(&RecentProject{})
	if result.Error != nil {
		return fmt.Errorf("forget recent project: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecentProjectNotFound
	}
	return nil
}
