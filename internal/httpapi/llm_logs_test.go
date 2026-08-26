package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func newPublicUUID(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}

func TestProjectLLMLogsUnifyStoryAndChatCallsWithoutInternalIDs(t *testing.T) {
	ctx := context.Background()
	dataDirectory := filepath.Join(t.TempDir(), "app")
	appStore, err := appstore.Open(dataDirectory, config.SQLiteDSN(filepath.Join(dataDirectory, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(appStore)
	created, err := manager.Create(ctx, "LLM Logs", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close(); _ = appStore.Close() })

	providerUUID := newPublicUUID(t)
	storyLogUUID := newPublicUUID(t)
	chatLogUUID := newPublicUUID(t)
	premiseLogUUID := newPublicUUID(t)
	premiseProductionLogUUID := newPublicUUID(t)
	comicProductionLogUUID := newPublicUUID(t)
	workflowLogUUID := newPublicUUID(t)
	storyCreatedAt := time.Now().UTC().Add(-time.Minute)
	chatCreatedAt := storyCreatedAt.Add(30 * time.Second)
	premiseCreatedAt := chatCreatedAt.Add(15 * time.Second)
	premiseProductionCreatedAt := premiseCreatedAt.Add(5 * time.Second)
	comicProductionCreatedAt := premiseProductionCreatedAt.Add(5 * time.Second)
	workflowCreatedAt := comicProductionCreatedAt.Add(5 * time.Second)
	err = manager.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		var projectID int64
		if err := store.DB().Model(&project.Project{}).Where("uuid = ?", created.UUID).Pluck("id", &projectID).Error; err != nil {
			return err
		}
		taskUUID, resourceUUID := newPublicUUID(t), newPublicUUID(t)
		if err := store.DB().Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,retryable,provider_uuid,model,progress,attempt,max_attempts,error_code,error_message,created_at,updated_at) VALUES(?,?,?,?,1,'{}','completed','logs-test',0,?,'story-model',100,1,1,'','',?,?)`, taskUUID, projectID, "story_chapter_generation", resourceUUID, providerUUID, storyCreatedAt, storyCreatedAt).Error; err != nil {
			return err
		}
		var taskID int64
		if err := store.DB().Table("task_runs").Where("uuid = ?", taskUUID).Pluck("id", &taskID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO llm_logs(uuid,project_id,task_run_id,source_type,scenario,request_type,provider_uuid,provider_type,model,status,input_summary,output_summary,request_payload,response,input_tokens,cached_input_tokens,output_tokens,input_characters,output_characters,duration_ms,finish_reason,error_code,created_at,completed_at) VALUES(?,?,?,'story_generation','story_chapter_generation','text',?,'openai_compatible','story-model','completed','input','output','{"model":"story-model","prompt":"full input"}','{"content":"full output"}',11,4,7,15,11,1250,'stop','',?,?)`, storyLogUUID, projectID, taskID, providerUUID, storyCreatedAt, storyCreatedAt).Error; err != nil {
			return err
		}

		threadUUID, turnUUID, runUUID := newPublicUUID(t), newPublicUUID(t), newPublicUUID(t)
		if err := store.DB().Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(?,?,'Assistant','idle',?,'chat-model',2,1,1,?,?)`, threadUUID, projectID, providerUUID, chatCreatedAt, chatCreatedAt).Error; err != nil {
			return err
		}
		var threadID int64
		if err := store.DB().Table("chat_threads").Where("uuid = ?", threadUUID).Pluck("id", &threadID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO chat_turns(uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(?,?,'prompt',1,'Hello','completed',?,?)`, turnUUID, threadID, chatCreatedAt, chatCreatedAt).Error; err != nil {
			return err
		}
		var turnID int64
		if err := store.DB().Table("chat_turns").Where("uuid = ?", turnUUID).Pluck("id", &turnID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO chat_runs(uuid,thread_id,turn_id,trigger_type,status,step_count,max_steps,provider_uuid,model,context_bytes,created_at,updated_at) VALUES(?,?,?,'prompt','completed',1,12,?,'chat-model',100,?,?)`, runUUID, threadID, turnID, providerUUID, chatCreatedAt, chatCreatedAt).Error; err != nil {
			return err
		}
		var runID int64
		if err := store.DB().Table("chat_runs").Where("uuid = ?", runUUID).Pluck("id", &runID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO llm_logs(uuid,project_id,chat_thread_id,chat_run_id,source_type,scenario,request_type,provider_uuid,provider_type,model,status,input_tokens,output_tokens,duration_ms,finish_reason,error_code,created_at,completed_at) VALUES(?,?,?,?,'project_chat','project_chat','text',?,'openai_compatible','chat-model','completed',23,9,640,'stop','',?,?)`, chatLogUUID, projectID, threadID, runID, providerUUID, chatCreatedAt, chatCreatedAt).Error; err != nil {
			return err
		}

		premiseThreadUUID, premiseTurnUUID, premiseRunUUID := newPublicUUID(t), newPublicUUID(t), newPublicUUID(t)
		if err := store.DB().Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(?,?,'Premise Assistant','idle',?,'chat-model',2,1,1,?,?)`, premiseThreadUUID, projectID, providerUUID, premiseCreatedAt, premiseCreatedAt).Error; err != nil {
			return err
		}
		var premiseThreadID int64
		if err := store.DB().Table("chat_threads").Where("uuid = ?", premiseThreadUUID).Pluck("id", &premiseThreadID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO chat_turns(uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(?,?,'prompt',1,'Generate asset','completed',?,?)`, premiseTurnUUID, premiseThreadID, premiseCreatedAt, premiseCreatedAt).Error; err != nil {
			return err
		}
		var premiseTurnID int64
		if err := store.DB().Table("chat_turns").Where("uuid = ?", premiseTurnUUID).Pluck("id", &premiseTurnID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO chat_runs(uuid,thread_id,turn_id,trigger_type,status,step_count,max_steps,provider_uuid,model,context_bytes,created_at,updated_at) VALUES(?,?,?,'prompt','completed',1,12,?,'chat-model',100,?,?)`, premiseRunUUID, premiseThreadID, premiseTurnID, providerUUID, premiseCreatedAt, premiseCreatedAt).Error; err != nil {
			return err
		}
		var premiseRunID int64
		if err := store.DB().Table("chat_runs").Where("uuid = ?", premiseRunUUID).Pluck("id", &premiseRunID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO llm_logs(uuid,project_id,chat_thread_id,chat_run_id,source_type,scenario,request_type,provider_uuid,provider_type,model,status,input_tokens,output_tokens,duration_ms,finish_reason,error_code,created_at,completed_at) VALUES(?,?,?,?,'project_chat','premise_asset_generation','text',?,'openai_compatible','chat-model','completed',31,12,880,'stop','',?,?)`, premiseLogUUID, projectID, premiseThreadID, premiseRunID, providerUUID, premiseCreatedAt, premiseCreatedAt).Error; err != nil {
			return err
		}

		premiseTaskUUID, comicTaskUUID := newPublicUUID(t), newPublicUUID(t)
		if err := store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'premise_setting_generation',?,'{}','completed','premise-production-log',?,'image-model',100,1,3,?,?)`, premiseTaskUUID, projectID, newPublicUUID(t), providerUUID, premiseProductionCreatedAt, premiseProductionCreatedAt).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'comic_image_generation',?,'{}','failed','comic-production-log',?,'image-model',100,1,3,?,?)`, comicTaskUUID, projectID, newPublicUUID(t), providerUUID, comicProductionCreatedAt, comicProductionCreatedAt).Error; err != nil {
			return err
		}
		var premiseTaskID, comicTaskID int64
		if err := store.DB().Table("production_task_runs").Where("uuid = ?", premiseTaskUUID).Pluck("id", &premiseTaskID).Error; err != nil {
			return err
		}
		if err := store.DB().Table("production_task_runs").Where("uuid = ?", comicTaskUUID).Pluck("id", &comicTaskID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO llm_logs(uuid,project_id,production_task_run_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,input_summary,output_summary,duration_ms,created_at,completed_at) VALUES(?,?,?,'production','premise_setting_generation','image',1,?,'openai_compatible','image-model','completed','prompt; reference_images=0','mime_type=image/png; byte_size=64',400,?,?)`, premiseProductionLogUUID, projectID, premiseTaskID, providerUUID, premiseProductionCreatedAt, premiseProductionCreatedAt).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO llm_logs(uuid,project_id,production_task_run_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,error_code,error_message,http_status,provider_error_code,provider_request_id,duration_ms,created_at,completed_at) VALUES(?,?,?,'production','comic_image_generation','image',1,?,'openai_compatible','image-model','failed','image_provider_error','safe provider message',400,'InvalidParameter','image-request-400',250,?,?)`, comicProductionLogUUID, projectID, comicTaskID, providerUUID, comicProductionCreatedAt, comicProductionCreatedAt).Error; err != nil {
			return err
		}

		workflowUUID, workflowStepUUID := newPublicUUID(t), newPublicUUID(t)
		if err := store.DB().Exec(`INSERT INTO workflows(uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at) VALUES(?,?,?,'yolo_project_initialization','Yolo','completed',1,'{}','workflow-log-test',?,'chat-model',?,?)`, workflowUUID, projectID, threadID, providerUUID, workflowCreatedAt, workflowCreatedAt).Error; err != nil {
			return err
		}
		var workflowID int64
		if err := store.DB().Table("workflows").Where("uuid = ?", workflowUUID).Pluck("id", &workflowID).Error; err != nil {
			return err
		}
		if err := store.DB().Exec(`INSERT INTO workflow_steps(uuid,workflow_id,step_key,position,status,idempotency_key,input_json,output_json,created_at,updated_at) VALUES(?,?,'story',1,'completed','workflow-step-log-test','{}','{}',?,?)`, workflowStepUUID, workflowID, workflowCreatedAt, workflowCreatedAt).Error; err != nil {
			return err
		}
		var workflowStepID int64
		if err := store.DB().Table("workflow_steps").Where("uuid = ?", workflowStepUUID).Pluck("id", &workflowStepID).Error; err != nil {
			return err
		}
		return store.DB().Exec(`INSERT INTO llm_logs(uuid,project_id,workflow_id,workflow_step_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,input_tokens,output_tokens,duration_ms,finish_reason,created_at,completed_at) VALUES(?,?,?,?,'workflow','story_profile_generation','text',1,?,'openai_compatible','chat-model','completed',20,8,330,'stop',?,?)`, workflowLogUUID, projectID, workflowID, workflowStepID, providerUUID, workflowCreatedAt, workflowCreatedAt).Error
	})
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler
	handler := NewLLMLogHandler(manager)
	e.GET("/api/v1/projects/:project_uuid/llm-logs", handler.Index)
	e.GET("/api/v1/projects/:project_uuid/llm-logs/:log_uuid", handler.Show)
	first := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=1&per_page=1", nil)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"uuid":"`+workflowLogUUID+`"`) || !strings.Contains(first.Body.String(), `"workflow_uuid":"`) || !strings.Contains(first.Body.String(), `"workflow_step_uuid":"`) || !strings.Contains(first.Body.String(), `"pagination":{"per_page":1,"current_page":1,"last_page":6,"total":6}`) {
		t.Fatalf("first page = %d %s", first.Code, first.Body.String())
	}
	if !strings.Contains(first.Body.String(), `"request_type":"text"`) || !strings.Contains(first.Body.String(), `"attempt":1`) || !strings.Contains(first.Body.String(), `"http_status":0`) {
		t.Fatalf("LLM log diagnostics missing: %s", first.Body.String())
	}
	if !strings.Contains(first.Body.String(), `"filter_groups":{"providers":[`) || !strings.Contains(first.Body.String(), `"provider_types":["openai_compatible"]`) || !strings.Contains(first.Body.String(), `"request_types":["image","text"]`) {
		t.Fatalf("LLM log filter groups missing: %s", first.Body.String())
	}
	if strings.Contains(first.Body.String(), `"id":`) {
		t.Fatalf("LLM log response leaked internal id: %s", first.Body.String())
	}
	if strings.Contains(first.Body.String(), `"request_payload":`) || strings.Contains(first.Body.String(), `"response":`) {
		t.Fatalf("LLM log list included large detail fields: %s", first.Body.String())
	}
	detail := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs/"+storyLogUUID, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"request_payload":{"model":"story-model","prompt":"full input"}`) || !strings.Contains(detail.Body.String(), `"response":{"content":"full output"}`) || strings.Contains(detail.Body.String(), `"id":`) {
		t.Fatalf("LLM log detail = %d %s", detail.Code, detail.Body.String())
	}
	if !strings.Contains(detail.Body.String(), `"cached_input_tokens":4`) || !strings.Contains(detail.Body.String(), `"input_characters":15`) || !strings.Contains(detail.Body.String(), `"output_characters":11`) || !strings.Contains(detail.Body.String(), `"output_tokens_per_second":5.6`) || !strings.Contains(detail.Body.String(), `"output_characters_per_second":8.8`) {
		t.Fatalf("LLM usage metrics missing: %s", detail.Body.String())
	}
	legacyDetail := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs/"+workflowLogUUID, nil)
	if legacyDetail.Code != http.StatusOK || !strings.Contains(legacyDetail.Body.String(), `"request_payload":null`) || !strings.Contains(legacyDetail.Body.String(), `"response":null`) {
		t.Fatalf("legacy LLM log detail = %d %s", legacyDetail.Code, legacyDetail.Body.String())
	}
	missingDetail := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs/"+newPublicUUID(t), nil)
	if missingDetail.Code != http.StatusNotFound || !strings.Contains(missingDetail.Body.String(), `"code":"llm_log_not_found"`) {
		t.Fatalf("missing LLM log detail = %d %s", missingDetail.Code, missingDetail.Body.String())
	}
	projectOnly := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=1&per_page=5&scope=project", nil)
	if projectOnly.Code != http.StatusOK || !strings.Contains(projectOnly.Body.String(), `"uuid":"`+chatLogUUID+`"`) || !strings.Contains(projectOnly.Body.String(), premiseLogUUID) || !strings.Contains(projectOnly.Body.String(), comicProductionLogUUID) || !strings.Contains(projectOnly.Body.String(), workflowLogUUID) || !strings.Contains(projectOnly.Body.String(), `"scope":"project"`) || strings.Contains(projectOnly.Body.String(), premiseProductionLogUUID) || strings.Contains(projectOnly.Body.String(), storyLogUUID) || !strings.Contains(projectOnly.Body.String(), `"provider_error_code":"InvalidParameter"`) || !strings.Contains(projectOnly.Body.String(), `"provider_request_id":"image-request-400"`) || !strings.Contains(projectOnly.Body.String(), `"total":4`) {
		t.Fatalf("project scope = %d %s", projectOnly.Code, projectOnly.Body.String())
	}
	premiseOnly := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=1&per_page=5&scope=premise", nil)
	if premiseOnly.Code != http.StatusOK || !strings.Contains(premiseOnly.Body.String(), premiseProductionLogUUID) || !strings.Contains(premiseOnly.Body.String(), `"scope":"premise"`) || strings.Contains(premiseOnly.Body.String(), premiseLogUUID) || strings.Contains(premiseOnly.Body.String(), chatLogUUID) || strings.Contains(premiseOnly.Body.String(), comicProductionLogUUID) || strings.Contains(premiseOnly.Body.String(), workflowLogUUID) || !strings.Contains(premiseOnly.Body.String(), `"total":1`) {
		t.Fatalf("premise scope = %d %s", premiseOnly.Code, premiseOnly.Body.String())
	}
	filtered := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=1&per_page=5&provider_uuid="+providerUUID+"&provider_type=openai_compatible&model=story-model&scenario=story_chapter_generation&status=completed&request_type=text&keyword=input", nil)
	if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), `"uuid":"`+storyLogUUID+`"`) || !strings.Contains(filtered.Body.String(), `"total":1`) || strings.Contains(filtered.Body.String(), chatLogUUID) {
		t.Fatalf("combined filter = %d %s", filtered.Code, filtered.Body.String())
	}
	imageMetrics := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=1&request_type=image", nil)
	if imageMetrics.Code != http.StatusOK || !strings.Contains(imageMetrics.Body.String(), `"cached_input_tokens":null`) || !strings.Contains(imageMetrics.Body.String(), `"output_tokens_per_second":null`) {
		t.Fatalf("image metrics should be unavailable: %d %s", imageMetrics.Code, imageMetrics.Body.String())
	}
	last := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=6&per_page=1", nil)
	if last.Code != http.StatusOK || !strings.Contains(last.Body.String(), `"uuid":"`+storyLogUUID+`"`) || !strings.Contains(last.Body.String(), `"task_uuid":"`) {
		t.Fatalf("last page = %d %s", last.Code, last.Body.String())
	}
	invalid := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=0", nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid page = %d %s", invalid.Code, invalid.Body.String())
	}
	invalidScope := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=1&scope=admin", nil)
	if invalidScope.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid scope = %d %s", invalidScope.Code, invalidScope.Body.String())
	}
	invalidFilter := requestJSON(t, e, http.MethodGet, "/api/v1/projects/"+created.UUID+"/llm-logs?page=1&request_type=audio", nil)
	if invalidFilter.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid filter = %d %s", invalidFilter.Code, invalidFilter.Body.String())
	}
}
