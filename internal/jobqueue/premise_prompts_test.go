package jobqueue

import (
	"errors"
	"strings"
	"testing"

	"lumi/internal/files"
	"lumi/internal/imagegen"
	"lumi/internal/production"
	"lumi/internal/providerdiag"
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

func TestReferenceImageRejectionClassificationIsExplicit(t *testing.T) {
	for _, err := range []error{
		&imagegen.Error{Code: "image_invalid_input"},
		&imagegen.Error{Code: "image_provider_unsupported"},
		&imagegen.Error{Code: "image_provider_error", Diagnostic: providerdiag.Details{HTTPStatus: 400, Message: "reference image input is unsupported"}},
		&imagegen.Error{Code: "image_provider_error", Diagnostic: providerdiag.Details{HTTPStatus: 422, ProviderCode: "vision_not_enabled"}},
	} {
		if !referenceImageRejected(err) {
			t.Fatalf("expected reference rejection: %v", err)
		}
	}
	for _, err := range []error{
		errors.New("network failure"),
		&imagegen.Error{Code: "image_provider_error", Diagnostic: providerdiag.Details{HTTPStatus: 500, Message: "reference image failed"}},
		&imagegen.Error{Code: "image_provider_error", Diagnostic: providerdiag.Details{HTTPStatus: 400, Message: "invalid prompt"}},
	} {
		if referenceImageRejected(err) {
			t.Fatalf("unexpected reference rejection: %v", err)
		}
	}
}

func TestPremiseSelectionCandidatesDescribeTypeRoleAndBoundedSummary(t *testing.T) {
	asset := production.PremiseAsset{
		UUID: "asset", AssetType: "reference", Title: "Moonlight watercolor", Summary: strings.Repeat("水", 400),
		Tags:           []string{"project-creation-reference", "reference-role-style"},
		CurrentVariant: &production.AssetVariant{UUID: "variant", Asset: files.Asset{UUID: "file"}},
	}
	reference := premiseAssetReference(asset)
	if reference.AssetType != "reference" || reference.ReferenceRole != "style" || len([]rune(reference.Summary)) != 240 {
		t.Fatalf("candidate reference=%+v", reference)
	}
	line := premiseCandidateLines([]production.PremiseAssetReference{reference})
	for _, expected := range []string{"Moonlight watercolor", "asset_type=reference", "reference_role=style", "summary="} {
		if !strings.Contains(line, expected) {
			t.Fatalf("candidate line missing %q: %s", expected, line)
		}
	}
	candidates := make([]production.PremiseAssetReference, 200)
	for index := range candidates {
		candidates[index] = reference
		candidates[index].Title = strings.Repeat("T", 160)
	}
	bounded := premiseCandidateLines(candidates)
	if len([]rune(bounded)) > 24_000 || len(strings.Split(bounded, "\n")) >= len(candidates) {
		t.Fatalf("candidate prompt was not bounded: runes=%d lines=%d", len([]rune(bounded)), len(strings.Split(bounded, "\n")))
	}
}
