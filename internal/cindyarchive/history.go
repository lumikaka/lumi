package cindyarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"lumi/internal/project"
)

type Item struct {
	UUID       string    `json:"uuid"`
	Kind       string    `json:"kind"`
	Text       string    `json:"text"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Coverage struct {
	Kind            string    `json:"kind"`
	CapturedAt      time.Time `json:"captured_at"`
	CutoffAt        time.Time `json:"cutoff_at"`
	ImportedAt      time.Time `json:"imported_at"`
	OmittedMessages int       `json:"omitted_messages"`
	Warnings        []string  `json:"warnings"`
}

type Page struct {
	Items            []Item   `json:"items"`
	Coverage         Coverage `json:"coverage"`
	CursorPagination struct {
		PerPage    int    `json:"per_page"`
		NextCursor string `json:"next_cursor"`
		PrevCursor string `json:"prev_cursor"`
		HasMore    bool   `json:"has_more"`
	} `json:"cursor_pagination"`
}

func (s *Service) History(ctx context.Context, projectUUID, threadUUID, before, after string, limit int) (Page, error) {
	page := Page{Items: []Item{}}
	if limit < 1 || limit > 100 || (before != "" && after != "") {
		return page, invalid("cursor_invalid")
	}
	for _, value := range []string{projectUUID, threadUUID, before, after} {
		if value == "" {
			continue
		}
		id, err := uuid.Parse(value)
		if err != nil || id.Version() != 7 {
			return page, invalid("cursor_invalid")
		}
	}
	page.CursorPagination.PerPage = limit
	err := s.Projects.WithStore(ctx, projectUUID, func(store *project.Store) error {
		db, err := store.DB().DB()
		if err != nil {
			return storageError(ctx, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return storageError(ctx, err)
		}
		defer tx.Rollback()
		var sessionID int64
		var warnings string
		err = tx.QueryRowContext(ctx, `SELECT s.id,s.captured_at,s.cutoff_at,s.imported_at,s.omitted_messages,s.warning_codes FROM cindy_sessions s JOIN chat_threads t ON t.id=s.thread_id JOIN projects p ON p.id=t.project_id WHERE t.uuid=? AND p.uuid=? AND t.thread_type='cindy_session'`, threadUUID, projectUUID).Scan(&sessionID, &page.Coverage.CapturedAt, &page.Coverage.CutoffAt, &page.Coverage.ImportedAt, &page.Coverage.OmittedMessages, &warnings)
		if errors.Is(err, sql.ErrNoRows) {
			return invalid("not_found")
		}
		if err != nil {
			return storageError(ctx, err)
		}
		page.Coverage.Kind = "manual_public_text"
		if err := json.Unmarshal([]byte(warnings), &page.Coverage.Warnings); err != nil {
			return invalid("storage_failed")
		}
		query := `SELECT uuid,kind,text,recorded_at FROM cindy_session_items WHERE session_id=?`
		args := []any{sessionID}
		cursor := after
		if before != "" {
			cursor = before
		}
		if cursor != "" {
			var sortAt, sourceID string
			if err := tx.QueryRowContext(ctx, `SELECT sort_at,source_message_id FROM cindy_session_items WHERE session_id=? AND uuid=?`, sessionID, cursor).Scan(&sortAt, &sourceID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return invalid("cursor_invalid")
				}
				return storageError(ctx, err)
			}
			comparison := ">"
			if before != "" {
				comparison = "<"
			}
			query += ` AND (sort_at` + comparison + `? OR (sort_at=? AND source_message_id` + comparison + `?))`
			args = append(args, sortAt, sortAt, sourceID)
		}
		order := "ASC"
		if before != "" {
			order = "DESC"
		}
		query += ` ORDER BY sort_at ` + order + `,source_message_id ` + order + ` LIMIT ?`
		args = append(args, limit+1)
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return storageError(ctx, err)
		}
		defer rows.Close()
		for rows.Next() {
			var item Item
			if err := rows.Scan(&item.UUID, &item.Kind, &item.Text, &item.RecordedAt); err != nil {
				return storageError(ctx, err)
			}
			page.Items = append(page.Items, item)
		}
		if err := rows.Err(); err != nil {
			return storageError(ctx, err)
		}
		page.CursorPagination.HasMore = len(page.Items) > limit
		if page.CursorPagination.HasMore {
			page.Items = page.Items[:limit]
		}
		if before != "" {
			slices.Reverse(page.Items)
		}
		if len(page.Items) > 0 {
			page.CursorPagination.PrevCursor = page.Items[0].UUID
			page.CursorPagination.NextCursor = page.Items[len(page.Items)-1].UUID
		}
		return nil
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}
