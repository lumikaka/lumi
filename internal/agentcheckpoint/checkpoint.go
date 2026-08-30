package agentcheckpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	RouteChapterPermanentDelete      = "chapter.permanent_delete"
	RouteChapterTrashEmpty           = "chapter.trash.empty"
	RoutePremiseAssetPermanentDelete = "premise_asset.permanent_delete"
	RoutePremiseAssetTrashEmpty      = "premise_asset.trash.empty"

	checkpointKind    = "agent_tool_side_effect_checkpoint"
	checkpointVersion = 1
)

type snapshot struct {
	Kind          string          `json:"kind"`
	SchemaVersion int             `json:"schema_version"`
	RouteID       string          `json:"route_id"`
	Data          json.RawMessage `json:"data"`
	DataSHA256    string          `json:"data_sha256"`
}

// Read returns a side-effect checkpoint written by the same executing
// request_api call. A non-checkpoint result_json is rejected instead of being
// mistaken for a safe replay result.
func Read(ctx context.Context, tx *gorm.DB, executionUUID, routeID string) (json.RawMessage, bool, error) {
	if tx == nil || strings.TrimSpace(executionUUID) == "" || strings.TrimSpace(routeID) == "" {
		return nil, false, nil
	}
	var raw sql.NullString
	err := tx.WithContext(ctx).Raw(`SELECT result_json
		FROM agent_tool_executions
		WHERE uuid=? AND tool_name='request_api' AND state='executing'`, executionUUID).Row().Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("agent tool execution is not executing")
	}
	if err != nil {
		return nil, false, err
	}
	if !raw.Valid {
		return nil, false, nil
	}
	value, err := decode(raw.String, routeID)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

// Write stores data in the Agent execution row in the caller's transaction.
// This makes the domain side effect and its replay value one SQLite commit.
func Write(ctx context.Context, tx *gorm.DB, executionUUID, routeID string, data any, now time.Time) error {
	if tx == nil || strings.TrimSpace(executionUUID) == "" || strings.TrimSpace(routeID) == "" {
		return fmt.Errorf("agent tool checkpoint identity is incomplete")
	}
	encodedData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode agent tool checkpoint data: %w", err)
	}
	encoded, err := json.Marshal(snapshot{
		Kind:          checkpointKind,
		SchemaVersion: checkpointVersion,
		RouteID:       routeID,
		Data:          encodedData,
		DataSHA256:    checkpointDataHash(encodedData),
	})
	if err != nil {
		return fmt.Errorf("encode agent tool checkpoint: %w", err)
	}
	result := tx.WithContext(ctx).Exec(`UPDATE agent_tool_executions
		SET result_json=?,updated_at=?
		WHERE uuid=? AND tool_name='request_api' AND state='executing' AND result_json IS NULL`, string(encoded), now.UTC(), executionUUID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	existing, found, readErr := Read(ctx, tx, executionUUID, routeID)
	if readErr != nil {
		return readErr
	}
	if found && bytes.Equal(existing, encodedData) {
		return nil
	}
	return fmt.Errorf("agent tool checkpoint could not be persisted")
}

func decode(raw, routeID string) (json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	var value snapshot
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode agent tool checkpoint: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, fmt.Errorf("decode agent tool checkpoint: trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode agent tool checkpoint trailer: %w", err)
	}
	if value.Kind != checkpointKind || value.SchemaVersion != checkpointVersion || value.RouteID != routeID || len(value.Data) == 0 || !json.Valid(value.Data) || value.DataSHA256 != checkpointDataHash(value.Data) {
		return nil, fmt.Errorf("agent tool checkpoint does not match route %s", routeID)
	}
	return append(json.RawMessage(nil), value.Data...), nil
}

func checkpointDataHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
