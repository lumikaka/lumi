// Package taskexecution fences durable writes by the execution that owns them.
// It never reads or changes River tables. Queue adapters validate River identity
// before installing a fence; domain transactions validate the Lumi binding.
package taskexecution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Binding is backend-only. A zero ExecutionUUID denotes a verified legacy job.
type Binding struct {
	JobID         int64
	ExecutionUUID string
}

func (b Binding) NullableUUID() any {
	if b.ExecutionUUID == "" {
		return nil
	}
	return b.ExecutionUUID
}

type Owner struct {
	Table       string
	UUID        string
	ProjectUUID string
}

type Fence struct {
	Owner
	Binding
}

type contextKey struct{}

var ErrSuperseded = errors.New("task execution no longer owns the business task")

func WithFence(ctx context.Context, fence Fence) context.Context {
	return context.WithValue(ctx, contextKey{}, fence)
}

func FromContext(ctx context.Context) (Fence, bool) {
	f, ok := ctx.Value(contextKey{}).(Fence)
	return f, ok
}

func ValidTable(table string) bool {
	switch table {
	case "task_runs", "production_task_runs", "asset_maintenance_runs", "chat_turns", "workflow_steps", "workflow_awaits":
		return true
	}
	return false
}

type Queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Check must be called inside the same transaction as the protected writes.
// Contexts without a fence are normal REST calls, maintenance, or recovery.
func Check(ctx context.Context, tx Queryer) error {
	f, ok := FromContext(ctx)
	if !ok {
		return nil
	}
	if !ValidTable(f.Table) || f.UUID == "" || f.ProjectUUID == "" || f.JobID <= 0 {
		return ErrSuperseded
	}
	var matches bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+f.Table+` WHERE uuid=? AND river_job_id=? AND COALESCE(execution_uuid,'')=? AND EXISTS(SELECT 1 FROM projects WHERE uuid=?))`, f.UUID, f.JobID, f.ExecutionUUID, f.ProjectUUID).Scan(&matches)
	if err != nil {
		return err
	}
	if !matches {
		return ErrSuperseded
	}
	return nil
}

func Begin(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err := Check(ctx, tx); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func CheckGORM(tx *gorm.DB) error {
	if _, ok := FromContext(tx.Statement.Context); !ok {
		return nil
	}
	pool, ok := tx.Statement.ConnPool.(Queryer)
	if !ok {
		return fmt.Errorf("execution fence requires a database/sql transaction")
	}
	return Check(tx.Statement.Context, pool)
}

func Transaction(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := CheckGORM(tx); err != nil {
			return err
		}
		return fn(tx)
	})
}

// Write also fences standalone GORM writes in a transaction. It preserves
// RowsAffected for callers using optimistic transitions.
func Write(db *gorm.DB, fn func(*gorm.DB) *gorm.DB) *gorm.DB {
	if _, ok := FromContext(db.Statement.Context); !ok {
		return fn(db)
	}
	var result *gorm.DB
	err := Transaction(db, func(tx *gorm.DB) error {
		result = fn(tx)
		return result.Error
	})
	if result == nil {
		result = db.Session(&gorm.Session{})
	}
	if err != nil {
		result.Error = err
	}
	return result
}

// ScanRow consumes UPDATE ... RETURNING before the fencing transaction commits.
func ScanRow(db *gorm.DB, fn func(*gorm.DB) *sql.Row, dest ...any) error {
	if _, ok := FromContext(db.Statement.Context); !ok {
		return fn(db).Scan(dest...)
	}
	return Transaction(db, func(tx *gorm.DB) error { return fn(tx).Scan(dest...) })
}
