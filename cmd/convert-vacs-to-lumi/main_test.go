package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/production"
	"lumi/internal/story"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"lumi/internal/project"
)

func TestDocumentedInputFirstCommandAndOutputProtection(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "我的绘本.vacs.zip")
	file, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(file)
	w, err := z.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(`{"format":"vacs-project","version":1,"name":"CLI 中文项目","language":"zh-Hans","chapters":[],"assets":[],"settings":[],"files":[],"prompts":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	output := filepath.Join(directory, "我的绘本")
	var result bytes.Buffer
	if err = run(context.Background(), []string{input, "--output", output}, &result); err != nil {
		t.Fatal(err)
	}
	info, err := project.InspectDirectory(t.Context(), output)
	if err != nil || info.Name != "CLI 中文项目" {
		t.Fatalf("output=%+v err=%v", info, err)
	}
	if !strings.Contains(result.String(), output) {
		t.Fatal("success did not name output")
	}
	if err = run(t.Context(), []string{"--output", output, input}, &result); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output not protected: %v", err)
	}
}
func TestCLIUsageAndVersion(t *testing.T) {
	for _, args := range [][]string{{}, {"input.zip"}, {"input.zip", "--output", "out", "extra.zip"}} {
		var out bytes.Buffer
		if err := run(t.Context(), args, &out); err == nil {
			t.Fatalf("accepted invalid args: %v", args)
		}
	}
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		var out bytes.Buffer
		if err := run(t.Context(), args, &out); err != nil || out.Len() == 0 {
			t.Fatalf("args %v output=%q err=%v", args, out.String(), err)
		}
	}
}

// Executes the actual CLI entry point in a separate process, without depending
// on a prebuilt binary or fixtures outside this repository.
func TestCLIEntryProcess(t *testing.T) {
	if os.Getenv("LUMI_TEST_CLI_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestChineseTitleBoundariesViaCLIAndNormalReadWrite(t *testing.T) {
	for _, lengths := range [][2]int{{120, 160}, {121, 160}, {120, 161}, {121, 161}, {255, 255}} {
		n, pageLength := lengths[0], lengths[1]
		t.Run(fmt.Sprintf("name_%d_page_%d", n, pageLength), func(t *testing.T) {
			// Astral CJK characters make byte-based slicing fail as well.
			name := strings.Repeat("星", n-1) + "𠮷"
			title := strings.Repeat("页", pageLength-1) + "𠮷"
			m := map[string]any{"format": "vacs-project", "version": 1, "project_id": "source-project", "name": name, "language": "zh-Hans", "chapters": []any{map[string]any{"id": "one", "code": "vol01.ch01", "title": "第一章", "content": "正文", "format": "txt", "sections": []any{map[string]any{"id": "page", "title": title, "storyboard": "缺图页"}}}}, "assets": []any{}, "settings": []any{}, "files": []any{}, "prompts": []any{}}
			dir := t.TempDir()
			input := filepath.Join(dir, "input.vacs.zip")
			f, err := os.Create(input)
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(f)
			w, err := z.Create("manifest.json")
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(m)
			if _, err = w.Write(raw); err != nil {
				t.Fatal(err)
			}
			if err = z.Close(); err != nil {
				t.Fatal(err)
			}
			f.Close()
			root := filepath.Join(dir, "output")
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command(exe, "-test.run=^TestCLIEntryProcess$", "--", input, "--output", root)
			command.Env = append(os.Environ(), "LUMI_TEST_CLI_PROCESS=1")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("CLI failed: %v\n%s", err, output)
			}

			want := 0
			for _, bound := range []struct {
				over     bool
				location string
			}{{n > 120, "project/source-project"}, {pageLength > 160, "chapter/one/page/page"}} {
				if bound.over {
					want++
					if !strings.Contains(string(output), "Adjusted display title at "+bound.location) {
						t.Fatalf("missing locatable adjustment %s: %s", bound.location, output)
					}
				} else if strings.Contains(string(output), "Adjusted display title at "+bound.location) {
					t.Fatalf("boundary unexpectedly adjusted: %s", output)
				}
			}
			if want > 0 && !strings.Contains(string(output), "conversion-notices.json") {
				t.Fatalf("CLI did not identify original-title report: %s", output)
			}

			archived, err := os.ReadFile(filepath.Join(root, "user-data/vacs/manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var original map[string]any
			if err = json.Unmarshal(archived, &original); err != nil {
				t.Fatal(err)
			}
			if original["name"] != name || !strings.Contains(string(archived), title) {
				t.Fatal("full original titles were lost")
			}
			notices, err := os.ReadFile(filepath.Join(root, "user-data/vacs/conversion-notices.json"))
			if err != nil {
				t.Fatal(err)
			}
			var adjustments []map[string]any
			if err = json.Unmarshal(notices, &adjustments); err != nil {
				t.Fatal(err)
			}

			if len(adjustments) != want {
				t.Fatalf("adjustments=%s", notices)
			}
			for _, adjustment := range adjustments {
				originalTitle := title
				if strings.HasPrefix(adjustment["location"].(string), "project/") {
					originalTitle = name
				}
				if adjustment["original"] != originalTitle {
					t.Fatalf("report lost original: %s", notices)
				}
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
			edited := "长标题页面正常保存"
			for pass := 0; pass < 2; pass++ {
				err = manager.WithStore(t.Context(), opened.UUID, func(store *project.Store) error {
					detail, e := story.NewService(store).GetProject(t.Context())
					if e != nil {
						return e
					}
					if !utf8.ValidString(detail.Name) || utf8.RuneCountInString(detail.Name) != 120 {
						t.Fatalf("name=%q", detail.Name)
					}
					if n == 120 && detail.Name != name {
						t.Fatal("exact-limit project title changed")
					}
					chapters, e := story.NewService(store).ListChapters(t.Context(), "active")
					if e != nil {
						return e
					}
					p := production.NewService(store, nil)
					pages, e := p.ListSections(t.Context(), chapters[0].UUID)
					if e != nil {
						return e
					}
					if !utf8.ValidString(pages[0].Title) || utf8.RuneCountInString(pages[0].Title) != 160 {
						t.Fatalf("page title=%q", pages[0].Title)
					}
					if pageLength == 160 && pages[0].Title != title {
						t.Fatal("exact-limit page title changed")
					}
					if pass == 0 {
						_, e = p.UpdateSection(t.Context(), chapters[0].UUID, pages[0].UUID, production.UpdateSectionInput{StoryboardMD: &edited, ExpectedRevision: pages[0].Revision})
						return e
					}
					if pages[0].CurrentStoryboard.ContentMD != edited {
						t.Fatal("edit not persisted after reopening")
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = manager.CloseProject(t.Context(), opened.UUID); err != nil {
					t.Fatal(err)
				}
				if pass == 0 {
					opened, err = manager.OpenSelected(t.Context(), project.ExplicitExistingDirectory(root))
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			after, err := os.ReadFile(filepath.Join(root, "user-data/vacs/manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, archived) {
				t.Fatal("original titles changed after save/reopen")
			}
		})
	}
}
