package sessionimport

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"lumi/internal/project"
)

var ErrNotFound = errors.New("imported session not found")
var ErrCursor = errors.New("invalid session history cursor")

type Item struct {
	UUID       string    `json:"uuid"`
	Kind       string    `json:"kind"`
	Text       string    `json:"text"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Page struct {
	Items            []Item     `json:"items"`
	CursorPagination Pagination `json:"cursor_pagination"`
}

type Pagination struct {
	PerPage    int    `json:"per_page"`
	NextCursor string `json:"next_cursor"`
	PrevCursor string `json:"prev_cursor"`
	HasMore    bool   `json:"has_more"`
}

func (s *Service) History(ctx context.Context, projectUUID, threadUUID, before, after string, limit int) (Page, error) {
	page := Page{Items: []Item{}}
	if limit < 1 || limit > 100 || (before != "" && after != "") {
		return page, ErrCursor
	}
	for _, value := range []string{projectUUID, threadUUID, before, after} {
		if value == "" {
			continue
		}
		id, err := uuid.Parse(value)
		if err != nil || id.Version() != 7 {
			return page, ErrCursor
		}
	}
	page.CursorPagination.PerPage = limit
	err := s.Projects.WithStore(ctx, projectUUID, func(store *project.Store) error {
		var sessionID int64
		err := store.DB().WithContext(ctx).Table("codex_sessions s").Select("s.id").Joins("JOIN chat_threads t ON t.id=s.thread_id JOIN projects p ON p.id=t.project_id").Where("t.uuid=? AND p.uuid=? AND t.thread_type='codex_session'", threadUUID, projectUUID).Take(&sessionID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		query := store.DB().WithContext(ctx).Table("codex_session_items").Select("uuid,kind,text,recorded_at").Where("session_id=?", sessionID)
		cursor := after
		if before != "" {
			cursor = before
		}
		if cursor != "" {
			var offset int64
			if err := store.DB().WithContext(ctx).Table("codex_session_items").Select("source_offset").Where("session_id=? AND uuid=?", sessionID, cursor).Take(&offset).Error; err != nil {
				return ErrCursor
			}
			if before != "" {
				query = query.Where("source_offset<?", offset)
			} else {
				query = query.Where("source_offset>?", offset)
			}
		}
		order := "source_offset ASC"
		if before != "" {
			order = "source_offset DESC"
		}
		if err := query.Order(order).Limit(limit + 1).Scan(&page.Items).Error; err != nil {
			return err
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
	return page, err
}
