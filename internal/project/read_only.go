package project

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

	"gorm.io/gorm"
)

type readOnlyKey struct{}

// WithReadOnly marks an already authorized read. It never opens a project or
// grants access; the caller must still acquire the ordinary Manager lease.
func WithReadOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, readOnlyKey{}, true)
}

func IsReadOnly(ctx context.Context) bool {
	value, _ := ctx.Value(readOnlyKey{}).(bool)
	return value
}

// ReadOnly pins one existing SQLite connection and makes accidental lazy writes
// fail. It does not create another database handle, migrate, or acquire a lock.
// A detached Store avoids mutating the live store's cached metadata or mutexes.
func (store *Store) ReadOnly(ctx context.Context, callback func(*Store) error) error {
	return store.db.WithContext(ctx).Connection(func(db *gorm.DB) (err error) {
		if err := db.Exec("PRAGMA query_only = ON").Error; err != nil {
			return err
		}
		defer func() {
			resetDB := db.Session(&gorm.Session{NewDB: true, Context: context.WithoutCancel(ctx)})
			resetDB.Error = nil // A rejected callback write must not skip reset.
			reset := resetDB.Exec("PRAGMA query_only = OFF").Error
			if reset != nil {
				// A cancelled driver must never return a query-only connection to
				// the writable pool. Discard it if the reset cannot be confirmed.
				if conn, ok := db.Statement.ConnPool.(*sql.Conn); ok {
					_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				}
			}
			err = errors.Join(err, reset)
		}()
		store.metaMu.RLock()
		view := &Store{db: db, root: store.root, identity: store.identity, project: store.project, lock: store.lock}
		if store.pictureBook != nil {
			profile := clonePictureBookProfile(*store.pictureBook)
			view.pictureBook = &profile
		}
		store.metaMu.RUnlock()
		return callback(view)
	})
}
