package project

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lumi/internal/config"
	"lumi/internal/database"
	"lumi/internal/dbmigrate"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const SupportedFormatVersion int64 = 1

type Project struct {
	ID                 int64 `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID               string
	Name               string
	Description        string
	GenerationLanguage string
	Revision           int64
	FormatVersion      int64
	SchemaVersion      int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Project) TableName() string { return "projects" }

type Actor struct {
	ID        int64 `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID      string
	Name      string
	Kind      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Actor) TableName() string { return "actors" }

type Header struct {
	UUID          string
	Name          string
	FormatVersion int64
	SchemaVersion int64
}

type Store struct {
	db          *gorm.DB
	root        string
	metaMu      sync.RWMutex
	project     Project
	pictureBook PictureBookProfile
	lock        *projectLock
	fileMu      sync.Mutex
	closing     bool
}

func (store *Store) DB() *gorm.DB { return store.db }

func (store *Store) Root() string { return store.root }

func (store *Store) ProjectUUID() string {
	store.metaMu.RLock()
	defer store.metaMu.RUnlock()
	return store.project.UUID
}

func (store *Store) ProjectName() string {
	store.metaMu.RLock()
	defer store.metaMu.RUnlock()
	return store.project.Name
}

func (store *Store) PictureBookProfile() PictureBookProfile {
	store.metaMu.RLock()
	defer store.metaMu.RUnlock()
	return clonePictureBookProfile(store.pictureBook)
}

func (store *Store) RefreshProject(ctx context.Context) error {
	store.metaMu.RLock()
	projectID := store.project.ID
	store.metaMu.RUnlock()
	var current Project
	if err := store.db.WithContext(ctx).Where("id = ?", projectID).First(&current).Error; err != nil {
		return err
	}
	store.metaMu.Lock()
	defer store.metaMu.Unlock()
	store.project = current
	return nil
}

func (store *Store) ResolvePath(relative string) (string, error) {
	return ResolveRelativePath(store.root, relative)
}

// WithFileCommit serializes the filesystem/database commit boundary and lets
// Close reject new commits before it checkpoints the project database.
func (store *Store) WithFileCommit(callback func() error) error {
	store.fileMu.Lock()
	defer store.fileMu.Unlock()
	if store.closing || store.db == nil {
		return errors.New("project store is closing")
	}
	return callback()
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.fileMu.Lock()
	defer store.fileMu.Unlock()
	store.closing = true
	var checkpointErr, closeErr error
	if store.db != nil {
		checkpointErr = store.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
		if sqlDB, err := store.db.DB(); err != nil {
			closeErr = err
		} else {
			closeErr = sqlDB.Close()
		}
	}
	lockErr := store.lock.Close()
	store.db = nil
	store.lock = nil
	return errors.Join(checkpointErr, closeErr, lockErr)
}

func projectDSN(root string) string {
	return config.SQLiteDSN(filepath.Join(root, "project.sqlite"))
}

func projectReadOnlyDSN(root string) string {
	return config.SQLiteFileURI(filepath.Join(root, "project.sqlite")) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(1000)"
}

