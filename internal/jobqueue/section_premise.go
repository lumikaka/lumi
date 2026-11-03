package jobqueue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"strings"

	"lumi/internal/platformpath"
	"lumi/internal/production"

	"golang.org/x/image/font"
)

const maxSectionPremiseAssets = production.MaxSectionPremiseAssets
const sectionPremiseComposerVersion = production.SectionPremiseComposerVersion

type sectionPremiseSource = production.SectionPremiseSource
type sectionPremiseComposition = production.SectionPremiseComposition

var sectionPremiseFontPaths = platformpath.CJKFontPaths()

func composeSectionPremise(sources []sectionPremiseSource) (sectionPremiseComposition, error) {
	return production.ComposeSectionPremise(sources)
}
func composeSectionPremiseWithFace(sources []sectionPremiseSource, labels []string, face font.Face) (sectionPremiseComposition, error) {
	return production.ComposeSectionPremiseWithFace(sources, labels, face)
}
func loadSectionPremiseFont(labels, paths []string) (font.Face, error) {
	return production.LoadSectionPremiseFont(labels, paths)
}

func (runtime *projectRuntime) prepareSectionPremise(ctx context.Context, service *production.Service, record productionTaskRecord, generationUUID, sectionUUID string, selection sectionReferenceSelection) (*production.SectionPremise, []byte, error) {
	if len(selection.References) == 0 {
		return nil, nil, nil
	}
	if len(selection.References) > maxSectionPremiseAssets {
		return nil, nil, productionError("too_many_section_premise_assets", fmt.Sprintf("Section 设定参考图最多允许 %d 个", maxSectionPremiseAssets), false)
	}

	existing, err := service.GenerationSectionPremise(ctx, generationUUID)
	if err != nil {
		return nil, nil, err
	}
	canRepairExisting := sectionPremiseMatches(existing, selection)
	if canRepairExisting {
		if data, readErr := readSectionPremiseAsset(ctx, service, existing); readErr == nil {
			if eventErr := runtime.ensureSectionPremiseComposedEvent(ctx, record.ID, sectionUUID, *existing); eventErr != nil {
				return nil, nil, eventErr
			}
			return existing, data, nil
		}
	}

	sources, _, err := service.ReadSectionPremiseSources(ctx, selection.References)
	if err != nil {
		return nil, nil, err
	}

	composition, composeErr := composeSectionPremise(sources)
	if composeErr != nil {
		code := "section_premise_composition_failed"
		if strings.Contains(composeErr.Error(), "CJK font") {
			code = "section_premise_font_unavailable"
		}
		return nil, nil, productionError(code, fmt.Sprintf("无法生成 Section 设定参考合图：%v", composeErr), false)
	}
	titles := make([]string, len(selection.References))
	for index, reference := range selection.References {
		titles[index] = reference.Title
	}
	metadata := production.SectionPremiseMetadata{
		SelectedAssets:  append([]production.PremiseAssetReference(nil), selection.References...),
		SelectedTitles:  titles,
		SelectionReason: strings.TrimSpace(selection.Reason),
		ImageInfo: production.SectionPremiseImageInfo{
			Width: composition.Width, Height: composition.Height, MIMEType: "image/png", ComposerVersion: sectionPremiseComposerVersion,
		},
	}
	if canRepairExisting {
		repaired, repairErr := service.Files().RepairContent(ctx, existing.Asset.UUID, bytes.NewReader(composition.Bytes))
		if repairErr != nil {
			return nil, nil, repairErr
		}
		existing.Asset = repaired
		if eventErr := runtime.ensureSectionPremiseComposedEvent(ctx, record.ID, sectionUUID, *existing); eventErr != nil {
			return nil, nil, eventErr
		}
		return existing, composition.Bytes, nil
	}
	premise, commitErr := service.CommitGeneratedSectionPremise(ctx, generationUUID, sectionUUID, metadata, bytes.NewReader(composition.Bytes))
	if commitErr != nil {
		return nil, nil, commitErr
	}
	if eventErr := runtime.ensureSectionPremiseComposedEvent(ctx, record.ID, sectionUUID, premise); eventErr != nil {
		return nil, nil, eventErr
	}
	return &premise, composition.Bytes, nil
}

func sectionPremiseMatches(premise *production.SectionPremise, selection sectionReferenceSelection) bool {
	if premise == nil || premise.ImageInfo.ComposerVersion != sectionPremiseComposerVersion || premise.ImageInfo.MIMEType != "image/png" || premise.ImageInfo.Width <= 0 || premise.ImageInfo.Height <= 0 {
		return false
	}
	if len(premise.SelectedAssets) != len(selection.References) || premise.SelectionReason != strings.TrimSpace(selection.Reason) {
		return false
	}
	for index, expected := range selection.References {
		actual := premise.SelectedAssets[index]
		if actual.AssetUUID != expected.AssetUUID || actual.VariantUUID != expected.VariantUUID || actual.FileUUID != expected.FileUUID || actual.Title != expected.Title {
			return false
		}
	}
	return true
}

func readSectionPremiseAsset(ctx context.Context, service *production.Service, premise *production.SectionPremise) ([]byte, error) {
	if premise == nil || premise.Asset.UUID == "" || premise.Asset.MIMEType != "image/png" {
		return nil, errors.New("persisted section premise asset is not a PNG")
	}
	content, err := service.Files().OpenContent(ctx, premise.Asset.UUID)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(content.File, 64<<20+1))
	closeErr := content.File.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) == 0 || len(data) > 64<<20 {
		return nil, errors.New("persisted section premise PNG is empty or too large")
	}
	decoded, decodeErr := png.Decode(bytes.NewReader(data))
	if decodeErr != nil {
		return nil, decodeErr
	}
	if decoded.Bounds().Dx() != premise.ImageInfo.Width || decoded.Bounds().Dy() != premise.ImageInfo.Height {
		return nil, errors.New("persisted section premise PNG dimensions do not match metadata")
	}
	return data, nil
}

func (runtime *projectRuntime) ensureSectionPremiseComposedEvent(ctx context.Context, taskID int64, sectionUUID string, premise production.SectionPremise) error {
	var exists bool
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM production_task_events WHERE production_task_run_id=? AND event_type='section_premise_composed' AND json_extract(payload,'$.file_uuid')=?)`, taskID, premise.Asset.UUID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return runtime.appendProductionEvent(ctx, taskID, "section_premise_composed", map[string]any{
		"file_uuid": premise.Asset.UUID, "section_uuid": sectionUUID, "selected_titles": premise.SelectedTitles,
		"width": premise.ImageInfo.Width, "height": premise.ImageInfo.Height,
	})
}
