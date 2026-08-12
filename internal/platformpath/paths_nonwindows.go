//go:build !windows

package platformpath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func DefaultAppDataDir(environment string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	name := ".lumi_dev"
	if strings.EqualFold(strings.TrimSpace(environment), "production") {
		name = ".lumi"
	}
	return filepath.Join(home, name), nil
}

func DefaultProjectParentDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return defaultProjectParentDir(filepath.Join(home, "Documents")), nil
}

func CJKFontPaths() []string {
	return []string{
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/Library/Fonts/Arial Unicode.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/source-han-sans/SourceHanSansCN-Regular.otf",
		"/usr/share/fonts/truetype/arphic/ukai.ttc",
	}
}
