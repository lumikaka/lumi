package llmlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"

	"github.com/google/uuid"
)

type capturedRecorderEvent struct {
	topic   string
	event   string
	payload map[string]any
}

type recorderEventPublisher struct {
	events []capturedRecorderEvent
}

func (publisher *recorderEventPublisher) Broadcast(topic, event string, payload any) {
	publisher.events = append(publisher.events, capturedRecorderEvent{topic: topic, event: event, payload: payload.(map[string]any)})
}

type panickingRecorderEventPublisher struct{}

func (panickingRecorderEventPublisher) Broadcast(string, string, any) { panic("realtime unavailable") }

func TestSnapshotCharacterCountCountsUnicodeStringValuesOnly(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want int
	}{
		{name: "unicode runes", raw: json.RawMessage(`{"prompt":"月光🌙","max_tokens":1024,"stream":true}`), want: 3},
		{name: "nested text", raw: json.RawMessage(`{"messages":[{"role":"user","content":"你好"},{"role":"assistant","content":"ok"}]}`), want: 17},
		{name: "invalid JSON", raw: json.RawMessage(`{"prompt":`), want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := snapshotCharacterCount(test.raw); got != test.want {
				t.Fatalf("snapshotCharacterCount(%s) = %d, want %d", test.raw, got, test.want)
			}
		})
	}
}

func TestRecorderBroadcastsPublicStatusChangesAfterPersistence(t *testing.T) {
	ctx := context.Background()
	dataDirectory := filepath.Join(t.TempDir(), "app")
	appStore, err := appstore.Open(dataDirectory, config.SQLiteDSN(filepath.Join(dataDirectory, "lumi.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	manager := project.NewManager(appStore)
	created, err := manager.Create(ctx, "LLM realtime", project.ExplicitNewProjectParent(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close(); _ = appStore.Close() })

	err = manager.WithCurrentStore(ctx, created.UUID, func(store *project.Store) error {
		var projectID int64
		if err := store.DB().Model(&project.Project{}).Where("uuid = ?", created.UUID).Pluck("id", &projectID).Error; err != nil {
			return err
		}
		providerUUID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		taskUUID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		resourceUUID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := store.DB().Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,retryable,provider_uuid,model,progress,attempt,max_attempts,error_code,error_message,created_at,updated_at) VALUES(?,?,?,?,1,'{}','running','llm-realtime-test',0,?,'test-model',5,1,1,'','',?,?)`, taskUUID.String(), projectID, "story_chapter_generation", resourceUUID.String(), providerUUID.String(), now, now).Error; err != nil {
			return err
		}
		var taskID int64
		if err := store.DB().Table("task_runs").Where("uuid = ?", taskUUID.String()).Pluck("id", &taskID).Error; err != nil {
			return err
		}
		publisher := &recorderEventPublisher{}
		handle, err := Begin(ctx, store, publisher, StartInput{
			ProjectID: projectID, TaskRunID: taskID, SourceType: SourceStoryGeneration, Scenario: "story_chapter_generation", RequestType: RequestText,
			Attempt: 1, ProviderUUID: providerUUID.String(), ProviderType: "test", Model: "test-model",
			RequestPayload: json.RawMessage(`{"messages":[{"role":"user","content":"hello"}]}`),
		})
		if err != nil {
			return err
		}
		unsafeFinishReason := "Bearer finish-token https://signed.example/object /Users/private/file " + strings.Repeat("x", 400)
		if err := Finish(ctx, store, publisher, handle, FinishInput{Response: json.RawMessage(`{"content":"done"}`), FinishReason: unsafeFinishReason}); err != nil {
			return err
		}
		var persistedFinishReason string
		if err := store.DB().Table("llm_logs").Select("finish_reason").Where("id=?", handle.ID).Scan(&persistedFinishReason).Error; err != nil {
			return err
		}
		if len(persistedFinishReason) > 255 || strings.Contains(persistedFinishReason, "finish-token") || strings.Contains(persistedFinishReason, "signed.example") || strings.Contains(persistedFinishReason, "/Users/private") {
			return fmt.Errorf("unsafe persisted finish_reason=%q", persistedFinishReason)
		}
		if len(publisher.events) != 2 {
			t.Fatalf("events = %+v", publisher.events)
		}
		for index, status := range []string{"pending", "completed"} {
			event := publisher.events[index]
			if event.topic != "project:"+created.UUID || event.event != EventChanged {
				t.Fatalf("event[%d] = %+v", index, event)
			}
			if len(event.payload) != 3 || event.payload["project_uuid"] != created.UUID || event.payload["log_uuid"] != handle.UUID || event.payload["status"] != status {
				t.Fatalf("payload[%d] = %+v", index, event.payload)
			}
			encoded, _ := json.Marshal(event.payload)
			for _, forbidden := range []string{`"id"`, `"project_id"`, `"provider_uuid"`, `"request_payload"`, "hello"} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("payload leaked %q: %s", forbidden, encoded)
				}
			}
		}

		panicHandle, err := Begin(ctx, store, panickingRecorderEventPublisher{}, StartInput{
			ProjectID: projectID, TaskRunID: taskID, SourceType: SourceStoryGeneration, Scenario: "story_chapter_generation", RequestType: RequestText,
			Attempt: 2, ProviderUUID: providerUUID.String(), ProviderType: "test", Model: "test-model",
			RequestPayload: json.RawMessage(`{"messages":[]}`),
		})
		if err != nil {
			return err
		}
		if err := Finish(ctx, store, panickingRecorderEventPublisher{}, panicHandle, FinishInput{Response: json.RawMessage(`{"content":"done"}`)}); err != nil {
			return err
		}

		atomicHandle, err := Begin(ctx, store, publisher, StartInput{
			ProjectID: projectID, TaskRunID: taskID, SourceType: SourceStoryGeneration, Scenario: "story_chapter_generation", RequestType: RequestText,
			Attempt: 3, ProviderUUID: providerUUID.String(), ProviderType: "test", Model: "test-model",
			RequestPayload: json.RawMessage(`{"messages":[{"role":"user","content":"atomic"}]}`),
		})
		if err != nil {
			return err
		}
		mutationErr := errors.New("budget update failed")
		err = FinishAtomic(ctx, store, publisher, atomicHandle, FinishInput{Response: json.RawMessage(`{"content":"must roll back"}`)}, func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `UPDATE projects SET name='must-not-commit' WHERE id=?`, projectID); err != nil {
				return err
			}
			return mutationErr
		})
		if !errors.Is(err, mutationErr) {
			return fmt.Errorf("FinishAtomic error=%v", err)
		}
		var status, projectName string
		if err := store.DB().Table("llm_logs").Select("status").Where("id=?", atomicHandle.ID).Scan(&status).Error; err != nil {
			return err
		}
		if err := store.DB().Table("projects").Select("name").Where("id=?", projectID).Scan(&projectName).Error; err != nil {
			return err
		}
		if status != "pending" || projectName == "must-not-commit" {
			return fmt.Errorf("atomic rollback status=%q project_name=%q", status, projectName)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
