package agent

import (
	"context"
	"database/sql"
	"testing"
)

func TestExecuteToolStopsWhenIntentCannotTransitionToExecuting(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "读取文档"})
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
	execution, _, completed, err := harness.service.persistToolIntent(ctx, harness.store, tc, "transition-failure", "read_agent_doc", `{"path":"/api/v1/agent-docs/overview.md"}`)
	if err != nil || completed || execution.State != "intent" {
		t.Fatalf("execution=%+v completed=%v err=%v", execution, completed, err)
	}
	if err := harness.store.DB().Exec(`CREATE TRIGGER reject_tool_execution_claim BEFORE UPDATE OF state ON agent_tool_executions WHEN NEW.state='executing' BEGIN SELECT RAISE(ABORT,'reject claim'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := harness.service.executeTool(ctx, harness.store, tc, execution); err == nil {
		t.Fatal("tool executed after its intent claim failed")
	}
	var state string
	var result sql.NullString
	if err := harness.store.DB().Table("agent_tool_executions").Select("state,result_json").Where("id=?", execution.ID).Row().Scan(&state, &result); err != nil {
		t.Fatal(err)
	}
	if state != "intent" || result.Valid {
		t.Fatalf("state=%q result=%v", state, result)
	}
}

func TestExecuteCompletedToolRequiresAValidStoredEnvelope(t *testing.T) {
	harness := newAgentHarness(t)
	tc := toolContext{ToolMode: ToolModeProjectAPI}
	for name, result := range map[string]*string{
		"missing":    nil,
		"invalid":    stringPointerForToolStateTest(`{"success":`),
		"checkpoint": stringPointerForToolStateTest(`{"kind":"agent_tool_side_effect_checkpoint","schema_version":1,"route_id":"chapter.permanent_delete","data":null}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := harness.service.executeTool(context.Background(), harness.store, tc, toolExecutionRecord{ID: 1, State: "completed", ResultJSON: result})
			if errorCode(err) != CodeStateConflict {
				t.Fatalf("error=%v code=%s", err, errorCode(err))
			}
		})
	}
}

func TestExecutingSideEffectCheckpointIsNotExposedAsToolResult(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	thread := harness.createThread(t)
	turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: "checkpoint privacy"})
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
	execution, _, _, err := harness.service.persistToolIntent(ctx, harness.store, tc, "checkpoint-privacy", "read_agent_doc", `{"path":"/api/v1/agent-docs/overview.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := `{"kind":"agent_tool_side_effect_checkpoint","schema_version":1,"route_id":"chapter.permanent_delete","data":null}`
	if err := harness.store.DB().Table("agent_tool_executions").Where("id=?", execution.ID).Updates(map[string]any{"state": "executing", "result_json": checkpoint}).Error; err != nil {
		t.Fatal(err)
	}
	page, err := harness.service.ListTrajectory(ctx, harness.project.UUID, thread.UUID, "", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tools) != 1 || page.Tools[0].Status != "running" || len(page.Tools[0].Result) != 0 {
		t.Fatalf("trajectory tool=%+v", page.Tools)
	}
}

func stringPointerForToolStateTest(value string) *string { return &value }
