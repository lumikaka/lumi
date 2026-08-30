package agentcheckpoint

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/libtnb/sqlite"
	"gorm.io/gorm"
)

func checkpointTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE agent_tool_executions (
		uuid TEXT PRIMARY KEY,
		tool_name TEXT NOT NULL,
		state TEXT NOT NULL,
		result_json TEXT,
		updated_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return db
}

func insertCheckpointExecution(t *testing.T, db *gorm.DB, uuid, state string, result any) {
	t.Helper()
	if err := db.Exec(`INSERT INTO agent_tool_executions(uuid,tool_name,state,result_json,updated_at) VALUES(?,'request_api',?,?,?)`, uuid, state, result, time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointRoundTripAndRouteBinding(t *testing.T) {
	db := checkpointTestDB(t)
	ctx := context.Background()
	insertCheckpointExecution(t, db, "execution-1", "executing", nil)
	want := map[string]any{"deleted_count": 2, "blocked_items": []any{}}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return Write(ctx, tx, "execution-1", RouteChapterTrashEmpty, want, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	raw, found, err := Read(ctx, db, "execution-1", RouteChapterTrashEmpty)
	if err != nil || !found || !strings.Contains(string(raw), `"deleted_count":2`) {
		t.Fatalf("raw=%s found=%v err=%v", raw, found, err)
	}
	if _, _, err := Read(ctx, db, "execution-1", RoutePremiseAssetTrashEmpty); err == nil {
		t.Fatal("checkpoint was accepted for a different route")
	}
	if err := db.Exec(`UPDATE agent_tool_executions SET result_json=json_set(result_json,'$.data.deleted_count',9) WHERE uuid='execution-1'`).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(ctx, db, "execution-1", RouteChapterTrashEmpty); err == nil {
		t.Fatal("checkpoint with modified data was accepted")
	}
}

func TestCheckpointRejectsUnsafeExecutionStateAndForeignResult(t *testing.T) {
	db := checkpointTestDB(t)
	ctx := context.Background()
	insertCheckpointExecution(t, db, "intent-execution", "intent", nil)
	if err := Write(ctx, db, "intent-execution", RouteChapterTrashEmpty, map[string]any{}, time.Now()); err == nil {
		t.Fatal("intent execution accepted a side-effect checkpoint")
	}
	insertCheckpointExecution(t, db, "foreign-result", "executing", `{"success":true,"data":null}`)
	if _, _, err := Read(ctx, db, "foreign-result", RouteChapterPermanentDelete); err == nil {
		t.Fatal("normal Tool Result was mistaken for a checkpoint")
	}
}

func TestCheckpointRollsBackWithDomainTransaction(t *testing.T) {
	db := checkpointTestDB(t)
	ctx := context.Background()
	insertCheckpointExecution(t, db, "execution-rollback", "executing", nil)
	sentinel := errors.New("abort domain transaction")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := Write(ctx, tx, "execution-rollback", RouteChapterPermanentDelete, nil, time.Now()); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction error=%v", err)
	}
	if _, found, err := Read(ctx, db, "execution-rollback", RouteChapterPermanentDelete); err != nil || found {
		t.Fatalf("rolled-back checkpoint found=%v err=%v", found, err)
	}
}
