package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTextDraftInitializationRollbackAndDesktopRoundTrip(t *testing.T) {
	t.Run("rollback", func(t *testing.T) {
		manager, _ := testManager(t)
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("original instructions"), 0o600); err != nil {
			t.Fatal(err)
		}
		before := directorySnapshot(t, root)
		manager.WithOpenHook(func(ctx context.Context, store *Store) error {
			state, err := store.ProjectSetup(ctx)
			if err != nil || state.SetupStatus != SetupStatusDraft || state.OriginalInput != "original idea" {
				t.Fatalf("not a standard draft: %+v %v", state, err)
			}
			return errors.New("injected initialization hook failure")
		})
		_, err := manager.InitTextAt(t.Context(), root, CreateInput{}, "original idea")
		if err == nil || !reflect.DeepEqual(before, directorySnapshot(t, root)) {
			t.Fatalf("rollback changed original files: %v", err)
		}
		if len(manager.OpenProjectUUIDs()) != 0 {
			t.Fatal("failed project retained a manager binding")
		}
	})
	t.Run("desktop round trip", func(t *testing.T) {
		cli, app := testManager(t)
		p, err := cli.InitTextAt(t.Context(), t.TempDir(), CreateInput{Name: "Cindy draft"}, "original idea")
		if err != nil {
			t.Fatal(err)
		}
		desktop := NewManager(app)
		defer desktop.Close()
		if _, err := desktop.OpenSelected(t.Context(), ExplicitExistingDirectory(p.RootPath)); errorCode(err) != CodeLocked {
			t.Fatalf("desktop must honor CLI lock: %v", err)
		}
		if _, err := cli.CloseProject(t.Context(), p.UUID); err != nil {
			t.Fatal(err)
		}
		reopened, err := desktop.OpenSelected(t.Context(), ExplicitExistingDirectory(p.RootPath))
		if err != nil || reopened.UUID != p.UUID || reopened.SetupStatus != SetupStatusDraft {
			t.Fatalf("desktop cannot reopen Cindy draft: %+v %v", reopened, err)
		}
		if err := desktop.WithStore(t.Context(), p.UUID, func(store *Store) error {
			state, err := store.ProjectSetup(t.Context())
			if err != nil || state.OriginalInput != "original idea" || store.project.FormatVersion != SupportedFormatVersion {
				t.Fatalf("draft changed format or input: %+v %v", state, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		// Desktop-created ready projects use the exact same business initializer
		// on the Cindy path; it must preserve their existing profile and name.
		other, err := desktop.Create(t.Context(), "Desktop book", ExplicitNewProjectParent(t.TempDir()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := desktop.CloseProject(t.Context(), other.UUID); err != nil {
			t.Fatal(err)
		}
		adopted, err := cli.InitTextAt(t.Context(), other.RootPath, CreateInput{Name: "ignored"}, "ignored")
		if err != nil || adopted.Created || adopted.UUID != other.UUID || adopted.Name != other.Name {
			t.Fatalf("desktop adoption changed identity: %+v %v", adopted, err)
		}
		opened, err := cli.OpenSelected(t.Context(), ExplicitExistingDirectory(other.RootPath))
		if err != nil || opened.SetupStatus != SetupStatusReady || !reflect.DeepEqual(opened.PictureBook, other.PictureBook) {
			t.Fatalf("desktop adoption reset specification: %+v %v", opened, err)
		}
		for _, root := range []string{p.RootPath, other.RootPath} {
			for _, name := range []string{"AGENTS.md", ".codex", ".claude", "hooks"} {
				if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("business initialization created host guidance: %s %v", name, err)
				}
			}
		}
	})
}
