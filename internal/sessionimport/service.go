package sessionimport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"lumi/internal/project"
	"lumi/internal/realtime"
	"lumi/internal/taskexecution"
)

type Service struct {
	Projects *project.Manager
	Events   *realtime.Hub
}

type Result struct {
	ThreadUUID string   `json:"thread_uuid,omitempty"`
	Imported   int      `json:"imported"`
	Skipped    bool     `json:"skipped"`
	Warnings   []string `json:"warnings"`
}

func (r *Result) warn(code string) {
	for _, w := range r.Warnings {
		if w == code {
			return
		}
	}
	r.Warnings = append(r.Warnings, code)
}

type session struct {
	ID          int64
	ThreadID    int64
	ThreadUUID  string
	SessionUUID string
	ReadOffset  int64
	PrefixHash  string
}

// Import is explicitly invoked by the manual CLI command. There is no watcher,
// hook registration, stdin hook mode, or implicit import on project open.
func (s *Service) Import(ctx context.Context, root, path string) (Result, error) {
	result := Result{Warnings: []string{}}
	if strings.TrimSpace(path) == "" {
		result.Skipped = true
		result.warn("rollout_path_empty")
		return result, nil
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return result, errors.New("root and rollout must be absolute paths")
	}
	f, meta, warning, err := openRollout(ctx, path, root)
	if err != nil {
		return result, err
	}
	if warning != "" {
		result.Skipped = true
		result.warn(warning)
		return result, nil
	}
	defer f.Close()
	opened, err := s.Projects.OpenSelected(ctx, project.ExplicitExistingDirectory(root))
	if err != nil {
		return result, err
	}
	err = s.Projects.WithStore(ctx, opened.UUID, func(store *project.Store) error {
		return taskexecution.Transaction(store.DB().WithContext(ctx), func(tx *gorm.DB) error {
			var saved session
			err := tx.Table("codex_sessions s").Select("s.*, t.uuid AS thread_uuid").Joins("JOIN chat_threads t ON t.id=s.thread_id").Where("s.session_uuid=?", meta.SessionID).Take(&saved).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			result.ThreadUUID = saved.ThreadUUID
			stat, err := f.Stat()
			if err != nil {
				return err
			}
			if saved.ReadOffset > stat.Size() {
				result.Skipped = true
				result.warn("rollout_truncated")
				return nil
			}
			snapshotHash, err := hashPrefix(ctx, f, saved.ReadOffset)
			if err != nil {
				return err
			}
			if saved.ID != 0 {
				if hex.EncodeToString(snapshotHash.Sum(nil)) != saved.PrefixHash {
					result.Skipped = true
					result.warn("rollout_rewritten")
					return nil
				}
			}
			// Snapshot the length so a live writer cannot keep this command alive.
			r := bufio.NewReader(io.TeeReader(io.NewSectionReader(f, saved.ReadOffset, stat.Size()-saved.ReadOffset), snapshotHash))
			offset := saved.ReadOffset
			now := time.Now().UTC()
			for {
				line, size, err := readLine(ctx, r)
				if errors.Is(err, io.EOF) {
					if size > 0 {
						result.warn("rollout_incomplete_tail")
					}
					break
				}
				if err != nil {
					return err
				}
				start := offset
				offset += size
				if len(line) > 0 && blankLine(line) {
					continue
				}
				// A rotated file must not concatenate another session into this one.
				var header record
				if json.Unmarshal(line, &header) == nil && header.Type == "session_meta" {
					var current metadata
					if err := json.Unmarshal(header.Payload, &current); err != nil {
						return err
					}
					if current.SessionID == "" {
						current.SessionID = current.ID
					}
					if current.SessionID != meta.SessionID || current.Cwd != meta.Cwd || (current.ID != "" && current.ID != meta.SessionID) {
						return errors.New("rollout session metadata changed during import")
					}
				}
				kind, text, at, warning := projectRecord(line)
				if warning != "" {
					result.warn(warning)
				}
				if kind == "" {
					continue
				}
				if saved.ID == 0 {
					threadUUID, err := uuid.NewV7()
					if err != nil {
						return err
					}
					title := "Codex · " + meta.SessionID
					if kind == "user" {
						runes := []rune(strings.Join(strings.Fields(text), " "))
						if len(runes) > 60 {
							runes = runes[:60]
						}
						title = "Codex · " + string(runes)
					}
					if err := tx.Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,thread_type,provider_uuid,model,model_source,created_at,updated_at) SELECT ?,id,?,'idle','codex_session','','','',?,? FROM projects WHERE uuid=?`, threadUUID.String(), title, now, now, opened.UUID).Error; err != nil {
						return err
					}
					if err := tx.Raw("SELECT id FROM chat_threads WHERE uuid=?", threadUUID.String()).Scan(&saved.ThreadID).Error; err != nil {
						return err
					}
					if err := tx.Exec(`INSERT INTO codex_sessions(thread_id,session_uuid,rollout_path,read_offset,prefix_hash,imported_at) VALUES(?,?,?,0,'',?)`, saved.ThreadID, meta.SessionID, path, now).Error; err != nil {
						return err
					}
					if err := tx.Raw("SELECT id FROM codex_sessions WHERE session_uuid=?", meta.SessionID).Scan(&saved.ID).Error; err != nil {
						return err
					}
					result.ThreadUUID = threadUUID.String()
				}
				itemUUID, err := uuid.NewV7()
				if err != nil {
					return err
				}
				if at.IsZero() {
					at = now
				}
				if err := tx.Exec(`INSERT INTO codex_session_items(uuid,session_id,source_offset,kind,text,recorded_at) VALUES(?,?,?,?,?,?)`, itemUUID.String(), saved.ID, start, kind, text, at.UTC()).Error; err != nil {
					return err
				}
				result.Imported++
			}
			if saved.ID == 0 {
				result.Skipped = true
				result.warn("no_supported_messages")
				return nil
			}
			// Verify the exact snapshot that was parsed, including ignored bytes.
			// Appends beyond the snapshot are fine; rewrites during import roll back.
			verified, err := hashPrefix(ctx, f, stat.Size())
			if err != nil {
				return err
			}
			if !bytes.Equal(verified.Sum(nil), snapshotHash.Sum(nil)) {
				return errors.New("rollout changed during import; retry with a stable file")
			}
			consumed := verified
			if offset != stat.Size() {
				consumed, err = hashPrefix(ctx, f, offset)
				if err != nil {
					return err
				}
			}
			if err := tx.Exec("UPDATE codex_sessions SET read_offset=?, prefix_hash=?, rollout_path=?, imported_at=? WHERE id=?", offset, hex.EncodeToString(consumed.Sum(nil)), path, now, saved.ID).Error; err != nil {
				return err
			}
			if result.Imported > 0 {
				return tx.Exec("UPDATE chat_threads SET updated_at=? WHERE id=?", now, saved.ThreadID).Error
			}
			return nil
		})
	})
	if err != nil {
		return Result{Warnings: result.Warnings}, err
	}
	if result.Imported > 0 && s.Events != nil {
		s.Events.Broadcast(realtime.ProjectTopic(opened.UUID), "codex_session:changed", map[string]any{"project_uuid": opened.UUID, "thread_uuid": result.ThreadUUID})
	}
	return result, nil
}

func hashPrefix(ctx context.Context, f *os.File, size int64) (hash.Hash, error) {
	h := sha256.New()
	r := io.NewSectionReader(f, 0, size)
	buf := make([]byte, 64<<10)
	var read int64
	for read < size {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(buf)
		_, _ = h.Write(buf[:n])
		read += int64(n)
		if err != nil {
			return nil, fmt.Errorf("rollout changed while reading: %w", err)
		}
	}
	return h, nil
}
