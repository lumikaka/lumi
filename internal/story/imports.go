package story

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	filestore "lumi/internal/files"

	"gorm.io/gorm"
)

const (
	maxImportFiles = 50
	maxImportBytes = 10 << 20
)

var chapterCodeInFilename = regexp.MustCompile(`(?i)vol[0-9]{2,}\.ch[0-9]{2,}`)

type ImportFile struct {
	Filename    string
	ChapterCode string
	Title       string
	Content     string
}

type ImportResult struct {
	UUID        string          `json:"uuid,omitempty"`
	RequestHash string          `json:"request_hash"`
	Items       []Chapter       `json:"items"`
	Skipped     []ImportSkipped `json:"skipped"`
}

type ImportSkipped struct {
	Filename    string `json:"filename"`
	ChapterCode string `json:"chapter_code"`
	Reason      string `json:"reason"`
}

type validatedImport struct {
	filename    string
	chapterCode string
	title       string
	format      string
	content     string
	contentHash string
	volumeNo    int
	chapterNo   int
	sortOrder   int
}

func looksLikeText(value string) bool {
	controls := 0
	runes := 0
	for _, character := range value {
		runes++
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			controls++
		}
	}
	return controls == 0 || (runes > 0 && controls*100 <= runes)
}

func validateImportFiles(files []ImportFile) ([]validatedImport, string, error) {
	if len(files) == 0 || len(files) > maxImportFiles {
		return nil, "", storyError(CodeValidationFailed, "导入文件数量无效", fmt.Sprintf("一次必须导入 1 到 %d 个文件。", maxImportFiles), nil)
	}
	validated := make([]validatedImport, 0, len(files))
	totalBytes := 0
	for index, item := range files {
		filename := path.Base(strings.ReplaceAll(strings.TrimSpace(item.Filename), "\\", "/"))
		if filename == "." || filename == "" || len([]rune(filename)) > 255 {
			return nil, "", formatImportFailure(index, item.Filename, fmt.Errorf("文件名无效"))
		}
		extension := strings.ToLower(path.Ext(filename))
		if extension != ".txt" && extension != ".md" {
			return nil, "", formatImportFailure(index, filename, fmt.Errorf("只支持 .txt 和 .md"))
		}
		if err := validateText(item.Content, maxChapterBytes, "文件内容"); err != nil {
			return nil, "", formatImportFailure(index, filename, err)
		}
		if !looksLikeText(item.Content) {
			return nil, "", formatImportFailure(index, filename, fmt.Errorf("内容检测为非文本数据"))
		}
		totalBytes += len([]byte(item.Content))
		if totalBytes > maxImportBytes {
			return nil, "", storyError(CodeValidationFailed, "批量导入过大", fmt.Sprintf("单次导入总内容不能超过 %d 字节。", maxImportBytes), nil)
		}
		code := strings.TrimSpace(item.ChapterCode)
		if code == "" {
			code = chapterCodeInFilename.FindString(filename)
		}
		normalizedCode, volume, chapterNo, sortOrder, err := parseChapterCode(code)
		if err != nil {
			return nil, "", formatImportFailure(index, filename, err)
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			stem := strings.TrimSuffix(filename, extension)
			stem = chapterCodeInFilename.ReplaceAllString(stem, "")
			title = strings.Trim(strings.TrimSpace(stem), "-_. ")
		}
		if len([]rune(title)) > 255 {
			return nil, "", formatImportFailure(index, filename, fmt.Errorf("章节标题不能超过 255 个字符"))
		}
		format := strings.TrimPrefix(extension, ".")
		validated = append(validated, validatedImport{filename: filename, chapterCode: normalizedCode, title: title, format: format, content: item.Content, contentHash: contentHash(item.Content), volumeNo: volume, chapterNo: chapterNo, sortOrder: sortOrder})
	}
	var fingerprint strings.Builder
	for _, item := range validated {
		fmt.Fprintf(&fingerprint, "%s\x00%s\x00%s\x00%s\x00%s\x00", item.filename, item.chapterCode, item.title, item.format, item.contentHash)
	}
	return validated, contentHash(fingerprint.String()), nil
}

