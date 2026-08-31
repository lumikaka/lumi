package jobqueue

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"

	"lumi/internal/files"
	"lumi/internal/production"

	"gorm.io/gorm"
)

const (
	creationReferenceComposerVersion = "creation_reference_board/v1"
	maxCreationReferenceFiles        = 16
	maxCreationReferenceFileBytes    = int64(32 << 20)
)

func (runtime *projectRuntime) prepareCreationReferenceBoard(ctx context.Context, service *production.Service, record productionTaskRecord, snapshot production.GenerationSnapshot) ([]byte, string, error) {
	if len(snapshot.ReferenceFiles) == 0 {
		return nil, "", nil
	}
	if len(snapshot.ReferenceFiles) > maxCreationReferenceFiles || snapshot.ReferenceComposerVersion != creationReferenceComposerVersion {
		return nil, "", productionError("yolo_reference_board_failed", "创建参考图快照数量或合成器版本无效", true)
	}

	var existingUUID string
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT COALESCE(json_extract(output_json,'$.reference_board_file_uuid'),'') FROM premise_generation_steps WHERE task_uuid=?`, record.UUID).Scan(&existingUUID); err != nil {
		return nil, "", err
	}
	if existingUUID != "" {
		if data, err := readCreationReferenceBoard(ctx, service, existingUUID); err == nil {
			if eventErr := runtime.ensureCreationReferencesComposedEvent(ctx, record, snapshot, existingUUID); eventErr != nil {
				return nil, "", eventErr
			}
			return data, existingUUID, nil
		}
	}

	sources := make([]sectionPremiseSource, 0, len(snapshot.ReferenceFiles))
	labels := make([]string, 0, len(snapshot.ReferenceFiles))
	fileUUIDs := make([]string, 0, len(snapshot.ReferenceFiles))
	for index, reference := range snapshot.ReferenceFiles {
		content, err := service.Files().OpenContent(ctx, reference.FileUUID)
		if err != nil {
			return nil, "", productionError("yolo_reference_unavailable", fmt.Sprintf("无法读取创建参考图 %d「%s」：%v", index+1, reference.Title, err), true)
		}
		if content.Asset.Kind != "image" || content.Asset.Purpose != "project_chatbot_reference" || content.Asset.Status != files.ObjectReady || (content.Asset.MIMEType != "image/png" && content.Asset.MIMEType != "image/jpeg" && content.Asset.MIMEType != "image/webp") {
			_ = content.File.Close()
			return nil, "", productionError("yolo_reference_unavailable", fmt.Sprintf("创建参考图 %d「%s」不是可用的首页图片", index+1, reference.Title), true)
		}
		data, readErr := io.ReadAll(io.LimitReader(content.File, maxCreationReferenceFileBytes+1))
		closeErr := content.File.Close()
		if readErr != nil || closeErr != nil || len(data) == 0 || int64(len(data)) > maxCreationReferenceFileBytes {
			return nil, "", productionError("yolo_reference_unavailable", fmt.Sprintf("创建参考图 %d「%s」为空、过大或无法读取", index+1, reference.Title), true)
		}
		decoded, _, decodeErr := image.Decode(bytes.NewReader(data))
		if decodeErr != nil || decoded.Bounds().Dx() <= 0 || decoded.Bounds().Dy() <= 0 {
			return nil, "", productionError("yolo_reference_unavailable", fmt.Sprintf("创建参考图 %d「%s」无法解码", index+1, reference.Title), true)
		}
		decoded = orientCreationReferenceImage(decoded, creationReferenceEXIFOrientation(data))
		sources = append(sources, sectionPremiseSource{Reference: production.PremiseAssetReference{FileUUID: reference.FileUUID, Title: reference.Title}, Image: decoded})
		labels = append(labels, fmt.Sprintf("%d. [%s] %s", reference.Position, reference.ReferenceRole, reference.Title))
		fileUUIDs = append(fileUUIDs, reference.FileUUID)
	}
	face, err := loadSectionPremiseFont(labels, sectionPremiseFontPaths)
	if err != nil {
		return nil, "", productionError("yolo_reference_board_failed", fmt.Sprintf("创建参考板字体不可用：%v", err), true)
	}
	composition, composeErr := composeSectionPremiseWithFace(sources, labels, face)
	_ = face.Close()
	if composeErr != nil || len(composition.Bytes) == 0 {
		return nil, "", productionError("yolo_reference_board_failed", fmt.Sprintf("无法生成创建参考板：%v", composeErr), true)
	}
	if existingUUID != "" {
		asset, repairErr := service.Files().RepairContent(ctx, existingUUID, bytes.NewReader(composition.Bytes))
		if repairErr != nil {
			return nil, "", productionError("yolo_reference_board_failed", fmt.Sprintf("无法修复创建参考板：%v", repairErr), true)
		}
		if eventErr := runtime.ensureCreationReferencesComposedEvent(ctx, record, snapshot, asset.UUID); eventErr != nil {
			return nil, "", eventErr
		}
		return composition.Bytes, asset.UUID, nil
	}

	asset, err := service.Files().CommitReader(ctx, files.CommitInput{
		Purpose: "premise_reference_board", OriginalFilename: "creation-reference-board.png", DisplayName: "Creation reference board", SourceType: "derived",
		Metadata: map[string]any{"generation_task_uuid": record.UUID, "composer_version": creationReferenceComposerVersion, "reference_file_uuids": fileUUIDs, "reference_count": len(fileUUIDs)},
		Reader:   bytes.NewReader(composition.Bytes), Bind: func(tx *gorm.DB, fileID int64) error {
			var fileUUID string
			if err := tx.Table("files").Select("uuid").Where("id=?", fileID).Scan(&fileUUID).Error; err != nil {
				return err
			}
			result := tx.Exec(`UPDATE premise_generation_steps SET output_json=json_set(CASE WHEN json_valid(output_json) THEN output_json ELSE '{}' END,'$.reference_board_file_uuid',?,'$.reference_composer_version',?) WHERE task_uuid=? AND EXISTS(SELECT 1 FROM production_task_runs WHERE uuid=? AND status='running')`, fileUUID, creationReferenceComposerVersion, record.UUID, record.UUID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("premise generation task is no longer running")
			}
			return nil
		},
	})
	if err != nil {
		return nil, "", productionError("yolo_reference_board_failed", fmt.Sprintf("无法持久化创建参考板：%v", err), true)
	}
	if eventErr := runtime.ensureCreationReferencesComposedEvent(ctx, record, snapshot, asset.UUID); eventErr != nil {
		return nil, "", eventErr
	}
	return composition.Bytes, asset.UUID, nil
}

type orientedCreationReferenceImage struct {
	source      image.Image
	orientation int
	bounds      image.Rectangle
}

func orientCreationReferenceImage(source image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return source
	}
	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	if orientation >= 5 {
		width, height = height, width
	}
	return orientedCreationReferenceImage{source: source, orientation: orientation, bounds: image.Rect(0, 0, width, height)}
}

func (value orientedCreationReferenceImage) ColorModel() color.Model {
	return value.source.ColorModel()
}
func (value orientedCreationReferenceImage) Bounds() image.Rectangle { return value.bounds }
func (value orientedCreationReferenceImage) At(x, y int) color.Color {
	if !image.Pt(x, y).In(value.bounds) {
		return color.RGBA{}
	}
	sourceBounds := value.source.Bounds()
	width, height := sourceBounds.Dx(), sourceBounds.Dy()
	sx, sy := x, y
	switch value.orientation {
	case 2:
		sx = width - 1 - x
	case 3:
		sx, sy = width-1-x, height-1-y
	case 4:
		sy = height - 1 - y
	case 5:
		sx, sy = y, x
	case 6:
		sx, sy = y, height-1-x
	case 7:
		sx, sy = width-1-y, height-1-x
	case 8:
		sx, sy = width-1-y, x
	}
	return value.source.At(sourceBounds.Min.X+sx, sourceBounds.Min.Y+sy)
}

func creationReferenceEXIFOrientation(data []byte) int {
	tiff := []byte(nil)
	if marker := bytes.Index(data, []byte{'E', 'x', 'i', 'f', 0, 0}); marker >= 0 && marker+6 < len(data) {
		tiff = data[marker+6:]
	} else if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		for offset := 12; offset+8 <= len(data); {
			size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			start, end := offset+8, offset+8+size
			if size < 0 || end < start || end > len(data) {
				break
			}
			if string(data[offset:offset+4]) == "EXIF" {
				tiff = data[start:end]
				if bytes.HasPrefix(tiff, []byte{'E', 'x', 'i', 'f', 0, 0}) {
					tiff = tiff[6:]
				}
				break
			}
			offset = end + size%2
		}
	}
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 1
	}
	offset := int(order.Uint32(tiff[4:8]))
	if offset < 0 || offset+2 > len(tiff) {
		return 1
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	for index := 0; index < count; index++ {
		entry := offset + 2 + index*12
		if entry < 0 || entry+12 > len(tiff) {
			break
		}
		if order.Uint16(tiff[entry:entry+2]) != 0x0112 || order.Uint16(tiff[entry+2:entry+4]) != 3 || order.Uint32(tiff[entry+4:entry+8]) != 1 {
			continue
		}
		orientation := int(order.Uint16(tiff[entry+8 : entry+10]))
		if orientation >= 1 && orientation <= 8 {
			return orientation
		}
	}
	return 1
}

func readCreationReferenceBoard(ctx context.Context, service *production.Service, fileUUID string) ([]byte, error) {
	content, err := service.Files().OpenContent(ctx, fileUUID)
	if err != nil {
		return nil, err
	}
	defer content.File.Close()
	if content.Asset.Purpose != "premise_reference_board" || content.Asset.MIMEType != "image/png" || content.Asset.Status != files.ObjectReady {
		return nil, errors.New("persisted creation reference board is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(content.File, 64<<20+1))
	if err != nil || len(data) == 0 || len(data) > 64<<20 {
		return nil, errors.New("persisted creation reference board is empty or too large")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}

func (runtime *projectRuntime) ensureCreationReferencesComposedEvent(ctx context.Context, record productionTaskRecord, snapshot production.GenerationSnapshot, boardFileUUID string) error {
	var exists bool
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM production_task_events WHERE production_task_run_id=? AND event_type='creation_references_composed' AND json_extract(payload,'$.board_file_uuid')=?)`, record.ID, boardFileUUID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	references := make([]map[string]any, 0, len(snapshot.ReferenceFiles))
	for _, reference := range snapshot.ReferenceFiles {
		references = append(references, map[string]any{"reference_uuid": reference.ReferenceUUID, "file_uuid": reference.FileUUID, "position": reference.Position, "reference_role": reference.ReferenceRole, "title": strings.TrimSpace(reference.Title)})
	}
	return runtime.appendProductionEvent(ctx, record.ID, "creation_references_composed", map[string]any{"board_file_uuid": boardFileUUID, "composer_version": creationReferenceComposerVersion, "reference_count": len(references), "references": references})
}
