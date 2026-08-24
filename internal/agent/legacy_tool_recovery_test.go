package agent

import (
	"context"
	"strings"
	"testing"
)

func TestHistoricalRunProtocolUpgradeMatrix(t *testing.T) {
	harness := newAgentHarness(t)
	ctx := context.Background()
	type fixture struct {
		thread Thread
		turn   Turn
		tc     toolContext
	}
	create := func(name string) fixture {
		t.Helper()
		thread, err := harness.service.CreateThread(ctx, harness.project.UUID, CreateThreadInput{Title: name, Scope: ThreadScopeProject, ProviderUUID: harness.provider.UUID})
		if err != nil {
			t.Fatal(err)
		}
		turn, err := harness.service.CreateTurn(ctx, harness.project.UUID, thread.UUID, CreateTurnInput{InputText: name})
		if err != nil {
			t.Fatal(err)
		}
		tc, err := harness.service.loadToolContext(ctx, harness.store, thread.UUID, turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		return fixture{thread: thread, turn: turn, tc: tc}
	}
	setLegacy := func(value fixture) {
		t.Helper()
		if err := harness.store.DB().Exec(`UPDATE chat_items SET metadata_json=json_remove(json_set(metadata_json,'$.prompt_snapshot.tool_mode',?), '$.prompt_snapshot.tool_protocol') WHERE run_id=? AND turn_id=? AND item_type='user_message'`, ToolModeLegacyTyped, value.tc.Run.ID, value.tc.Turn.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	removeMode := func(value fixture) {
		t.Helper()
		if err := harness.store.DB().Exec(`UPDATE chat_items SET metadata_json=json_remove(metadata_json,'$.prompt_snapshot.tool_mode','$.prompt_snapshot.tool_protocol') WHERE run_id=? AND turn_id=? AND item_type='user_message'`, value.tc.Run.ID, value.tc.Turn.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	loadMode := func(value fixture) string {
		t.Helper()
		reloaded, err := harness.service.loadToolContext(ctx, harness.store, value.thread.UUID, value.turn.UUID)
		if err != nil {
			t.Fatal(err)
		}
		mode, err := harness.service.loadRunToolMode(ctx, harness.store, reloaded)
		if err != nil {
			t.Fatal(err)
		}
		return mode
	}

	t.Run("completed legacy audit remains unchanged", func(t *testing.T) {
		value := create("completed legacy")
		setLegacy(value)
		if err := harness.service.claimRun(ctx, harness.store, &value.tc); err != nil {
			t.Fatal(err)
		}
		value.tc.ToolMode = loadMode(value)
		execution, _, completed, err := harness.service.persistToolIntent(ctx, harness.store, value.tc, "legacy-completed-call", "get_story_profile", `{}`)
		if err != nil || completed {
			t.Fatalf("intent=%+v completed=%v err=%v", execution, completed, err)
		}
		result, err := harness.service.executeTool(ctx, harness.store, value.tc, execution)
		if err != nil || !strings.Contains(string(result), `"success":true`) {
			t.Fatalf("result=%s err=%v", result, err)
		}
		if err := harness.service.persistToolResult(ctx, harness.store, value.tc, execution, result); err != nil {
			t.Fatal(err)
		}
		if err := harness.store.DB().Exec("UPDATE chat_runs SET status=? WHERE id=?", TurnCompleted, value.tc.Run.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := harness.store.DB().Exec("UPDATE chat_turns SET status=? WHERE id=?", TurnCompleted, value.tc.Turn.ID).Error; err != nil {
			t.Fatal(err)
		}
		var name string
		if err := harness.store.DB().Table("agent_tool_executions").Select("tool_name").Where("id=?", execution.ID).Scan(&name).Error; err != nil || name != "get_story_profile" {
			t.Fatalf("historical audit name=%q err=%v", name, err)
		}
	})

	for _, test := range []struct {
		name       string
		status     string
		missing    bool
		wantMode   string
		wantLegacy bool
	}{
		{name: "queued legacy", status: TurnQueued, wantMode: ToolModeLegacyTyped, wantLegacy: true},
		{name: "in-progress legacy", status: TurnInProgress, wantMode: ToolModeLegacyTyped, wantLegacy: true},
		{name: "waiting legacy", status: TurnWaitingForInput, wantMode: ToolModeLegacyTyped, wantLegacy: true},
		{name: "project API", status: TurnQueued, wantMode: ToolModeProjectAPI},
		{name: "missing mode snapshot", status: TurnQueued, missing: true, wantMode: ToolModeLegacyTyped, wantLegacy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := create(test.name)
			if test.missing {
				removeMode(value)
			} else if test.wantLegacy {
				setLegacy(value)
			}
			if test.status != TurnQueued {
				if err := harness.store.DB().Exec("UPDATE chat_runs SET status=? WHERE id=?", test.status, value.tc.Run.ID).Error; err != nil {
					t.Fatal(err)
				}
				if err := harness.store.DB().Exec("UPDATE chat_turns SET status=? WHERE id=?", test.status, value.tc.Turn.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode := loadMode(value); mode != test.wantMode {
				t.Fatalf("mode=%q want=%q", mode, test.wantMode)
			}
			reloaded, err := harness.service.loadToolContext(ctx, harness.store, value.thread.UUID, value.turn.UUID)
			if err != nil {
				t.Fatal(err)
			}
			tools := definitionNames(llmToolDefinitionsForMode(reloaded.Thread, test.wantMode))
			if test.wantLegacy {
				if !containsString(tools, "get_story_profile") || containsString(tools, "request_api") {
					t.Fatalf("legacy recovery tools=%v", tools)
				}
				if mode := loadMode(value); mode != test.wantMode {
					t.Fatalf("second load mode=%q", mode)
				}
				var events int64
				if err := harness.store.DB().Table("chat_events").Where("run_id=? AND event_type='legacy_tool_recovery'", value.tc.Run.ID).Count(&events).Error; err != nil || events != 1 {
					t.Fatalf("recovery events=%d err=%v", events, err)
				}
			} else if containsString(tools, "get_story_profile") || !containsString(tools, "request_api") {
				t.Fatalf("project tools=%v", tools)
			}
		})
	}

	newRun := create("new run after legacy recovery")
	if mode := loadMode(newRun); mode != ToolModeProjectAPI {
		t.Fatalf("new run inherited legacy mode: %q", mode)
	}
}
