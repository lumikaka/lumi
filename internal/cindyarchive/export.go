// Package cindyarchive validates explicitly supplied Cindy public-history exports
// and stores their text projection only on an explicit project import.
// It has no filesystem discovery, Host history access or automatic hooks.
package cindyarchive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	Format         = "lumi.cindy-public-history"
	MaxExportBytes = 16 << 20
	MaxTextBytes   = 256 << 10
	maxMessages    = 20000
	maxPages       = 1000
)

// Error contains only a fixed code, never source text, identifiers or paths.
type Error struct{ Code string }

func (e *Error) Error() string  { return e.Code }
func invalid(code string) error { return &Error{Code: "cindy_archive_" + code} }

type Message struct {
	SourceMessageID  string    `json:"source_message_id"`
	Role             string    `json:"role"`
	Text             string    `json:"text"`
	CreatedAt        time.Time `json:"created_at"`
	SourceTextSHA256 string    `json:"source_text_sha256"`
}

type Archive struct {
	SourceSessionID   string    `json:"source_session_id"`
	Title             string    `json:"title"`
	CapturedAt        time.Time `json:"captured_at"`
	CutoffAt          time.Time `json:"cutoff_at"`
	Messages          []Message `json:"messages"`
	OmittedMessages   int       `json:"omitted_messages"`
	DuplicateMessages int       `json:"duplicate_messages"`
	Warnings          []string  `json:"warnings"`
}

type snapshot struct {
	Format          string `json:"format"`
	Version         int    `json:"version"`
	Source          string `json:"source"`
	SourceSessionID string `json:"source_session_id"`
	CapturedAt      string `json:"captured_at"`
	CutoffAt        string `json:"cutoff_at"`
	Pages           []page `json:"pages"`
}
type queryRequest struct {
	SessionIDs []string `json:"session_ids"`
	Roles      []string `json:"roles"`
	To         string   `json:"to"`
	Limit      int      `json:"limit"`
	Order      string   `json:"order"`
	Cursor     string   `json:"cursor,omitempty"`
}
type queryReceipt struct {
	SessionIDs         []string `json:"session_ids"`
	To                 string   `json:"to"`
	Roles              []string `json:"roles"`
	RolesDefaulted     bool     `json:"roles_defaulted"`
	IncludeRewound     bool     `json:"include_rewound"`
	IncludeFullHistory bool     `json:"include_full_history"`
	AutoWindowDays     int      `json:"auto_window_days"`
	EffectiveFrom      *string  `json:"effective_from"`
	Limit              int      `json:"limit"`
	Order              string   `json:"order"`
}
type sourceSession struct {
	Title      string `json:"title"`
	AgentKind  string `json:"agentKind"`
	WorkingDir string `json:"workingDir"`
}
type sourceMessage struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	CreatedAt string          `json:"createdAt"`
}
type page struct {
	Request  queryRequest `json:"request"`
	Response struct {
		OK         bool                     `json:"ok"`
		Sessions   map[string]sourceSession `json:"sessions"`
		Messages   []sourceMessage          `json:"messages"`
		NextCursor *string                  `json:"nextCursor"`
		HasMore    *bool                    `json:"hasMore"`
		Query      queryReceipt             `json:"query"`
	} `json:"response"`
}

func identifier(s string) bool {
	if s == "" || len(s) > 256 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true // Host identifiers are opaque, not Lumi UUIDv7s.
}
func roles(values []string) bool {
	return len(values) == 2 && ((values[0] == "user" && values[1] == "assistant") || (values[0] == "assistant" && values[1] == "user"))
}
func oneSession(values []string, selected string) bool {
	return len(values) == 1 && values[0] == selected
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}

// Reject ambiguous JSON keys and excessive nesting before decoding any schema.
func uniqueJSON(ctx context.Context, d *json.Decoder, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > 32 {
		return invalid("nesting_limit")
	}
	token, err := d.Token()
	if err != nil {
		return invalid("invalid_json")
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return invalid("invalid_json")
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return invalid("duplicate_key")
			}
			keys[name] = true
			if err := uniqueJSON(ctx, d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(ctx, d, depth+1); err != nil {
				return err
			}
		}
	default:
		return invalid("invalid_json")
	}
	end, err := d.Token()
	if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return invalid("invalid_json")
	}
	return nil
}
func decode(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return invalid("format_unknown")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return invalid("invalid_json")
	}
	return nil
}

