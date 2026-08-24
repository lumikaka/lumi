package prompts

import (
	"embed"
	"fmt"
	"strings"
)

// files contains the reviewed, version-controlled defaults used by the Agent
// prompt catalog. Runtime callers never read prompt content from disk.
//
//go:embed *.md
var files embed.FS

// MustRead returns one embedded Agent prompt. Missing or blank built-in
// prompts are packaging errors and intentionally fail fast.
func MustRead(key, language string) string {
	return mustRead(key + "." + language + ".md")
}

// SystemTemplate returns the trusted template used to assemble the primary
// Agent system message.
func SystemTemplate() string {
	return mustRead("system.md")
}

func mustRead(path string) string {
	content, err := files.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("read embedded agent prompt %q: %v", path, err))
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		panic(fmt.Sprintf("embedded agent prompt %q is blank", path))
	}
	return value
}
