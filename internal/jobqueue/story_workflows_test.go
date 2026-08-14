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
	"lumi/internal/production"
)

func TestComicMomentCountPlanTracksSectionLimitDeterministically(t *testing.T) {
	tests := []struct {
		limit int
		want  []int
	}{
		{limit: 1, want: []int{2}},
		{limit: 6, want: []int{2, 3, 1, 2, 3, 1}},
		{limit: 12, want: []int{2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1}},
		{limit: 24, want: []int{2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1}},
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
		{name: "maximum", value: intPointer(production.MaxGeneratedComicSections), want: production.MaxGeneratedComicSections},
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
	for _, invalid := range []int{0, production.MaxGeneratedComicSections + 1} {
		_, err := harness.queue.CreateStoryWorkflow(context.Background(), harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
			ProviderUUID: harness.provider.UUID, MaxSectionCount: &invalid, IdempotencyKey: "comic-invalid-" + jsonNumber(invalid),
		})
		var queueErr *Error
		if !errors.As(err, &queueErr) || queueErr.Code != CodeInvalidTask || !strings.Contains(queueErr.Details, jsonNumber(production.MaxGeneratedComicSections)) {
			t.Fatalf("max_section_count=%d error=%v", invalid, err)
		}
	}
}

func TestComicStoryboardResponseProjectsContractMaximum(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	sections := make([]map[string]any, production.MaxGeneratedComicSections)
	for index := range sections {
		sections[index] = map[string]any{
			"section_no": index + 1,
			"title":      "Page " + jsonNumber(index+1),
			"storyboard": "Storyboard " + jsonNumber(index+1),
		}
	}
	raw, err := json.Marshal(map[string]any{
		"chapter_code": chapter.ChapterCode,
		"title":        "Contract maximum pages",
		"sections":     sections,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := harness.runtime(t).applyStoryWorkflowResponse(context.Background(), taskRecord{Kind: KindComicStoryboardGeneration}, storyGenerationSnapshot{
		ChapterUUID: chapter.UUID, ChapterCode: chapter.ChapterCode, MaxSectionCount: production.MaxGeneratedComicSections,
	}, string(raw))
	if err != nil {
		t.Fatal(err)
	}
	sectionUUIDs, ok := payload["section_uuids"].([]string)
	if !ok || len(sectionUUIDs) != production.MaxGeneratedComicSections {
		t.Fatalf("storyboard payload=%+v", payload)
	}
}

func TestTerminalComicStoryboardResolutionOnlyAcceptsMatchingOutcome(t *testing.T) {
	if !terminalComicStoryboardResolution(`{"overwritten":true}`, ComicStoryboardConflictOverwrite) {
		t.Fatal("completed overwrite was not recognized")
	}
	if !terminalComicStoryboardResolution(`{"resolution":"keep_existing"}`, ComicStoryboardConflictKeepExisting) {
		t.Fatal("completed keep-existing resolution was not recognized")
	}
	for _, candidate := range []struct {
		output, action string
	}{
		{output: `{}`, action: ComicStoryboardConflictOverwrite},
		{output: `{"resolution":"keep_existing"}`, action: ComicStoryboardConflictOverwrite},
		{output: `{"overwritten":true}`, action: ComicStoryboardConflictKeepExisting},
		{output: `{bad json`, action: ComicStoryboardConflictKeepExisting},
	} {
		if terminalComicStoryboardResolution(candidate.output, candidate.action) {
			t.Fatalf("unexpected terminal resolution for output=%s action=%s", candidate.output, candidate.action)
		}
	}
}

func TestComicStoryboardConflictWaitsForOverwriteAndReusesPersistedResult(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	chapter := harness.createChapter(t, "vol01.ch01")
	productionService := production.NewService(harness.runtime(t).store, nil)
	existing, err := productionService.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Existing page", StoryboardMD: "Existing storyboard"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := harness.queue.CreateStoryWorkflow(ctx, harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
		ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-overwrite-confirmation",
	})
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusWaitingForInput)
	if waiting.Progress != 95 || waiting.ErrorCode != "" {
		t.Fatalf("waiting task=%+v", waiting)
	}
	workflowStatus, stepStatus, threadStatus := comicStoryboardWorkflowState(t, harness, created.UUID)
	if workflowStatus != StatusRunning || stepStatus != "waiting" || threadStatus != StatusWaitingForInput {
		t.Fatalf("waiting workflow state=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
	}
	agents := agent.NewService(harness.projects, harness.queue.providers, nil, harness.queue, nil)
	workflows, err := agents.ListWorkflows(ctx, harness.project.UUID)
	if err != nil || len(workflows) != 1 || len(workflows[0].Steps) != 1 {
		t.Fatalf("waiting workflows=%+v error=%v", workflows, err)
	}
	workflow := workflows[0]
	var confirmation comicStoryboardOverwriteRequest
	if err := json.Unmarshal(workflow.Steps[0].Output, &confirmation); err != nil || confirmation.ActionRequired != comicStoryboardOverwriteActionRequired || confirmation.ExistingSectionCount != 1 || confirmation.GeneratedSectionCount != 1 {
		t.Fatalf("confirmation=%+v error=%v output=%s", confirmation, err, workflow.Steps[0].Output)
	}
	resolution, err := harness.queue.ResolveComicStoryboardConflict(ctx, harness.project.UUID, workflow.UUID, ResolveComicStoryboardConflictInput{
		Action: ComicStoryboardConflictOverwrite, ExpectedComicStateRevision: &confirmation.ExpectedComicStateRevision,
	})
	if err != nil || resolution.Status != StatusCompleted || resolution.TaskUUID != created.UUID || resolution.ThreadUUID != workflow.ThreadUUID {
		t.Fatalf("overwrite resolution=%+v error=%v", resolution, err)
	}
	sections, err := productionService.ListSections(ctx, chapter.UUID)
	if err != nil || len(sections) != 1 || sections[0].UUID == existing.UUID || sections[0].Title != "相遇" {
		t.Fatalf("overwritten sections=%+v error=%v", sections, err)
	}
	var llmLogCount int64
	if err := harness.runtime(t).store.DB().Table("llm_logs").Where("task_run_id=(SELECT id FROM task_runs WHERE uuid=?)", created.UUID).Count(&llmLogCount).Error; err != nil || llmLogCount != 1 {
		t.Fatalf("LLM logs after overwrite=%d error=%v", llmLogCount, err)
	}
	idempotent, err := harness.queue.ResolveComicStoryboardConflict(ctx, harness.project.UUID, workflow.UUID, ResolveComicStoryboardConflictInput{
		Action: ComicStoryboardConflictOverwrite, ExpectedComicStateRevision: &confirmation.ExpectedComicStateRevision,
	})
	if err != nil || idempotent.Status != StatusCompleted {
		t.Fatalf("idempotent resolution=%+v error=%v", idempotent, err)
	}
}

