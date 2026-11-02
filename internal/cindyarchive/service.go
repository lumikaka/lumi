package cindyarchive

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/taskexecution"
)

type Service struct {
	Projects *project.Manager
	Events   *realtime.Hub
}

type Result struct {
	ThreadUUID        string   `json:"thread_uuid"`
	ImportUUID        string   `json:"import_uuid"`
	Imported          int      `json:"imported"`
	Replayed          bool     `json:"replayed"`
	OmittedMessages   int      `json:"omitted_messages"`
	DuplicateMessages int      `json:"duplicate_messages"`
	Warnings          []string `json:"warnings"`
}

const sortLayout = "2006-01-02T15:04:05.000000000Z"

// ImportAt is the CLI adapter. Validation precedes opening the explicitly
// selected project; no path or workingDir in the export selects a target.
func (s *Service) ImportAt(ctx context.Context, root, selected string, reader io.Reader) (Result, error) {
	h := sha256.New()
	a, err := Parse(ctx, io.TeeReader(reader, h), selected)
	if err != nil {
		return Result{}, err
	}
	opened, err := s.Projects.OpenSelected(ctx, project.ExplicitExistingDirectory(root))
	if err != nil {
		return Result{}, err
	}
	return s.save(ctx, opened.UUID, a, hex.EncodeToString(h.Sum(nil)))
}

func (s *Service) Import(ctx context.Context, projectUUID, selected string, reader io.Reader) (Result, error) {
	h := sha256.New()
	a, err := Parse(ctx, io.TeeReader(reader, h), selected)
	if err != nil {
		return Result{}, err
	}
	return s.save(ctx, projectUUID, a, hex.EncodeToString(h.Sum(nil)))
}

// database/sql avoids GORM's interpolated SQL logger on failed inserts. Never
// return driver errors: they may contain source text or schema diagnostics.
func storageError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	return invalid("storage_failed")
}

