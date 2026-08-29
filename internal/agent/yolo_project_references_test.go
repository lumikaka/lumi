package agent

import (
	"strings"
	"testing"
)

func TestYoloCompletionMessageUsesInlineProjectReferences(t *testing.T) {
	chapterUUID := mustAgentUUID(t)
	sectionUUID := mustAgentUUID(t)
	message, err := yoloCompletionMessage(map[string]any{
		"chapter_uuid": chapterUUID,
		"section_uuid": sectionUUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[第一章正文](@project/chapters/" + chapterUUID + "/body)",
		"[Story Profile](@project/story-profile)",
		"[Premise](@project/premise)",
		"[六个 Comic Sections](@project/chapters/" + chapterUUID + ")",
		"[首图](@project/chapters/" + chapterUUID + "/sections/" + sectionUUID + ")",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q does not contain %q", message, want)
		}
	}
}

func TestYoloCompletionMessageRejectsInvalidResourceUUID(t *testing.T) {
	if _, err := yoloCompletionMessage(map[string]any{
		"chapter_uuid": "550e8400-e29b-41d4-a716-446655440000",
		"section_uuid": mustAgentUUID(t),
	}); err == nil {
		t.Fatal("expected invalid chapter UUID to be rejected")
	}
}