func TestComicStoryboardOverwriteRejectsStaleConfirmation(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	chapter := harness.createChapter(t, "vol01.ch01")
	productionService := production.NewService(harness.runtime(t).store, nil)
	existing, err := productionService.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Existing page", StoryboardMD: "Existing storyboard"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := harness.queue.CreateStoryWorkflow(ctx, harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
		ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-overwrite-stale",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusWaitingForInput)
	var workflowUUID, outputJSON string
	if err := harness.runtime(t).store.DB().Raw(`SELECT workflows.uuid,steps.output_json FROM workflows JOIN workflow_steps steps ON steps.workflow_id=workflows.id WHERE steps.task_uuid=?`, created.UUID).Row().Scan(&workflowUUID, &outputJSON); err != nil {
		t.Fatal(err)
	}
	var confirmation comicStoryboardOverwriteRequest
	if err := json.Unmarshal([]byte(outputJSON), &confirmation); err != nil {
		t.Fatal(err)
	}
	newTitle := "Changed while waiting"
	if _, err := productionService.UpdateSection(ctx, chapter.UUID, existing.UUID, production.UpdateSectionInput{Title: &newTitle, ExpectedRevision: existing.Revision}); err != nil {
		t.Fatal(err)
	}
	_, err = harness.queue.ResolveComicStoryboardConflict(ctx, harness.project.UUID, workflowUUID, ResolveComicStoryboardConflictInput{
		Action: ComicStoryboardConflictOverwrite, ExpectedComicStateRevision: &confirmation.ExpectedComicStateRevision,
	})
	var productionErr *production.Error
	if !errors.As(err, &productionErr) || productionErr.Code != production.CodeStateConflict {
		t.Fatalf("stale resolution error=%v", err)
	}
	stillWaiting, err := harness.queue.GetTask(ctx, harness.project.UUID, created.UUID)
	if err != nil || stillWaiting.Status != StatusWaitingForInput {
		t.Fatalf("task after stale resolution=%+v error=%v", stillWaiting, err)
	}
	sections, err := productionService.ListSections(ctx, chapter.UUID)
	if err != nil || len(sections) != 1 || sections[0].UUID != existing.UUID || sections[0].Title != newTitle {
		t.Fatalf("sections after stale resolution=%+v error=%v", sections, err)
	}
	var refreshedJSON string
	if err := harness.runtime(t).store.DB().Raw(`SELECT steps.output_json FROM workflow_steps steps JOIN workflows ON workflows.id=steps.workflow_id WHERE workflows.uuid=?`, workflowUUID).Row().Scan(&refreshedJSON); err != nil {
		t.Fatal(err)
	}
	var refreshed comicStoryboardOverwriteRequest
	if err := json.Unmarshal([]byte(refreshedJSON), &refreshed); err != nil || refreshed.ExpectedComicStateRevision <= confirmation.ExpectedComicStateRevision || refreshed.ExistingSectionCount != 1 {
		t.Fatalf("refreshed confirmation=%+v error=%v", refreshed, err)
	}
	resolution, err := harness.queue.ResolveComicStoryboardConflict(ctx, harness.project.UUID, workflowUUID, ResolveComicStoryboardConflictInput{
		Action: ComicStoryboardConflictOverwrite, ExpectedComicStateRevision: &refreshed.ExpectedComicStateRevision,
	})
	if err != nil || resolution.Status != StatusCompleted {
		t.Fatalf("refreshed overwrite resolution=%+v error=%v", resolution, err)
	}
	replaced, err := productionService.ListSections(ctx, chapter.UUID)
	if err != nil || len(replaced) != 1 || replaced[0].UUID == existing.UUID || replaced[0].Title != "相遇" {
		t.Fatalf("sections after refreshed overwrite=%+v error=%v", replaced, err)
	}
}

