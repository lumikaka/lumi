package vacsimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/story"
)

// These directories are created by running the standalone CLI on real VACS
// package ZIPs. They are inspected with the same services as the desktop app. See docs/vacs-import.md for the two-repo run.
func TestVACSExportNormalOpen(t *testing.T) {
	for _, env := range []string{"VACS_CONVERTED_SAMPLE", "VACS_CONVERTED_TEXT", "VACS_CONVERTED_UPDATED", "VACS_CONVERTED_SETTINGS", "VACS_CONVERTED_REVIEW"} {
		path := os.Getenv(env)
		if path == "" {
			continue
		}
		t.Run(env, func(t *testing.T) {
			root := cloneConvertedProject(t, path)
			raw, err := os.ReadFile(filepath.Join(root, "user-data/vacs/manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var original Manifest
			if err = json.Unmarshal(raw, &original); err != nil {
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
			adjusted, _ := adaptDisplayTitles(original)
			var contentMap map[string]string
			mappingBytes, err := os.ReadFile(filepath.Join(root, "user-data/vacs/content-mapping.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(mappingBytes, &contentMap); err != nil {
				t.Fatal(err)
			}
			var changedChapter, changedPage string
			err = manager.WithStore(t.Context(), opened.UUID, func(store *project.Store) error {
				s := story.NewService(store)
				p := production.NewService(store, nil)
				detail, e := s.GetProject(t.Context())
				if e != nil {
					return e
				}
				if detail.Name != adjusted.Name || detail.GenerationLanguage != original.Language {
					t.Fatalf("identity mismatch: %+v", detail)
				}
				chapters, e := s.ListChapters(t.Context(), "active")
				if e != nil {
					return e
				}
				if len(chapters) != len(original.Chapters) {
					t.Fatal("chapter count")
				}
				if original.StoryMD != "" {
					profile, e := s.GetStoryProfile(t.Context())
					if e != nil {
						return e
					}
					if profile.StoryMD != original.StoryMD {
						t.Fatal("story profile mismatch")
					}
				}
				shared := map[string]string{}
				fileMap := map[string]File{}
				for _, f := range original.Files {
					fileMap[f.ID] = f
					data, err := os.ReadFile(filepath.Join(root, "user-data/vacs/files", f.ID+filepath.Ext(f.Filename)))
					if err != nil {
						t.Fatal(err)
					}
					sum := sha256.Sum256(data)
					if hex.EncodeToString(sum[:]) != f.SHA256 {
						t.Fatalf("attachment differs from package checksum: %s", f.ID)
					}
				}
				verifyFile := func(uuid, id string) error {
					content, e := p.Files().OpenContent(t.Context(), uuid)
					if e != nil {
						return e
					}
					data, e := io.ReadAll(content.File)
					content.File.Close()
					if e != nil {
						return e
					}
					f := fileMap[id]
					expected, e := os.ReadFile(filepath.Join(root, "user-data/vacs/files", id+filepath.Ext(f.Filename)))
					if e != nil {
						return e
					}
					if !bytes.Equal(data, expected) {
						t.Fatalf("wrong content for %s", id)
					}
					return nil
				}
				for i, c := range chapters {
					want := original.Chapters[i]
					if c.ChapterCode != want.Code || c.Title != want.Title || c.CurrentStory.Content != want.Content {
						t.Fatalf("chapter mismatch: %+v", c)
					}
					pages, e := p.ListSections(t.Context(), c.UUID)
					if e != nil {
						return e
					}
					if len(pages) != len(want.Sections) {
						t.Fatal("page count")
					}
					for j, page := range pages {
						wantPage := want.Sections[j]
						displayPage := adjusted.Chapters[i].Sections[j]
						if page.Title != displayPage.Title || page.CurrentStoryboard.ContentMD != wantPage.Storyboard {
							t.Fatal("storyboard mismatch")
						}
						if wantPage.ImageFileID == "" {
							if page.CurrentImage != nil {
								t.Fatal("missing page has image")
							}
							if changedPage == "" {
								changedChapter, changedPage = c.UUID, page.UUID
								edit := "真实 VACS 缺图页编辑并保存"
								if _, e = p.UpdateSection(t.Context(), c.UUID, page.UUID, production.UpdateSectionInput{StoryboardMD: &edit, ExpectedRevision: page.Revision}); e != nil {
									return e
								}
							}
						} else {
							if wantPage.ReferenceFileID != "" {
								if page.CurrentImage.SectionPremise == nil {
									t.Fatal("current artwork reference missing")
								}
								if e = verifyFile(page.CurrentImage.SectionPremise.Asset.UUID, wantPage.ReferenceFileID); e != nil {
									return e
								}
							}
							if page.CurrentImage == nil {
								t.Fatal("current image missing")
							}
							var snapshot production.ImportedImageSnapshot
							if e = json.Unmarshal(page.CurrentImage.InputSnapshot, &snapshot); e != nil {
								return e
							}
							if snapshot.Prompt != wantPage.ImageProvenance.Prompt || snapshot.PromptSHA256 != wantPage.ImageProvenance.PromptSHA256 || snapshot.RevisedPrompt != wantPage.ImageProvenance.RevisedPrompt {
								t.Fatal("image provenance mismatch")
							}
							if e = verifyFile(page.CurrentImage.Asset.UUID, wantPage.ImageFileID); e != nil {
								return e
							}
						}
						if len(page.PremiseAssets) != len(wantPage.AssetIDs) {
							t.Fatal("reference count")
						}
						for k, ref := range page.PremiseAssets {
							id := wantPage.AssetIDs[k]
							if previous := shared[id]; previous != "" && previous != ref.AssetUUID {
								t.Fatal("shared setting duplicated")
							}
							shared[id] = ref.AssetUUID
							asset, e := p.GetPremiseAsset(t.Context(), ref.AssetUUID)
							if e != nil {
								return e
							}
							for _, wantAsset := range original.Assets {
								if wantAsset.ID == id {
									if asset.Title != wantAsset.Title || ref.AssetUUID != contentMap["asset/"+id] {
										t.Fatal("page selected wrong same-file asset identity")
									}
									if e = verifyFile(asset.CurrentVariant.Asset.UUID, wantAsset.FileID); e != nil {
										return e
									}
								}
							}
						}
					}
				}
				settings, e := p.ListSettingImages(t.Context())
				if e != nil {
					return e
				}
				if len(settings) != len(original.Settings) {
					t.Fatal("setting originals missing")
				}

				for _, want := range original.Settings {
					if e = verifyFile(contentMap["file/"+want.FileID], want.FileID); e != nil {
						return e
					}
				}
				formal, e := p.ListPremiseAssets(t.Context(), "", "active")
				if e != nil {
					return e
				}
				if len(formal) != len(original.Assets) {
					t.Fatal("formal assets missing")
				}
				for _, want := range original.Assets {
					asset, e := p.GetPremiseAsset(t.Context(), contentMap["asset/"+want.ID])
					if e != nil {
						return e
					}
					if asset.Title != want.Title || asset.Summary != want.Summary {
						t.Fatal("asset metadata mismatch")
					}
					if want.SettingID != "" {
						if asset.CurrentVariant.SourceSettingImageUUID != contentMap["setting/"+want.SettingID] {
							t.Fatal("split asset lost original setting association")
						}
						var gotCrop, wantCrop any
						if e = json.Unmarshal(asset.CurrentVariant.Crop, &gotCrop); e != nil {
							return e
						}
						if e = json.Unmarshal(want.Crop, &wantCrop); e != nil {
							return e
						}
						if !reflect.DeepEqual(gotCrop, wantCrop) {
							t.Fatalf("crop mismatch: %v / %v", gotCrop, wantCrop)
						}
					}
					if want.FileID != "" {
						if e = verifyFile(asset.CurrentVariant.Asset.UUID, want.FileID); e != nil {
							return e
						}
					}
				}
				for _, prompt := range original.Prompts {
					if prompt.Group == "premise_style" && prompt.Key == "project_overall_style" {
						got, e := s.EffectivePrompt(t.Context(), prompt.Group, prompt.Key)
						if e != nil {
							return e
						}
						if got != prompt.Value {
							t.Fatal("style mismatch")
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.CloseProject(t.Context(), opened.UUID); err != nil {
				t.Fatal(err)
			}
			reopened, err := manager.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
			if err != nil {
				t.Fatal(err)
			}
			if changedPage != "" {
				if err = manager.WithStore(t.Context(), reopened.UUID, func(store *project.Store) error {
					page, e := production.NewService(store, nil).GetSection(t.Context(), changedChapter, changedPage)
					if e == nil && page.CurrentStoryboard.ContentMD != "真实 VACS 缺图页编辑并保存" {
						t.Fatal("edit not durable")
					}
					return e
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if os.Getenv("VACS_CONVERTED_SAMPLE") == "" && os.Getenv("VACS_CONVERTED_TEXT") == "" && os.Getenv("VACS_CONVERTED_UPDATED") == "" && os.Getenv("VACS_CONVERTED_SETTINGS") == "" && os.Getenv("VACS_CONVERTED_REVIEW") == "" {
		t.Skip("set VACS_CONVERTED_SAMPLE / VACS_CONVERTED_TEXT to directories created by the local CLI")
	}
}

func cloneConvertedProject(t *testing.T, source string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "Lumi-project")
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		to := filepath.Join(target, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(to, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected non-regular project file: %s", path)
		}
		return copyFile(path, to)
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}
