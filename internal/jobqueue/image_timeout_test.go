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