func importResultWithSkipped(result ImportResult, candidates []validatedImport) ImportResult {
	createdCounts := make(map[string]int, len(result.Items))
	for _, item := range result.Items {
		createdCounts[item.ChapterCode]++
	}
	seen := make(map[string]struct{}, len(candidates))
	result.Skipped = make([]ImportSkipped, 0)
	for _, item := range candidates {
		if createdCounts[item.chapterCode] > 0 {
			createdCounts[item.chapterCode]--
			seen[item.chapterCode] = struct{}{}
			continue
		}
		reason := "existing_chapter"
		if _, duplicate := seen[item.chapterCode]; duplicate {
			reason = "duplicate_code"
		}
		seen[item.chapterCode] = struct{}{}
		result.Skipped = append(result.Skipped, ImportSkipped{Filename: item.filename, ChapterCode: item.chapterCode, Reason: reason})
	}
	return result
}

func (service *Service) importResult(ctx context.Context, source storySourceRecord, candidates []validatedImport) (ImportResult, error) {
	var sourceItems []storySourceItemRecord
	if err := service.store.DB().WithContext(ctx).Where("source_id = ?", source.ID).Order("ordinal ASC").Find(&sourceItems).Error; err != nil {
		return ImportResult{}, err
	}
	items := make([]Chapter, 0, len(sourceItems))
	for _, item := range sourceItems {
		if item.ChapterID == nil {
			continue
		}
		var chapter chapterRecord
		if err := service.store.DB().WithContext(ctx).First(&chapter, *item.ChapterID).Error; err != nil {
			return ImportResult{}, err
		}
		dto, err := service.chapterDTO(ctx, chapter)
		if err != nil {
			return ImportResult{}, err
		}
		items = append(items, dto)
	}
	sort.SliceStable(items, func(left, right int) bool { return items[left].SortOrder < items[right].SortOrder })
	return importResultWithSkipped(ImportResult{UUID: source.UUID, RequestHash: source.RequestHash, Items: items}, candidates), nil
}

