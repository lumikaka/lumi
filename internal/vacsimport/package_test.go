package vacsimport

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func packagePayloads(t *testing.T, filename string) map[string][]byte {
	t.Helper()
	z, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	result := map[string][]byte{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		result[f.Name] = data
	}
	return result
}
func changeManifest(t *testing.T, payloads map[string][]byte, change func(map[string]any)) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(payloads["manifest.json"], &m); err != nil {
		t.Fatal(err)
	}
	change(m)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	payloads["manifest.json"] = data
}
func TestRejectInvalidPackagesWithoutPublishingProject(t *testing.T) {
	source, _ := sample(t)
	validZIP := packManifest(t, source)
	tests := []struct {
		name, want string
		change     func(*testing.T, map[string][]byte)
	}{
		{"unsupported-version", "unsupported VACS package version 99", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) { m["version"] = 99 })
		}},
		{"wrong-format", "unsupported package format", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) { m["format"] = "lumi-native" })
		}},
		{"legacy-native-ZIP", "root manifest.json is missing", func(t *testing.T, p map[string][]byte) {
			delete(p, "manifest.json")
			p["Lumi-project/project.sqlite"] = []byte("old project")
		}},
		{"bad-json", "invalid manifest.json", func(t *testing.T, p map[string][]byte) { p["manifest.json"] = []byte("{") }},
		{"missing-image", "file image", func(t *testing.T, p map[string][]byte) { delete(p, "files/image.png") }},
		{"corrupt-image", "sha256 mismatch for files/image.png", func(t *testing.T, p map[string][]byte) { p["files/image.png"][0] ^= 255 }},
		{"wrong-size", "size mismatch for files/image.png", func(t *testing.T, p map[string][]byte) { p["files/image.png"] = append(p["files/image.png"], 0) }},
		{"missing-hash", "missing sha256", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) { delete(m["files"].([]any)[0].(map[string]any), "sha256") })
		}},
		{"manifest-traversal", "invalid payload path", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) { m["files"].([]any)[0].(map[string]any)["path"] = "../escape.png" })
		}},
		{"reserved-prompt-path", ".txt payload path", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) { m["prompts"].([]any)[0].(map[string]any)["path"] = "prompts/mapping.json" })
		}},
		{"damaged-image-prompts", "sha256 mismatch for image-prompts.json", func(t *testing.T, p map[string][]byte) { p["image-prompts.json"] = []byte("{}") }},
		{"missing-readme", "missing ZIP file: README.md", func(t *testing.T, p map[string][]byte) { delete(p, "README.md") }},
		{"traversal", "unsafe ZIP path", func(t *testing.T, p map[string][]byte) { p["../escape.png"] = []byte("bad") }},
		{"windows-path", "unsafe ZIP path", func(t *testing.T, p map[string][]byte) { p["C:/escape.png"] = []byte("bad") }},
		{"backslash", "unsafe ZIP path", func(t *testing.T, p map[string][]byte) { p["files\\escape.png"] = []byte("bad") }},
		{"path-conflict", "path conflict", func(t *testing.T, p map[string][]byte) { p["files"] = []byte("conflict") }},
		{"unlisted-file", "unlisted ZIP file", func(t *testing.T, p map[string][]byte) { p["unexpected.txt"] = []byte("not listed") }},
		{"prompt-tamper", "value differs", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) { m["prompts"].([]any)[0].(map[string]any)["value"] = "forged prompt" })
		}},
		{"missing-reference", "missing asset", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) {
				m["chapters"].([]any)[0].(map[string]any)["sections"].([]any)[0].(map[string]any)["asset_ids"] = []any{"absent"}
			})
		}},
		{"semantic-conversion-failure", "项目生成语言无效", func(t *testing.T, p map[string][]byte) {
			changeManifest(t, p, func(m map[string]any) { m["language"] = "unsupported" })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloads := packagePayloads(t, validZIP)
			tt.change(t, payloads)
			out := filepath.Join(t.TempDir(), "project")
			err := ConvertPackage(t.Context(), writeZIP(t, payloads), out)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v; expected %q", err, tt.want)
			}
			if _, err = os.Lstat(out); !os.IsNotExist(err) {
				t.Fatal("failed conversion published an output")
			}
			stages, err := filepath.Glob(filepath.Join(filepath.Dir(out), ".vacs-to-lumi-*"))
			if err != nil || len(stages) > 0 {
				t.Fatalf("conversion stages not cleaned: %v %v", stages, err)
			}
		})
	}
}
func TestRejectDuplicateAndSymlinkZIPEntries(t *testing.T) {
	for _, kind := range []string{"duplicate", "case-duplicate", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "unsafe.zip")
			out, err := os.Create(filename)
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(out)
			first, err := z.Create("manifest.json")
			if err != nil {
				t.Fatal(err)
			}
			first.Write([]byte(`{}`))
			name := "manifest.json"
			if kind == "case-duplicate" {
				name = "Manifest.json"
			}
			if kind == "symlink" {
				h := &zip.FileHeader{Name: "files/link"}
				h.SetMode(os.ModeSymlink | 0777)
				w, err := z.CreateHeader(h)
				if err != nil {
					t.Fatal(err)
				}
				w.Write([]byte("/etc/passwd"))
			} else {
				w, err := z.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				w.Write([]byte(`{}`))
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			out.Close()
			err = ConvertPackage(t.Context(), filename, filepath.Join(t.TempDir(), "output"))
			if err == nil {
				t.Fatal("accepted unsafe ZIP")
			}
		})
	}
}
func TestExistingOutputAndExclusivePublicationAreProtected(t *testing.T) {
	manifest, _ := sample(t)
	input := packManifest(t, manifest)
	for _, kind := range []string{"empty-directory", "populated-directory", "file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			out := filepath.Join(parent, "existing")
			switch kind {
			case "empty-directory", "populated-directory":
				if err := os.Mkdir(out, 0755); err != nil {
					t.Fatal(err)
				}
				if kind == "populated-directory" {
					if err := os.WriteFile(filepath.Join(out, "keep.txt"), []byte("keep"), 0644); err != nil {
						t.Fatal(err)
					}
				}
			case "file":
				if err := os.WriteFile(out, []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(filepath.Join(parent, "missing"), out); err != nil {
					t.Skip(err)
				}
			}
			before, err := os.Lstat(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := ConvertPackage(t.Context(), input, out); err == nil || !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("overwrote output: %v", err)
			}
			after, err := os.Lstat(out)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("output identity changed")
			}
			if kind == "populated-directory" {
				data, err := os.ReadFile(filepath.Join(out, "keep.txt"))
				if err != nil || string(data) != "keep" {
					t.Fatal("existing file changed")
				}
			}
		})
	}
	source := filepath.Join(t.TempDir(), "source")
	target := filepath.Join(t.TempDir(), "race-target")
	os.Mkdir(source, 0755)
	os.Mkdir(target, 0755)
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishDirectory(source, target); err == nil {
		t.Fatal("publication replaced a concurrently created empty directory")
	}
	after, err := os.Stat(target)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("concurrent output changed")
	}
}
