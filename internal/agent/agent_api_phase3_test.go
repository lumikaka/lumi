package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lumi/internal/files"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

func TestPhase3RouteContractsAreCompleteAndSecure(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	otherProjectUUID := mustAgentUUID(t)
	resourceUUID := mustAgentUUID(t)
	tc := toolContext{
		ProjectUUID: projectUUID,
		ToolMode:    ToolModeProjectAPI,
		Thread:      threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject},
	}
	routes := phase3AgentAPIRoutes()
	if len(routes) != 57 || len(agentAPIRoutes()) != 83 {
		t.Fatalf("phase3 routes=%d total=%d want=57/83", len(routes), len(agentAPIRoutes()))
	}
	seenIDs := map[string]bool{}
	seenMethodPaths := map[string]bool{}
	for _, route := range routes {
		t.Run(route.ID, func(t *testing.T) {
			if route.ID == "" || route.Action == "" || route.Handler != route.ID || route.Projector == "" || route.DocPath == "" || route.Risk == "" {
				t.Fatalf("incomplete route: %+v", route)
			}
			if seenIDs[route.ID] {
				t.Fatalf("duplicate route ID %s", route.ID)
			}
			seenIDs[route.ID] = true
			methodPath := route.Method + " " + route.PathTemplate
			if seenMethodPaths[methodPath] {
				t.Fatalf("duplicate method/path %s", methodPath)
			}
			seenMethodPaths[methodPath] = true
			if _, ok := agentAPIProjectorByKey(route.Projector); !ok {
				t.Fatalf("missing projector %s", route.Projector)
			}
			if !validAgentDocPath(route.DocPath) {
				t.Fatalf("missing document %s", route.DocPath)
			}
			doc, err := renderAgentDoc(route.DocPath)
			docMarker := "`" + route.Method + " " + route.PathTemplate + "`"
			if err != nil || !strings.Contains(doc, docMarker) {
				t.Fatalf("route is absent from static API Contract: err=%v", err)
			}
			if route.Risk == RiskDangerous && !route.RequiresConfirmation {
				t.Fatal("dangerous route does not require confirmation")
			}
			if route.Method == "GET" && (!route.ReadOnly || route.BodySchema != nil) {
				t.Fatal("GET route is not a body-free read")
			}

			path := phase3TestPath(route.PathTemplate, projectUUID, resourceUUID)
			filter := recommendedAgentAPIResponseFilter(route)
			if filter == "" || filter == ".data" {
				t.Fatalf("route has no narrow recommended response_filter: %+v", route)
			}
			args := map[string]any{"method": route.Method, "url": path, "response_filter": filter}
			if route.QuerySchema != nil {
				args["query"] = phase3TestSchemaObject(route.QuerySchema, resourceUUID)
			}
			if route.BodySchema != nil {
				args["request_body"] = phase3TestSchemaObject(route.BodySchema, resourceUUID)
			}
			request, err := parseAgentAPIRequest(tc, args)
			if err != nil || request.Route.ID != route.ID {
				t.Fatalf("valid contract rejected: request=%+v err=%v args=%+v", request, err, args)
			}
			if _, err := parseAgentAPIRequest(tc, mergePhase3Args(args, "method", strings.ToLower(route.Method))); err == nil {
				t.Fatal("non-canonical method was accepted")
			}
			if _, err := parseAgentAPIRequest(tc, mergePhase3Args(args, "url", path+"?encoded=true")); err == nil {
				t.Fatal("query string path was accepted")
			}
			if _, err := parseAgentAPIRequest(tc, mergePhase3Args(args, "url", strings.Replace(path, projectUUID, "not-a-uuid", 1))); err == nil {
				t.Fatal("invalid UUID path was accepted")
			}
			if _, err := parseAgentAPIRequest(tc, mergePhase3Args(args, "url", strings.Replace(path, projectUUID, otherProjectUUID, 1))); err == nil {
				t.Fatal("cross-project path was accepted")
			}
			invalid := cloneToolArguments(args)
			if route.QuerySchema != nil {
				query := cloneToolArguments(invalid["query"].(map[string]any))
				query["unknown"] = true
				invalid["query"] = query
			} else if route.BodySchema != nil {
				body := cloneToolArguments(invalid["request_body"].(map[string]any))
				body["internal_id"] = float64(9)
				invalid["request_body"] = body
			} else {
				invalid["query"] = map[string]any{"unknown": true}
			}
			if _, err := parseAgentAPIRequest(tc, invalid); err == nil {
				t.Fatal("invalid query/body was accepted")
			}

			raw := phase3TestProjectorValue(route.Projector, resourceUUID)
			projected, err := compactAgentRouteValue(route, raw)
			if err != nil || validateAgentAPIResponse(projected) != nil {
				t.Fatalf("projector rejected safe value: value=%+v err=%v", projected, err)
			}
			encoded, _ := json.Marshal(projected)
			if containsInternalID(encoded) || strings.Contains(string(encoded), "must-not-leak") || strings.Contains(string(encoded), "content_url") || strings.Contains(string(encoded), "download_url") {
				t.Fatalf("projector leaked internal value: %s", encoded)
			}
		})
	}
}