func (service *Service) ImportChapters(ctx context.Context, files []ImportFile) (ImportResult, error) {
	validated, requestHash, err := validateImportFiles(files)
	if err != nil {
		return ImportResult{}, err
	}
	projectRecord, actor, err := service.projectAndActor(ctx, service.store.DB())
	if err != nil {
		return ImportResult{}, err
	}
	var existing storySourceRecord
	err = service.store.DB().WithContext(ctx).Where("project_id = ? AND source_type = ? AND request_hash = ?", projectRecord.ID, "file_import", requestHash).First(&existing).Error
	if err == nil {
		return service.importResult(ctx, existing, validated)
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return ImportResult{}, err
	}
	codes := make([]string, 0, len(validated))
	for _, item := range validated {
		codes = append(codes, item.chapterCode)
	}
	var existingCodes []string
	if err := service.store.DB().WithContext(ctx).Model(&chapterRecord{}).
		Where("project_id = ? AND deleted_at IS NULL AND chapter_code IN ?", projectRecord.ID, codes).
		Pluck("chapter_code", &existingCodes).Error; err != nil {
		return ImportResult{}, err
	}
	blockedCodes := make(map[string]struct{}, len(existingCodes)+len(validated))
	for _, code := range existingCodes {
		blockedCodes[code] = struct{}{}
	}
	toCreate := make([]validatedImport, 0, len(validated))
	for _, item := range validated {
		if _, blocked := blockedCodes[item.chapterCode]; blocked {
			continue
		}
		blockedCodes[item.chapterCode] = struct{}{}
		toCreate = append(toCreate, item)
	}
	if len(toCreate) == 0 {
		return importResultWithSkipped(ImportResult{RequestHash: requestHash, Items: []Chapter{}}, validated), nil
	}

	assetService := filestore.NewService(service.store, nil)
	assetIDs := make([]int64, len(toCreate))
	assetUUIDs := make([]string, 0, len(toCreate))
	cleanupAssets := func() {
		for _, assetUUID := range assetUUIDs {
			_, _ = assetService.SoftDelete(context.WithoutCancel(ctx), assetUUID)
		}
	}
	for index, item := range toCreate {
		capturedIndex := index
		asset, commitErr := assetService.CommitReader(ctx, filestore.CommitInput{
			Purpose: "story_import", OriginalFilename: item.filename, DisplayName: item.title,
			SourceType: "imported", Metadata: map[string]any{"chapter_code": item.chapterCode, "content_format": item.format},
			Reader: bytes.NewBufferString(item.content), Bind: func(_ *gorm.DB, fileID int64) error { assetIDs[capturedIndex] = fileID; return nil },
		})
		if commitErr != nil {
			cleanupAssets()
			return ImportResult{}, commitErr
		}
		assetUUIDs = append(assetUUIDs, asset.UUID)
	}
	sourceUUID, err := newUUIDv7()
	if err != nil {
		return ImportResult{}, err
	}
	now := service.now().UTC()
	source := storySourceRecord{UUID: sourceUUID, ProjectID: projectRecord.ID, ActorID: actor.ID, SourceType: "file_import", RequestHash: requestHash, ItemCount: len(toCreate), CreatedAt: now}
	err = service.store.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		for index, item := range toCreate {
			chapterUUID, err := newUUIDv7()
			if err != nil {
				return err
			}
			chapter := chapterRecord{UUID: chapterUUID, ProjectID: projectRecord.ID, VolumeNo: item.volumeNo, ChapterNo: item.chapterNo, ChapterCode: item.chapterCode, SortOrder: item.sortOrder, Title: item.title, Revision: 0, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			itemUUID, err := newUUIDv7()
			if err != nil {
				return err
			}
			filename := item.filename
			fileID := assetIDs[index]
			sourceItem := storySourceItemRecord{UUID: itemUUID, SourceID: source.ID, ChapterID: &chapter.ID, Ordinal: index + 1, OriginalFilename: &filename, ContentFormat: item.format, ContentHash: item.contentHash, ByteSize: int64(len([]byte(item.content))), FileID: &fileID, CreatedAt: now}
			if err := tx.Create(&sourceItem).Error; err != nil {
				return err
			}
			storyUUID, err := newUUIDv7()
			if err != nil {
				return err
			}
			story := chapterStoryRecord{UUID: storyUUID, ChapterID: chapter.ID, ActorID: actor.ID, StorySourceID: source.ID, StorySourceItemID: sourceItem.ID, VersionNo: 1, SourceType: "file_import", Content: item.content, ContentFormat: item.format, ContentHash: item.contentHash, CharCount: len([]rune(item.content)), CreatedAt: now}
			if err := tx.Create(&story).Error; err != nil {
				return err
			}
			if err := tx.Model(&chapter).Updates(map[string]any{"current_story_id": story.ID, "revision": 1, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		cleanupAssets()
		if uniqueConflict(err) {
			var concurrent storySourceRecord
			if lookupErr := service.store.DB().WithContext(ctx).Where("project_id = ? AND source_type = ? AND request_hash = ?", projectRecord.ID, "file_import", requestHash).First(&concurrent).Error; lookupErr == nil {
				return service.importResult(ctx, concurrent, validated)
			}
			return ImportResult{}, storyError(CodeChapterConflict, "导入章节并发冲突", "导入期间章节列表发生变化，请重试；已提交的文件已安全回收。", err)
		}
		return ImportResult{}, err
	}
	return service.importResult(ctx, source, validated)
}
