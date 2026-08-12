package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"lumi/internal/agent"
)

func TestComicMomentCountPlanTracksSectionLimitDeterministically(t *testing.T) {
	tests := []struct {
		limit int
		want  []int
	}{
		{limit: 1, want: []int{2}},
		{limit: 6, want: []int{2, 3, 1, 2, 3, 1}},
		{limit: 12, want: []int{2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1}},
	}
	for _, test := range tests {
		if got := comicMomentCountPlan(test.limit); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("limit %d plan=%v want=%v", test.limit, got, test.want)
		}
		if got := comicMomentCountPlan(test.limit); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("limit %d was not deterministic: %v", test.limit, got)
		}
	}
}

func TestComicStoryboardSectionLimitDefaultsValidatesAndFreezes(t *testing.T) {
	tests := []struct {
		name  string
		value *int
		want  int
	}{
		{name: "default", want: 6},
		{name: "one", value: intPointer(1), want: 1},
		{name: "six", value: intPointer(6), want: 6},
		{name: "twelve", value: intPointer(12), want: 12},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newQueueHarness(t)
			chapter := harness.createChapter(t, "vol01.ch01")
			task, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
				ProviderUUID: harness.provider.UUID, MaxSectionCount: test.value, IdempotencyKey: "comic-limit-" + test.name,
			})
			if err != nil {
				t.Fatal(err)
			}
			var snapshot storyGenerationSnapshot
			if err := json.Unmarshal(task.InputSnapshot, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.MaxSectionCount != test.want || len(snapshot.MomentCountPlan) != test.want || !reflect.DeepEqual(snapshot.MomentCountPlan, comicMomentCountPlan(test.want)) {
				t.Fatalf("snapshot=%+v", snapshot)
			}
			if !strings.Contains(snapshot.Prompt, "1 到 "+jsonNumber(test.want)+" 之间") {
				t.Fatalf("prompt did not freeze max_section_count=%d: %s", test.want, snapshot.Prompt)
			}
		})
	}

	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	for _, invalid := range []int{0, 13} {
		_, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
			ProviderUUID: harness.provider.UUID, MaxSectionCount: &invalid, IdempotencyKey: "comic-invalid-" + jsonNumber(invalid),
		})
		var queueErr *Error
		if !errors.As(err, &queueErr) || queueErr.Code != CodeInvalidTask {
			t.Fatalf("max_section_count=%d error=%v", invalid, err)
		}
	}
}