func (s *Service) save(ctx context.Context, projectUUID string, a Archive, exportHash string) (Result, error) {
	result := Result{OmittedMessages: a.OmittedMessages, DuplicateMessages: a.DuplicateMessages, Warnings: a.Warnings}
	err := s.Projects.WithStore(ctx, projectUUID, func(store *project.Store) error {
		db, err := store.DB().DB()
		if err != nil {
			return storageError(ctx, err)
		}
		tx, err := taskexecution.Begin(ctx, db)
		if err != nil {
			return storageError(ctx, err)
		}
		defer tx.Rollback()
		var sessionID int64
		var oldCutoff time.Time
		err = tx.QueryRowContext(ctx, `SELECT s.id,t.uuid,s.cutoff_at FROM cindy_sessions s JOIN chat_threads t ON t.id=s.thread_id JOIN projects p ON p.id=t.project_id WHERE s.source_session_id=? AND p.uuid=? AND t.thread_type='cindy_session'`, a.SourceSessionID, projectUUID).Scan(&sessionID, &result.ThreadUUID, &oldCutoff)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return storageError(ctx, err)
		}
		now := time.Now().UTC()
		if sessionID != 0 {
			err = tx.QueryRowContext(ctx, `SELECT uuid FROM cindy_session_imports WHERE session_id=? AND export_sha256=?`, sessionID, exportHash).Scan(&result.ImportUUID)
			if err == nil {
				result.Replayed = true
				return nil // No writes, timestamps or notifications on exact replay.
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return storageError(ctx, err)
			}
			incoming := make(map[string]Message, len(a.Messages))
			for _, m := range a.Messages {
				incoming[m.SourceMessageID] = m
			}
			rows, err := tx.QueryContext(ctx, `SELECT source_message_id,source_text_sha256,sort_at FROM cindy_session_items WHERE session_id=?`, sessionID)
			if err != nil {
				return storageError(ctx, err)
			}
			for rows.Next() {
				var id, hash, sortAt string
				if err := rows.Scan(&id, &hash, &sortAt); err != nil {
					rows.Close()
					return storageError(ctx, err)
				}
				m, exists := incoming[id]
				if exists && m.SourceTextSHA256 != hash {
					rows.Close()
					return invalid("source_changed")
				}
				if !exists && sortAt <= a.CutoffAt.UTC().Format(sortLayout) {
					rows.Close()
					return invalid("source_missing")
				}
				delete(incoming, id)
			}
			readErr := rows.Err()
			rows.Close()
			if readErr != nil {
				return storageError(ctx, readErr)
			}
			// Keep parser order for inserts, including older backfilled messages.
			remaining := make([]Message, 0, len(incoming))
			for _, m := range a.Messages {
				if _, ok := incoming[m.SourceMessageID]; ok {
					remaining = append(remaining, m)
				}
			}
			a.Messages = remaining
		} else {
			threadUUID, err := uuid.NewV7()
			if err != nil {
				return invalid("storage_failed")
			}
			result.ThreadUUID = threadUUID.String()
			title := []rune(strings.TrimSpace(a.Title))
			if len(title) == 0 {
				title = []rune("Cindy 创作记录")
			}
			if len(title) > 160 {
				title = title[:160]
			}
			row, err := tx.ExecContext(ctx, `INSERT INTO chat_threads(uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,created_at,updated_at) SELECT ?,id,?,'idle','cindy_session','','','',?,? FROM projects WHERE uuid=?`, result.ThreadUUID, string(title), now, now, projectUUID)
			if err != nil {
				return storageError(ctx, err)
			}
			threadID, err := row.LastInsertId()
			if err != nil {
				return storageError(ctx, err)
			}
			row, err = tx.ExecContext(ctx, `INSERT INTO cindy_sessions(thread_id,source_session_id,captured_at,cutoff_at,imported_at,warning_codes,omitted_messages) VALUES(?,?,?,?,?,'[]',0)`, threadID, a.SourceSessionID, a.CapturedAt.UTC(), a.CutoffAt.UTC(), now)
			if err != nil {
				return storageError(ctx, err)
			}
			sessionID, err = row.LastInsertId()
			if err != nil {
				return storageError(ctx, err)
			}
		}
		for _, m := range a.Messages {
			id, err := uuid.NewV7()
			if err != nil {
				return invalid("storage_failed")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO cindy_session_items(uuid,session_id,source_message_id,source_text_sha256,kind,text,recorded_at,sort_at) VALUES(?,?,?,?,?,?,?,?)`, id.String(), sessionID, m.SourceMessageID, m.SourceTextSHA256, m.Role, m.Text, m.CreatedAt.UTC(), m.CreatedAt.UTC().Format(sortLayout)); err != nil {
				return storageError(ctx, err)
			}
			result.Imported++
		}
		id, err := uuid.NewV7()
		if err != nil {
			return invalid("storage_failed")
		}
		result.ImportUUID = id.String()
		warnings, _ := json.Marshal(a.Warnings)
		if _, err := tx.ExecContext(ctx, `INSERT INTO cindy_session_imports(uuid,session_id,export_sha256,captured_at,cutoff_at,imported_at,imported_count,omitted_messages,duplicate_messages,warning_codes) VALUES(?,?,?,?,?,?,?,?,?,?)`, result.ImportUUID, sessionID, exportHash, a.CapturedAt.UTC(), a.CutoffAt.UTC(), now, result.Imported, a.OmittedMessages, a.DuplicateMessages, string(warnings)); err != nil {
			return storageError(ctx, err)
		}
		// Older files may add missing earlier text but never move coverage back.
		if !a.CutoffAt.Before(oldCutoff) {
			if _, err := tx.ExecContext(ctx, `UPDATE cindy_sessions SET captured_at=?,cutoff_at=?,imported_at=?,warning_codes=?,omitted_messages=? WHERE id=?`, a.CapturedAt.UTC(), a.CutoffAt.UTC(), now, string(warnings), a.OmittedMessages, sessionID); err != nil {
				return storageError(ctx, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET updated_at=? WHERE uuid=?`, now, result.ThreadUUID); err != nil {
			return storageError(ctx, err)
		}
		return storageError(ctx, tx.Commit())
	})
	if err != nil {
		return Result{}, err // A rollback must not advertise partial imports.
	}
	if !result.Replayed && s.Events != nil {
		s.Events.Broadcast(realtime.ProjectTopic(projectUUID), "cindy_session:changed", map[string]any{"project_uuid": projectUUID, "thread_uuid": result.ThreadUUID})
	}
	return result, nil
}
