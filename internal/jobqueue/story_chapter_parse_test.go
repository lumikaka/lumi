package jobqueue

import "testing"

func TestParseStoryChapterResponseDefaultsMissingContentFormat(t *testing.T) {
	snapshot := storyGenerationSnapshot{Version: 2, ChapterCode: "vol01.ch01"}
	content, title, format, err := parseStoryChapterResponse(`{"chapter_code":"vol01.ch01","title":"旧楼","content":"正文"}`, snapshot)
	if err != nil || content != "正文" || title != "旧楼" || format != "txt" {
		t.Fatalf("got %q %q %q %v", content, title, format, err)
	}
	if _, _, _, err := parseStoryChapterResponse(`{"chapter_code":"vol01.ch01","title":"旧楼","content":"正文","content_format":"md"}`, snapshot); err == nil {
		t.Fatal("expected non-txt content_format to be rejected")
	}
}
