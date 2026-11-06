package vacsimport

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const Format = "vacs-project"
const maxManifestBytes = 64 << 20
const maxEntryBytes = 2 << 30
const maxPackageBytes = 20 << 30
const maxEntries = 100000

// ConvertPackage validates a self-contained VACS ZIP, creates a closed native
// project with Lumi services, then publishes it without replacing any output.
// There is no HTTP client, VACS connection, provider or daemon in this path.
func ConvertPackage(ctx context.Context, inputZIP, outputDirectory string, onNotice ...func(ConversionNotice)) error {
	output, err := filepath.Abs(outputDirectory)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(output); err == nil {
		return fmt.Errorf("output already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".vacs-to-lumi-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	inputDir := filepath.Join(stage, "input")
	if err := os.Mkdir(inputDir, 0700); err != nil {
		return err
	}
	m, raw, err := readPackage(ctx, inputZIP, inputDir)
	if err != nil {
		return err
	}
	m, notices := adaptDisplayTitles(m)
	root := filepath.Join(stage, "project")
	if err := os.Mkdir(root, 0755); err != nil {
		return err
	}
	if err := createProject(ctx, root, filepath.Join(stage, "app"), inputDir, m, raw); err != nil {
		return fmt.Errorf("convert VACS project: %w", err)
	}
	noticeJSON, err := json.MarshalIndent(notices, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "user-data", "vacs", "conversion-notices.json"), noticeJSON, 0644); err != nil {
		return err
	}
	// Exclusive rename also protects a destination created during conversion.
	if err := publishDirectory(root, output); err != nil {
		return fmt.Errorf("publish output %s (existing directories are protected): %w", output, err)
	}
	for _, notice := range notices {
		for _, emit := range onNotice {
			if emit != nil {
				emit(notice)
			}
		}
	}
	return nil
}

func safeArchivePath(name string) bool {
	if !utf8.ValidString(name) || strings.ContainsAny(name, "\\:\x00") {
		return false
	}
	trimmed := strings.TrimSuffix(name, "/")
	return fs.ValidPath(trimmed) && trimmed != "." && path.Clean(trimmed) == trimmed
}

func readPackage(ctx context.Context, inputZIP, directory string) (Manifest, []byte, error) {
	var m Manifest
	archive, err := zip.OpenReader(inputZIP)
	if err != nil {
		return m, nil, fmt.Errorf("open VACS ZIP %s: %w", inputZIP, err)
	}
	defer archive.Close()
	if len(archive.File) > maxEntries {
		return m, nil, fmt.Errorf("VACS ZIP contains more than %d entries", maxEntries)
	}
	entries := map[string]*zip.File{}
	portableNames := map[string]bool{}
	var total uint64
	for _, entry := range archive.File {
		if !safeArchivePath(entry.Name) {
			return m, nil, fmt.Errorf("unsafe ZIP path: %q", entry.Name)
		}
		canonical := strings.TrimSuffix(entry.Name, "/")
		folded := strings.ToLower(canonical)
		if portableNames[folded] {
			return m, nil, fmt.Errorf("duplicate ZIP path: %s", entry.Name)
		}
		portableNames[folded] = true
		if !entry.Mode().IsRegular() && !entry.FileInfo().IsDir() {
			return m, nil, fmt.Errorf("ZIP entry must be a regular file or directory: %s", entry.Name)
		}
		if entry.UncompressedSize64 > maxEntryBytes || total > maxPackageBytes-entry.UncompressedSize64 {
			return m, nil, fmt.Errorf("ZIP size limit exceeded at %s", entry.Name)
		}
		total += entry.UncompressedSize64
		if !entry.FileInfo().IsDir() {
			entries[entry.Name] = entry
		}
	}
	for name := range entries {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := entries[parent]; exists {
				return m, nil, fmt.Errorf("ZIP file/directory path conflict: %s", name)
			}
		}
	}
	manifestEntry := entries["manifest.json"]
	if manifestEntry == nil {
		return m, nil, fmt.Errorf("not a VACS project package: root manifest.json is missing (native Lumi ZIPs are not conversion inputs)")
	}
	if manifestEntry.UncompressedSize64 > maxManifestBytes {
		return m, nil, fmt.Errorf("manifest.json exceeds %d bytes", maxManifestBytes)
	}
	reader, err := manifestEntry.Open()
	if err != nil {
		return m, nil, fmt.Errorf("read manifest.json: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	closeErr := reader.Close()
	if readErr != nil {
		return m, nil, fmt.Errorf("read manifest.json: %w", readErr)
	}
	if closeErr != nil {
		return m, nil, closeErr
	}
	if len(raw) > maxManifestBytes {
		return m, nil, fmt.Errorf("manifest.json is too large")
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, nil, fmt.Errorf("invalid manifest.json: %w", err)
	}
	if m.Format != Format {
		return m, nil, fmt.Errorf("unsupported package format %q; expected %s", m.Format, Format)
	}
	if m.Version != Version {
		return m, nil, fmt.Errorf("unsupported VACS package version %d; supported: %d", m.Version, Version)
	}
	if err := validateManifest(m); err != nil {
		return m, nil, err
	}
	declared := map[string]bool{"manifest.json": true}
	for _, f := range m.Files {
		if err := ctx.Err(); err != nil {
			return m, nil, err
		}
		if err := extractChecked(entries, f.Path, directory, f.Size, f.SHA256, f.SHA1); err != nil {
			return m, nil, fmt.Errorf("file %s at %v: %w", f.ID, f.Locations, err)
		}
		declared[f.Path] = true
	}
	for _, prompt := range m.Prompts {
		if err := extractChecked(entries, prompt.Path, directory, prompt.Size, prompt.SHA256, ""); err != nil {
			return m, nil, fmt.Errorf("prompt %s/%s: %w", prompt.Group, prompt.Key, err)
		}
		sum := sha256.Sum256([]byte(prompt.Value))
		if hex.EncodeToString(sum[:]) != prompt.SHA256 {
			return m, nil, fmt.Errorf("prompt %s/%s value differs from %s checksum", prompt.Group, prompt.Key, prompt.Path)
		}
		declared[prompt.Path] = true
	}
	for _, attachment := range m.Attachments {
		if err := extractChecked(entries, attachment.Path, directory, attachment.Size, attachment.SHA256, ""); err != nil {
			return m, nil, fmt.Errorf("attachment %s: %w", attachment.Path, err)
		}
		declared[attachment.Path] = true
	}
	for name := range entries {
		if !declared[name] {
			return m, nil, fmt.Errorf("unlisted ZIP file: %s", name)
		}
	}
	return m, raw, nil
}

func validateManifest(m Manifest) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("manifest.name is required")
	}
	files := map[string]bool{}
	attachments := map[string]bool{}
	paths := map[string]bool{}
	for _, f := range m.Files {
		if !safePart(f.ID) || files[f.ID] {
			return fmt.Errorf("invalid or duplicate file id: %q", f.ID)
		}
		files[f.ID] = true
		attachment := strings.ToLower(f.ID + filepath.Ext(f.Filename))
		if attachments[attachment] {
			return fmt.Errorf("duplicate attachment filename for file %s", f.ID)
		}
		attachments[attachment] = true
		if err := validatePayloadPath(f.Path, "files/", f.Size, f.SHA256, paths); err != nil {
			return fmt.Errorf("file %s: %w", f.ID, err)
		}
		if f.SHA1 != "" && !validDigest(f.SHA1, 20) {
			return fmt.Errorf("file %s: invalid sha1", f.ID)
		}
	}
	prompts := map[string]bool{}
	for _, p := range m.Prompts {
		if !strings.HasSuffix(p.Path, ".txt") {
			return fmt.Errorf("prompt %s/%s must use a .txt payload path", p.Group, p.Key)
		}
		key := p.Group + "/" + p.Key
		if prompts[key] {
			return fmt.Errorf("duplicate prompt: %s", key)
		}
		prompts[key] = true
		if err := validatePayloadPath(p.Path, "prompts/", p.Size, p.SHA256, paths); err != nil {
			return fmt.Errorf("prompt %s: %w", key, err)
		}
	}
	for _, a := range m.Attachments {
		if a.Path != "README.md" && a.Path != "image-prompts.json" {
			return fmt.Errorf("unsupported attachment path %q", a.Path)
		}
		if paths[strings.ToLower(a.Path)] {
			return fmt.Errorf("duplicate attachment path %q", a.Path)
		}
		paths[strings.ToLower(a.Path)] = true
		if a.Size < 0 || a.Size > maxEntryBytes || !validDigest(a.SHA256, 32) {
			return fmt.Errorf("invalid attachment size or sha256: %s", a.Path)
		}
	}
	checkFile := func(id, location string) error {
		if id != "" && !files[id] {
			return fmt.Errorf("%s references missing file id %s", location, id)
		}
		return nil
	}
	settings := map[string]bool{}
	for _, setting := range m.Settings {
		if setting.ID == "" || settings[setting.ID] {
			return fmt.Errorf("invalid/duplicate setting id %q", setting.ID)
		}
		settings[setting.ID] = true
		if setting.FileID == "" {
			return fmt.Errorf("setting %s has no file", setting.ID)
		}
		if err := checkFile(setting.FileID, "setting "+setting.ID); err != nil {
			return err
		}
	}
	assets := map[string]bool{}
	for _, asset := range m.Assets {
		if asset.ID == "" || assets[asset.ID] {
			return fmt.Errorf("invalid/duplicate asset id %q", asset.ID)
		}
		assets[asset.ID] = true
		if err := checkFile(asset.FileID, "asset "+asset.ID); err != nil {
			return err
		}
		if asset.SettingID != "" && !settings[asset.SettingID] {
			return fmt.Errorf("asset %s references missing setting %s", asset.ID, asset.SettingID)
		}
	}
	chapters := map[string]bool{}
	pages := map[string]bool{}
	for _, chapter := range m.Chapters {
		if chapter.ID == "" || chapters[chapter.ID] {
			return fmt.Errorf("invalid/duplicate chapter id %q", chapter.ID)
		}
		chapters[chapter.ID] = true
		for _, page := range chapter.Sections {
			location := fmt.Sprintf("chapter %s page %s", chapter.Code, page.ID)
			if page.ID == "" || pages[page.ID] {
				return fmt.Errorf("invalid/duplicate page id %q", page.ID)
			}
			pages[page.ID] = true
			if err := checkFile(page.ImageFileID, location); err != nil {
				return err
			}
			if err := checkFile(page.ReferenceFileID, location+" reference"); err != nil {
				return err
			}
			for _, id := range page.AssetIDs {
				if !assets[id] {
					return fmt.Errorf("%s references missing asset %s", location, id)
				}
			}
		}
	}
	return nil
}
func validatePayloadPath(name, prefix string, size int64, digest string, seen map[string]bool) error {
	if !safeArchivePath(name) || strings.HasSuffix(name, "/") || !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("invalid payload path %q", name)
	}
	folded := strings.ToLower(name)
	if seen[folded] {
		return fmt.Errorf("duplicate payload path %q", name)
	}
	seen[folded] = true
	if size < 0 || size > maxEntryBytes {
		return fmt.Errorf("invalid byte size for %s", name)
	}
	if !validDigest(digest, 32) {
		return fmt.Errorf("invalid or missing sha256 for %s", name)
	}
	return nil
}
func validDigest(value string, size int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == size && value == strings.ToLower(value)
}
func extractChecked(entries map[string]*zip.File, name, directory string, size int64, expectedSHA256, expectedSHA1 string) error {
	entry := entries[name]
	if entry == nil {
		return fmt.Errorf("missing ZIP file: %s", name)
	}
	if entry.UncompressedSize64 != uint64(size) {
		return fmt.Errorf("size mismatch for %s: manifest %d, ZIP %d", name, size, entry.UncompressedSize64)
	}
	input, err := entry.Open()
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	defer input.Close()
	target := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	hash256, hash1 := sha256.New(), sha1.New()
	n, copyErr := io.Copy(io.MultiWriter(output, hash256, hash1), io.LimitReader(input, size+1))
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("read %s: %w", name, copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if n != size {
		return fmt.Errorf("size mismatch for %s: read %d, expected %d", name, n, size)
	}
	if hex.EncodeToString(hash256.Sum(nil)) != expectedSHA256 {
		return fmt.Errorf("sha256 mismatch for %s", name)
	}
	if expectedSHA1 != "" && hex.EncodeToString(hash1.Sum(nil)) != expectedSHA1 {
		return fmt.Errorf("sha1 mismatch for %s", name)
	}
	return nil
}
