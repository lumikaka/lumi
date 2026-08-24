package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/production"
)

func TestCurrentProjectAPIToolDerivesUpdatesAndSoftDeletesIdempotently(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	productionService := production.NewService(harness.store, nil)
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "source.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	source, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "月光邮差", Summary: "银发邮差", Tags: []string{"courier"}})
	if err != nil {
		t.Fatal(err)
	}
	originalFileUUID := source.CurrentVariant.Asset.UUID
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "引用设定", Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: source.UUID, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "创建一位搭档，然后更新当前角色"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	// This fixture exercises the isolated recovery path for an already
	// persisted phase-two legacy execution, not new Run assembly.
	tc.ToolMode = ToolModeLegacyTyped
	harness.service.WithImageClient(&imageClientFake{response: imagegen.Response{Bytes: agentTestPNG(t), MIMEType: "image/png", RevisedPrompt: "moonlit courier companion"}})

	generateImage := func(prompt string) string {
		t.Helper()
		executionUUID, _ := newUUIDv7()
		value, generateErr := harness.service.executeImageGenTool(ctx, harness.store, tc, toolExecutionRecord{UUID: executionUUID, ToolName: "image_gen"}, map[string]any{"prompt": prompt})
		if generateErr != nil || !isUUIDv7(stringArg(value, "file_uuid")) {
			t.Fatalf("image_gen value=%+v err=%v", value, generateErr)
		}
		return stringArg(value, "file_uuid")
	}
	call := func(executionUUID string, args map[string]any) (any, error) {
		t.Helper()
		return executeCurrentProjectAPITool(ctx, productionService, tc, toolExecutionRecord{UUID: executionUUID, ToolName: currentProjectAPIToolName}, args)
	}
	assetURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + source.UUID
	collectionURL := "/api/v1/projects/" + harness.project.UUID + "/premise-assets"

	readValue, err := call(mustAgentUUID(t), map[string]any{"method": "GET", "url": assetURL})
	if err != nil || readValue.(production.PremiseAsset).UUID != source.UUID {
		t.Fatalf("GET current value=%+v err=%v", readValue, err)
	}
	listValue, err := call(mustAgentUUID(t), map[string]any{"method": "GET", "url": collectionURL})
	if err != nil || len(listValue.(map[string]any)["items"].([]production.PremiseAsset)) != 1 {
		t.Fatalf("GET list value=%+v err=%v", listValue, err)
	}

	derivedFileUUID := generateImage("在整体画风中创建来源角色的新搭档")
	createExecutionUUID := mustAgentUUID(t)
	createArgs := map[string]any{"method": "POST", "url": collectionURL, "request_body": map[string]any{"file_uuid": derivedFileUUID, "asset_type": "character", "title": "星光分拣员", "summary": "月光邮差的搭档", "tags": []any{"companion", "courier"}}}
	createdValue, err := call(createExecutionUUID, createArgs)
	if err != nil {
		t.Fatal(err)
	}
	derived := createdValue.(production.PremiseAsset)
	if derived.UUID == source.UUID || derived.CurrentVariant == nil || derived.CurrentVariant.Asset.UUID != derivedFileUUID {
		t.Fatalf("derived asset=%+v", derived)
	}
	originalAfterCreate, err := productionService.GetPremiseAsset(ctx, source.UUID)
	if err != nil || originalAfterCreate.Revision != source.Revision || originalAfterCreate.Title != source.Title || originalAfterCreate.CurrentVariant.Asset.UUID != originalFileUUID {
		t.Fatalf("POST mutated source=%+v err=%v", originalAfterCreate, err)
	}
	replayedCreate, err := call(createExecutionUUID, createArgs)
	if err != nil || replayedCreate.(production.PremiseAsset).UUID != derived.UUID {
		t.Fatalf("POST replay=%+v err=%v", replayedCreate, err)
	}
	if _, err := call(mustAgentUUID(t), createArgs); productionErrorCode(err) != production.CodeStateConflict {
		t.Fatalf("consumed image was accepted by another POST: %v", err)
	}
	var createEventPayload string
	if err := harness.store.DB().Table("premise_asset_events AS events").Select("events.payload").Joins("JOIN premise_assets AS assets ON assets.id=events.premise_asset_id").Where("assets.uuid=? AND events.event_type='asset_created_from_chat_image'", derived.UUID).Order("events.id DESC").Limit(1).Scan(&createEventPayload).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(createEventPayload, source.UUID) || !strings.Contains(createEventPayload, createExecutionUUID) {
		t.Fatalf("derived event lost public lineage or tool audit: %s", createEventPayload)
	}

	metadataExecutionUUID := mustAgentUUID(t)
	metadataArgs := map[string]any{"method": "PATCH", "url": assetURL, "request_body": map[string]any{"expected_revision": source.Revision, "summary": "可靠的银发月光邮差", "tags": []any{"courier", "winter"}}}
	metadataValue, err := call(metadataExecutionUUID, metadataArgs)
	if err != nil {
		t.Fatal(err)
	}
	metadataUpdated := metadataValue.(production.PremiseAsset)
	if metadataUpdated.Summary != "可靠的银发月光邮差" || metadataUpdated.Revision != source.Revision+1 {
		t.Fatalf("metadata PATCH=%+v", metadataUpdated)
	}
	replayedMetadata, err := call(metadataExecutionUUID, metadataArgs)
	if err != nil || replayedMetadata.(production.PremiseAsset).Revision != metadataUpdated.Revision {
		t.Fatalf("metadata PATCH replay=%+v err=%v", replayedMetadata, err)
	}
	staleArgs := map[string]any{"method": "PATCH", "url": assetURL, "request_body": map[string]any{"expected_revision": source.Revision, "title": "过期标题"}}
	if _, err := call(mustAgentUUID(t), staleArgs); productionErrorCode(err) != production.CodeConflict {
		t.Fatalf("stale PATCH error=%v", err)
	}

	replacementFileUUID := generateImage("保持角色身份和整体画风，换成冬季制服")
	imageExecutionUUID := mustAgentUUID(t)
	imageArgs := map[string]any{"method": "PATCH", "url": assetURL, "request_body": map[string]any{"expected_revision": metadataUpdated.Revision, "file_uuid": replacementFileUUID, "title": "月光邮差·冬季"}}
	imageValue, err := call(imageExecutionUUID, imageArgs)
	if err != nil {
		t.Fatal(err)
	}
	imageUpdated := imageValue.(production.PremiseAsset)
	if imageUpdated.CurrentVariant.Asset.UUID != replacementFileUUID || imageUpdated.Title != "月光邮差·冬季" || imageUpdated.Revision != metadataUpdated.Revision+1 {
		t.Fatalf("image PATCH=%+v", imageUpdated)
	}
	replayedImage, err := call(imageExecutionUUID, imageArgs)
	if err != nil || replayedImage.(production.PremiseAsset).CurrentVariant.Asset.UUID != replacementFileUUID {
		t.Fatalf("image PATCH replay=%+v err=%v", replayedImage, err)
	}

	deleteExecutionUUID := mustAgentUUID(t)
	deleteArgs := map[string]any{"method": "DELETE", "url": assetURL + "?expected_revision=" + jsonInt(imageUpdated.Revision)}
	deletedValue, err := call(deleteExecutionUUID, deleteArgs)
	if err != nil || deletedValue.(production.PremiseAsset).DeletedAt == nil {
		t.Fatalf("DELETE value=%+v err=%v", deletedValue, err)
	}
	replayedDelete, err := call(deleteExecutionUUID, deleteArgs)
	if err != nil || replayedDelete.(production.PremiseAsset).DeletedAt == nil {
		t.Fatalf("DELETE replay=%+v err=%v", replayedDelete, err)
	}
	if _, err := call(mustAgentUUID(t), deleteArgs); productionErrorCode(err) != production.CodeStateConflict {
		t.Fatalf("new DELETE after trash error=%v", err)
	}
	if _, err := call(mustAgentUUID(t), map[string]any{"method": "GET", "url": assetURL}); errorCode(err) != CodeToolNotAllowed {
		t.Fatalf("trashed reference still allowed GET: %v", err)
	}
	active, err := productionService.ListPremiseAssets(ctx, "", "active")
	if err != nil || len(active) != 1 || active[0].UUID != derived.UUID {
		t.Fatalf("soft delete active assets=%+v err=%v", active, err)
	}
	trashed, err := productionService.GetPremiseAsset(ctx, source.UUID)
	if err != nil || trashed.DeletedAt == nil {
		t.Fatalf("soft-deleted source was not recoverable=%+v err=%v", trashed, err)
	}
	if _, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "继续修改"}); errorCode(err) != CodeToolNotAllowed {
		t.Fatalf("trashed asset accepted a later reference turn: %v", err)
	}
}

