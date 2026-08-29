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
		"[Comic Sections](@project/chapters/" + chapterUUID + ")",
		"[首图](@project/chapters/" + chapterUUID + "/sections/" + sectionUUID + ")",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q does not contain %q", message, want)
		}
	}
}

func TestYoloCompletionMessageUsesSegmentTermsForVerticalV5(t *testing.T) {
	chapterUUID := mustAgentUUID(t)
	bodyUUID := mustAgentUUID(t)
	bodyImageUUID := mustAgentUUID(t)
	message, err := yoloCompletionMessage(map[string]any{
		"chapter_uuid": chapterUUID, "section_uuid": bodyUUID,
		"body_section_uuid": bodyUUID, "body_image_variant_uuid": bodyImageUUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[画面段落](@project/chapters/" + chapterUUID + ")",
		"[首个画面段落](@project/chapters/" + chapterUUID + "/sections/" + bodyUUID + ")",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q does not contain %q", message, want)
		}
	}
}

func TestYoloCompletionMessageLinksCoverAndFirstBodyPage(t *testing.T) {
	chapterUUID := mustAgentUUID(t)
	coverUUID := mustAgentUUID(t)
	bodyUUID := mustAgentUUID(t)
	message, err := yoloCompletionMessage(map[string]any{
		"chapter_uuid": chapterUUID, "section_uuid": bodyUUID,
		"body_section_uuid": bodyUUID, "cover_section_uuid": coverUUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[页面脚本](@project/chapters/" + chapterUUID + ")",
		"[封面](@project/chapters/" + chapterUUID + "/sections/" + coverUUID + ")",
		"[正文第一页](@project/chapters/" + chapterUUID + "/sections/" + bodyUUID + ")",
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
