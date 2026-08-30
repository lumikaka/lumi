package production

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"lumi/internal/agentcheckpoint"
)

func insertProductionCheckpointExecution(t *testing.T, service *Service, routeID string) string {
	t.Helper()
	db := service.store.DB()
	now := time.Now().UTC()
	threadUUID, _ := newUUIDv7()
	turnUUID, _ := newUUIDv7()
	runUUID, _ := newUUIDv7()
	itemUUID, _ := newUUIDv7()
	executionUUID, _ := newUUIDv7()
	toolCallUUID, _ := newUUIDv7()
	providerUUID, _ := newUUIDv7()
	var projectID int64
	if err := db.Table("projects").Where("uuid=?", service.store.ProjectUUID()).Pluck("id", &projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,created_at,updated_at) VALUES(?,?,'checkpoint','busy',?,'test-model',?,?)`, threadUUID, projectID, providerUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_turns(uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),'prompt',1,'checkpoint','in_progress',?,?)`, turnUUID, threadUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_runs(uuid,thread_id,turn_id,trigger_type,status,provider_uuid,model,created_at,updated_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),(SELECT id FROM chat_turns WHERE uuid=?),'prompt','in_progress',?,'test-model',?,?)`, runUUID, threadUUID, turnUUID, providerUUID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO chat_items(uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,remote_item_uuid,tool_name,metadata_json,created_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),(SELECT id FROM chat_turns WHERE uuid=?),(SELECT id FROM chat_runs WHERE uuid=?),1,'tool_call','assistant','{}','json','in_progress',?,'request_api','{}',?)`, itemUUID, threadUUID, turnUUID, runUUID, toolCallUUID, now).Error; err != nil {
		t.Fatal(err)
	}
	arguments := fmt.Sprintf(`{"method":"DELETE","response_filter":".data","__route_id":%q}`, routeID)
	if err := db.Exec(`INSERT INTO agent_tool_executions(uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,target_uuid,arguments_json,idempotency_key,state,started_at,created_at,updated_at) VALUES(?,(SELECT id FROM chat_threads WHERE uuid=?),(SELECT id FROM chat_runs WHERE uuid=?),(SELECT id FROM chat_turns WHERE uuid=?),(SELECT id FROM chat_items WHERE uuid=?),?,'request_api','',?,?,'executing',?,?,?)`, executionUUID, threadUUID, runUUID, turnUUID, itemUUID, toolCallUUID, arguments, "checkpoint:"+executionUUID, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	return executionUUID
}

func TestPremiseAssetPermanentDeleteToolCheckpointSurvivesCrashWindow(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{
		UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, 91)), AssetType: AssetProp, Title: "Delete once",
	})
	if err != nil {
		t.Fatal(err)
	}
	asset, err = h.service.SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision)
	if err != nil {
		t.Fatal(err)
	}
	executionUUID := insertProductionCheckpointExecution(t, h.service, agentcheckpoint.RoutePremiseAssetPermanentDelete)
	want, err := h.service.PermanentlyDeletePremiseAssetFromTool(ctx, asset.UUID, asset.Revision, executionUUID)
	if err != nil || want.DeletedCount != 1 {
		t.Fatalf("first result=%+v err=%v", want, err)
	}
	got, err := h.service.PermanentlyDeletePremiseAssetFromTool(ctx, asset.UUID, asset.Revision, executionUUID)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("checkpoint result=%+v want=%+v err=%v", got, want, err)
	}
	var assets int64
	if err := h.service.store.DB().Table("premise_assets").Where("uuid=?", asset.UUID).Count(&assets).Error; err != nil || assets != 0 {
		t.Fatalf("asset count=%d err=%v", assets, err)
	}
}

func TestEmptyPremiseAssetTrashToolCheckpointKeepsOriginalResultAndScope(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	createTrashed := func(title string, seed uint8) PremiseAsset {
		asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, seed)), AssetType: AssetProp, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		asset, err = h.service.SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision)
		if err != nil {
			t.Fatal(err)
		}
		return asset
	}
	createTrashed("First trash", 92)
	executionUUID := insertProductionCheckpointExecution(t, h.service, agentcheckpoint.RoutePremiseAssetTrashEmpty)
	want, err := h.service.EmptyPremiseAssetTrashFromTool(ctx, executionUUID)
	if err != nil || want.DeletedCount != 1 {
		t.Fatalf("first result=%+v err=%v", want, err)
	}
	late := createTrashed("Late trash", 93)
	got, err := h.service.EmptyPremiseAssetTrashFromTool(ctx, executionUUID)
	if err != nil || got.DeletedCount != want.DeletedCount || got.FileSoftDeletedCount != want.FileSoftDeletedCount || got.RetainedFileCount != want.RetainedFileCount || len(got.BlockedItems) != len(want.BlockedItems) {
		t.Fatalf("checkpoint result=%+v want=%+v err=%v", got, want, err)
	}
	var remaining int64
	if err := h.service.store.DB().Table("premise_assets").Where("uuid=? AND deleted_at IS NOT NULL", late.UUID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("late trash count=%d err=%v", remaining, err)
	}
}

func TestPremiseAssetDeleteRollsBackWhenCheckpointCannotCommit(t *testing.T) {
	h := newProductionHarness(t)
	ctx := context.Background()
	asset, err := h.service.ImportPremiseAsset(ctx, CreateAssetInput{
		UploadUUID: upload(t, h.service, "premise_asset", imageBytes(t, 94)), AssetType: AssetProp, Title: "Rollback",
	})
	if err != nil {
		t.Fatal(err)
	}
	asset, err = h.service.SetPremiseAssetTrashed(ctx, asset.UUID, true, asset.Revision)
	if err != nil {
		t.Fatal(err)
	}
	executionUUID := insertProductionCheckpointExecution(t, h.service, agentcheckpoint.RoutePremiseAssetPermanentDelete)
	if err := h.service.store.DB().Exec(`CREATE TRIGGER reject_premise_checkpoint BEFORE UPDATE OF result_json ON agent_tool_executions BEGIN SELECT RAISE(ABORT,'reject checkpoint'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.PermanentlyDeletePremiseAssetFromTool(ctx, asset.UUID, asset.Revision, executionUUID); err == nil {
		t.Fatal("delete succeeded without an atomic checkpoint")
	}
	if err := h.service.store.DB().Exec(`DROP TRIGGER reject_premise_checkpoint`).Error; err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := h.service.store.DB().Table("premise_assets").Where("uuid=? AND deleted_at IS NOT NULL", asset.UUID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("rolled-back asset count=%d err=%v", remaining, err)
	}
}
