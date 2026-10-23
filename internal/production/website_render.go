package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	"gorm.io/gorm"
	"lumi/internal/durablefs"
	"lumi/internal/project"
)

//go:embed website_static/*
var websiteStatic embed.FS

type websiteFile struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}
type websiteManifest struct {
	Files map[string]websiteFile `json:"files"`
	Feed  websiteFeedState       `json:"feed"`
}
type websitePresentation struct {
	Title        string                  `json:"title"`
	Format       string                  `json:"format"`
	MissingCount int                     `json:"missing_count"`
	Chapters     []websiteDisplayChapter `json:"chapters"`
}
type websiteDisplayChapter struct {
	UUID  string               `json:"uuid"`
	Code  string               `json:"code"`
	Title string               `json:"title"`
	Pages []websiteDisplayPage `json:"pages"`
}
type websiteDisplayPage struct {
	Title  string `json:"title"`
	Role   string `json:"role"`
	Number int    `json:"number"`
	Source string `json:"src"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func websiteMediaKey(sha, processor string) string { return hashJSON([]byte(sha + "\n" + processor)) }
func websiteDigest(b []byte) string                { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func websiteWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	return errors.Join(writeErr, f.Sync(), f.Close())
}
func websiteRead(root, relative string) ([]byte, error) {
	path, err := project.ResolveRelativePath(root, relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("website file is not regular")
	}
	return os.ReadFile(path)
}
func websiteFilesPresent(root string, m websiteManifest) bool {
	if len(m.Files) == 0 {
		return false
	}
	for relative, meta := range m.Files {
		path, err := project.ResolveRelativePath(root, relative)
		if err != nil {
			return false
		}
		stat, err := os.Lstat(path)
		if err != nil || !stat.Mode().IsRegular() || stat.Size() != meta.Size {
			return false
		}
	}
	return true
}
func websiteValidate(root string, m websiteManifest) error {
	if len(m.Files) == 0 {
		return fmt.Errorf("empty website manifest")
	}
	for relative, meta := range m.Files {
		b, err := websiteRead(root, relative)
		if err != nil {
			return err
		}
		if int64(len(b)) != meta.Size || websiteDigest(b) != meta.SHA256 {
			return fmt.Errorf("website verification failed")
		}
	}
	return nil
}

func (s *Service) RenderWebsite(ctx context.Context, taskUUID string, progress func(int) error) error {
	return s.store.WithWebsiteBuild(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var generation websiteGeneration
		if err := s.store.DB().WithContext(ctx).Raw(`SELECT g.* FROM website_generations g JOIN production_task_runs t ON t.id=g.task_id WHERE t.uuid=?`, taskUUID).Scan(&generation).Error; err != nil {
			return err
		}
		if generation.ID == 0 {
			return fmt.Errorf("website generation not found")
		}
		// A process exit after the publication transaction must not regenerate.
		if generation.CompletedAt != nil {
			return nil
		}
		var snapshot WebsiteSnapshot
		if err := json.Unmarshal([]byte(generation.SnapshotJSON), &snapshot); err != nil {
			return err
		}
		if snapshot.Version != 1 || snapshot.Processor != WebsiteProcessor {
			return domainError(CodeExportUnavailable, "网站预设已更新", "请重新生成网站。", nil)
		}
		if err := s.RecoverWebsite(ctx); err != nil {
			return err
		}
		stageRelative := ".lumi/website-staging/" + generation.UUID
		stage, err := s.store.ResolvePath(stageRelative)
		if err != nil {
			return err
		}
		if err = os.RemoveAll(stage); err != nil {
			return err
		}
		if err = os.MkdirAll(stage, 0755); err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		previous := websiteManifest{Files: map[string]websiteFile{}}
		old, err := s.currentWebsite(ctx)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if old.ID != 0 {
			if err = json.Unmarshal([]byte(old.ManifestJSON), &previous); err != nil {
				return err
			}
		}
		output, err := s.store.ResolvePath("website")
		if err != nil {
			return err
		}
		if _, err := os.Lstat(output); err == nil && old.ID == 0 {
			return domainError(CodeConflict, "website 目录已存在", "请先将已有 website 目录移走；Lumi 不会接管未生成的目录。", nil)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		manifest := websiteManifest{Files: map[string]websiteFile{}}
		presentation := websitePresentation{Title: snapshot.Title, Format: snapshot.Format, MissingCount: len(snapshot.Missing), Chapters: []websiteDisplayChapter{}}
		total := 0
		for _, chapter := range snapshot.Chapters {
			total += len(chapter.Pages)
		}
		processed, reused, done := 0, 0, 0
		lastProgress := -1
		for _, chapter := range snapshot.Chapters {
			display := websiteDisplayChapter{UUID: chapter.UUID, Code: chapter.Code, Title: chapter.Title, Pages: []websiteDisplayPage{}}
			for _, page := range chapter.Pages {
				if err := ctx.Err(); err != nil {
					return err
				}
				relative := "media/" + websiteMediaKey(page.SHA256, snapshot.Processor) + ".jpg"
				meta, exists := manifest.Files[relative]
				if !exists {
					var b []byte
					if cached, ok := previous.Files[relative]; ok {
						candidate, readErr := websiteRead(output, relative)
						if readErr == nil && int64(len(candidate)) == cached.Size && websiteDigest(candidate) == cached.SHA256 {
							b, meta = candidate, cached
							reused++
						}
					}
					if b == nil {
						var err error
						b, meta, err = s.compressWebsiteImage(ctx, page)
						if err != nil {
							return err
						}
						processed++
					}
					if err := websiteWrite(filepath.Join(stage, relative), b); err != nil {
						return err
					}
					manifest.Files[relative] = meta
				}
				display.Pages = append(display.Pages, websiteDisplayPage{Title: page.Title, Role: page.Role, Number: page.Number, Source: relative, Width: meta.Width, Height: meta.Height})
				done++
				percent := 5 + done*80/max(total, 1)
				if progress != nil && percent != lastProgress {
					if err := progress(percent); err != nil {
						return err
					}
					lastProgress = percent
				}
			}
			presentation.Chapters = append(presentation.Chapters, display)
		}
		data, err := json.Marshal(presentation)
		if err != nil {
			return err
		}
		generated := map[string][]byte{"data/site.js": append([]byte("window.LUMI_WEBSITE = "), append(data, []byte(";\n")...)...), "data/site.json": data}
		feed, feedState, err := renderWebsiteRSS(presentation, previous.Feed, s.now())
		if err != nil {
			return err
		}
		generated["rss.xml"], manifest.Feed = feed, feedState
		for _, name := range []string{"index.html", "README.md", "reader.js", "style.css"} {
			b, err := websiteStatic.ReadFile("website_static/" + name)
			if err != nil {
				return err
			}
			target := "static/" + name
			if name == "index.html" {
				target = name
				b = []byte(strings.ReplaceAll(string(b), "__TITLE__", html.EscapeString(snapshot.Title)))
			} else if name == "README.md" {
				target = name
			}
			generated[target] = b
		}
		marker, _ := json.Marshal(map[string]string{"generation_uuid": generation.UUID})
		generated["data/build.json"] = marker
		for name, b := range generated {
			if err = websiteWrite(filepath.Join(stage, name), b); err != nil {
				return err
			}
			manifest.Files[name] = websiteFile{SHA256: websiteDigest(b), Size: int64(len(b))}
		}
		if err = websiteValidate(stage, manifest); err != nil {
			return err
		}
		if progress != nil {
			if err = progress(95); err != nil {
				return err
			}
		}
		return s.publishWebsite(ctx, taskUUID, generation, stage, previous, manifest, processed, reused)
	})
}

func (s *Service) compressWebsiteImage(ctx context.Context, page WebsitePage) ([]byte, websiteFile, error) {
	content, err := s.files.OpenContent(ctx, page.AssetUUID)
	if err != nil {
		return nil, websiteFile{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(content.File, (64<<20)+1))
	content.File.Close()
	if err != nil {
		return nil, websiteFile{}, err
	}
	if len(raw) > 64<<20 || websiteDigest(raw) != page.SHA256 {
		return nil, websiteFile{}, domainError(CodeExportUnavailable, "源图片校验失败", "请修复图片后重试。", nil)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, websiteFile{}, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 80_000_000 {
		return nil, websiteFile{}, fmt.Errorf("invalid website image dimensions")
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, websiteFile{}, err
	}
	if err = ctx.Err(); err != nil {
		return nil, websiteFile{}, err
	}
	width := min(config.Width, WebsiteWidth)
	height := max(1, int(int64(config.Height)*int64(width)/int64(config.Width)))
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(resized, resized.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(resized, resized.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
	var buffer bytes.Buffer
	if err = jpeg.Encode(&buffer, resized, &jpeg.Options{Quality: 92}); err != nil {
		return nil, websiteFile{}, err
	}
	b := buffer.Bytes()
	return b, websiteFile{SHA256: websiteDigest(b), Size: int64(len(b)), Width: width, Height: height}, nil
}

func websiteSyncTree(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return durablefs.SyncDirectory(path)
		}
		return nil
	})
}
