package taskexecution

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"lumi/internal/config"
	"lumi/internal/database"

	"gorm.io/gorm"
)

func TestExecutionFenceProtectsTransactionsAndStandaloneWrites(t *testing.T) {
	db, err := database.Open(config.SQLiteDSN(filepath.Join(t.TempDir(), "execution.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, query := range []string{
		`CREATE TABLE projects(uuid TEXT)`,
		`CREATE TABLE task_runs(uuid TEXT,river_job_id INTEGER,execution_uuid TEXT,progress INTEGER)`,
		`INSERT INTO projects VALUES('project')`,
		`INSERT INTO task_runs VALUES('task',9,'old-execution',0)`,
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	old := Fence{Owner: Owner{Table: "task_runs", UUID: "task", ProjectUUID: "project"}, Binding: Binding{JobID: 9, ExecutionUUID: "old-execution"}}
	ctx := WithFence(t.Context(), old)
	if err := Write(db.WithContext(ctx), func(tx *gorm.DB) *gorm.DB { return tx.Exec(`UPDATE task_runs SET progress=25`) }).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE task_runs SET execution_uuid='new-execution'`).Error; err != nil {
		t.Fatal(err)
	}
	if tx, err := Begin(context.WithoutCancel(ctx), sqlDB); !errors.Is(err, ErrSuperseded) {
		if tx != nil {
			tx.Rollback()
		}
		t.Fatalf("old SQL fence: %v", err)
	}
	for name, write := range map[string]func() error{
		"transaction": func() error {
			return Transaction(db.WithContext(ctx), func(tx *gorm.DB) error { return tx.Exec(`UPDATE task_runs SET progress=100`).Error })
		},
		"standalone": func() error {
			return Write(db.WithContext(ctx), func(tx *gorm.DB) *gorm.DB { return tx.Exec(`UPDATE task_runs SET progress=100`) }).Error
		},
		"returning": func() error {
			var progress int
			return ScanRow(db.WithContext(ctx), func(tx *gorm.DB) *sql.Row {
				return tx.Raw(`UPDATE task_runs SET progress=100 RETURNING progress`).Row()
			}, &progress)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := write(); !errors.Is(err, ErrSuperseded) {
				t.Fatalf("stale write=%v", err)
			}
		})
	}
	current := old
	current.ExecutionUUID = "new-execution"
	var progress int
	if err := ScanRow(db.WithContext(WithFence(t.Context(), current)), func(tx *gorm.DB) *sql.Row {
		return tx.Raw(`UPDATE task_runs SET progress=progress+1 RETURNING progress`).Row()
	}, &progress); err != nil {
		t.Fatal(err)
	}
	if progress != 26 {
		t.Fatalf("stale write changed persisted progress to %d", progress)
	}
	current.ProjectUUID = "other-project"
	if err := Transaction(db.WithContext(WithFence(t.Context(), current)), func(*gorm.DB) error { t.Fatal("wrong project entered transaction"); return nil }); !errors.Is(err, ErrSuperseded) {
		t.Fatalf("project check=%v", err)
	}
	current.ProjectUUID = "project"
	if err := db.Exec(`DROP TABLE task_runs`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := Begin(WithFence(t.Context(), current), sqlDB); err == nil || errors.Is(err, ErrSuperseded) {
		t.Fatalf("database read error was hidden: %v", err)
	}
}