func readHeader(ctx context.Context, root string) (Header, error) {
	db, err := sql.Open("sqlite", projectReadOnlyDSN(root))
	if err != nil {
		return Header{}, projectError(CodeInvalidProject, "无法打开项目数据库", "project.sqlite 不是可读取的 SQLite 数据库。", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT uuid, name, format_version, schema_version FROM projects LIMIT 2")
	if err != nil {
		return Header{}, projectError(CodeInvalidProject, "项目数据库缺少身份信息", "projects 表不存在或结构无效。", err)
	}
	defer rows.Close()
	var headers []Header
	for rows.Next() {
		var header Header
		if err := rows.Scan(&header.UUID, &header.Name, &header.FormatVersion, &header.SchemaVersion); err != nil {
			return Header{}, projectError(CodeInvalidProject, "项目身份信息无效", "无法读取项目 UUID 与版本。", err)
		}
		headers = append(headers, header)
	}
	if err := rows.Err(); err != nil {
		return Header{}, projectError(CodeInvalidProject, "项目身份信息无效", "读取项目身份时发生错误。", err)
	}
	if len(headers) != 1 || !isUUIDv7(headers[0].UUID) {
		return Header{}, projectError(CodeInvalidProject, "项目身份信息无效", "项目库必须且只能包含一个有效 UUIDv7 项目记录。", nil)
	}
	if headers[0].FormatVersion < 1 || headers[0].SchemaVersion < 1 {
		return Header{}, projectError(CodeInvalidProject, "项目版本信息无效", "format_version 与 schema_version 必须是正整数。", nil)
	}
	if headers[0].FormatVersion > SupportedFormatVersion {
		return Header{}, projectError(CodeFormatTooNew, "项目格式比当前 Lumi 更新", "请升级 Lumi 后再打开；当前版本不会写入此项目。", nil)
	}
	return headers[0], nil
}

func isUUIDv7(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == 7
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return value.String(), nil
}

type migrationRunner interface {
	LatestVersion() uint
	NeedsUp() (bool, error)
	Up() error
	Close() error
}

type migrationOpener func(string) (migrationRunner, error)

func migrateProject(ctx context.Context, root string, header *Header, now time.Time) (uint, error) {
	return migrateProjectWith(ctx, root, header, now, func(dsn string) (migrationRunner, error) {
		return dbmigrate.OpenProject(dsn)
	})
}

func migrateProjectWith(ctx context.Context, root string, header *Header, now time.Time, open migrationOpener) (uint, error) {
	dsn := projectDSN(root)
	runner, err := open(dsn)
	if err != nil {
		return 0, projectError(CodeMigrationFailed, "无法准备项目迁移", "项目数据库 migration 无法启动。", err)
	}
	latest := runner.LatestVersion()
	if header != nil && header.SchemaVersion > int64(latest) {
		_ = runner.Close()
		return latest, projectError(CodeFormatTooNew, "项目 schema 比当前 Lumi 更新", "请升级 Lumi 后再打开；当前版本不会写入此项目。", nil)
	}
	needsUp, err := runner.NeedsUp()
	if err != nil {
		_ = runner.Close()
		return latest, projectError(CodeMigrationFailed, "无法检查项目 migration", "项目 schema 版本无效或处于未完成状态。", err)
	}
	var backupPath string
	if needsUp && header != nil {
		backupUUID, uuidErr := newUUIDv7()
		if uuidErr != nil {
			_ = runner.Close()
			return latest, projectError(CodeMigrationFailed, "无法命名项目备份", "migration 尚未执行。", uuidErr)
		}
		backupPath = filepath.Join(root, ".lumi", "backups", fmt.Sprintf("project-before-%s-%s.sqlite", now.UTC().Format("20060102T150405.000000000Z"), backupUUID))
		if err := database.OnlineBackup(ctx, dsn, backupPath); err != nil {
			_ = runner.Close()
			return latest, projectError(CodeMigrationFailed, "无法创建 migration 备份", "项目未被修改；请检查 .lumi/backups 的写权限与可用空间。", err)
		}
	}
	migrateErr := runner.Up()
	if dbmigrate.IsNoChange(migrateErr) {
		migrateErr = nil
	}
	closeErr := runner.Close()
	if migrateErr == nil && closeErr == nil {
		return latest, nil
	}
	migrationFailure := errors.Join(migrateErr, closeErr)
	if backupPath != "" {
		if restoreErr := database.OnlineRestore(ctx, dsn, backupPath); restoreErr != nil {
			return latest, projectError(CodeMigrationFailed, "项目 migration 失败且自动恢复未完成", "请保留 .lumi/backups 中的备份并停止写入项目。", errors.Join(migrationFailure, restoreErr))
		}
	}
	details := "新项目数据库未完成初始化。"
	if backupPath != "" {
		details = "项目已从 migration 前的一致性备份恢复。"
	}
	return latest, projectError(CodeMigrationFailed, "项目 migration 失败", details, migrationFailure)
}

func initializeStore(ctx context.Context, root, projectUUID, projectName, generationLanguage, actorUUID string, pictureBook PictureBookProfile, overallStyle string, now time.Time, lock *projectLock) (*Store, error) {
	latest, err := migrateProject(ctx, root, nil, now)
	if err != nil {
		return nil, err
	}
	premiseUUID, err := newUUIDv7()
	if err != nil {
		return nil, err
	}
	styleVersionUUID, err := newUUIDv7()
	if err != nil {
		return nil, err
	}
	db, err := database.Open(projectDSN(root))
	if err != nil {
		return nil, projectError(CodeInvalidProject, "无法打开新项目数据库", "项目目录已创建，但数据库连接失败。", err)
	}
	project := Project{
		UUID: projectUUID, Name: projectName, GenerationLanguage: generationLanguage, FormatVersion: SupportedFormatVersion,
		SchemaVersion: int64(latest), Revision: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	actor := Actor{UUID: actorUUID, Name: "本地创作者", Kind: "local_user", CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	styleHash := fmt.Sprintf("%x", sha256.Sum256([]byte(overallStyle)))
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		if err := tx.Create(&actor).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO premise_profiles(uuid, project_id, default_style, revision, created_at, updated_at)
			VALUES(?, ?, ?, 0, ?, ?)
		`, premiseUUID, project.ID, overallStyle, now.UTC(), now.UTC()).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO project_prompt_versions(
				uuid, project_id, actor_id, prompt_group, prompt_key, version_no,
				prompt, prompt_hash, source_type, created_at
			) VALUES(?, ?, ?, 'premise_style', 'project_overall_style', 1, ?, ?, 'project_created', ?)
		`, styleVersionUUID, project.ID, actor.ID, overallStyle, styleHash, now.UTC()).Error; err != nil {
			return err
		}
		return tx.Create(&pictureBookProfileRecord{
			ProjectID: project.ID, Format: pictureBook.Format,
			AspectRatioMode: pictureBook.AspectRatio.Mode, AspectWidth: pictureBook.AspectRatio.Width, AspectHeight: pictureBook.AspectRatio.Height,
			LargeImageMinimalText: pictureBook.LargeImageMinimalText, InteractionMode: pictureBook.InteractionMode, ComicLayout: pictureBook.ComicLayout,
			CreatedAt: now.UTC(),
		}).Error
	}); err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, projectError(CodeInvalidProject, "无法初始化项目身份", "项目数据库未能创建默认项目和本地创作者。", err)
	}
	return &Store{db: db, root: root, project: project, pictureBook: clonePictureBookProfile(pictureBook), lock: lock}, nil
}

