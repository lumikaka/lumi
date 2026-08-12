package project

import (
	"strings"
	"testing"
)

func TestGenerationLanguageInstructions(t *testing.T) {
	tests := []struct {
		value      string
		want       string
		visualWant string
	}{
		{value: "", want: "简体中文", visualWant: "简体中文"},
		{value: "zh-CN", want: "简体中文", visualWant: "简体中文"},
		{value: "en", want: "English", visualWant: "English"},
	}
	for _, test := range tests {
		if instruction := GenerationLanguageInstruction(test.value); !strings.Contains(instruction, test.want) {
			t.Fatalf("GenerationLanguageInstruction(%q) = %q", test.value, instruction)
		}
		if instruction := GenerationLanguageVisualInstruction(test.value); !strings.Contains(instruction, test.visualWant) {
			t.Fatalf("GenerationLanguageVisualInstruction(%q) = %q", test.value, instruction)
		}
	}
	if _, valid := NormalizeGenerationLanguage("fr"); valid {
		t.Fatal("unsupported generation language was accepted")
	}
}
