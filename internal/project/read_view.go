package project

import "gorm.io/gorm"

// WithReadView lends a detached store to a reader using the caller's SQLite
// transaction. It never opens another connection or transfers store ownership.
// Readers must not retain or close the view, or mutate project facts through it.
func (store *Store) WithReadView(db *gorm.DB, read func(*Store) error) error {
	store.metaMu.RLock()
	view := &Store{db: db, root: store.root, identity: store.identity, project: store.project, lock: store.lock}
	if store.pictureBook != nil {
		profile := clonePictureBookProfile(*store.pictureBook)
		view.pictureBook = &profile
	}
	store.metaMu.RUnlock()
	return read(view)
}
