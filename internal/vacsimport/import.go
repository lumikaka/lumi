// Package vacsimport converts a versioned VACS content snapshot using Lumi's
// normal project, story, production and Asset Store services. It never creates
// a second SQLite schema or imports execution/session history.
package vacsimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm/logger"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/files"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"
)

const Version = 1

type Attachment struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Attachments []Attachment `json:"attachments"`
	Format      string       `json:"format"`
	Version     int          `json:"version"`
	ProjectID   string       `json:"project_id"`
	Name        string       `json:"name"`
	Language    string       `json:"language"`
	StoryMD     string       `json:"story_md"`
	Chapters    []Chapter    `json:"chapters"`
	Assets      []Asset      `json:"assets"`
	Settings    []Setting    `json:"settings"`
	Files       []File       `json:"files"`
	Prompts     []Prompt     `json:"prompts"`
}
type Chapter struct {
	ID       string    `json:"id"`
	Code     string    `json:"code"`
	Title    string    `json:"title"`
	Content  string    `json:"content"`
	Format   string    `json:"format"`
	Sections []Section `json:"sections"`
}
type Section struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Storyboard      string   `json:"storyboard"`
	ImageFileID     string   `json:"image_file_id"`
	AssetIDs        []string `json:"asset_ids"`
	ReferenceFileID string   `json:"reference_file_id"`
	ImageProvenance struct {
		Prompt          string `json:"prompt"`
		PromptSHA256    string `json:"prompt_sha256"`
		RevisedPrompt   string `json:"revised_prompt"`
		OriginalMissing bool   `json:"original_missing"`
	} `json:"image_provenance"`
}
type Asset struct {
	SettingID string          `json:"setting_id"`
	Crop      json.RawMessage `json:"crop_box"`
	Position  int             `json:"position"`
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Summary   string          `json:"summary"`
	FileID    string          `json:"file_id"`
	Type      string          `json:"type"`
	Tags      []string        `json:"tags"`
}
type Setting struct {
	ID     string `json:"id"`
	FileID string `json:"file_id"`
	Prompt string `json:"prompt"`
}
type File struct {
	Size            int64    `json:"size"`
	SHA256          string   `json:"sha256"`
	SHA1            string   `json:"sha1"`
	ID              string   `json:"id"`
	RegisterPurpose string   `json:"register_purpose"`
	Path            string   `json:"path"`
	Filename        string   `json:"filename"`
	Locations       []string `json:"locations"`
	// The original manifest (including provenance fields not interpreted here)
	// is preserved verbatim in user-data/vacs/manifest.json.
}
type Prompt struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Group  string `json:"group"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}
type Mapping struct {
	Group  string `json:"group"`
	Key    string `json:"key"`
	Path   string `json:"path"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
}

// createProject owns the native Lumi lifecycle; the package reader owns input
// verification and exclusive publication of the resulting directory.
func createProject(ctx context.Context, root, appDir, inputDir string, m Manifest, raw []byte) error {
	app, err := appstore.Open(appDir, config.SQLiteDSN(filepath.Join(appDir, "lumi.sqlite")))
	if err != nil {
		return err
	}
	defer app.Close()
	manager := project.NewManager(app)
	defer manager.Close()
	initialized, err := manager.InitAt(ctx, root, project.CreateInput{Name: m.Name, GenerationLanguage: m.Language, PictureBook: &project.PictureBookInput{Format: project.PictureBookVertical}})
	if err != nil {
		return err
	}
	err = manager.WithStore(ctx, initialized.UUID, func(store *project.Store) error {
		return populate(ctx, store, m, raw, inputDir)
	})
	if err != nil {
		return err
	}
	if _, err = manager.CloseProject(ctx, initialized.UUID); err != nil {
		return err
	}
	// Exercise the same open contract as the desktop "Open existing project".
	opened, err := manager.OpenSelected(ctx, project.ExplicitExistingDirectory(root))
	if err != nil {
		return err
	}
	if _, err = manager.CloseProject(ctx, opened.UUID); err != nil {
		return err
	}
	return nil
}

