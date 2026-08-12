package platformpath

import (
	"path/filepath"
	"strings"
)

const (
	windowsProductionDataDirectory  = "dev.lumi.Lumi"
	windowsDevelopmentDataDirectory = "dev.lumi.Lumi-dev"
)

var windowsCJKFontNames = []string{
	"msyh.ttc",
	"msyhbd.ttc",
	"simhei.ttf",
	"simsun.ttc",
	"msjh.ttc",
}

func windowsAppDataDir(base, environment string) string {
	name := windowsDevelopmentDataDirectory
	if strings.EqualFold(strings.TrimSpace(environment), "production") {
		name = windowsProductionDataDirectory
	}
	return filepath.Join(base, name)
}

func defaultProjectParentDir(documents string) string {
	return filepath.Join(documents, "Lumi")
}

func fontPaths(directories []string, names []string) []string {
	paths := make([]string, 0, len(directories)*len(names))
	for _, directory := range directories {
		if strings.TrimSpace(directory) == "" {
			continue
		}
		for _, name := range names {
			paths = append(paths, filepath.Join(directory, name))
		}
	}
	return paths
}
