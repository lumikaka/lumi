package jobqueue

import (
	"strings"
	"testing"
)

func TestPremiseRenderersDoNotReinsertRemovedPlaceholders(t *testing.T) {
	setting := renderPremiseSettingImagePrompt("CUSTOM SETTING ONLY", "SOURCE SHOULD STAY OUT", "STYLE SHOULD STAY OUT", "zh-Hans", "CUSTOM LANGUAGE")
	if !strings.Contains(setting, "CUSTOM LANGUAGE") || !strings.Contains(setting, "CUSTOM SETTING ONLY") || strings.Contains(setting, "SOURCE SHOULD STAY OUT") || strings.Contains(setting, "STYLE SHOULD STAY OUT") {
		t.Fatalf("setting prompt unexpectedly rewrote removed placeholders: %q", setting)
	}
	breakdown := renderPremiseAssetBreakdownPrompt("CUSTOM BREAKDOWN ONLY", "SOURCE SHOULD STAY OUT", "STYLE SHOULD STAY OUT", "zh-Hans", "CUSTOM LANGUAGE", map[string]any{"secret": "IMAGE INFO SHOULD STAY OUT"})
	if !strings.Contains(breakdown, "CUSTOM LANGUAGE") || !strings.Contains(breakdown, "CUSTOM BREAKDOWN ONLY") || strings.Contains(breakdown, "SOURCE SHOULD STAY OUT") || strings.Contains(breakdown, "STYLE SHOULD STAY OUT") || strings.Contains(breakdown, "IMAGE INFO SHOULD STAY OUT") {
		t.Fatalf("breakdown prompt unexpectedly rewrote removed placeholders: %q", breakdown)
	}
}
