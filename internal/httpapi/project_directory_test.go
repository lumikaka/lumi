package httpapi

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRenameOptionAndPreviewUsePublicRESTContract(t *testing.T) {
	e, manager := projectAPIHarness(t)
	e.PATCH("/api/v1/projects/:project_uuid", NewStoryHandler(manager).UpdateProject)
	e.GET("/api/v1/projects/:project_uuid/directory-name-preview", NewProjectHandler(manager).DirectoryNamePreview)
	created := requestJSON(t, e, http.MethodPost, "/api/v1/projects", map[string]any{"name": "Original", "parent_path": t.TempDir()})
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	data := envelopeData(t, created)
	projectUUID, oldRoot := data["uuid"].(string), data["root_path"].(string)
	base := "/api/v1/projects/" + projectUUID
	name := "勇敢的小火车"
	preview := requestJSON(t, e, http.MethodGet, base+"/directory-name-preview?name="+url.QueryEscape(name), nil)
	if preview.Code != http.StatusOK {
		t.Fatal(preview.Body.String())
	}
	wantRoot := filepath.Join(filepath.Dir(oldRoot), name)
	if envelopeData(t, preview)["root_path"] != wantRoot || strings.Contains(preview.Body.String(), `"id"`) {
		t.Fatal(preview.Body.String())
	}
	for _, rename := range []bool{false, true} {
		detail := envelopeData(t, requestJSON(t, e, http.MethodGet, base, nil))
		updated := requestJSON(t, e, http.MethodPatch, base, map[string]any{
			"name": name, "description": "preserved", "expected_revision": detail["revision"], "rename_directory": rename,
		})
		if updated.Code != http.StatusOK || envelopeData(t, updated)["name"] != name {
			t.Fatal(updated.Body.String())
		}
		if err := manager.ApplyPendingDirectoryRenames(t.Context()); err != nil {
			t.Fatal(err)
		}
		items, err := manager.RecentProjects(t.Context())
		if err != nil || len(items) != 1 {
			t.Fatalf("items=%+v err=%v", items, err)
		}
		want := oldRoot
		if rename {
			want = wantRoot
		}
		if items[0].RootPath != want {
			t.Fatalf("root=%s want=%s", items[0].RootPath, want)
		}
	}
	// A stale revision must not schedule a new directory operation.
	stale := requestJSON(t, e, http.MethodPatch, base, map[string]any{"name": "Stale", "description": "", "expected_revision": 1, "rename_directory": true})
	if stale.Code != http.StatusConflict {
		t.Fatal(stale.Body.String())
	}
	if err := manager.ApplyPendingDirectoryRenames(t.Context()); err != nil {
		t.Fatal(err)
	}
	items, _ := manager.RecentProjects(t.Context())
	if items[0].RootPath != wantRoot || items[0].Name != name {
		t.Fatalf("stale request changed project: %+v", items[0])
	}
}
