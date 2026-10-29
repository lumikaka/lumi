package agent

import (
	"context"
	"errors"
	"testing"

	"lumi/internal/project"
)

func TestInitializationBlocksOtherChatWritesUntilHandoff(t *testing.T) {
	ctx := context.Background()
	h := newAgentHarness(t)
	thread, err := h.service.CreateThread(ctx, h.project.UUID, CreateThreadInput{Title: "已有对话"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "已有消息"}); err != nil {
		t.Fatal(err)
	}
	workflow, err := h.service.CreateYoloWorkflow(ctx, h.project.UUID, CreateYoloInput{Title: "初始化", StoryPrompt: "一只狐狸送信。", IdempotencyKey: "initialization-guard"})
	if err != nil {
		t.Fatal(err)
	}
	checkLocked := func(err error) {
		t.Helper()
		var projectErr *project.Error
		if !errors.As(err, &projectErr) || projectErr.Code != project.CodeProjectInitializing {
			t.Fatalf("expected initialization guard, got %v", err)
		}
	}
	_, err = h.service.CreateThread(ctx, h.project.UUID, CreateThreadInput{Title: "新对话"})
	checkLocked(err)
	_, err = h.service.CreateTurn(ctx, h.project.UUID, thread.UUID, CreateTurnInput{InputText: "插队消息"})
	checkLocked(err)
	_, err = h.service.CreateFollowUp(ctx, h.project.UUID, thread.UUID, CreateFollowUpInput{InputText: "追问"})
	checkLocked(err)
	_, err = h.service.Steer(ctx, h.project.UUID, thread.UUID, SteeringInput{InputText: "改一下"})
	checkLocked(err)
	if _, err := h.service.CancelWorkflow(ctx, h.project.UUID, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	_, err = h.service.CreateThread(ctx, h.project.UUID, CreateThreadInput{Title: "取消后仍须接管"})
	checkLocked(err)
	if _, err := h.store.TakeOverInitialization(ctx, workflow.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.CreateThread(ctx, h.project.UUID, CreateThreadInput{Title: "接管后对话"}); err != nil {
		t.Fatalf("handoff did not restore chat: %v", err)
	}
}
