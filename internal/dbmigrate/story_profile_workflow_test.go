package dbmigrate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func TestStoryProfileWorkflowMigrationPreservesGraphAndCosts(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "profiles.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.migrator.Migrate(20260905000038); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const (
		projectUUID          = "01a06a00-0000-7000-8000-000000000001"
		providerUUID         = "01a06a00-0000-7000-8000-000000000002"
		conversationUUID     = "01a06a00-0000-7000-8000-000000000003"
		batchThreadUUID      = "01a06a00-0000-7000-8000-000000000004"
		turnUUID             = "01a06a00-0000-7000-8000-000000000005"
		runUUID              = "01a06a00-0000-7000-8000-000000000006"
		existingItemUUID     = "01a06a00-0000-7000-8000-000000000007"
		batchItemUUID        = "01a06a00-0000-7000-8000-000000000008"
		existingToolUUID     = "01a06a00-0000-7000-8000-000000000009"
		batchToolUUID        = "01a06a00-0000-7000-8000-000000000010"
		existingToolCallUUID = "01a06a00-0000-7000-8000-000000000011"
		batchToolCallUUID    = "01a06a00-0000-7000-8000-000000000012"
		existingWorkflowUUID = "01a06a00-0000-7000-8000-000000000013"
		batchWorkflowUUID    = "01a06a00-0000-7000-8000-000000000014"
		inlineWorkflowUUID   = "01a06a00-0000-7000-8000-000000000015"
		existingStepUUID     = "01a06a00-0000-7000-8000-000000000016"
		batchStepUUID        = "01a06a00-0000-7000-8000-000000000017"
		inlineStepUUID       = "01a06a00-0000-7000-8000-000000000018"
		existingEventUUID    = "01a06a00-0000-7000-8000-000000000019"
		existingAwaitUUID    = "01a06a00-0000-7000-8000-000000000020"
		batchAwaitUUID       = "01a06a00-0000-7000-8000-000000000021"
		existingLogUUID      = "01a06a00-0000-7000-8000-000000000022"
		taskUUID             = "01a06a00-0000-7000-8000-000000000023"
		resourceUUID         = "01a06a00-0000-7000-8000-000000000024"
		now                  = "2026-09-04T00:00:00Z"
	)
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'` + projectUUID + `','Batch workflow migration',1,37,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,model_source,thread_type,created_at,updated_at) VALUES(1,'` + conversationUUID + `',1,'Conversation','completed','` + providerUUID + `','model','provider_default','conversation','` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,model_source,thread_type,created_at,updated_at) VALUES(2,'` + batchThreadUUID + `',1,'Batch','completed','` + providerUUID + `','model','provider_default','workflow','` + now + `','` + now + `')`,
		`INSERT INTO chat_turns(id,uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(1,'` + turnUUID + `',1,'prompt',1,'Generate images','completed','` + now + `','` + now + `')`,
		`INSERT INTO chat_runs(id,uuid,thread_id,turn_id,trigger_type,status,provider_uuid,model,model_source,created_at,updated_at) VALUES(1,'` + runUUID + `',1,1,'prompt','completed','` + providerUUID + `','model','provider_default','` + now + `','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,metadata_json,created_at) VALUES(1,'` + existingItemUUID + `',1,1,1,1,'tool_call','assistant','{}','json','completed','{}','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,metadata_json,created_at) VALUES(2,'` + batchItemUUID + `',1,1,1,2,'tool_call','assistant','{}','json','completed','{}','` + now + `')`,
		`INSERT INTO agent_tool_executions(id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,arguments_json,idempotency_key,state,result_json,created_at,updated_at) VALUES(1,'` + existingToolUUID + `',1,1,1,1,'` + existingToolCallUUID + `','existing_tool','{}','existing-tool-idempotency','completed','{}','` + now + `','` + now + `')`,
		`INSERT INTO agent_tool_executions(id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,tool_name,arguments_json,idempotency_key,state,result_json,created_at,updated_at) VALUES(2,'` + batchToolUUID + `',1,1,1,2,'` + batchToolCallUUID + `','batch_tool','{}','batch-tool-idempotency','completed','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(1,'` + existingWorkflowUUID + `',1,1,'yolo_project_initialization','Existing','completed',1,'{}','existing-workflow','` + providerUUID + `','model','provider_default','project_initialization','` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(2,'` + batchWorkflowUUID + `',1,2,'comic_image_generation_batch','Batch dedicated','completed',1,'{}','batch-dedicated-workflow','` + providerUUID + `','model','provider_default','generate_section_image:001','` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(3,'` + inlineWorkflowUUID + `',1,1,'comic_image_generation_batch','Batch inline','completed',1,'{}','batch-inline-workflow','` + providerUUID + `','model','provider_default','generate_section_image:001','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,input_json,output_json,created_at,updated_at) VALUES(1,'` + existingStepUUID + `',1,'project_initialization',1,'completed','existing-workflow-step','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(2,'` + batchStepUUID + `',2,'generate_section_image:001',1,'completed','batch-dedicated-step','` + taskUUID + `','` + resourceUUID + `','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(3,'` + inlineStepUUID + `',3,'generate_section_image:001',1,'completed','batch-inline-step','` + taskUUID + `','` + resourceUUID + `','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_events(id,uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at) VALUES(1,'` + existingEventUUID + `',1,1,1,'workflow_completed','{}','` + now + `')`,
		`INSERT INTO workflow_awaits(id,uuid,workflow_id,chat_thread_id,chat_turn_id,chat_run_id,tool_execution_id,status,created_at,ready_at,resumed_at,updated_at) VALUES(1,'` + existingAwaitUUID + `',1,1,1,1,1,'resumed','` + now + `','` + now + `','` + now + `','` + now + `')`,
		`INSERT INTO workflow_awaits(id,uuid,workflow_id,chat_thread_id,chat_turn_id,chat_run_id,tool_execution_id,status,created_at,ready_at,resumed_at,updated_at) VALUES(2,'` + batchAwaitUUID + `',3,1,1,1,2,'resumed','` + now + `','` + now + `','` + now + `','` + now + `')`,
		`INSERT INTO llm_logs(id,uuid,project_id,workflow_id,workflow_step_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,created_at,completed_at,request_payload,response) VALUES(1,'` + existingLogUUID + `',1,1,1,'workflow','migration_fixture','text',1,'` + providerUUID + `','openai_compatible','model','completed','` + now + `','` + now + `','{}','{}')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatalf("seed comic image batch workflow migration: %v\n%s", err, statement)
		}
	}

	for _, q := range []string{
		`UPDATE llm_logs SET price_snapshot='{"price":1}',billing_usage='{"tokens":10}',cost_status='calculated',cost_nanos=123,cost_currency='USD',cost_origin='provider',cost_breakdown='{"total":123}' WHERE id=1`,
		`INSERT INTO llm_cost_backfills(id,uuid,project_id,created_at) VALUES(1,'01a06a00-0000-7000-8000-000000000031',1,CURRENT_TIMESTAMP)`,
		`INSERT INTO llm_cost_backfill_items(backfill_id,llm_log_id,estimate_json,applied) VALUES(1,1,'{"cost":123}',1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	tables := []string{"workflows", "workflow_steps", "workflow_events", "workflow_awaits", "chat_threads", "chat_turns", "chat_runs", "chat_items", "agent_tool_executions", "llm_logs", "llm_cost_backfills", "llm_cost_backfill_items"}
	snapshot := func() string {
		t.Helper()
		graph := map[string][]any{}
		for _, table := range tables {
			rows, err := db.Query("SELECT * FROM " + table + " ORDER BY id")
			if err != nil {
				t.Fatal(err)
			}
			cols, err := rows.Columns()
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				values := make([]any, len(cols))
				refs := make([]any, len(cols))
				for i := range values {
					refs[i] = &values[i]
				}
				if err := rows.Scan(refs...); err != nil {
					t.Fatal(err)
				}
				graph[table] = append(graph[table], values)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
		}
		data, err := json.Marshal(graph)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	before := snapshot()
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatal("up migration changed the existing graph or costs")
	}
	for i, kind := range []string{"story_profile_generation", "story_profile_from_chapters"} {
		uuid := fmt.Sprintf("01a06a00-0000-7000-8000-%012d", 40+i)
		if _, err := db.Exec(`INSERT INTO workflows(uuid,project_id,thread_id,kind,title,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at) VALUES(?,1,1,?,'Profile','{}',?,?, 'model',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, uuid, kind, "profile-"+kind, providerUUID); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.migrator.Migrate(20260905000038); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatal("down migration changed the old graph, conversation, or costs")
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration left foreign key violations")
	}
}
