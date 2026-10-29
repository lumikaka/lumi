package project

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func seedInitializationWorkflow(t *testing.T, store *Store, status string) string {
	t.Helper()
	workflowUUID := setupTestUUID(t)
	now := time.Now().UTC()
	err := store.DB().Exec(`INSERT INTO workflows(uuid,project_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at)
		VALUES(?,?,'yolo_project_initialization','初始化',?,1,'{}',?,?,'test',?,?)`, workflowUUID, store.project.ID, status, workflowUUID, setupTestUUID(t), now, now).Error
	if err != nil {
		t.Fatal(err)
	}
	return workflowUUID
}

func TestInitializationLockRequiresExplicitHandoffAndSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "初始化接管", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreForTest(t, manager, created.UUID)
	state, err := store.InitializationState(ctx)
	if err != nil || state.Status != "idle" || state.Locked {
		t.Fatalf("idle state=%+v err=%v", state, err)
	}
	workflowUUID := seedInitializationWorkflow(t, store, "running")
	if errorCode(store.RequireInitializationAvailable(ctx)) != CodeProjectInitializing {
		t.Fatal("running initialization did not lock project")
	}
	if _, err := store.TakeOverInitialization(ctx, workflowUUID); errorCode(err) != CodeInitializationHandoffConflict {
		t.Fatalf("active handoff error=%v", err)
	}
	if err := store.DB().Exec("UPDATE workflows SET status='failed' WHERE uuid=?", workflowUUID).Error; err != nil {
		t.Fatal(err)
	}
	state, err = store.InitializationState(ctx)
	if err != nil || !state.Locked || !state.CanTakeOver {
		t.Fatalf("failed state=%+v err=%v", state, err)
	}
	state, err = store.TakeOverInitialization(ctx, workflowUUID)
	if err != nil || state.Locked || state.Status != "handed_off" {
		t.Fatalf("handoff state=%+v err=%v", state, err)
	}
	if _, err := store.TakeOverInitialization(ctx, workflowUUID); err != nil {
		t.Fatalf("idempotent handoff: %v", err)
	}
	if err := manager.CloseCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.OpenRecent(ctx, created.UUID); err != nil {
		t.Fatal(err)
	}
	store = openStoreForTest(t, manager, created.UUID)
	state, err = store.InitializationState(ctx)
	if err != nil || state.Locked || state.Status != "handed_off" {
		t.Fatalf("reopened state=%+v err=%v", state, err)
	}
	secondUUID := seedInitializationWorkflow(t, store, "queued")
	if _, err := store.TakeOverInitialization(ctx, workflowUUID); errorCode(err) != CodeInitializationHandoffConflict {
		t.Fatalf("stale handoff unlocked new workflow: %v", err)
	}
	if err := store.DB().Exec("UPDATE workflows SET status='completed' WHERE uuid=?", secondUUID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.RequireInitializationAvailable(ctx); err != nil {
		t.Fatalf("completion did not unlock: %v", err)
	}
}

func TestInitializationHandoffWaitsForChildrenBeforeCheckpoint(t *testing.T) {
	ctx := context.Background()
	manager, _ := testManager(t)
	created, err := manager.Create(ctx, "取消收尾", ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreForTest(t, manager, created.UUID)
	workflowUUID := seedInitializationWorkflow(t, store, "cancelled")
	stepUUID, taskUUID := setupTestUUID(t), setupTestUUID(t)
	now := time.Now().UTC()
	if err := store.DB().Exec(`INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,created_at,updated_at)
		SELECT ?,id,'premise',1,'cancelled',?,?,? FROM workflows WHERE uuid=?`, stepUUID, stepUUID, now, now, workflowUUID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,created_at,updated_at)
		VALUES(?,?,'premise_setting_generation',?,'{}','running',?,?,?)`, taskUUID, store.project.ID, setupTestUUID(t), stepUUID+":setting", now, now).Error; err != nil {
		t.Fatal(err)
	}
	state, err := store.InitializationState(ctx)
	if err != nil || !state.Locked || state.CanTakeOver {
		t.Fatalf("active child state=%+v err=%v", state, err)
	}
	if _, err := store.TakeOverInitialization(ctx, workflowUUID); errorCode(err) != CodeInitializationHandoffConflict {
		t.Fatalf("handoff while child active: %v", err)
	}
	if err := store.DB().Exec("UPDATE production_task_runs SET status='cancelled' WHERE uuid=?", taskUUID).Error; err != nil {
		t.Fatal(err)
	}
	state, err = store.TakeOverInitialization(ctx, workflowUUID)
	if err != nil || state.Locked {
		t.Fatalf("settled child handoff=%+v err=%v", state, err)
	}
	raw, _ := json.Marshal(state)
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	for _, key := range []string{"id", "workflow_id", "thread_id"} {
		if _, exists := payload[key]; exists {
			t.Fatalf("private ID exposed: %s", raw)
		}
	}
}
