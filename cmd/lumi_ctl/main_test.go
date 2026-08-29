package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"--help"}, &output, time.Now, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if output.String() != usage {
		t.Fatalf("help = %q", output.String())
	}
}

func TestCreateMigration(t *testing.T) {
	directory := t.TempDir()
	now := func() time.Time { return time.Date(2026, 8, 7, 9, 8, 7, 0, time.FixedZone("CST", 8*60*60)) }
	var output bytes.Buffer
	if err := run([]string{"migrate", "create", "project", "create_picture_books"}, &output, now, directory); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".up.sql", ".down.sql"} {
		path := filepath.Join(directory, "project", "20260807010807_create_picture_books"+suffix)
		content, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(content), "do not add BEGIN or COMMIT") {
			t.Fatalf("%s content = %q, error = %v", path, content, err)
		}
	}
	if err := run([]string{"migrate", "create", "project", "create_picture_books"}, &output, now, directory); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate create error = %v", err)
	}
}

func TestCreateRejectsInvalidName(t *testing.T) {
	err := run([]string{"migrate", "create", "app", "Create-Books"}, &bytes.Buffer{}, time.Now, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "snake_case") {
		t.Fatalf("error = %v", err)
	}
}

func TestProjectMigrationRejectsRelativeRoot(t *testing.T) {
	err := run([]string{"migrate", "project", "up", "relative/project"}, &bytes.Buffer{}, time.Now, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative project root error = %v", err)
	}
}

func TestMigrationCommandsUseEmbeddedAppMigrations(t *testing.T) {
	t.Setenv("DATABASE_DSN", "file:"+filepath.Join(t.TempDir(), "ctl.sqlite3"))
	for _, scenario := range []struct {
		args []string
		want string
	}{
		{args: []string{"migrate", "up"}, want: "migrations applied"},
		{args: []string{"migrate", "up"}, want: "no migrations to apply"},
		{args: []string{"migrate", "version"}, want: "version: 20260828000004, dirty: false"},
		{args: []string{"migrate", "down"}, want: "rolled back 1 migration"},
	} {
		var output bytes.Buffer
		if err := run(scenario.args, &output, time.Now, t.TempDir()); err != nil {
			t.Fatalf("run(%v) error = %v", scenario.args, err)
		}
		if !strings.Contains(output.String(), scenario.want) {
			t.Fatalf("run(%v) output = %q", scenario.args, output.String())
		}
	}
}

func TestDownRejectsInvalidSteps(t *testing.T) {
	err := run([]string{"migrate", "down", "0"}, &bytes.Buffer{}, time.Now, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "positive integer") {
		t.Fatalf("error = %v", err)
	}
}