var dataURI = regexp.MustCompile(`(?i)data:[^\s\)\]"<>]+`)
var markdownImage = regexp.MustCompile(`!\[[^\]]*\]\([^\n]*?\)`)

func textContent(raw json.RawMessage) (string, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", false, invalid("content_unknown")
	}
	var text string
	attachments := false
	if raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil {
			return "", false, invalid("content_unknown")
		}
	} else if raw[0] == '{' {
		var content struct {
			Text               *string           `json:"text"`
			Images             []json.RawMessage `json:"images"`
			Files              []json.RawMessage `json:"files"`
			SlashCommandRanges []json.RawMessage `json:"slashCommandRanges"`
		}
		if decode(raw, &content) != nil || content.Text == nil {
			return "", false, invalid("content_unknown")
		}
		text, attachments = *content.Text, len(content.Images)+len(content.Files) > 0
	} else {
		return "", false, invalid("content_unknown")
	}
	if len(text) > MaxTextBytes {
		return "", false, invalid("message_too_large")
	}
	return text, attachments, nil
}
func injectedUserContext(text string) bool {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"# AGENTS.md instructions for ", "<environment_context>", "<codex_internal_context", "<user_instructions>", "[UI language]", "[UI_ACTION_TRIGGER]"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// Parse validates a Lumi wrapper around observed public get_chat_history replies.
// selectedSession is supplied independently by the caller's explicit selection;
// a file's source/path claims never grant project or Host access. Complete means
// the selected public text projection as of cutoff, not a full transcript.
func Parse(ctx context.Context, reader io.Reader, selectedSession string) (Archive, error) {
	result := Archive{Messages: []Message{}, Warnings: []string{}}
	if !identifier(selectedSession) {
		return result, invalid("selection_required")
	}
	raw, err := io.ReadAll(io.LimitReader(contextReader{ctx, reader}, MaxExportBytes+1))
	if err != nil {
		if cancelled := ctx.Err(); cancelled != nil {
			return result, cancelled
		}
		return result, invalid("read_failed")
	}
	if len(raw) > MaxExportBytes {
		return result, invalid("too_large")
	}
	if !utf8.Valid(raw) {
		return result, invalid("invalid_utf8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := uniqueJSON(ctx, d, 0); err != nil {
		return result, err
	}
	if _, err := d.Token(); err != io.EOF {
		return result, invalid("invalid_json")
	}
	var input snapshot
	if err := decode(raw, &input); err != nil {
		return result, err
	}
	if input.Format != Format || input.Version != 1 || input.Source != "cindy_helper.get_chat_history" {
		return result, invalid("format_unknown")
	}
	if input.SourceSessionID != selectedSession {
		return result, invalid("source_mismatch")
	}
	result.SourceSessionID = selectedSession
	result.CapturedAt, err = time.Parse(time.RFC3339Nano, input.CapturedAt)
	if err != nil {
		return result, invalid("timestamp_invalid")
	}
	result.CutoffAt, err = time.Parse(time.RFC3339Nano, input.CutoffAt)
	if err != nil || result.CapturedAt.Before(result.CutoffAt) {
		return result, invalid("timestamp_invalid")
	}
	if len(input.Pages) == 0 || len(input.Pages) > maxPages {
		return result, invalid("pages_invalid")
	}
	seen := map[string]Message{}
	warnings := map[string]bool{}
	cursors := map[string]bool{}
	var previous *Message
	var metadata *sourceSession
	var cursor string
	total := 0
	for index, p := range input.Pages {
		if err := ctx.Err(); err != nil {
			return Archive{}, err
		}
		req, resp := p.Request, p.Response
		if !oneSession(req.SessionIDs, selectedSession) || !roles(req.Roles) || req.To != input.CutoffAt || req.Order != "desc" || req.Limit < 1 || req.Limit > 1000 {
			return Archive{}, invalid("scope_invalid")
		}
		q := resp.Query
		if !resp.OK || !oneSession(q.SessionIDs, selectedSession) || q.To != input.CutoffAt || !roles(q.Roles) || q.RolesDefaulted || q.IncludeRewound || q.IncludeFullHistory || q.AutoWindowDays != 0 || q.EffectiveFrom != nil || q.Limit != req.Limit || q.Order != req.Order {
			return Archive{}, invalid("scope_invalid")
		}
		if req.Cursor != cursor || resp.HasMore == nil || (index < len(input.Pages)-1) != *resp.HasMore {
			return Archive{}, invalid("pages_incomplete")
		}
		if len(resp.Messages) > req.Limit || (index > 0 && len(resp.Messages) == 0) {
			return Archive{}, invalid("pages_invalid")
		}
		for id, item := range resp.Sessions {
			if id != selectedSession || len(item.Title) > 1024 {
				return Archive{}, invalid("source_mismatch")
			}
			if metadata != nil && *metadata != item {
				return Archive{}, invalid("source_metadata_changed")
			}
			copy := item
			metadata = &copy
			result.Title = item.Title
		}
		if len(resp.Messages) > 0 && metadata == nil {
			return Archive{}, invalid("source_mismatch")
		}
		for _, m := range resp.Messages {
			total++
			if total > maxMessages {
				return Archive{}, invalid("messages_limit")
			}
			if !identifier(m.ID) || m.SessionID != selectedSession || (m.Role != "user" && m.Role != "assistant") {
				return Archive{}, invalid("message_scope_invalid")
			}
			at, err := time.Parse(time.RFC3339Nano, m.CreatedAt)
			if err != nil || !at.Before(result.CutoffAt) {
				return Archive{}, invalid("timestamp_invalid")
			}
			text, attachments, err := textContent(m.Content)
			if err != nil {
				return Archive{}, err
			}
			hash := sha256.Sum256([]byte(m.Role + "\x00" + at.UTC().Format(time.RFC3339Nano) + "\x00" + text))
			item := Message{SourceMessageID: m.ID, Role: m.Role, Text: text, CreatedAt: at, SourceTextSHA256: hex.EncodeToString(hash[:])}
			if saved, exists := seen[m.ID]; exists {
				if saved.SourceTextSHA256 != item.SourceTextSHA256 {
					return Archive{}, invalid("message_conflict")
				}
				result.DuplicateMessages++
			} else {
				seen[m.ID] = item
				if attachments {
					warnings["attachments_omitted"] = true
				}
				if m.Role == "user" && injectedUserContext(text) {
					result.OmittedMessages++
					warnings["injected_context_omitted"] = true
				} else if strings.TrimSpace(text) == "" {
					result.OmittedMessages++
					warnings["empty_text_omitted"] = true
				} else {
					item.Text = markdownImage.ReplaceAllString(dataURI.ReplaceAllString(text, "[image omitted]"), "[image omitted]")
					if item.Text != text {
						warnings["inline_media_omitted"] = true
					}
					result.Messages = append(result.Messages, item)
				}
			}
			if previous != nil && (at.After(previous.CreatedAt) || (at.Equal(previous.CreatedAt) && m.ID > previous.SourceMessageID)) {
				return Archive{}, invalid("order_invalid")
			}
			previous = &item
		}
		if *resp.HasMore {
			if resp.NextCursor == nil || *resp.NextCursor == "" || cursors[*resp.NextCursor] {
				return Archive{}, invalid("cursor_invalid")
			}
			cursor = *resp.NextCursor
			cursors[cursor] = true
		}
	}
	sort.Slice(result.Messages, func(i, j int) bool {
		a, b := result.Messages[i], result.Messages[j]
		if a.CreatedAt.Equal(b.CreatedAt) {
			return a.SourceMessageID < b.SourceMessageID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	for warning := range warnings {
		result.Warnings = append(result.Warnings, warning)
	}
	sort.Strings(result.Warnings)
	return result, nil
}

// Summary is safe to use in acceptance output: it deliberately excludes bodies,
// source identifiers, paths and the user-supplied title.
func (a Archive) Summary() string {
	return fmt.Sprintf("Cindy public text snapshot: messages=%d omitted=%d duplicates=%d warnings=%d", len(a.Messages), a.OmittedMessages, a.DuplicateMessages, len(a.Warnings))
}