func populate(ctx context.Context, store *project.Store, m Manifest, raw []byte, inputDir string) error {
	store.DB().Logger = store.DB().Logger.LogMode(logger.Silent)
	s := story.NewService(store)
	p := production.NewService(store, nil)
	fsrv := p.Files()
	attachmentDir := filepath.Join(store.Root(), "user-data", "vacs")
	if err := os.MkdirAll(filepath.Join(attachmentDir, "files"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(attachmentDir, "prompts"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(attachmentDir, "manifest.json"), raw, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(attachmentDir, "README.md"), []byte("# VACS content archive\n\nThe Lumi project is ready to edit. These files preserve the VACS originals.\n\n- manifest.json: chapters, pages, assets, candidates and their original IDs.\n- content-mapping.json: VACS chapter/page/asset IDs to Lumi UUIDs.\n- conversion-notices.json: display title adjustments, original labels and source locations.\n- files/: originals and candidate images, named by VACS file ID.\n- prompts/: effective project prompts and mapping.json with mapping results.\n- image-prompts.json: original/revised image prompts and hashes; original_missing marks unavailable originals.\n\nCandidates are attached files; they are not imported as Lumi history. No chats or execution/billing history were migrated.\n"), 0644); err != nil {
		return err
	}
	fileMap := map[string]File{}
	contentMap := map[string]string{}
	for _, f := range m.Files {
		if _, exists := fileMap[f.ID]; exists || f.ID == "" {
			return fmt.Errorf("duplicate/empty file id %q", f.ID)
		}
		// IDs and relative paths are data, never filesystem authority.
		if !safePart(f.ID) || !fs.ValidPath(f.Path) {
			return fmt.Errorf("invalid file path for %q", f.ID)
		}
		source := filepath.Join(inputDir, filepath.FromSlash(f.Path))
		info, err := os.Lstat(source)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("unreadable file %s at %v: %v", f.ID, f.Locations, err)
		}
		// Retain every original, including candidates and unknown formats, unchanged.
		f.Path = filepath.Join(attachmentDir, "files", f.ID+filepath.Ext(f.Filename))
		if err := copyFile(source, f.Path); err != nil {
			return err
		}
		fileMap[f.ID] = f
	}
	upload := func(id, purpose string) (files.Upload, error) {
		f, ok := fileMap[id]
		if !ok {
			return files.Upload{}, fmt.Errorf("unresolved file %s (%s)", id, purpose)
		}
		in, err := os.Open(f.Path)
		if err != nil {
			return files.Upload{}, err
		}
		defer in.Close()
		return fsrv.CreateUpload(ctx, files.CreateUploadInput{Purpose: purpose, OriginalFilename: f.Filename, Reader: in})
	}
	// Register originals not used by domain imports as formal files too, whenever
	// their purpose/format is supported. Candidates remain in the attached archive.
	for _, setting := range m.Settings {
		u, err := upload(setting.FileID, "premise_setting_image")
		if err != nil {
			return err
		}
		imported, err := p.ImportSettingImage(ctx, u.UUID, "", setting.Prompt)
		if err != nil {
			return err
		}
		contentMap["file/"+setting.FileID] = imported.Asset.UUID
		contentMap["setting/"+setting.ID] = imported.UUID
	}
	assets := map[string]string{}
	nativeAssets := map[string]production.PremiseAsset{}
	for _, a := range m.Assets {
		kind := a.Type
		if kind == "" {
			kind = production.AssetReference
		}
		input := production.CreateAssetInput{AssetType: kind, Title: a.Title, Summary: a.Summary, Tags: a.Tags, Position: map[string]any{"vacs_order": a.Position}}
		if len(a.Crop) > 0 {
			input.Crop = a.Crop
		}
		if a.SettingID != "" {
			input.SourceSettingImageUUID = contentMap["setting/"+a.SettingID]
			if input.SourceSettingImageUUID == "" {
				return fmt.Errorf("asset %s: source setting %s is missing", a.ID, a.SettingID)
			}
		}
		var result production.PremiseAsset
		var err error
		if a.FileID != "" {
			u, e := upload(a.FileID, "premise_asset")
			if e != nil {
				return e
			}
			input.UploadUUID = u.UUID
			result, err = p.ImportPremiseAsset(ctx, input)
		} else {
			profile, e := p.GetPremise(ctx)
			if e != nil {
				return e
			}
			result, err = p.CreateTextPremiseAsset(ctx, input, profile.Revision)
		}
		if err != nil {
			return fmt.Errorf("asset %s: %w", a.ID, err)
		}
		assets[a.ID] = result.UUID
		nativeAssets[a.ID] = result
		contentMap["asset/"+a.ID] = result.UUID
		if result.CurrentVariant != nil {
			contentMap["file/"+a.FileID] = result.CurrentVariant.Asset.UUID
		}
	}
	chapterOrder := []string{}
	for _, c := range m.Chapters {
		chapter, err := s.CreateChapter(ctx, story.CreateChapterInput{ChapterCode: c.Code, Title: c.Title, Content: c.Content, ContentFormat: c.Format})
		if err != nil {
			return fmt.Errorf("chapter %s: %w", c.ID, err)
		}
		chapterOrder = append(chapterOrder, chapter.UUID)
		contentMap["chapter/"+c.ID] = chapter.UUID
		for _, section := range c.Sections {
			page, err := p.CreateSection(ctx, chapter.UUID, production.CreateSectionInput{Title: section.Title, StoryboardMD: section.Storyboard})
			if err != nil {
				return err
			}
			contentMap["page/"+section.ID] = page.UUID
			if section.ImageFileID != "" {
				u, e := upload(section.ImageFileID, "comic_section_image")
				if e != nil {
					return e
				}
				snapshot := production.ImportedImageSnapshot{Prompt: section.ImageProvenance.Prompt, PromptSHA256: section.ImageProvenance.PromptSHA256, RevisedPrompt: section.ImageProvenance.RevisedPrompt, OriginalMissing: section.ImageProvenance.OriginalMissing || section.ImageProvenance.Prompt == ""}
				if section.ReferenceFileID != "" {
					referenceUUID := contentMap["file/"+section.ReferenceFileID]
					if referenceUUID == "" {
						u, e := upload(section.ReferenceFileID, "premise_asset")
						if e != nil {
							return e
						}
						f, e := fsrv.FinalizeUpload(ctx, u.UUID, "premise_asset")
						if e != nil {
							return e
						}
						referenceUUID = f.UUID
						contentMap["file/"+section.ReferenceFileID] = f.UUID
					}
					snapshot.ReferenceFileUUID = referenceUUID
					for _, id := range section.AssetIDs {
						a := nativeAssets[id]
						if a.CurrentVariant != nil {
							snapshot.ReferenceMetadata.SelectedAssets = append(snapshot.ReferenceMetadata.SelectedAssets, production.PremiseAssetReference{AssetUUID: a.UUID, VariantUUID: a.CurrentVariant.UUID, FileUUID: a.CurrentVariant.Asset.UUID, Title: a.Title})
							snapshot.ReferenceMetadata.SelectedTitles = append(snapshot.ReferenceMetadata.SelectedTitles, a.Title)
						}
					}
				}
				page, err = p.ImportSectionImageWithSnapshot(ctx, chapter.UUID, page.UUID, u.UUID, snapshot, page.Revision)
				if err != nil {
					return err
				}
				contentMap["file/"+section.ImageFileID] = page.CurrentImage.Asset.UUID
			}
			refs := []string{}
			for _, id := range section.AssetIDs {
				uuid, ok := assets[id]
				if !ok {
					return fmt.Errorf("page %s references unknown asset %s", section.ID, id)
				}
				refs = append(refs, uuid)
			}
			if len(refs) > 0 {
				if _, err = p.SetSectionPremiseAssets(ctx, chapter.UUID, page.UUID, refs, page.Revision); err != nil {
					return err
				}
			}
		}
	}
	if len(chapterOrder) > 0 {
		if _, err := s.ReorderChapters(ctx, chapterOrder); err != nil {
			return err
		}
	}
	if m.StoryMD != "" {
		profile, err := s.GetStoryProfile(ctx)
		if err != nil {
			return err
		}
		if _, err = s.UpdateStoryProfile(ctx, m.StoryMD, profile.Revision); err != nil {
			return err
		}
	}
	for _, f := range m.Files {
		if f.RegisterPurpose == "" || contentMap["file/"+f.ID] != "" {
			continue
		}
		u, err := upload(f.ID, f.RegisterPurpose)
		if err != nil {
			return err
		}
		a, err := fsrv.FinalizeUpload(ctx, u.UUID, f.RegisterPurpose)
		if err != nil {
			return err
		}
		contentMap["file/"+f.ID] = a.UUID
	}
	var original map[string]json.RawMessage
	if err := json.Unmarshal(raw, &original); err != nil {
		return err
	}
	if prompts, ok := original["image_prompts"]; ok {
		if err := os.WriteFile(filepath.Join(attachmentDir, "image-prompts.json"), prompts, 0644); err != nil {
			return err
		}
	}
	mapped, err := json.MarshalIndent(contentMap, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(attachmentDir, "content-mapping.json"), mapped, 0644); err != nil {
		return err
	}
	mappings := []Mapping{}
	for i, prompt := range m.Prompts {
		path := prompt.Path
		if path == "" {
			path = fmt.Sprintf("prompts/%03d.txt", i+1)
		}
		if err := os.WriteFile(filepath.Join(attachmentDir, path), []byte(prompt.Value), 0644); err != nil {
			return err
		}
		mapping := Mapping{Group: prompt.Group, Key: prompt.Key, Path: path, Reason: "Archived: VACS and Lumi task/output contracts differ; no automatic mapping."}
		if prompt.Group == "premise_style" && prompt.Key == "project_overall_style" {
			mapping.Reason = "Archived: style is empty or contains unsupported template variables."
		}
		// A free-form project visual style has the same purpose in both systems.
		// Templates are deliberately excluded even if they share a name.
		if prompt.Group == "premise_style" && prompt.Key == "project_overall_style" && len(promptcatalog.Placeholders(prompt.Value)) == 0 && strings.TrimSpace(prompt.Value) != "" {
			catalog, err := s.ListPromptCatalog(ctx, "premise_style")
			if err != nil {
				return err
			}
			expected := 0
			for _, item := range catalog {
				if item.PromptKey == prompt.Key && item.CurrentVersion != nil {
					expected = item.CurrentVersion.VersionNo
				}
			}
			if _, err = s.CreatePromptVersion(ctx, story.CreatePromptInput{PromptGroup: prompt.Group, PromptKey: prompt.Key, Prompt: prompt.Value, ExpectedCurrentVersion: expected}); err != nil {
				return err
			}
			mapping.Target = "premise_style/project_overall_style"
			mapping.Reason = "Mapped: compatible free-form visual style; no template variables."
		}
		mappings = append(mappings, mapping)
	}
	encoded, err := json.MarshalIndent(mappings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(attachmentDir, "prompts", "mapping.json"), encoded, 0644)
}
func safePart(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\:\x00")
}
func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return joinClose(err, out.Close())
}
func joinClose(err, closeErr error) error {
	if err != nil {
		return err
	}
	return closeErr
}
