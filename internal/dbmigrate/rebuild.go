package dbmigrate

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/golang-migrate/migrate/v4/database"
)

// SQLite cannot change a parent table's CHECK constraints in place. This
// opt-in rebuild runs atomically without cascading deletes into its children.
// Ordinary migrations retain the existing driver transaction behavior.
type rebuildDriver struct {
	database.Driver
	db *sql.DB
}

func (d *rebuildDriver) Run(reader io.Reader) error {
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(b, []byte("-- lumi: rebuild-without-foreign-key-actions\n")) {
		return d.Driver.Run(bytes.NewReader(b))
	}
	ctx := context.Background()
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var enabled int
	if err = conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, fmt.Sprintf("PRAGMA foreign_keys=%d", enabled))
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, string(b)); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	invalid := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if invalid {
		return fmt.Errorf("table rebuild violates foreign key integrity")
	}
	return tx.Commit()
}