func TestCurrentProjectAPIToolRejectsRoutesBodiesAndInvalidFiles(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	productionService := production.NewService(harness.store, nil)
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "source.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetScene, Title: "月亮邮局"})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "引用设定", Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: asset.UUID, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "验证边界"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/projects/" + harness.project.UUID
	otherProjectUUID, otherSubjectUUID := mustAgentUUID(t), mustAgentUUID(t)
	invalid := []map[string]any{
		{"method": "GET", "url": "https://example.com" + base + "/premise"},
		{"method": "GET", "url": "/api/v1/projects/" + otherProjectUUID + "/premise"},
		{"method": "GET", "url": base + "/premise-assets/" + otherSubjectUUID},
		{"method": "GET", "url": base + "/premise-assets/../premise"},
		{"method": "GET", "url": base + "/premise-assets/%2e%2e"},
		{"method": "GET", "url": base + "/premise-assets/"},
		{"method": "PUT", "url": base + "/premise-assets/" + asset.UUID},
		{"method": "DELETE", "url": base + "/premise-assets/" + asset.UUID + "/permanent?expected_revision=0"},
		{"method": "DELETE", "url": base + "/premise-assets/" + asset.UUID},
		{"method": "DELETE", "url": base + "/premise-assets/" + asset.UUID + "?expected_revision=0", "request_body": map[string]any{}},
		{"method": "GET", "url": base + "/premise-assets?state=active"},
		{"method": "POST", "url": base + "/premise-assets", "request_body": map[string]any{"file_uuid": mustAgentUUID(t), "asset_type": "scene", "title": "派生场景", "expected_revision": 0}},
		{"method": "PATCH", "url": base + "/premise-assets/" + asset.UUID, "request_body": map[string]any{"expected_revision": 0, "permanent": true}},
		{"method": "POST", "url": base + "/premise-assets/empty-trash", "request_body": map[string]any{"file_uuid": mustAgentUUID(t), "asset_type": "scene", "title": "越界"}},
	}
	for index, args := range invalid {
		if _, err := parseCurrentProjectAPIRequest(tc, args); err == nil {
			t.Fatalf("invalid route %d accepted: %+v", index, args)
		}
	}

	validFileUUID := mustAgentUUID(t)
	valid := []map[string]any{
		{"method": "GET", "url": base + "/premise"},
		{"method": "GET", "url": base + "/premise-assets"},
		{"method": "GET", "url": base + "/premise-assets/" + asset.UUID},
		{"method": "POST", "url": base + "/premise-assets", "request_body": map[string]any{"file_uuid": validFileUUID, "asset_type": "scene", "title": "派生场景"}},
		{"method": "PATCH", "url": base + "/premise-assets/" + asset.UUID, "request_body": map[string]any{"expected_revision": asset.Revision, "summary": "更新"}},
		{"method": "DELETE", "url": base + "/premise-assets/" + asset.UUID + "?expected_revision=0"},
	}
	for index, args := range valid {
		if _, err := parseCurrentProjectAPIRequest(tc, args); err != nil {
			t.Fatalf("valid route %d rejected: %+v err=%v", index, args, err)
		}
	}

	validRaw := `{"url":"` + base + `/premise-assets","method":"POST","request_body":{"file_uuid":"` + validFileUUID + `","asset_type":"scene","title":"派生场景"}}`
	if _, err := validateToolArgumentsForMode(currentProjectAPIToolName, validRaw, ToolModeLegacyTyped); err != nil {
		t.Fatalf("valid generic tool arguments rejected: %v", err)
	}
	for _, raw := range []string{
		`{"url":"` + base + `/premise-assets","method":"POST","request_body":{"file_uuid":"bad","asset_type":"scene","title":"派生场景"}}`,
		`{"url":"` + base + `/premise-assets","method":"POST","request_body":{"file_uuid":"` + validFileUUID + `","asset_type":"scene","title":"派生场景","unknown":true}}`,
	} {
		if _, err := validateToolArgumentsForMode(currentProjectAPIToolName, raw, ToolModeLegacyTyped); err == nil {
			t.Fatalf("invalid generic arguments accepted: %s", raw)
		}
	}

	wrongUpload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "project_chatbot_reference", OriginalFilename: "wrong-purpose.png", Metadata: map[string]any{"chat_thread_uuid": thread.UUID, "premise_asset_uuid": asset.UUID}, Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	wrongFile, err := productionService.Files().FinalizeUpload(ctx, wrongUpload.UUID, "project_chatbot_reference")
	if err != nil {
		t.Fatal(err)
	}
	createReferenceFile := func(chatThreadUUID, sourceAssetUUID string) string {
		t.Helper()
		upload, createErr := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{
			Purpose:          "project_chat_asset_reference_image",
			OriginalFilename: "wrong-binding.png",
			Metadata: map[string]any{
				"source":              "project_chat_image_gen",
				"tool_execution_uuid": mustAgentUUID(t),
				"chat_thread_uuid":    chatThreadUUID,
				"chat_run_uuid":       tc.Run.UUID,
				"premise_asset_uuid":  sourceAssetUUID,
			},
			Reader: bytes.NewReader(agentTestPNG(t)),
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		file, finalizeErr := productionService.Files().FinalizeUpload(ctx, upload.UUID, "project_chat_asset_reference_image")
		if finalizeErr != nil {
			t.Fatal(finalizeErr)
		}
		return file.UUID
	}
	wrongThreadFileUUID := createReferenceFile(mustAgentUUID(t), asset.UUID)
	wrongSourceFileUUID := createReferenceFile(thread.UUID, mustAgentUUID(t))
	for label, fileUUID := range map[string]string{"malformed": "not-a-uuid", "wrong purpose": wrongFile.UUID, "wrong thread": wrongThreadFileUUID, "wrong source": wrongSourceFileUUID, "missing or other project": validFileUUID} {
		args := map[string]any{"method": "POST", "url": base + "/premise-assets", "request_body": map[string]any{"file_uuid": fileUUID, "asset_type": "scene", "title": "非法图片 " + label}}
		_, err := executeCurrentProjectAPITool(ctx, productionService, tc, toolExecutionRecord{UUID: mustAgentUUID(t)}, args)
		if err == nil {
			t.Fatalf("%s file was accepted", label)
		}
	}
}

func TestPersistedLegacyToolIntentAuditsSubjectAndRecoversExecution(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	productionService := production.NewService(harness.store, nil)
	upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: "premise_asset", OriginalFilename: "source.png", Reader: bytes.NewReader(agentTestPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: upload.UUID, AssetType: production.AssetCharacter, Title: "水彩邮差", Summary: "来自旧 Prompt 快照"})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "引用设定", Scope: ThreadScopePremise, Scene: SceneAssetReference, SubjectUUID: asset.UUID, ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取当前项"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	// This fixture exercises the isolated recovery path for an already
	// persisted phase-two legacy execution, not new Run assembly.
	tc.ToolMode = ToolModeLegacyTyped
	raw, _ := json.Marshal(map[string]any{"method": "GET", "url": "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID})
	execution, _, completed, err := harness.service.persistToolIntent(ctx, harness.store, tc, "provider-call-current-project-api", currentProjectAPIToolName, string(raw))
	if err != nil || completed || execution.ToolName != currentProjectAPIToolName || execution.TargetUUID != asset.UUID || !isUUIDv7(execution.UUID) {
		t.Fatalf("generic tool audit=%+v completed=%v err=%v", execution, completed, err)
	}

	legacyExecutionUUID := mustAgentUUID(t)
	legacyArguments, _ := json.Marshal(map[string]any{
		"method": "PATCH",
		"url":    "/api/v1/projects/" + harness.project.UUID + "/premise-assets/" + asset.UUID,
		"request_body": map[string]any{
			"expected_revision": asset.Revision,
			"summary":           "旧 execution 已恢复",
		},
	})
	legacyResult, err := harness.service.executeTool(ctx, harness.store, tc, toolExecutionRecord{UUID: legacyExecutionUUID, ToolName: currentProjectAPIToolName, ArgumentsJSON: string(legacyArguments)})
	if err != nil || !strings.Contains(string(legacyResult), `"success":true`) {
		t.Fatalf("persisted legacy typed execution could not recover: result=%s err=%v", legacyResult, err)
	}
	recovered, err := productionService.GetPremiseAsset(ctx, asset.UUID)
	if err != nil || recovered.Summary != "旧 execution 已恢复" {
		t.Fatalf("legacy typed execution did not update asset=%+v err=%v", recovered, err)
	}
}

func mustAgentUUID(t *testing.T) string {
	t.Helper()
	uuid, err := newUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	return uuid
}

func jsonInt(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func productionErrorCode(err error) string {
	var productionErr *production.Error
	if errors.As(err, &productionErr) {
		return productionErr.Code
	}
	return ""
}
