package production

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// PageReferenceFiles owns the host CLI's temporary PNGs. Only the viewing
// agent can know which of these files it has actually seen.
type PageReferenceFiles struct {
	mu    sync.Mutex
	files map[pageReferenceKey]pageReferenceFile
}

type pageReferenceKey struct {
	projectUUID string
	digest      [sha256.Size]byte
}

type pageReferenceFile struct {
	path string
	info os.FileInfo
}

func (file pageReferenceFile) ownsPath() bool {
	info, err := os.Lstat(file.path)
	return err == nil && info.Mode().IsRegular() && os.SameFile(info, file.info)
}

func (file pageReferenceFile) matches(digest [sha256.Size]byte) bool {
	if !file.ownsPath() {
		return false
	}
	f, err := os.Open(file.path)
	if err != nil {
		return false
	}
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, file.info.Size()+1))
	return err == nil && n == file.info.Size() && bytes.Equal(hash.Sum(nil), digest[:])
}

func (files *PageReferenceFiles) save(projectUUID string, png []byte) (string, string, error) {
	key := pageReferenceKey{projectUUID: projectUUID, digest: sha256.Sum256(png)}
	digest := hex.EncodeToString(key.digest[:])
	files.mu.Lock()
	defer files.mu.Unlock()
	if cached, ok := files.files[key]; ok {
		if cached.matches(key.digest) {
			return cached.path, digest, nil
		}
		if cached.ownsPath() {
			_ = os.Remove(cached.path)
		}
		delete(files.files, key)
	}
	dir, err := filepath.Abs(os.TempDir())
	if err != nil {
		return "", "", err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", "", err
	}
	f, err := os.CreateTemp(dir, "lumi-page-premise-*.png")
	if err != nil {
		return "", "", err
	}
	_, err = f.Write(png)
	info, statErr := f.Stat()
	closeErr := f.Close()
	if err = errors.Join(err, statErr, closeErr); err != nil {
		_ = os.Remove(f.Name())
		return "", "", err
	}
	if files.files == nil {
		files.files = make(map[pageReferenceKey]pageReferenceFile)
	}
	files.files[key] = pageReferenceFile{path: f.Name(), info: info}
	return f.Name(), digest, nil
}

func (files *PageReferenceFiles) Close() {
	files.mu.Lock()
	defer files.mu.Unlock()
	for _, file := range files.files {
		if file.ownsPath() {
			_ = os.Remove(file.path)
		}
	}
	files.files = nil
}
