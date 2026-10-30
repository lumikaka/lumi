package production

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"lumi/internal/project"
	"lumi/internal/story"
)

// TextBook freezes every authored text field in a portable, public-UUID DTO.
type TextBook struct {
	Project      story.ProjectDetail `json:"project"`
	Setup        project.SetupState  `json:"setup"`
	StoryProfile story.StoryProfile  `json:"story_profile"`
	Premise      PremiseProfile      `json:"premise"`
	Sources      []PremiseSource     `json:"sources"`
	Settings     []PremiseAsset      `json:"settings"`
	Chapters     []TextChapter       `json:"chapters"`
}
type TextChapter struct {
	Chapter story.Chapter  `json:"chapter"`
	Pages   []ComicSection `json:"pages"`
}
type TextExport struct {
	UUID              string `json:"uuid"`
	Format            string `json:"format"`
	Path              string `json:"path"`
	ByteSize          int64  `json:"byte_size"`
	MissingImageCount int    `json:"missing_image_count"`
}

func (book TextBook) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Setup\n\n", book.Project.Name)
	setup, _ := json.MarshalIndent(book.Setup, "", "  ")
	fmt.Fprintf(&b, "```json\n%s\n```\n\n## Story Profile\n\n%s\n\n## Visual style\n\n%s\n", setup, book.StoryProfile.StoryMD, book.Premise.DefaultStyle)
	for _, source := range book.Sources {
		fmt.Fprintf(&b, "\n## Premise source %s\n\n%s\n", source.UUID, source.SourceText)
	}
	for _, setting := range book.Settings {
		fmt.Fprintf(&b, "\n## Setting: %s (%s)\n\n%s\n\nTags: %s\n", setting.Title, setting.AssetType, setting.Summary, strings.Join(setting.Tags, ", "))
	}
	for _, chapter := range book.Chapters {
		fmt.Fprintf(&b, "\n## %s · %s\n", chapter.Chapter.ChapterCode, chapter.Chapter.Title)
		if chapter.Chapter.CurrentStory != nil {
			fmt.Fprintf(&b, "\n%s\n", chapter.Chapter.CurrentStory.Content)
		}
		for _, page := range chapter.Pages {
			fmt.Fprintf(&b, "\n### Page %d · %s (%s)\n\nUUID: %s\n\n%s\n", page.SectionNo, page.Title, page.PageRole, page.UUID, page.DescriptionMD)
			if page.CurrentStoryboard != nil {
				fmt.Fprintf(&b, "\n#### Visual description\n\n%s\n", page.CurrentStoryboard.ContentMD)
			}
		}
	}
	return b.String()
}

// ExportTextBook is synchronous and provider-free. PDF uses the established
// image snapshot/renderer. Text exports also work with zero generated images.
// Exports live in .lumi/exports and never accept a caller-selected output path.
func (s *Service) ExportTextBook(ctx context.Context, book TextBook, format string, allowMissing bool) (TextExport, error) {
	uuid, err := newUUIDv7()
	if err != nil {
		return TextExport{}, err
	}
	result := TextExport{UUID: uuid, Format: format}
	pageCount := 0
	for _, ch := range book.Chapters {
		for _, page := range ch.Pages {
			pageCount++
			if page.CurrentImage == nil {
				result.MissingImageCount++
			}
		}
	}
	if pageCount == 0 {
		return result, domainError(CodeExportEmpty, "没有可导出的页面", "先创建正文页面及画面描述。", nil)
	}
	if result.MissingImageCount > 0 && !allowMissing {
		return result, domainError(CodeExportIncomplete, "绘本仍有缺图页面", "补齐图片或设置 allow_missing_images=true。", nil)
	}
	ext := format
	if format == "markdown" {
		ext = "md"
	}
	if format != "markdown" && format != "zip" && format != "pdf" {
		return result, domainError(CodeValidation, "导出格式无效", "format 支持 markdown、zip 或 pdf。", nil)
	}
	relative := filepath.ToSlash(filepath.Join(".lumi", "exports", "text-"+uuid+"."+ext))
	if format == "pdf" {
		snapshot, _, err := s.BuildExportSnapshotForFormat(ctx, "project", "", allowMissing, ExportFormatPDF)
		if err != nil {
			return result, err
		}
		result.ByteSize, _, _, err = s.renderExportToPath(ctx, relative, ExportFormatPDF, snapshot, nil)
		if err == nil {
			result.Path, err = s.store.ResolvePath(relative)
		}
		return result, err
	}
	err = s.store.WithFileCommit(func() error {
		target, err := s.store.ResolvePath(relative)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		// Revalidate after directory creation, using the same traversal checks as
		// existing exports. A unique name and exclusive creation avoid overwrite.
		target, err = s.store.ResolvePath(relative)
		if err != nil {
			return err
		}
		part, err := s.store.ResolvePath(relative + ".part")
		if err != nil {
			return err
		}
		f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		defer os.Remove(part)
		if format == "markdown" {
			_, err = io.WriteString(f, book.Markdown())
		} else {
			err = s.writeTextZIP(ctx, f, book)
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if err := os.Rename(part, target); err != nil {
			return err
		}
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		result.Path, result.ByteSize = target, info.Size()
		return nil
	})
	return result, err
}

func (s *Service) writeTextZIP(ctx context.Context, output io.Writer, book TextBook) error {
	archive := zip.NewWriter(output)
	defer archive.Close()
	write := func(name string, data []byte) error {
		w, err := archive.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := write("book.md", []byte(book.Markdown())); err != nil {
		return err
	}
	b, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return err
	}
	if err := write("book.json", b); err != nil {
		return err
	}
	for _, ch := range book.Chapters {
		bodyPageNo := 0
		for _, page := range ch.Pages {
			if page.PageRole == PageRoleBody {
				bodyPageNo++
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if page.CurrentImage == nil {
				continue
			}
			content, err := s.files.OpenContent(ctx, page.CurrentImage.Asset.UUID)
			if err != nil {
				return err
			}
			extensions, _ := mime.ExtensionsByType(content.Asset.MIMEType)
			extension := "bin"
			if len(extensions) > 0 {
				extension = strings.TrimPrefix(extensions[0], ".")
			}
			entry := ExportEntry{BodyPageNo: bodyPageNo, ChapterCode: ch.Chapter.ChapterCode, SectionUUID: page.UUID, SectionNo: page.SectionNo, PageRole: page.PageRole, Extension: extension}
			name, err := exportZIPEntryName(exportSnapshotV6, entry)
			if err != nil {
				content.File.Close()
				return err
			}
			w, err := archive.Create("images/" + name)
			if err == nil {
				_, err = io.Copy(w, content.File)
			}
			content.File.Close()
			if err != nil {
				return err
			}
		}
	}
	return archive.Close()
}
