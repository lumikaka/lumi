package dbmigrate

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	embeddedmigrations "lumi/db/migrations"
	"lumi/internal/database"

	"github.com/golang-migrate/migrate/v4"
	migratedatabase "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

type Runner struct {
	migrator *migrate.Migrate
	empty    bool
	latest   uint
}

// Open is retained as the app-store migration entry point.
func Open(dsn string) (*Runner, error) {
	return OpenApp(dsn)
}

func OpenApp(dsn string) (*Runner, error) {
	return OpenWithFS(dsn, embeddedmigrations.Files, "app")
}

func OpenProject(dsn string) (*Runner, error) {
	return OpenWithFS(dsn, embeddedmigrations.Files, "project")
}

func OpenWithFS(dsn string, migrationFS fs.FS, path string) (*Runner, error) {
	if err := database.EnsureParentDirectory(dsn); err != nil {
		return nil, err
	}

	sourceDriver, err := iofs.New(migrationFS, path)
	if err != nil {
		return nil, fmt.Errorf("open migration source: %w", err)
	}
	first, firstErr := sourceDriver.First()
	empty := errors.Is(firstErr, fs.ErrNotExist)
	if firstErr != nil && !empty {
		_ = sourceDriver.Close()
		return nil, fmt.Errorf("read first migration: %w", firstErr)
	}
	var latest uint
	if !empty {
		latest = first
		for {
			next, nextErr := sourceDriver.Next(latest)
			if errors.Is(nextErr, fs.ErrNotExist) {
				break
			}
			if nextErr != nil {
				_ = sourceDriver.Close()
				return nil, fmt.Errorf("read next migration after %d: %w", latest, nextErr)
			}
			latest = next
		}
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = sourceDriver.Close()
		return nil, fmt.Errorf("open migration database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	databaseDriver, err := migratedatabase.WithInstance(sqlDB, &migratedatabase.Config{DatabaseName: dsn})
	if err != nil {
		_ = sourceDriver.Close()
		_ = sqlDB.Close()
		return nil, fmt.Errorf("initialize migration database: %w", err)
	}
	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", databaseDriver)
	if err != nil {
		_ = sourceDriver.Close()
		_ = databaseDriver.Close()
		return nil, fmt.Errorf("initialize migrator: %w", err)
	}
	return &Runner{migrator: migrator, empty: empty, latest: latest}, nil
}

func (runner *Runner) LatestVersion() uint {
	return runner.latest
}

func (runner *Runner) NeedsUp() (bool, error) {
	if runner.empty {
		return false, nil
	}
	version, dirty, applied, err := runner.Version()
	if err != nil {
		return false, err
	}
	if dirty {
		return false, fmt.Errorf("migration version %d is dirty", version)
	}
	if applied && version > runner.latest {
		return false, fmt.Errorf("database schema version %d is newer than supported version %d", version, runner.latest)
	}
	return !applied || version < runner.latest, nil
}

func (runner *Runner) Up() error {
	if runner.empty {
		return migrate.ErrNoChange
	}
	return runner.migrator.Up()
}

func (runner *Runner) Down(steps int) error {
	if steps <= 0 {
		return fmt.Errorf("migration steps must be positive")
	}
	if runner.empty {
		return migrate.ErrNoChange
	}
	return runner.migrator.Steps(-steps)
}

func (runner *Runner) Version() (version uint, dirty bool, applied bool, err error) {
	version, dirty, err = runner.migrator.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, err
	}
	return version, dirty, true, nil
}

func (runner *Runner) Close() error {
	sourceErr, databaseErr := runner.migrator.Close()
	return errors.Join(sourceErr, databaseErr)
}

func IsNoChange(err error) bool {
	return errors.Is(err, migrate.ErrNoChange)
}
