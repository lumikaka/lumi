package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	moderncsqlite "modernc.org/sqlite"
)

type onlineBackuper interface {
	NewBackup(string) (*moderncsqlite.Backup, error)
}

type onlineRestorer interface {
	NewRestore(string) (*moderncsqlite.Backup, error)
}

// OnlineBackup uses SQLite's backup API so WAL-backed databases are copied
// consistently while the source database is open.
func OnlineBackup(ctx context.Context, sourceDSN, destinationPath string) (finalErr error) {
	if _, err := os.Stat(destinationPath); err == nil {
		return fmt.Errorf("backup destination already exists: %s", destinationPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination: %w", err)
	}
	defer func() {
		if finalErr != nil {
			_ = os.Remove(destinationPath)
		}
	}()
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	finalErr = withRawConnection(ctx, sourceDSN, func(driverConnection any) error {
		connection, ok := driverConnection.(onlineBackuper)
		if !ok {
			return errors.New("SQLite driver does not support the online backup API")
		}
		backup, err := connection.NewBackup(destinationPath)
		if err != nil {
			return fmt.Errorf("start SQLite backup: %w", err)
		}
		if _, err := backup.Step(-1); err != nil {
			return errors.Join(fmt.Errorf("copy SQLite backup: %w", err), backup.Finish())
		}
		if err := backup.Finish(); err != nil {
			return fmt.Errorf("finish SQLite backup: %w", err)
		}
		return nil
	})
	return finalErr
}

// OnlineRestore replaces the destination database contents from a backup via
// SQLite, avoiding raw file replacement of an opened database.
func OnlineRestore(ctx context.Context, destinationDSN, sourcePath string) error {
	return withRawConnection(ctx, destinationDSN, func(driverConnection any) error {
		connection, ok := driverConnection.(onlineRestorer)
		if !ok {
			return errors.New("SQLite driver does not support the online restore API")
		}
		restore, err := connection.NewRestore(sourcePath)
		if err != nil {
			return fmt.Errorf("start SQLite restore: %w", err)
		}
		if _, err := restore.Step(-1); err != nil {
			return errors.Join(fmt.Errorf("restore SQLite backup: %w", err), restore.Finish())
		}
		if err := restore.Finish(); err != nil {
			return fmt.Errorf("finish SQLite restore: %w", err)
		}
		return nil
	})
}

func withRawConnection(ctx context.Context, dsn string, action func(any) error) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open SQLite connection: %w", err)
	}
	defer db.Close()
	connection, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite connection: %w", err)
	}
	defer connection.Close()
	return connection.Raw(action)
}
