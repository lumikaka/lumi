package jobqueue

import (
	"github.com/riverqueue/river"
	"lumi/internal/agent"
	"lumi/internal/imagegen"
	"testing"
	"time"
)

func TestImageWorkerBudgetsFitRequestAndRescue(t *testing.T) {
	worker := &productionWorker{}
	for _, kind := range []string{KindPremiseSettingGeneration, KindPremiseAssetGeneration, KindComicImageGeneration} {
		budget := worker.Timeout(&river.Job[productionArgs]{Args: productionArgs{TaskKind: kind}})
		if budget <= imagegen.RequestTimeout || budget >= stuckJobRescueAfter {
			t.Fatalf("%s budget %s does not fit request/rescue window", kind, budget)
		}
	}
	for _, kind := range []string{KindPremiseAssetBreakdown, KindComicExport} {
		if budget := worker.Timeout(&river.Job[productionArgs]{Args: productionArgs{TaskKind: kind}}); budget != 0 {
			t.Fatalf("non-image task %s changed timeout to %s", kind, budget)
		}
	}
	agentWorker := &agentWorker{}
	for _, kind := range []string{agent.JobChatTurn, agent.JobChatResume} {
		if budget := agentWorker.Timeout(&river.Job[agentArgs]{Args: agentArgs{JobKind: kind}}); budget <= imagegen.RequestTimeout || budget >= stuckJobRescueAfter {
			t.Fatalf("image tool parent timeout=%s", budget)
		}
	}
	if stuckJobRescueAfter <= 5*time.Minute {
		t.Fatal("rescue must exceed the queue default")
	}
}

func TestWorkflowBudgetsFitRequestsAndRescue(t *testing.T) {
	for _, timeout := range []time.Duration{15 * time.Minute, time.Hour} {
		service := agent.NewService(nil, nil, nil, nil, nil).WithWorkflowRequestTimeout(timeout)
		manager := NewManager(nil, nil, nil).WithAgentService(service)
		worker := &agentWorker{service: service}
		budget := worker.Timeout(&river.Job[agentArgs]{Args: agentArgs{JobKind: agent.JobWorkflowStep}})
		if budget <= 2*timeout || manager.rescueStuckJobsAfter() <= budget {
			t.Fatalf("request=%s worker=%s rescue=%s", timeout, budget, manager.rescueStuckJobsAfter())
		}
		storyWorker := &storyGenerationWorker{runtime: &projectRuntime{manager: manager}}
		if got := storyWorker.Timeout(&river.Job[riverArgs]{Args: riverArgs{TaskKind: KindComicStoryboardGeneration}}); got != budget {
			t.Fatalf("storyboard budget=%s", got)
		}
		if got := storyWorker.Timeout(&river.Job[riverArgs]{Args: riverArgs{TaskKind: KindStoryChapterGeneration}}); got != 0 {
			t.Fatalf("ordinary story budget=%s", got)
		}
	}
}

func TestWorkflowEnqueueHasBoundedRetryBudget(t *testing.T) {
	h, r := stoppedExecutionHarness(t)
	service := agent.NewService(h.projects, h.queue.providers, nil, h.queue, nil)
	workflow, err := service.CreateYoloWorkflow(t.Context(), h.project.UUID, agent.CreateYoloInput{Title: "有限重试", StoryPrompt: "小狐狸出发。", ProviderUUID: h.provider.UUID, IdempotencyKey: "bounded-workflow"})
	if err != nil {
		t.Fatal(err)
	}
	var maxAttempts int
	if err := r.sqlDB.QueryRowContext(t.Context(), `SELECT j.max_attempts FROM river_job j JOIN workflow_steps s ON s.river_job_id=j.id WHERE s.uuid=?`, workflow.Steps[0].UUID).Scan(&maxAttempts); err != nil {
		t.Fatal(err)
	}
	if maxAttempts != agent.WorkflowMaxAttempts {
		t.Fatalf("persisted max_attempts=%d", maxAttempts)
	}
}