func TestComicStoryboardRetryKeepsFrozenLimitAndPlan(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	limit := 12
	created, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
		ProviderUUID: harness.provider.UUID, Prompt: "[retry]", MaxSectionCount: &limit, IdempotencyKey: "comic-limit-retry",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusFailed)
	workflowStatus, stepStatus, threadStatus := comicStoryboardWorkflowState(t, harness, created.UUID)
	if workflowStatus != StatusFailed || stepStatus != StatusFailed || threadStatus != StatusFailed {
		t.Fatalf("failed workflow state=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
	}
	agents := agent.NewService(harness.projects, harness.queue.providers, nil, harness.queue, nil)
	workflows, err := agents.ListWorkflows(context.Background(), harness.project.UUID)
	if err != nil || len(workflows) != 1 {
		t.Fatalf("failed storyboard workflows=%+v err=%v", workflows, err)
	}
	retriedWorkflow, err := agents.RetryWorkflow(context.Background(), harness.project.UUID, workflows[0].UUID)
	if err != nil || (retriedWorkflow.Status != StatusQueued && retriedWorkflow.Status != StatusRunning) {
		t.Fatalf("retried workflow=%+v err=%v", retriedWorkflow, err)
	}
	retried, err := harness.queue.GetTask(context.Background(), harness.project.UUID, created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	var before, after storyGenerationSnapshot
	if err := json.Unmarshal(created.InputSnapshot, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(retried.InputSnapshot, &after); err != nil {
		t.Fatal(err)
	}
	if before.MaxSectionCount != 12 || !reflect.DeepEqual(before.MomentCountPlan, after.MomentCountPlan) || after.MaxSectionCount != before.MaxSectionCount {
		t.Fatalf("retry drifted before=%+v after=%+v", before, after)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	workflowStatus, stepStatus, threadStatus = comicStoryboardWorkflowState(t, harness, created.UUID)
	if workflowStatus != StatusCompleted || stepStatus != StatusCompleted || threadStatus != StatusCompleted {
		t.Fatalf("retried workflow state=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
	}
}

func TestComicStoryboardWorkflowCreationIsAtomic(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	runtime, err := harness.queue.runtimeFor(harness.project.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.store.DB().Exec(`CREATE TRIGGER reject_storyboard_workflow BEFORE INSERT ON workflows WHEN NEW.kind='comic_storyboard_generation' BEGIN SELECT RAISE(ABORT,'injected workflow failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	_, createErr := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
		ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-atomic-failure",
	})
	if createErr == nil {
		t.Fatal("injected workflow failure created a task")
	}
	var counts struct {
		Tasks, Threads, Workflows, AgentThreads, AgentRuns int64
	}
	if err := runtime.store.DB().Raw(`SELECT
		(SELECT COUNT(*) FROM task_runs) AS tasks,
		(SELECT COUNT(*) FROM chat_threads) AS threads,
		(SELECT COUNT(*) FROM workflows) AS workflows,
		(SELECT COUNT(*) FROM agent_threads) AS agent_threads,
		(SELECT COUNT(*) FROM agent_runs) AS agent_runs`).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Tasks != 0 || counts.Threads != 0 || counts.Workflows != 0 || counts.AgentThreads != 0 || counts.AgentRuns != 0 {
		t.Fatalf("failed creation leaked rows: %+v", counts)
	}
	if err := runtime.store.DB().Exec(`DROP TRIGGER reject_storyboard_workflow`).Error; err != nil {
		t.Fatal(err)
	}
}

func TestComicStoryboardCancellationProjectsToChatAreaWorkflow(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	created, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
		ProviderUUID: harness.provider.UUID, Prompt: "[cancel]", IdempotencyKey: "comic-workflow-cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFakeStarted(t, harness.fakeModel, "cancel")
	deadline := time.Now().Add(5 * time.Second)
	runningProjected := false
	for time.Now().Before(deadline) {
		workflowStatus, stepStatus, threadStatus := comicStoryboardWorkflowState(t, harness, created.UUID)
		if workflowStatus == StatusRunning && stepStatus == StatusRunning && threadStatus == "busy" {
			runningProjected = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !runningProjected {
		t.Fatal("storyboard workflow never projected running state")
	}
	agents := agent.NewService(harness.projects, harness.queue.providers, nil, harness.queue, nil)
	workflows, err := agents.ListWorkflows(context.Background(), harness.project.UUID)
	if err != nil || len(workflows) != 1 {
		t.Fatalf("running storyboard workflows=%+v err=%v", workflows, err)
	}
	cancelled, err := agents.CancelWorkflow(context.Background(), harness.project.UUID, workflows[0].UUID)
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancelled workflow=%+v err=%v", cancelled, err)
	}
	workflowStatus, stepStatus, threadStatus := comicStoryboardWorkflowState(t, harness, created.UUID)
	if workflowStatus != StatusCancelled || stepStatus != StatusCancelled || threadStatus != StatusCancelled {
		t.Fatalf("cancelled workflow state=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
	}
}

func TestComicStoryboardWorkflowReconcilesExistingProjectionWithoutBackfill(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	created, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
		ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-workflow-reconcile",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
	runtime := harness.runtime(t)
	var workflowID, threadID int64
	if err := runtime.store.DB().Raw(`SELECT workflows.id,workflows.thread_id FROM workflows JOIN workflow_steps steps ON steps.workflow_id=workflows.id WHERE steps.task_uuid=?`, created.UUID).Row().Scan(&workflowID, &threadID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.store.DB().Exec(`UPDATE workflows SET status='running',completed_at=NULL WHERE id=?`, workflowID).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.store.DB().Exec(`UPDATE workflow_steps SET status='running',output_json='{}',completed_at=NULL WHERE workflow_id=?`, workflowID).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.store.DB().Exec(`UPDATE chat_threads SET status='busy' WHERE id=?`, threadID).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileComicStoryboardWorkflows(context.Background(), runtime.sqlDB, runtime.projectID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	workflowStatus, stepStatus, threadStatus := comicStoryboardWorkflowState(t, harness, created.UUID)
	if workflowStatus != StatusCompleted || stepStatus != StatusCompleted || threadStatus != StatusCompleted {
		t.Fatalf("reconciled workflow state=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
	}
	var output string
	if err := runtime.store.DB().Raw(`SELECT output_json FROM workflow_steps WHERE workflow_id=?`, workflowID).Scan(&output).Error; err != nil || !strings.Contains(output, "section_uuids") {
		t.Fatalf("reconciled output=%s err=%v", output, err)
	}
	if err := runtime.store.DB().Exec(`DELETE FROM workflows WHERE id=?`, workflowID).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.store.DB().Exec(`DELETE FROM chat_threads WHERE id=?`, threadID).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileComicStoryboardWorkflows(context.Background(), runtime.sqlDB, runtime.projectID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var workflowCount int64
	if err := runtime.store.DB().Table("workflows").Where("kind=?", agent.WorkflowComicStoryboard).Count(&workflowCount).Error; err != nil || workflowCount != 0 {
		t.Fatalf("reconciliation backfilled historical task count=%d err=%v", workflowCount, err)
	}
}

func TestComicStoryboardRealtimePayloadUsesOnlyPublicUUIDs(t *testing.T) {
	values := make([]string, 6)
	for index := range values {
		value, err := newUUIDv7()
		if err != nil {
			t.Fatal(err)
		}
		values[index] = value
	}
	payload, ok := comicStoryboardRealtimePayload(values[0], values[1], values[2], values[3], values[4], values[5], StatusRunning)
	if !ok || len(payload) != 7 || payload["status"] != StatusRunning {
		t.Fatalf("public workflow payload=%+v ok=%v", payload, ok)
	}
	for key, value := range payload {
		if strings.HasSuffix(key, "_uuid") && !isUUIDv7(value.(string)) {
			t.Fatalf("payload %s is not UUIDv7: %+v", key, payload)
		}
		if key == "id" || strings.HasSuffix(key, "_id") {
			t.Fatalf("payload exposed internal ID: %+v", payload)
		}
	}
	if invalid, ok := comicStoryboardRealtimePayload(values[0], values[1], values[2], values[3], "not-a-uuid", values[5], StatusRunning); ok || invalid != nil {
		t.Fatalf("invalid workflow payload=%+v ok=%v", invalid, ok)
	}
}

func TestLegacyComicSnapshotUsesHistoricalDefaultLimit(t *testing.T) {
	if got := normalizedComicMaxSectionCount(0); got != 6 {
		t.Fatalf("legacy zero limit normalized to %d", got)
	}
}

func intPointer(value int) *int { return &value }

func jsonNumber(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func comicStoryboardWorkflowState(t *testing.T, harness *queueHarness, taskUUID string) (string, string, string) {
	t.Helper()
	var workflowStatus, stepStatus, threadStatus string
	err := harness.runtime(t).store.DB().Raw(`SELECT workflows.status,steps.status,threads.status
		FROM workflows
		JOIN workflow_steps steps ON steps.workflow_id=workflows.id
		JOIN chat_threads threads ON threads.id=workflows.thread_id
		WHERE workflows.kind=? AND steps.task_uuid=?`, "comic_storyboard_generation", taskUUID).
		Row().Scan(&workflowStatus, &stepStatus, &threadStatus)
	if err != nil {
		t.Fatal(err)
	}
	return workflowStatus, stepStatus, threadStatus
}