func TestComicStoryboardConflictCanKeepExistingSections(t *testing.T) {
	harness := newQueueHarness(t)
	ctx := context.Background()
	chapter := harness.createChapter(t, "vol01.ch01")
	productionService := production.NewService(harness.runtime(t).store, nil)
	existing, err := productionService.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: "Keep this", StoryboardMD: "Existing storyboard"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := harness.queue.CreateStoryWorkflow(ctx, harness.project.UUID, KindComicStoryboardGeneration, chapter.UUID, CreateStoryWorkflowInput{
		ProviderUUID: harness.provider.UUID, IdempotencyKey: "comic-keep-existing",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusWaitingForInput)
	var workflowUUID, outputJSON string
	if err := harness.runtime(t).store.DB().Raw(`SELECT workflows.uuid,steps.output_json FROM workflows JOIN workflow_steps steps ON steps.workflow_id=workflows.id WHERE steps.task_uuid=?`, created.UUID).Row().Scan(&workflowUUID, &outputJSON); err != nil {
		t.Fatal(err)
	}
	var confirmation comicStoryboardOverwriteRequest
	if err := json.Unmarshal([]byte(outputJSON), &confirmation); err != nil {
		t.Fatal(err)
	}
	resolution, err := harness.queue.ResolveComicStoryboardConflict(ctx, harness.project.UUID, workflowUUID, ResolveComicStoryboardConflictInput{
		Action: ComicStoryboardConflictKeepExisting, ExpectedComicStateRevision: &confirmation.ExpectedComicStateRevision,
	})
	if err != nil || resolution.Status != StatusCancelled {
		t.Fatalf("keep resolution=%+v error=%v", resolution, err)
	}
	workflowStatus, stepStatus, threadStatus := comicStoryboardWorkflowState(t, harness, created.UUID)
	if workflowStatus != StatusCancelled || stepStatus != StatusCancelled || threadStatus != StatusCancelled {
		t.Fatalf("kept workflow state=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
	}
	sections, err := productionService.ListSections(ctx, chapter.UUID)
	if err != nil || len(sections) != 1 || sections[0].UUID != existing.UUID {
		t.Fatalf("kept sections=%+v error=%v", sections, err)
	}
}

func TestComicStoryboardRetryKeepsFrozenLimitAndPlan(t *testing.T) {
	harness := newQueueHarness(t)
	chapter := harness.createChapter(t, "vol01.ch01")
	limit := production.MaxGeneratedComicSections
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
	if before.MaxSectionCount != production.MaxGeneratedComicSections || !reflect.DeepEqual(before.MomentCountPlan, after.MomentCountPlan) || after.MaxSectionCount != before.MaxSectionCount {
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

func TestChapterWorkflowCreationRollsBackEveryProjectionBoundary(t *testing.T) {
	tests := []struct {
		name       string
		triggerSQL string
	}{
		{name: "thread", triggerSQL: `CREATE TRIGGER reject_chapter_thread BEFORE INSERT ON chat_threads WHEN NEW.title='story_chapter_generation' BEGIN SELECT RAISE(ABORT,'injected thread failure'); END`},
		{name: "workflow", triggerSQL: `CREATE TRIGGER reject_chapter_workflow BEFORE INSERT ON workflows WHEN NEW.kind='story_chapter_generation' BEGIN SELECT RAISE(ABORT,'injected workflow failure'); END`},
		{name: "step", triggerSQL: `CREATE TRIGGER reject_chapter_step BEFORE INSERT ON workflow_steps WHEN NEW.step_key='story_chapter' BEGIN SELECT RAISE(ABORT,'injected step failure'); END`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newQueueHarness(t)
			chapter := harness.createChapter(t, "vol01.ch01")
			runtime := harness.runtime(t)
			if err := runtime.store.DB().Exec(test.triggerSQL).Error; err != nil {
				t.Fatal(err)
			}
			_, createErr := harness.queue.CreateChapterGeneration(context.Background(), harness.project.UUID, CreateGenerationInput{
				ChapterUUID: chapter.UUID, ProviderUUID: harness.provider.UUID, Prompt: "原子失败", IdempotencyKey: "chapter-atomic-" + test.name,
			})
			if createErr == nil {
				t.Fatal("injected projection failure created a task")
			}
			var counts struct {
				Tasks, ChatThreads, Workflows, Steps, AgentThreads, AgentRuns, RiverJobs int64
			}
			if err := runtime.store.DB().Raw(`SELECT
				(SELECT COUNT(*) FROM task_runs) AS tasks,
				(SELECT COUNT(*) FROM chat_threads) AS chat_threads,
				(SELECT COUNT(*) FROM workflows) AS workflows,
				(SELECT COUNT(*) FROM workflow_steps) AS steps,
				(SELECT COUNT(*) FROM agent_threads) AS agent_threads,
				(SELECT COUNT(*) FROM agent_runs) AS agent_runs,
				(SELECT COUNT(*) FROM river_job WHERE kind <> 'lumi_comic_export_cleanup_v1') AS river_jobs`).Scan(&counts).Error; err != nil {
				t.Fatal(err)
			}
			if counts.Tasks != 0 || counts.ChatThreads != 0 || counts.Workflows != 0 || counts.Steps != 0 || counts.AgentThreads != 0 || counts.AgentRuns != 0 || counts.RiverJobs != 0 {
				t.Fatalf("failed chapter creation leaked rows: %+v", counts)
			}
		})
	}
}

func TestChapterWorkflowControlsUseOriginalTaskAndProjection(t *testing.T) {
	t.Run("workflow cancellation", func(t *testing.T) {
		harness := newQueueHarness(t)
		chapter := harness.createChapter(t, "vol01.ch01")
		created, err := harness.queue.CreateChapterGeneration(context.Background(), harness.project.UUID, CreateGenerationInput{
			ChapterUUID: chapter.UUID, ProviderUUID: harness.provider.UUID, Prompt: "[cancel] 等待 Workflow 取消", IdempotencyKey: "chapter-workflow-cancel",
		})
		if err != nil {
			t.Fatal(err)
		}
		waitFakeStarted(t, harness.fakeModel, "cancel")
		agents := agent.NewService(harness.projects, harness.queue.providers, nil, harness.queue, nil)
		workflows, err := agents.ListWorkflows(context.Background(), harness.project.UUID)
		if err != nil || len(workflows) != 1 {
			t.Fatalf("chapter workflows=%+v err=%v", workflows, err)
		}
		workflowUUID, threadUUID := workflows[0].UUID, workflows[0].ThreadUUID
		cancelled, err := agents.CancelWorkflow(context.Background(), harness.project.UUID, workflowUUID)
		if err != nil || cancelled.Status != StatusCancelled {
			t.Fatalf("cancelled workflow=%+v err=%v", cancelled, err)
		}
		waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCancelled)
		workflowStatus, stepStatus, threadStatus := storyTaskWorkflowState(t, harness, created.UUID)
		if workflowStatus != StatusCancelled || stepStatus != StatusCancelled || threadStatus != StatusCancelled {
			t.Fatalf("cancelled chapter workflow state=%s/%s/%s", workflowStatus, stepStatus, threadStatus)
		}
		workflows, err = agents.ListWorkflows(context.Background(), harness.project.UUID)
		if err != nil || len(workflows) != 1 || workflows[0].UUID != workflowUUID || workflows[0].ThreadUUID != threadUUID {
			t.Fatalf("chapter cancellation duplicated projection=%+v err=%v", workflows, err)
		}
	})

	t.Run("workflow retry", func(t *testing.T) {
		harness := newQueueHarness(t)
		chapter := harness.createChapter(t, "vol01.ch01")
		created, err := harness.queue.CreateChapterGeneration(context.Background(), harness.project.UUID, CreateGenerationInput{
			ChapterUUID: chapter.UUID, ProviderUUID: harness.provider.UUID, Prompt: "[retry] 通过 Workflow 重试", IdempotencyKey: "chapter-workflow-retry",
		})
		if err != nil {
			t.Fatal(err)
		}
		waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusFailed)
		agents := agent.NewService(harness.projects, harness.queue.providers, nil, harness.queue, nil)
		workflows, err := agents.ListWorkflows(context.Background(), harness.project.UUID)
		if err != nil || len(workflows) != 1 {
			t.Fatalf("failed chapter workflows=%+v err=%v", workflows, err)
		}
		workflowUUID, threadUUID := workflows[0].UUID, workflows[0].ThreadUUID
		retried, err := agents.RetryWorkflow(context.Background(), harness.project.UUID, workflowUUID)
		if err != nil || (retried.Status != StatusQueued && retried.Status != StatusRunning) {
			t.Fatalf("retried chapter workflow=%+v err=%v", retried, err)
		}
		waitTaskStatus(t, harness.queue, harness.project.UUID, created.UUID, StatusCompleted)
		workflows, err = agents.ListWorkflows(context.Background(), harness.project.UUID)
		if err != nil || len(workflows) != 1 || workflows[0].UUID != workflowUUID || workflows[0].ThreadUUID != threadUUID || workflows[0].Status != StatusCompleted || workflows[0].Steps[0].TaskUUID != created.UUID {
			t.Fatalf("chapter retry duplicated or drifted projection=%+v err=%v", workflows, err)
		}
	})
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
	progressPayload, ok := storyTaskRealtimePayload(values[0], values[1], values[2], values[3], values[4], values[5], StatusRunning, 37)
	if !ok || len(progressPayload) != 8 || progressPayload["progress"] != 37 {
		t.Fatalf("chapter progress payload=%+v ok=%v", progressPayload, ok)
	}
	if invalid, ok := storyTaskRealtimePayload(values[0], values[1], values[2], values[3], values[4], values[5], StatusRunning, 101); ok || invalid != nil {
		t.Fatalf("invalid progress payload=%+v ok=%v", invalid, ok)
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

func storyTaskWorkflowState(t *testing.T, harness *queueHarness, taskUUID string) (string, string, string) {
	t.Helper()
	var workflowStatus, stepStatus, threadStatus string
	err := harness.runtime(t).store.DB().Raw(`SELECT workflows.status,steps.status,threads.status
		FROM workflows
		JOIN workflow_steps steps ON steps.workflow_id=workflows.id
		JOIN chat_threads threads ON threads.id=workflows.thread_id
		WHERE steps.task_uuid=?`, taskUUID).Row().Scan(&workflowStatus, &stepStatus, &threadStatus)
	if err != nil {
		t.Fatal(err)
	}
	return workflowStatus, stepStatus, threadStatus
}
