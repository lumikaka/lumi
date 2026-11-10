// Package websiteprompt renders the desktop website card's publishing prompt
// for plugin export receipts.
package websiteprompt

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed messages.json
var messages []byte

var templates map[string]struct{ Source, Body string }

func init() {
	if err := json.Unmarshal(messages, &templates); err != nil {
		panic(err)
	}
}

func Render(language, directory string) string {
	template, ok := templates[language]
	if !ok {
		template = templates["zh-Hans"]
	}
	source := strings.Replace(template.Source, "{path}", strconv.Quote(directory), 1)
	return strings.Replace(template.Body, "{source}", source, 1)
}
