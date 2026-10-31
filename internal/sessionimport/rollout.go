// Package sessionimport stores a deliberately small, read-only projection of
// Codex Desktop rollouts. Rollouts are private, unstable files, not an API.
package sessionimport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxRecordBytes = 8 << 20
const maxTextBytes = 256 << 10

type record struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type metadata struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// readLine bounds allocations even for image-heavy or malformed records.
// Incomplete last lines are left for the next import, including valid JSON
// without a newline: the writer may still be appending to that record.
func readLine(ctx context.Context, r *bufio.Reader) ([]byte, int64, error) {
	var line []byte
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, size, err
		}
		part, err := r.ReadSlice('\n')
		size += int64(len(part))
		if size <= maxRecordBytes {
			line = append(line, part...)
		} else {
			line = nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, size, err
		}
	}
}

func openRollout(ctx context.Context, path, root string) (*os.File, metadata, string, error) {
	var meta metadata
	stat, err := os.Stat(path)
	if err != nil {
		return nil, meta, "rollout_unavailable", nil
	}
	if !stat.Mode().IsRegular() {
		return nil, meta, "rollout_not_regular", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, meta, "rollout_unavailable", nil
	}
	fail := func(warning string) (*os.File, metadata, string, error) {
		_ = f.Close()
		return nil, meta, warning, nil
	}
	stat, err = f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return fail("rollout_not_regular")
	}
	line, _, err := readLine(ctx, bufio.NewReader(f))
	if ctx.Err() != nil {
		_ = f.Close()
		return nil, meta, "", ctx.Err()
	}
	var first record
	if err != nil || json.Unmarshal(line, &first) != nil || first.Type != "session_meta" || json.Unmarshal(first.Payload, &meta) != nil {
		return fail("rollout_format_unknown")
	}
	if meta.SessionID == "" {
		meta.SessionID = meta.ID
	}
	parsed, err := uuid.Parse(meta.SessionID)
	if err != nil || parsed.Version() != 7 || (meta.ID != "" && meta.ID != meta.SessionID) {
		return fail("session_identity_invalid")
	}
	// Import only the current Lumi project's session, including real subdirs.
	// Never guess a project from a prompt or from a tool's arbitrary arguments.
	cwd, err := filepath.EvalSymlinks(meta.Cwd)
	if err != nil || !filepath.IsAbs(meta.Cwd) {
		return fail("session_project_mismatch")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fail("session_project_mismatch")
	}
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail("session_project_mismatch")
	}
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		_ = f.Close()
		return nil, meta, "", err
	}
	return f, meta, "", nil
}

var dataURI = regexp.MustCompile(`(?i)data:[^\s\)\]"<>]+`)
var markdownImage = regexp.MustCompile(`!\[[^\]]*\]\([^\n]*?\)`)

// projectRecord ignores duplicate event projections, reasoning, developer /
// system instructions, compaction replacement history, and all tool output.
func projectRecord(line []byte) (kind, text string, at time.Time, warning string) {
	var r record
	if len(line) == 0 {
		return "", "", at, "record_too_large"
	}
	if json.Unmarshal(line, &r) != nil {
		return "", "", at, "record_invalid_json"
	}
	if r.Type != "response_item" {
		switch r.Type {
		case "session_meta", "event_msg", "turn_context", "world_state", "token_usage_record", "compacted":
			return
		default:
			return "", "", at, "record_type_unknown"
		}
	}
	var p struct {
		Type      string `json:"type"`
		Role      string `json:"role"`
		Channel   string `json:"channel"`
		Phase     string `json:"phase"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(r.Payload, &p) != nil {
		return "", "", at, "record_payload_unknown"
	}
	at = r.Timestamp
	switch p.Type {
	case "message":
		if p.Role != "user" && p.Role != "assistant" {
			return
		}
		if p.Role == "assistant" && ((p.Channel != "" && p.Channel != "final" && p.Channel != "commentary") || (p.Phase != "" && p.Phase != "final_answer" && p.Phase != "commentary")) {
			return
		}
		var parts []string
		for _, c := range p.Content {
			if c.Type == "input_text" || c.Type == "output_text" {
				if p.Role == "user" && injectedContext(c.Text) {
					continue
				}
				parts = append(parts, c.Text)
			}
		}
		text = strings.TrimSpace(strings.Join(parts, "\n"))
		if text == "" {
			return
		}
		text = dataURI.ReplaceAllString(text, "[image omitted]")
		text = markdownImage.ReplaceAllString(text, "[image omitted]")
		if len(text) > maxTextBytes {
			text = text[:maxTextBytes]
			for !utf8.ValidString(text) {
				text = text[:len(text)-1]
			}
			text += "\n[truncated]"
			warning = "message_truncated"
		}
		return p.Role, text, at, warning
	case "function_call", "custom_tool_call":
		name := p.Name
		if p.Namespace != "" && p.Namespace != "functions" {
			name = p.Namespace + "__" + name
		}
		// These are the new cwd-bound Lumi plugin names. Do not inspect shell
		// commands / exec scripts or copy arbitrary arguments to infer actions.
		for _, prefix := range []string{"mcp__lumi__", "mcp__lumi_review__"} {
			if tool, ok := strings.CutPrefix(name, prefix); ok && knownLumiTools[tool] {
				return "lumi_tool", "Lumi · " + tool, at, ""
			}
		}
	case "reasoning", "function_call_output", "custom_tool_call_output", "web_search_call", "image_generation_call", "local_shell_call", "additional_tools", "compaction":
	default:
		warning = "response_type_unknown"
	}
	return
}

func injectedContext(text string) bool {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"# AGENTS.md instructions for ", "<environment_context>", "<codex_internal_context", "<external_codex_apps_open_page>", "<turn_aborted>", "<user_instructions>"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

var knownLumiTools = map[string]bool{
	"project_overview": true, "project_init": true, "project_setup": true,
	"story_profile": true, "chapters": true, "settings": true, "pages": true,
	"book_outline": true, "prompt_catalog": true, "render_prompt": true,
	"image_target": true, "import_image": true, "read_image": true, "export_book": true,
	"read_app_image": true, "read_app_connection": true, "watch_app_changes": true,
	"review_book": true, "review_page": true, "review_settings": true, "review_candidates": true,
}

func blankLine(line []byte) bool { return len(bytes.TrimSpace(line)) == 0 }
