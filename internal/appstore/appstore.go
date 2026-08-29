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
var ErrProjectCreationSessionNotFound = errors.New("project creation session not found")

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

type ProjectCreationSession struct {
	ID                 int64 `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID               string
	IdempotencyKey     string
	InputText          string
	Status             string
	PlannedProjectUUID string
	PlannedRootPath    string
	RecentProjectID    *int64
	ThreadUUID         string
	TurnUUID           string
	ErrorCode          string
	ErrorMessage       string
	AttemptCount       int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
	FailedAt           *time.Time
}

func (ProjectCreationSession) TableName() string { return "project_creation_sessions" }

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

func (store *Store) CreateOrGetProjectCreationSession(ctx context.Context, session ProjectCreationSession) (ProjectCreationSession, bool, error) {
	var result ProjectCreationSession
	created := false
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lookup := tx.Where("idempotency_key = ?", session.IdempotencyKey).Limit(1).Find(&result)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 1 {
			return nil
		}
		if err := tx.Create(&session).Error; err != nil {
			// A racing retry may have inserted the unique idempotency key.
			if retryErr := tx.Where("idempotency_key = ?", session.IdempotencyKey).First(&result).Error; retryErr == nil {
				return nil
			}
			return err
		}
		result, created = session, true
		return nil
	})
	if err != nil {
		return ProjectCreationSession{}, false, fmt.Errorf("create project creation session: %w", err)
	}
	return result, created, nil
}

func (store *Store) ProjectCreationSession(ctx context.Context, sessionUUID string) (ProjectCreationSession, error) {
	var session ProjectCreationSession
	result := store.db.WithContext(ctx).Where("uuid = ?", sessionUUID).Limit(1).Find(&session)
	if result.Error != nil {
		return ProjectCreationSession{}, fmt.Errorf("read project creation session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ProjectCreationSession{}, ErrProjectCreationSessionNotFound
	}
	return session, nil
}

func (store *Store) ResumableProjectCreationSessions(ctx context.Context) ([]ProjectCreationSession, error) {
	var sessions []ProjectCreationSession
	err := store.db.WithContext(ctx).
		Where("status IN ?", []string{"pending", "creating_project", "creating_conversation", "failed"}).
		Order("updated_at, id").Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("list resumable project creation sessions: %w", err)
	}
	return sessions, nil
}

func (store *Store) UpdateProjectCreationSession(ctx context.Context, sessionUUID string, updates map[string]any) (ProjectCreationSession, error) {
	result := store.db.WithContext(ctx).Model(&ProjectCreationSession{}).Where("uuid = ?", sessionUUID).Updates(updates)
	if result.Error != nil {
		return ProjectCreationSession{}, fmt.Errorf("update project creation session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		if _, err := store.ProjectCreationSession(ctx, sessionUUID); err != nil {
			return ProjectCreationSession{}, err
		}
	}
	return store.ProjectCreationSession(ctx, sessionUUID)
}

func (store *Store) RecentProjectID(ctx context.Context, projectUUID string) (int64, error) {
	var id int64
	if err := store.db.WithContext(ctx).Table("recent_projects").Select("id").Where("uuid = ?", projectUUID).Scan(&id).Error; err != nil {
		return 0, fmt.Errorf("read recent project id: %w", err)
	}
	if id == 0 {
		return 0, ErrRecentProjectNotFound
	}
	return id, nil
}
