package mcpserver

import (
	"path/filepath"
	"testing"
	"time"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"
)

func TestConfirmationListingUsesUTCPersistenceAcrossTimezones(t *testing.T) {
	dir := t.TempDir()
	app, err := appstore.Open(dir, config.SQLiteDSN(filepath.Join(dir, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	projects := project.NewManager(app)
	defer projects.Close()
	p, err := projects.Create(t.Context(), "timezone", project.ExplicitNewProjectParent(dir))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(app, projects, agent.NewExternalProjectAPI(nil, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 16, 0, 0, 0, time.FixedZone("Shanghai", 8*60*60))
	s.now = func() time.Time { return now }
	g, _, err := s.CreateGrant(t.Context(), p.UUID, "local", "edit")
	if err != nil {
		t.Fatal(err)
	}
	result := s.Call(t.Context(), g, map[string]any{"method": "DELETE", "url": "/api/v1/projects/" + p.UUID + "/chapters/" + newUUID(), "request_body": map[string]any{"expected_revision": float64(1)}, "response_filter": ".data | {uuid,title,revision}"}, "pending")
	if result["success"] != true {
		t.Fatalf("%+v", result)
	}
	items, err := s.Calls(t.Context(), p.UUID)
	if err != nil || len(items) != 1 || items[0]["call"].(Call).Status != "pending_confirmation" {
		t.Fatalf("premature expiration: %+v %v", items, err)
	}
	now = now.Add(16 * time.Minute)
	items, err = s.Calls(t.Context(), p.UUID)
	if err != nil || items[0]["call"].(Call).Status != "expired" {
		t.Fatalf("missing expiration: %+v %v", items, err)
	}
}
