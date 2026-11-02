// archive-verify is a read-only development verifier, not a shipped import tool.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	"lumi/internal/cindyarchive"
)

func main() {
	file := flag.String("file", "", "Explicit public-history snapshot file")
	selected := flag.String("source-session", "", "Explicitly selected opaque Cindy task identifier")
	flag.Parse()
	fail := func(code string) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"success": false, "data": nil, "error": map[string]string{"code": code, "message": "归档样本验证未通过；请核对选择的任务和导出格式。", "details": ""}})
		os.Exit(1)
	}
	info, err := os.Lstat(*file)
	if err != nil || !info.Mode().IsRegular() {
		fail("cindy_archive_file_unavailable")
	}
	f, err := os.Open(*file)
	if err != nil {
		fail("cindy_archive_file_unavailable")
	}
	defer f.Close()
	a, err := cindyarchive.Parse(context.Background(), f, *selected)
	if err != nil {
		fail(err.Error())
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"success": true, "data": map[string]any{"kind": "explicit-public-history-text-validation-not-import", "messages": len(a.Messages), "omitted_messages": a.OmittedMessages, "duplicate_messages": a.DuplicateMessages, "warnings": a.Warnings, "database_written": false, "host_history_read": false, "body_printed": false}})
}