func TestRequestAPIRejectsDuplicateQueryAndBodyFields(t *testing.T) {
	projectUUID := mustAgentUUID(t)
	for _, raw := range []string{
		`{"method":"GET","url":"/api/v1/projects/` + projectUUID + `/premise-sources","query":{"page":1,"page":2}}`,
		`{"method":"PATCH","url":"/api/v1/projects/` + projectUUID + `/premise","request_body":{"default_style":"one","default_style":"two","expected_revision":0}}`,
	} {
		if _, err := validateToolArguments("request_api", raw); err == nil || errorCode(err) != CodeToolValidation {
			t.Fatalf("duplicate JSON field was accepted: %s err=%v", raw, err)
		}
	}
	tc := toolContext{ProjectUUID: projectUUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	for _, metadata := range []map[string]any{
		{"internal_id": float64(9)},
		{"nested": map[string]any{"absolute_path": "/tmp/secret"}},
		{"nested": map[string]any{"access_token": "must-not-store"}},
	} {
		_, err := parseAgentAPIRequest(tc, map[string]any{
			"method": "PATCH", "url": "/api/v1/projects/" + projectUUID + "/assets/" + mustAgentUUID(t),
			"request_body": map[string]any{"metadata": metadata}, "response_filter": ".data | {uuid}",
		})
		if err == nil || errorCode(err) != CodeToolValidation {
			t.Fatalf("forbidden nested metadata was accepted: %+v err=%v", metadata, err)
		}
	}
}

func TestEveryPhase3RouteExecutesItsInProcessSuccessPath(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "Phase 3 reviewed route coverage"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	tc = seedReadyBootstrapYoloAuthorization(t, harness, tc, mustAgentUUID(t))
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	executed := map[string]bool{}
	call := func(routeID string, params map[string]string, query, body map[string]any) any {
		t.Helper()
		var route agentAPIRoute
		for _, candidate := range phase3AgentAPIRoutes() {
			if candidate.ID == routeID {
				route = candidate
				break
			}
		}
		if route.ID == "" {
			t.Fatalf("unknown phase3 route %s", routeID)
		}
		path := strings.ReplaceAll(route.PathTemplate, "{project_uuid}", harness.project.UUID)
		for key, value := range params {
			path = strings.ReplaceAll(path, "{"+key+"}", value)
		}
		if strings.Contains(path, "{") {
			t.Fatalf("route %s has unresolved path %s", routeID, path)
		}
		args := map[string]any{"method": route.Method, "url": path, "response_filter": recommendedAgentAPIResponseFilter(route)}
		if query != nil {
			args["query"] = query
		}
		if body != nil {
			args["request_body"] = body
		}
		request, err := parseAgentAPIRequest(tc, args)
		if err != nil {
			t.Fatalf("parse %s: %v args=%+v", routeID, err, args)
		}
		if route.ExpectedRevision {
			staleArgs := cloneToolArguments(args)
			staleBody := cloneToolArguments(staleArgs["request_body"].(map[string]any))
			staleBody["expected_revision"] = staleBody["expected_revision"].(float64) + 1000
			staleArgs["request_body"] = staleBody
			staleRequest, parseErr := parseAgentAPIRequest(tc, staleArgs)
			if parseErr != nil {
				t.Fatalf("parse stale revision %s: %v", routeID, parseErr)
			}
			if _, staleErr := executeAgentAPIRoute(ctx, harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "phase3-stale:" + routeID}, staleRequest); staleErr == nil {
				t.Fatalf("route %s accepted a stale expected_revision", routeID)
			}
		}
		execution := toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "phase3-success:" + routeID}
		if routeID == RouteYoloWorkflowCreate {
			encoded, marshalErr := json.Marshal(args)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var completed bool
			var replay json.RawMessage
			execution, replay, completed, err = harness.service.persistToolIntent(ctx, harness.store, tc, "phase3-success:"+routeID, "request_api", string(encoded))
			if err != nil || completed || replay != nil {
				t.Fatalf("persist %s: execution=%+v completed=%v replay=%s err=%v", routeID, execution, completed, replay, err)
			}
		}
		value, err := executeAgentAPIRoute(ctx, harness.service, harness.store, tc, execution, request)
		if routeID == RouteYoloWorkflowCreate && errors.Is(err, ErrWaitingWorkflow) {
			workflows, listErr := harness.service.ListWorkflows(ctx, harness.project.UUID)
			if listErr != nil || len(workflows) == 0 {
				t.Fatalf("list inline Yolo: workflows=%+v err=%v", workflows, listErr)
			}
			value, err = agentYoloWorkflowValue(workflows[0]), nil
		}
		if err != nil {
			t.Fatalf("execute %s: %v args=%+v", routeID, err, args)
		}
		value, err = compactAgentRouteValue(route, value)
		if err != nil || validateAgentAPIResponse(value) != nil {
			t.Fatalf("project %s: value=%+v err=%v", routeID, value, err)
		}
		executed[routeID] = true
		return value
	}
	revision := func(value int64) float64 { return float64(value) }
	storyService := story.NewService(harness.store)
	productionService := production.NewService(harness.store, nil)
	createUpload := func(purpose, filename string) files.Upload {
		t.Helper()
		upload, err := productionService.Files().CreateUpload(ctx, files.CreateUploadInput{Purpose: purpose, OriginalFilename: filename, Reader: bytes.NewReader(agentTestPNG(t))})
		if err != nil {
			t.Fatal(err)
		}
		return upload
	}

	project, err := storyService.GetProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	call(RouteProjectGet, nil, nil, nil)
	yoloWorkflow := call(RouteYoloWorkflowCreate, nil, nil, map[string]any{"story_prompt": "一只小狐狸替月亮送信。"}).(map[string]any)
	if _, err := harness.service.CancelWorkflow(ctx, harness.project.UUID, stringArg(yoloWorkflow, "uuid")); err != nil {
		t.Fatalf("cancel inline Yolo route fixture: %v", err)
	}
	if resumed, err := harness.service.resumeWorkflowAwait(ctx, harness.store, tc); err != nil || !resumed {
		t.Fatalf("resume inline Yolo route fixture: resumed=%v err=%v", resumed, err)
	}
	tc, err = harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	tc.ToolMode = ToolModeProjectAPI
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	call(RouteProjectUpdate, nil, nil, map[string]any{"name": "Phase 3 Agent API", "description": "route integration", "generation_language": "zh-Hans", "expected_revision": revision(project.Revision)})

	call(RouteChapterCreate, nil, nil, map[string]any{"chapter_code": "vol09.ch01", "title": "Phase 3", "content": "initial", "content_format": "md"})
	chapters, err := storyService.ListChapters(ctx, "active")
	if err != nil || len(chapters) != 1 {
		t.Fatalf("chapters=%+v err=%v", chapters, err)
	}
	chapter := chapters[0]
	chapterParams := map[string]string{"chapter_uuid": chapter.UUID}
	call(RouteChapterUpdate, chapterParams, nil, map[string]any{"title": "Phase 3 updated", "expected_revision": revision(chapter.Revision)})
	chapter, _ = storyService.GetChapter(ctx, chapter.UUID)
	call(RouteChapterStoryList, chapterParams, nil, nil)
	call(RouteChapterTrash, chapterParams, nil, map[string]any{"expected_revision": revision(chapter.Revision)})
	chapter, _ = storyService.GetChapter(ctx, chapter.UUID)
	call(RouteChapterRestore, chapterParams, nil, map[string]any{"expected_revision": revision(chapter.Revision)})

	profile, err := storyService.GetStoryProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	call(RouteStoryProfileList, nil, nil, nil)
	storyPath := filepath.Join(harness.store.Root(), "STORY.md")
	if err := os.WriteFile(storyPath, []byte("# External phase 3 story\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile, err = storyService.GetStoryProfile(ctx)
	if err != nil || profile.ProjectionState != "conflict" {
		t.Fatalf("profile=%+v err=%v", profile, err)
	}
	call(RouteStoryProfileImport, nil, nil, map[string]any{"expected_revision": revision(profile.Revision)})
	profile, _ = storyService.GetStoryProfile(ctx)
	if err := os.WriteFile(storyPath, []byte("# Another external phase 3 edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(RouteStoryProfileRegenerate, nil, nil, map[string]any{"expected_revision": revision(profile.Revision)})
	storyTask := call(RouteStoryProfileGenerationCreate, nil, nil, map[string]any{"prompt": "generate profile"}).(map[string]any)
	call(RouteStoryProfileRebuildCreate, nil, nil, map[string]any{})
	call(RouteChapterBatchPlanCreate, nil, nil, map[string]any{"prompt": "plan chapters", "chapter_count": float64(3)})
	call(RouteComicStoryboardGenerationCreate, chapterParams, nil, map[string]any{"prompt": "plan storyboard", "max_section_count": float64(8)})

	premise, err := productionService.GetPremise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	call(RoutePremiseUpdate, nil, nil, map[string]any{"default_style": "indigo watercolor", "expected_revision": revision(premise.Revision)})
	call(RoutePremiseSourceCreate, nil, nil, map[string]any{"source_text": "A moonlit post office", "style_snapshot": "indigo watercolor", "source_type": "manual"})
	sources, err := productionService.ListPremiseSources(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources=%+v err=%v", sources, err)
	}
	source := sources[0]
	call(RoutePremiseSourceList, nil, map[string]any{"page": float64(1), "per_page": float64(10)}, nil)

	settingUpload := createUpload("premise_setting_image", "setting.png")
	call(RouteSettingImageImport, nil, nil, map[string]any{"upload_uuid": settingUpload.UUID, "source_uuid": source.UUID, "prompt": "setting"})
	settingImages, err := productionService.ListSettingImages(ctx)
	if err != nil || len(settingImages) != 1 {
		t.Fatalf("setting images=%+v err=%v", settingImages, err)
	}
	settingImage := settingImages[0]
	call(RouteSettingImageList, nil, map[string]any{"source_uuids": []any{source.UUID}}, nil)
	call(RouteSettingImageSelect, map[string]string{"setting_image_uuid": settingImage.UUID}, nil, map[string]any{})
	call(RoutePremiseSourceUpdate, map[string]string{"source_uuid": source.UUID}, nil, map[string]any{"ignored": true, "expected_revision": revision(source.Revision)})

	assetUpload := createUpload("premise_asset", "asset.png")
	asset, err := productionService.ImportPremiseAsset(ctx, production.CreateAssetInput{UploadUUID: assetUpload.UUID, AssetType: production.AssetCharacter, Title: "Courier"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := productionService.SetPremiseAssetTrashedFromTool(ctx, asset.UUID, true, asset.Revision, mustAgentUUID(t)); err != nil {
		t.Fatal(err)
	}
	asset, _ = productionService.GetPremiseAsset(ctx, asset.UUID)
	assetParams := map[string]string{"premise_asset_uuid": asset.UUID}
	call(RoutePremiseAssetRestore, assetParams, nil, map[string]any{"expected_revision": revision(asset.Revision)})
	asset, _ = productionService.GetPremiseAsset(ctx, asset.UUID)
	call(RoutePremiseAssetVariantList, assetParams, nil, nil)
	variantUpload := createUpload("premise_asset", "asset-variant.png")
	call(RoutePremiseAssetVariantCreate, assetParams, nil, map[string]any{"upload_uuid": variantUpload.UUID, "expected_revision": revision(asset.Revision)})
	asset, _ = productionService.GetPremiseAsset(ctx, asset.UUID)
	variants, err := productionService.ListAssetVariants(ctx, asset.UUID)
	if err != nil || len(variants) < 2 {
		t.Fatalf("asset variants=%+v err=%v", variants, err)
	}
	call(RoutePremiseAssetVariantSelect, map[string]string{"premise_asset_uuid": asset.UUID, "variant_uuid": variants[0].UUID}, nil, map[string]any{"expected_revision": revision(asset.Revision)})

	call(RouteComicStateGet, chapterParams, nil, nil)
	bodyPage := call(RouteComicSectionCreate, chapterParams, nil, map[string]any{"title": "Opening", "description_md": "Moonlight", "storyboard_md": "# Opening", "page_role": "body"}).(map[string]any)
	if bodyPage["page_role"] != production.PageRoleBody {
		t.Fatalf("body-page route result=%+v", bodyPage)
	}
	secondBodyPage := call(RouteComicSectionCreate, chapterParams, nil, map[string]any{"title": "Continuation", "description_md": "Dawn", "storyboard_md": "# Continuation", "page_role": "body"}).(map[string]any)
	if secondBodyPage["page_role"] != production.PageRoleBody {
		t.Fatalf("second body-page route result=%+v", secondBodyPage)
	}
	sections, err := productionService.ListSections(ctx, chapter.UUID)
	if err != nil || len(sections) != 2 || sections[0].PageRole != production.PageRoleBody || sections[1].PageRole != production.PageRoleBody {
		t.Fatalf("sections=%+v err=%v", sections, err)
	}
	section := sections[0]
	sectionParams := map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": section.UUID}
	call(RouteComicSectionList, chapterParams, nil, nil)
	call(RouteComicSectionUpdate, sectionParams, nil, map[string]any{"title": "Opening updated", "description_md": "Moonlit station", "page_role": "body", "expected_revision": revision(section.Revision)})
	section, _ = productionService.GetSection(ctx, chapter.UUID, section.UUID)
	call(RouteComicSectionReorder, chapterParams, nil, map[string]any{"section_uuids": []any{section.UUID, stringArg(secondBodyPage, "uuid")}})
	section, _ = productionService.GetSection(ctx, chapter.UUID, section.UUID)
	call(RouteStoryboardList, sectionParams, nil, nil)
	storyboards, err := productionService.ListStoryboards(ctx, chapter.UUID, section.UUID)
	if err != nil || len(storyboards) == 0 {
		t.Fatalf("storyboards=%+v err=%v", storyboards, err)
	}
	call(RouteStoryboardSelect, map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": section.UUID, "variant_uuid": storyboards[0].UUID}, nil, map[string]any{"expected_revision": revision(section.Revision)})
	section, _ = productionService.GetSection(ctx, chapter.UUID, section.UUID)
	sectionUpload := createUpload("comic_section_image", "section.png")
	call(RouteComicSectionImageImport, sectionParams, nil, map[string]any{"upload_uuid": sectionUpload.UUID, "expected_revision": revision(section.Revision)})
	section, _ = productionService.GetSection(ctx, chapter.UUID, section.UUID)
	call(RouteComicImageGenerationBatchCreate, chapterParams, nil, map[string]any{"section_uuids": []any{section.UUID}})
	if len(harness.queue.batchRequests) != 1 || harness.queue.batchRequests[0].IdempotencyKey != "phase3-success:"+RouteComicImageGenerationBatchCreate || len(harness.queue.batchRequests[0].ResourceUUIDs) != 1 || harness.queue.batchRequests[0].ResourceUUIDs[0] != section.UUID {
		t.Fatalf("batch request did not use Tool Execution idempotency or safe targets: %+v", harness.queue.batchRequests)
	}
	call(RouteComicImageVariantList, sectionParams, nil, nil)
	imageVariants, err := productionService.ListImageVariants(ctx, chapter.UUID, section.UUID)
	if err != nil || len(imageVariants) == 0 {
		t.Fatalf("image variants=%+v err=%v", imageVariants, err)
	}
	call(RouteComicImageVariantSelect, map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": section.UUID, "variant_uuid": imageVariants[0].UUID}, nil, map[string]any{"expected_revision": revision(section.Revision)})
	section, _ = productionService.GetSection(ctx, chapter.UUID, section.UUID)
	call(RouteComicSnapshotList, chapterParams, nil, nil)
	snapshots, err := productionService.ListChapterSnapshots(ctx, chapter.UUID)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("snapshots=%+v err=%v", snapshots, err)
	}
	snapshotParams := map[string]string{"chapter_uuid": chapter.UUID, "snapshot_uuid": snapshots[0].UUID}
	call(RouteComicSnapshotGet, snapshotParams, nil, nil)
	call(RouteComicSectionDelete, sectionParams, nil, map[string]any{"expected_revision": revision(section.Revision)})
	call(RouteComicSnapshotRestore, snapshotParams, nil, map[string]any{})
	restoredSections, err := productionService.ListSections(ctx, chapter.UUID)
	if err != nil || len(restoredSections) != 2 || restoredSections[0].PageRole != production.PageRoleBody || restoredSections[1].PageRole != production.PageRoleBody {
		t.Fatalf("snapshot restore lost page roles: sections=%+v err=%v", restoredSections, err)
	}

	call(RouteComicExportReadiness, nil, map[string]any{"scope": "chapter", "chapter_uuid": chapter.UUID}, nil)
	call(RouteComicExportList, nil, map[string]any{"page": float64(1), "per_page": float64(10)}, nil)
	productionTask := call(RouteComicExportCreate, nil, nil, map[string]any{"scope": "chapter", "chapter_uuid": chapter.UUID, "format": "zip", "allow_missing_images": true}).(map[string]any)
	storyTaskUUID, _ := storyTask["uuid"].(string)
	productionTaskUUID, _ := productionTask["uuid"].(string)
	call(RouteStoryTaskList, nil, map[string]any{"status": "queued", "limit": float64(10)}, nil)
	call(RouteStoryTaskEventList, map[string]string{"task_uuid": storyTaskUUID}, map[string]any{"limit": float64(10)}, nil)
	call(RouteStoryTaskCancel, map[string]string{"task_uuid": storyTaskUUID}, nil, map[string]any{})
	call(RouteStoryTaskRetry, map[string]string{"task_uuid": storyTaskUUID}, nil, map[string]any{})
	call(RouteProductionTaskList, nil, map[string]any{"status": "queued", "limit": float64(10)}, nil)
	call(RouteProductionTaskEventList, map[string]string{"task_uuid": productionTaskUUID}, map[string]any{"limit": float64(10)}, nil)
	call(RouteProductionTaskCancel, map[string]string{"task_uuid": productionTaskUUID}, nil, map[string]any{})
	call(RouteProductionTaskRetry, map[string]string{"task_uuid": productionTaskUUID}, nil, map[string]any{})

	assetUUID := settingImage.Asset.UUID
	projectAssetParams := map[string]string{"asset_uuid": assetUUID}
	call(RouteProjectAssetList, nil, map[string]any{"kind": "image", "limit": float64(20)}, nil)
	call(RouteProjectAssetGet, projectAssetParams, map[string]any{"include_trashed": false}, nil)
	call(RouteProjectAssetUpdate, projectAssetParams, nil, map[string]any{"display_name": "Phase 3 setting", "metadata": map[string]any{"label": "public"}})
	call(RouteProjectAssetTrash, projectAssetParams, nil, map[string]any{})
	call(RouteProjectAssetRestore, projectAssetParams, nil, map[string]any{})

	if len(executed) != len(phase3AgentAPIRoutes()) {
		missing := []string{}
		for _, route := range phase3AgentAPIRoutes() {
			if !executed[route.ID] {
				missing = append(missing, route.ID)
			}
		}
		t.Fatalf("executed=%d want=%d missing=%v", len(executed), len(phase3AgentAPIRoutes()), missing)
	}
}

func TestComicSectionAgentRoutesRoundTripPictureBookPageRoles(t *testing.T) {
	harness := newAgentHarnessWithPictureBook(t, &project.PictureBookInput{
		Format: project.PictureBookClassic, AspectRatio: &project.AspectRatioInput{Mode: project.AspectPortrait},
	})
	ctx := context.Background()
	chapter, err := story.NewService(harness.store).CreateChapter(ctx, story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Page roles", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	chapterParams := map[string]string{"chapter_uuid": chapter.UUID}
	create := func(title, role string) map[string]any {
		t.Helper()
		value, err := executePageRoleAgentRoute(t, harness, RouteComicSectionCreate, chapterParams, map[string]any{"title": title, "storyboard_md": "# " + title, "page_role": role})
		if err != nil {
			t.Fatalf("create %s: %v", role, err)
		}
		result, _ := value.(map[string]any)
		if !isUUIDv7(stringArg(result, "uuid")) || stringArg(result, "page_role") != role {
			t.Fatalf("created %s result=%+v", role, result)
		}
		return result
	}

	if _, err := executePageRoleAgentRoute(t, harness, RouteComicSectionCreate, chapterParams, map[string]any{"title": "Premature front", "page_role": production.PageRoleFrontCover}); err == nil {
		t.Fatal("Agent route created a front cover before the first body page")
	}
	bodyOne := create("Body one", production.PageRoleBody)
	front := create("Front cover", production.PageRoleFrontCover)
	bodyTwo := create("Body two", production.PageRoleBody)
	back := create("Back cover", production.PageRoleBackCover)
	if _, err := executePageRoleAgentRoute(t, harness, RouteComicSectionCreate, chapterParams, map[string]any{"title": "Duplicate front", "page_role": production.PageRoleFrontCover}); err == nil {
		t.Fatal("Agent route created a duplicate active front cover")
	}

	backParams := map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": stringArg(back, "uuid")}
	updated, err := executePageRoleAgentRoute(t, harness, RouteComicSectionUpdate, backParams, map[string]any{"page_role": production.PageRoleBody, "expected_revision": float64(intArg(back, "revision"))})
	if err != nil || stringArg(updated.(map[string]any), "page_role") != production.PageRoleBody {
		t.Fatalf("update back cover to body: value=%+v err=%v", updated, err)
	}
	updatedMap := updated.(map[string]any)
	updated, err = executePageRoleAgentRoute(t, harness, RouteComicSectionUpdate, backParams, map[string]any{"page_role": production.PageRoleBackCover, "expected_revision": float64(intArg(updatedMap, "revision"))})
	if err != nil || stringArg(updated.(map[string]any), "page_role") != production.PageRoleBackCover {
		t.Fatalf("restore back-cover role: value=%+v err=%v", updated, err)
	}

	reordered, err := executePageRoleAgentRoute(t, harness, RouteComicSectionReorder, chapterParams, map[string]any{"section_uuids": []any{stringArg(bodyTwo, "uuid"), stringArg(bodyOne, "uuid")}})
	if err != nil {
		t.Fatal(err)
	}
	assertAgentSectionRoles(t, reordered, []string{production.PageRoleFrontCover, production.PageRoleBody, production.PageRoleBody, production.PageRoleBackCover})
	items := reordered.(map[string]any)["items"].([]any)
	if stringArg(items[1].(map[string]any), "uuid") != stringArg(bodyTwo, "uuid") || stringArg(items[2].(map[string]any), "uuid") != stringArg(bodyOne, "uuid") {
		t.Fatalf("body reorder result=%+v", reordered)
	}

	readBody, err := executePageRoleAgentRoute(t, harness, RouteComicSectionGet, map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": stringArg(bodyTwo, "uuid")}, nil)
	if err != nil || stringArg(readBody.(map[string]any), "page_role") != production.PageRoleBody {
		t.Fatalf("read body role: value=%+v err=%v", readBody, err)
	}

	productionService := production.NewService(harness.store, nil)
	snapshots, err := productionService.ListChapterSnapshots(ctx, chapter.UUID)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("snapshots=%+v err=%v", snapshots, err)
	}
	snapshotParams := map[string]string{"chapter_uuid": chapter.UUID, "snapshot_uuid": snapshots[0].UUID}
	detail, err := executePageRoleAgentRoute(t, harness, RouteComicSnapshotGet, snapshotParams, nil)
	if err != nil {
		t.Fatal(err)
	}
	detailSections := detail.(map[string]any)["sections"].([]any)
	if len(detailSections) != 4 || stringArg(detailSections[0].(map[string]any), "page_role") != production.PageRoleFrontCover || stringArg(detailSections[3].(map[string]any), "page_role") != production.PageRoleBackCover {
		t.Fatalf("snapshot detail lost roles: %+v", detail)
	}

	frontParams := map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": stringArg(front, "uuid")}
	if _, err := executePageRoleAgentRoute(t, harness, RouteComicSectionDelete, frontParams, map[string]any{"expected_revision": float64(intArg(front, "revision"))}); err != nil {
		t.Fatal(err)
	}
	restored, err := executePageRoleAgentRoute(t, harness, RouteComicSnapshotRestore, snapshotParams, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	assertAgentSectionRoles(t, restored, []string{production.PageRoleFrontCover, production.PageRoleBody, production.PageRoleBody, production.PageRoleBackCover})
	restoredItems := restored.(map[string]any)["items"].([]any)
	firstRestoredBody := restoredItems[1].(map[string]any)
	firstRestoredBodyParams := map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": stringArg(firstRestoredBody, "uuid")}
	if _, err := executePageRoleAgentRoute(t, harness, RouteComicSectionDelete, firstRestoredBodyParams, map[string]any{"expected_revision": float64(intArg(firstRestoredBody, "revision"))}); err != nil {
		t.Fatalf("delete one of two classic body pages: %v", err)
	}
	remaining, err := executePageRoleAgentRoute(t, harness, RouteComicSectionList, chapterParams, nil)
	if err != nil {
		t.Fatal(err)
	}
	remainingItems := remaining.(map[string]any)["items"].([]any)
	var lastBody map[string]any
	for _, item := range remainingItems {
		section := item.(map[string]any)
		if stringArg(section, "page_role") == production.PageRoleBody {
			lastBody = section
			break
		}
	}
	if lastBody == nil {
		t.Fatalf("classic picture book lost every body page: %+v", remaining)
	}
	lastBodyParams := map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": stringArg(lastBody, "uuid")}
	if _, err := executePageRoleAgentRoute(t, harness, RouteComicSectionDelete, lastBodyParams, map[string]any{"expected_revision": float64(intArg(lastBody, "revision"))}); err == nil {
		t.Fatal("classic picture-book Agent route deleted the last body page")
	}
}

func TestComicSectionAgentRouteRejectsCoverForVerticalStrip(t *testing.T) {
	harness := newAgentHarness(t)
	chapter, err := story.NewService(harness.store).CreateChapter(context.Background(), story.CreateChapterInput{ChapterCode: "vol01.ch01", Title: "Strip", Content: "Story", ContentFormat: "md"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = executePageRoleAgentRoute(t, harness, RouteComicSectionCreate, map[string]string{"chapter_uuid": chapter.UUID}, map[string]any{"title": "Invalid cover", "page_role": production.PageRoleFrontCover})
	if err == nil {
		t.Fatal("vertical_strip Agent route accepted a front cover")
	}
	chapterParams := map[string]string{"chapter_uuid": chapter.UUID}
	body, err := executePageRoleAgentRoute(t, harness, RouteComicSectionCreate, chapterParams, map[string]any{"title": "Only panel", "page_role": production.PageRoleBody})
	if err != nil {
		t.Fatalf("create vertical_strip body: %v", err)
	}
	bodyMap := body.(map[string]any)
	bodyParams := map[string]string{"chapter_uuid": chapter.UUID, "section_uuid": stringArg(bodyMap, "uuid")}
	if _, err := executePageRoleAgentRoute(t, harness, RouteComicSectionDelete, bodyParams, map[string]any{"expected_revision": float64(intArg(bodyMap, "revision"))}); err != nil {
		t.Fatalf("vertical_strip should allow deleting its final body: %v", err)
	}
	remaining, err := executePageRoleAgentRoute(t, harness, RouteComicSectionList, chapterParams, nil)
	if err != nil {
		t.Fatal(err)
	}
	if items := remaining.(map[string]any)["items"].([]any); len(items) != 0 {
		t.Fatalf("vertical_strip should return to empty after deleting its final body: %+v", remaining)
	}
}

func executePageRoleAgentRoute(t *testing.T, harness *agentHarness, routeID string, params map[string]string, body map[string]any) (any, error) {
	t.Helper()
	var route agentAPIRoute
	for _, candidate := range agentAPIRoutes() {
		if candidate.ID == routeID {
			route = candidate
			break
		}
	}
	if route.ID == "" {
		t.Fatalf("unknown route %s", routeID)
	}
	path := strings.ReplaceAll(route.PathTemplate, "{project_uuid}", harness.project.UUID)
	for key, value := range params {
		path = strings.ReplaceAll(path, "{"+key+"}", value)
	}
	args := map[string]any{"method": route.Method, "url": path, "response_filter": recommendedAgentAPIResponseFilter(route)}
	if body != nil {
		args["request_body"] = body
	}
	tc := toolContext{ProjectUUID: harness.project.UUID, ToolMode: ToolModeProjectAPI, Thread: threadRecord{UUID: mustAgentUUID(t), Scope: ThreadScopeProject}}
	request, err := parseAgentAPIRequest(tc, args)
	if err != nil {
		return nil, err
	}
	value, err := executeAgentAPIRoute(context.Background(), harness.service, harness.store, tc, toolExecutionRecord{UUID: mustAgentUUID(t), IdempotencyKey: "page-role:" + routeID + ":" + mustAgentUUID(t)}, request)
	if err != nil {
		return nil, err
	}
	return compactAgentRouteValue(route, value)
}

func assertAgentSectionRoles(t *testing.T, value any, want []string) {
	t.Helper()
	root, _ := value.(map[string]any)
	items, _ := root["items"].([]any)
	if len(items) != len(want) {
		t.Fatalf("section count=%d want=%d value=%+v", len(items), len(want), value)
	}
	for index, role := range want {
		item, _ := items[index].(map[string]any)
		if stringArg(item, "page_role") != role || intArg(item, "section_no") != int64(index+1) {
			t.Fatalf("section %d=%+v want role=%s section_no=%d", index, item, role, index+1)
		}
	}
}

func TestPhase3WriteIntentIsIdempotentAcrossRestart(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Phase 3 idempotency", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "创建章节"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, tc)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"method":"POST","url":"/api/v1/projects/` + harness.project.UUID + `/chapters","request_body":{"chapter_code":"vol10.ch01","title":"Idempotent chapter","content":"once","content_format":"md"},"response_filter":".data | {uuid,chapter_code,title,revision}"}`
	execution, replay, completed, err := harness.service.persistToolIntent(ctx, harness.store, tc, "phase3-write-call", "request_api", raw)
	if err != nil || completed || replay != nil {
		t.Fatalf("first intent=%+v replay=%s completed=%v err=%v", execution, replay, completed, err)
	}
	result, err := harness.service.executeTool(ctx, harness.store, tc, execution)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.persistToolResult(ctx, harness.store, tc, execution, result); err != nil {
		t.Fatal(err)
	}
	restarted, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	restarted.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, restarted)
	if err != nil || restarted.ToolMode != ToolModeProjectAPI {
		t.Fatalf("restart mode=%q err=%v", restarted.ToolMode, err)
	}
	replayedExecution, replay, completed, err := harness.service.persistToolIntent(ctx, harness.store, restarted, "phase3-write-call", "request_api", raw)
	if err != nil || !completed || replayedExecution.ID != execution.ID || string(replay) != string(result) {
		t.Fatalf("replay execution=%+v completed=%v replay=%s err=%v", replayedExecution, completed, replay, err)
	}
	chapters, err := story.NewService(harness.store).ListChapters(ctx, "active")
	if err != nil || len(chapters) != 1 || chapters[0].ChapterCode != "vol10.ch01" {
		t.Fatalf("chapters=%+v err=%v", chapters, err)
	}
}

func TestEveryPhase3RouteIntentMetadataAndResultRecoverAcrossRestart(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Phase 3 route recovery", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "exercise every phase 3 route intent"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, tc)
	if err != nil || tc.ToolMode != ToolModeProjectAPI {
		t.Fatalf("tool mode=%q err=%v", tc.ToolMode, err)
	}

	type persistedRoute struct {
		route     agentAPIRoute
		callID    string
		raw       string
		execution toolExecutionRecord
		result    json.RawMessage
	}
	resourceUUID := mustAgentUUID(t)
	persisted := make([]persistedRoute, 0, len(phase3AgentAPIRoutes()))
	for _, route := range phase3AgentAPIRoutes() {
		args := map[string]any{
			"method":          route.Method,
			"url":             phase3TestPath(route.PathTemplate, harness.project.UUID, resourceUUID),
			"response_filter": recommendedAgentAPIResponseFilter(route),
		}
		if route.QuerySchema != nil {
			args["query"] = phase3TestSchemaObject(route.QuerySchema, resourceUUID)
		}
		if route.BodySchema != nil {
			args["request_body"] = phase3TestSchemaObject(route.BodySchema, resourceUUID)
		}
		encoded, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		callID := "phase3-recovery:" + route.ID
		execution, replay, completed, err := harness.service.persistToolIntent(ctx, harness.store, tc, callID, "request_api", string(encoded))
		if err != nil || completed || replay != nil {
			t.Fatalf("persist %s: execution=%+v completed=%v replay=%s err=%v", route.ID, execution, completed, replay, err)
		}
		if execution.RouteID != route.ID || execution.Action != route.Action || execution.Method != route.Method || execution.Path != args["url"] || execution.TargetUUID == "" {
			t.Fatalf("route metadata mismatch for %s: %+v", route.ID, execution)
		}
		result := json.RawMessage(`{"success":true,"data":{"recovered_route":"` + route.ID + `"}}`)
		if err := harness.service.persistToolResult(ctx, harness.store, tc, execution, result); err != nil {
			t.Fatalf("persist result %s: %v", route.ID, err)
		}
		persisted = append(persisted, persistedRoute{route: route, callID: callID, raw: string(encoded), execution: execution, result: result})
	}

	restarted, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	restarted.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, restarted)
	if err != nil || restarted.ToolMode != ToolModeProjectAPI {
		t.Fatalf("restart mode=%q err=%v", restarted.ToolMode, err)
	}
	for _, item := range persisted {
		execution, replay, completed, err := harness.service.persistToolIntent(ctx, harness.store, restarted, item.callID, "request_api", item.raw)
		if err != nil || !completed || execution.ID != item.execution.ID || string(replay) != string(item.result) {
			t.Fatalf("recover %s: execution=%+v completed=%v replay=%s err=%v", item.route.ID, execution, completed, replay, err)
		}
	}
}

func TestEveryPhase3DangerousRouteRequiresFingerprintConfirmation(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: "Phase 3 dangerous policy", ProviderUUID: harness.provider.UUID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "请删除"})
	if err != nil {
		t.Fatal(err)
	}
	tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.claimRun(ctx, harness.store, &tc); err != nil {
		t.Fatal(err)
	}
	tc.ToolMode, err = harness.service.loadRunToolMode(ctx, harness.store, tc)
	if err != nil {
		t.Fatal(err)
	}

	resourceUUID := mustAgentUUID(t)
	checked := 0
	for _, route := range phase3AgentAPIRoutes() {
		if !route.RequiresConfirmation || route.Risk != RiskDangerous {
			continue
		}
		args := map[string]any{"method": route.Method, "url": phase3TestPath(route.PathTemplate, harness.project.UUID, resourceUUID), "response_filter": recommendedAgentAPIResponseFilter(route)}
		if route.QuerySchema != nil {
			args["query"] = phase3TestSchemaObject(route.QuerySchema, resourceUUID)
		}
		if route.BodySchema != nil {
			args["request_body"] = phase3TestSchemaObject(route.BodySchema, resourceUUID)
		}
		encoded, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		execution, _, _, err := harness.service.persistToolIntent(ctx, harness.store, tc, "phase3-dangerous:"+route.ID, "request_api", string(encoded))
		if err != nil {
			t.Fatalf("persist %s: %v", route.ID, err)
		}
		result, err := harness.service.executeTool(ctx, harness.store, tc, execution)
		if err != nil {
			t.Fatalf("execute %s: %v", route.ID, err)
		}
		if !bytes.Contains(result, []byte(`"code":"`+CodeToolConfirmation+`"`)) || !bytes.Contains(result, []byte("request_fingerprint")) || !bytes.Contains(result, []byte(route.ID)) {
			t.Fatalf("route %s did not return an exact fingerprint confirmation: %s", route.ID, result)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no Phase 3 dangerous routes were checked")
	}
}

func phase3TestPath(template, projectUUID, resourceUUID string) string {
	path := strings.ReplaceAll(template, "{project_uuid}", projectUUID)
	for _, name := range []string{"chapter_uuid", "section_uuid", "source_uuid", "setting_image_uuid", "premise_asset_uuid", "variant_uuid", "snapshot_uuid", "task_uuid", "asset_uuid"} {
		path = strings.ReplaceAll(path, "{"+name+"}", resourceUUID)
	}
	return path
}

func phase3TestSchemaObject(schema map[string]any, resourceUUID string) map[string]any {
	result := map[string]any{}
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]string)
	for _, key := range required {
		child, _ := properties[key].(map[string]any)
		result[key] = phase3TestSchemaValue(key, child, resourceUUID)
	}
	return result
}

func phase3TestSchemaValue(key string, schema map[string]any, resourceUUID string) any {
	if values, ok := schema["enum"].([]string); ok && len(values) > 0 {
		return values[0]
	}
	switch schema["type"] {
	case "integer":
		minimum, _ := schema["minimum"].(int)
		return float64(minimum)
	case "boolean":
		return true
	case "array":
		item, _ := schema["items"].(map[string]any)
		return []any{phase3TestSchemaValue(strings.TrimSuffix(key, "s"), item, resourceUUID)}
	case "object":
		return phase3TestSchemaObject(schema, resourceUUID)
	default:
		if strings.HasSuffix(key, "_uuid") || strings.HasSuffix(key, "_uuids") {
			return resourceUUID
		}
		return "test"
	}
}

func phase3TestProjectorValue(projectorKey, resourceUUID string) any {
	projector, _ := agentAPIProjectorByKey(projectorKey)
	itemProjector := projector
	if projector.List {
		itemProjector, _ = agentAPIProjectorByKey(projector.ItemProjector)
	}
	item := map[string]any{"internal_id": float64(9), "secret": "must-not-leak"}
	for _, field := range itemProjector.Fields {
		switch field.Type {
		case "integer":
			item[field.Name] = float64(1)
		case "object | null", "object":
			item[field.Name] = map[string]any{"uuid": resourceUUID, "internal_id": float64(9), "content_url": "must-not-leak", "download_url": "must-not-leak"}
		default:
			if strings.HasSuffix(field.Name, "uuid") {
				item[field.Name] = resourceUUID
			} else {
				item[field.Name] = "public"
			}
		}
	}
	if projector.List {
		return map[string]any{"items": []any{item}, "ignored_id": float64(7)}
	}
	return item
}

func mergePhase3Args(input map[string]any, key string, value any) map[string]any {
	result := cloneToolArguments(input)
	result[key] = value
	return result
}