func openStore(ctx context.Context, root string, header Header, now time.Time, lock *projectLock) (*Store, error) {
	latest, err := migrateProject(ctx, root, &header, now)
	if err != nil {
		return nil, err
	}
	db, err := database.Open(projectDSN(root))
	if err != nil {
		return nil, projectError(CodeInvalidProject, "无法打开项目数据库", "请检查 project.sqlite 是否可读写且未损坏。", err)
	}
	var projects []Project
	if err := db.WithContext(ctx).Limit(2).Find(&projects).Error; err != nil || len(projects) != 1 || projects[0].UUID != header.UUID {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, projectError(CodeInvalidProject, "项目身份验证失败", "项目库必须包含且只包含同一个项目记录。", err)
	}
	var actorCount int64
	if err := db.WithContext(ctx).Model(&Actor{}).Where("kind = ?", "local_user").Count(&actorCount).Error; err != nil || actorCount < 1 {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, projectError(CodeInvalidProject, "项目缺少本地创作者", "项目数据库必须包含默认 local_user actor。", err)
	}
	if projects[0].SchemaVersion != int64(latest) {
		if err := db.WithContext(ctx).Model(&projects[0]).Updates(map[string]any{"schema_version": int64(latest), "updated_at": now.UTC()}).Error; err != nil {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				_ = sqlDB.Close()
			}
			return nil, projectError(CodeMigrationFailed, "无法记录项目 schema 版本", "migration 已执行，但项目版本标记更新失败。", err)
		}
		projects[0].SchemaVersion = int64(latest)
	}
	pictureBook, err := loadPictureBookProfile(ctx, db, projects[0].ID)
	if err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, projectError(CodeInvalidProject, "项目绘本配置无效", "项目必须包含一份有效且不可变的绘本形式配置。", err)
	}
	return &Store{db: db, root: root, project: projects[0], pictureBook: pictureBook, lock: lock}, nil
}

func removeNewProject(root string) {
	if root == "" || filepath.Dir(root) == root {
		return
	}
	_ = os.RemoveAll(root)
}
