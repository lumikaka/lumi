package vacsimport

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

func sample(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "one.png"), data.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Version: 1, Name: "VACS sample", Language: "zh-Hans", StoryMD: "# 故事\n两位朋友出发。", Assets: []Asset{{ID: "shared", Title: "朋友", Summary: "共同设定图", FileID: "image"}}, Files: []File{{ID: "image", Path: "one.png", Filename: "one.png", Locations: []string{"asset/shared", "chapter/one/page/one"}}, {ID: "candidate", Path: "one.png", Filename: "candidate.png", Locations: []string{"chapter/one/page/one/candidate"}}}, Prompts: []Prompt{{Group: "premise_style", Key: "project_overall_style", Value: "自定义水彩风格"}, {Group: "story", Key: "story_profile", Value: "VACS-only {{input_prompt}}"}}, Chapters: []Chapter{{ID: "one", Code: "vol01.ch01", Title: "第一章", Content: "第一章正文", Format: "md", Sections: []Section{{ID: "page1", Title: "出发", Storyboard: "分镜一", ImageFileID: "image", ReferenceFileID: "image", AssetIDs: []string{"shared"}}, {ID: "page2", Title: "未出图", Storyboard: "分镜二", AssetIDs: []string{"shared"}}}}, {ID: "two", Code: "vol01.ch02", Title: "第二章", Content: "第二章正文", Format: "txt"}}}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	return path, data.Bytes()
}

// packManifest constructs an independent producer fixture using the public ZIP
// contract, including payload checksums and readable project prompt files.
func packManifest(t *testing.T, manifestPath string) string {
	t.Helper()
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Format = Format
	payloads := map[string][]byte{}
	for i, f := range m.Files {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(manifestPath), f.Path))
		if err != nil {
			t.Fatal(err)
		}
		f.Path = "files/" + f.ID + filepath.Ext(f.Filename)
		f.Size = int64(len(data))
		sum := sha256.Sum256(data)
		f.SHA256 = hex.EncodeToString(sum[:])
		m.Files[i] = f
		payloads[f.Path] = data
	}
	for i, p := range m.Prompts {
		p.Path = fmt.Sprintf("prompts/%03d.txt", i+1)
		p.Size = int64(len(p.Value))
		sum := sha256.Sum256([]byte(p.Value))
		p.SHA256 = hex.EncodeToString(sum[:])
		m.Prompts[i] = p
		payloads[p.Path] = []byte(p.Value)
	}
	for _, a := range []struct {
		path string
		data []byte
	}{{"README.md", []byte("VACS project package")}, {"image-prompts.json", []byte("[]")}} {
		sum := sha256.Sum256(a.data)
		m.Attachments = append(m.Attachments, Attachment{Path: a.path, Size: int64(len(a.data)), SHA256: hex.EncodeToString(sum[:])})
		payloads[a.path] = a.data
	}
	raw, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	payloads["manifest.json"] = raw
	return writeZIP(t, payloads)
}
func writeZIP(t *testing.T, payloads map[string][]byte) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "输入项目.vacs.zip")
	out, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(out)
	for name, data := range payloads {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}
func TestSampleZIPNormalOpenEditSaveReopen(t *testing.T) {
	ctx := context.Background()
	manifest, imageBytes := sample(t)
	inputZIP := packManifest(t, manifest)
	root := filepath.Join(t.TempDir(), "sample-project")
	if err := ConvertPackage(ctx, inputZIP, root); err != nil {
		t.Fatal(err)
	}
	appDir := t.TempDir()
	app, err := appstore.Open(appDir, config.SQLiteDSN(filepath.Join(appDir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manager := project.NewManager(app)
	defer manager.Close()
	opened, err := manager.OpenSelected(ctx, project.ExplicitExistingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	var chapterID, pageID string
	err = manager.WithStore(ctx, opened.UUID, func(store *project.Store) error {
		s := story.NewService(store)
		profileSpec := store.OptionalPictureBookProfile()
		if profileSpec == nil || profileSpec.Format != project.PictureBookVertical {
			t.Fatalf("wrong project spec: %+v", profileSpec)
		}
		p := production.NewService(store, nil)
		chapters, e := s.ListChapters(ctx, "active")
		if e != nil {
			return e
		}
		if len(chapters) != 2 || chapters[0].Title != "第一章" || chapters[1].CurrentStory.Content != "第二章正文" || chapters[0].CurrentStory.Content != "第一章正文" {
			t.Fatalf("chapters=%+v", chapters)
		}
		profile, e := s.GetStoryProfile(ctx)
		if e != nil {
			return e
		}
		if profile.StoryMD != "# 故事\n两位朋友出发。" {
			t.Fatalf("profile=%+v", profile)
		}
		pages, e := p.ListSections(ctx, chapters[0].UUID)
		if e != nil {
			return e
		}
		if len(pages) != 2 || pages[0].CurrentStoryboard.ContentMD != "分镜一" || pages[1].CurrentStoryboard.ContentMD != "分镜二" || pages[1].CurrentImage != nil {
			t.Fatalf("pages=%+v", pages)
		}
		if pages[0].CurrentImage.SectionPremise == nil || len(pages[0].CurrentImage.SectionPremise.SelectedAssets) != 1 {
			t.Fatal("current artwork reference snapshot missing")
		}
		if len(pages[0].PremiseAssets) != 1 || len(pages[1].PremiseAssets) != 1 || pages[0].PremiseAssets[0].AssetUUID != pages[1].PremiseAssets[0].AssetUUID {
			t.Fatalf("shared references=%+v", pages)
		}
		content, e := p.Files().OpenContent(ctx, pages[0].CurrentImage.Asset.UUID)
		if e != nil {
			return e
		}
		got, e := io.ReadAll(content.File)
		content.File.Close()
		if e != nil {
			return e
		}
		if !bytes.Equal(got, imageBytes) {
			t.Fatal("image bytes changed")
		}
		setting, e := p.GetPremiseAsset(ctx, pages[0].PremiseAssets[0].AssetUUID)
		if e != nil {
			return e
		}
		assetContent, e := p.Files().OpenContent(ctx, setting.CurrentVariant.Asset.UUID)
		if e != nil {
			return e
		}
		got, e = io.ReadAll(assetContent.File)
		assetContent.File.Close()
		if e != nil {
			return e
		}
		if !bytes.Equal(got, imageBytes) {
			t.Fatal("shared image bytes changed")
		}
		style, e := s.EffectivePrompt(ctx, "premise_style", "project_overall_style")
		if e != nil {
			return e
		}
		if style != "自定义水彩风格" {
			t.Fatal(style)
		}
		chapterID, pageID = chapters[0].UUID, pages[1].UUID
		edited := "缺图页修改后分镜"
		_, e = p.UpdateSection(ctx, chapterID, pageID, production.UpdateSectionInput{StoryboardMD: &edited, ExpectedRevision: pages[1].Revision})
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.CloseProject(ctx, opened.UUID); err != nil {
		t.Fatal(err)
	}
	opened, err = manager.OpenSelected(ctx, project.ExplicitExistingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.WithStore(ctx, opened.UUID, func(store *project.Store) error {
		page, e := production.NewService(store, nil).GetSection(ctx, chapterID, pageID)
		if e == nil && page.CurrentStoryboard.ContentMD != "缺图页修改后分镜" {
			t.Fatalf("saved page=%+v", page)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"user-data/vacs/files/candidate.png", "user-data/vacs/prompts/002.txt", "user-data/vacs/prompts/mapping.json"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestTextOnlyAndPromptUpdate(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Version: 1, Name: "文字", Language: "en", Chapters: []Chapter{{ID: "text", Code: "vol01.ch01", Title: "Text", Content: "Only text", Format: "txt"}}}
	for _, style := range []string{"first style", "updated style"} {
		m.Prompts = []Prompt{{Group: "premise_style", Key: "project_overall_style", Value: style}}
		raw, _ := json.Marshal(m)
		manifest := filepath.Join(dir, "manifest.json")
		if err := os.WriteFile(manifest, raw, 0644); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(t.TempDir(), "文字项目")
		if err := ConvertPackage(t.Context(), packManifest(t, manifest), output); err != nil {
			t.Fatal(err)
		}
		root := output
		appDir := t.TempDir()
		app, err := appstore.Open(appDir, config.SQLiteDSN(filepath.Join(appDir, "app.sqlite")))
		if err != nil {
			t.Fatal(err)
		}
		manager := project.NewManager(app)
		opened, err := manager.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
		if err != nil {
			t.Fatal(err)
		}
		err = manager.WithStore(t.Context(), opened.UUID, func(store *project.Store) error {
			got, e := story.NewService(store).EffectivePrompt(t.Context(), "premise_style", "project_overall_style")
			if e == nil && got != style {
				t.Fatalf("style=%s", got)
			}
			return e
		})
		manager.Close()
		app.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDistinctAssetsSharingFileKeepSelectedIdentity(t *testing.T) {
	manifest, _ := sample(t)
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Assets = append(m.Assets, Asset{ID: "second", Title: "第二个设定", Summary: "第二个身份", FileID: "image"})
	for i := range m.Chapters[0].Sections {
		m.Chapters[0].Sections[i].AssetIDs = []string{"second"}
	}
	raw, _ = json.Marshal(m)
	if err = os.WriteFile(manifest, raw, 0644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "aliases")
	if err = ConvertPackage(t.Context(), packManifest(t, manifest), root); err != nil {
		t.Fatal(err)
	}
	appDir := t.TempDir()
	app, err := appstore.Open(appDir, config.SQLiteDSN(filepath.Join(appDir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manager := project.NewManager(app)
	defer manager.Close()
	opened, err := manager.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	err = manager.WithStore(t.Context(), opened.UUID, func(store *project.Store) error {
		chapters, e := story.NewService(store).ListChapters(t.Context(), "active")
		if e != nil {
			return e
		}
		p := production.NewService(store, nil)
		pages, e := p.ListSections(t.Context(), chapters[0].UUID)
		if e != nil {
			return e
		}
		mappingRaw, e := os.ReadFile(filepath.Join(root, "user-data/vacs/content-mapping.json"))
		if e != nil {
			return e
		}
		var mapping map[string]string
		if e = json.Unmarshal(mappingRaw, &mapping); e != nil {
			return e
		}
		if mapping["asset/shared"] == mapping["asset/second"] {
			t.Fatal("distinct assets collapsed")
		}
		for _, page := range pages {
			if len(page.PremiseAssets) != 1 || page.PremiseAssets[0].AssetUUID != mapping["asset/second"] {
				t.Fatalf("wrong selected identity: %+v", page.PremiseAssets)
			}
			asset, e := p.GetPremiseAsset(t.Context(), page.PremiseAssets[0].AssetUUID)
			if e != nil {
				return e
			}
			if asset.Title != "第二个设定" {
				t.Fatalf("selected first alias: %s", asset.Title)
			}
		}
		if pages[0].CurrentImage.SectionPremise.SelectedAssets[0].AssetUUID != mapping["asset/second"] {
			t.Fatal("artwork snapshot selected wrong alias")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
