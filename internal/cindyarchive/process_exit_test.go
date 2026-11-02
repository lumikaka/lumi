package cindyarchive

import (
	"bufio"
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/project"

	"modernc.org/sqlite"
)

const crashChildEnvironment = "LUMI_CINDY_ARCHIVE_CRASH_CHILD"
const transactionPaused = "CINDY_ARCHIVE_TRANSACTION_PAUSED"

func TestImportProcessExitRollsBackAndAllowsReplay(t *testing.T) {
	if base := os.Getenv(crashChildEnvironment); base != "" {
		pauseArchiveTransaction(t, base)
		return
	}

	base := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestImportProcessExitRollsBackAndAllowsReplay$", "-test.timeout=15s")
	cmd.Env = append(os.Environ(), crashChildEnvironment+"="+base)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	waited := false
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // Only the subprocess created by this test.
		if !waited {
			<-wait
		}
	})
	paused := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == transactionPaused {
				close(paused)
				return
			}
		}
	}()
	select {
	case <-paused:
	case err := <-wait:
		waited = true
		t.Fatalf("child exited before reaching an uncommitted insert: %v %s", err, stderr.String())
	case <-ctx.Done():
		t.Fatal("child did not reach the transaction barrier")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = <-wait
	waited = true
	if err == nil {
		t.Fatal("terminated child unexpectedly reported success")
	}

	// A new manager must acquire the OS project lock and recover SQLite after
	// the writer died with a thread, session and two items still uncommitted.
	data := filepath.Join(base, "reopened-data")
	app, err := appstore.Open(data, config.SQLiteDSN(filepath.Join(data, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manager := project.NewManager(app)
	defer manager.Close()
	p, err := manager.OpenSelected(t.Context(), project.ExplicitExistingDirectory(filepath.Join(base, "project")))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.WithStore(t.Context(), p.UUID, func(store *project.Store) error {
		for _, table := range []string{"chat_threads", "cindy_sessions", "cindy_session_items", "cindy_session_imports"} {
			var count int64
			if err := store.DB().Table(table).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				t.Fatalf("process exit left %d partial rows in %s", count, table)
			}
		}
		return store.DB().Exec("DROP TRIGGER pause_archive_import").Error
	}); err != nil {
		t.Fatal(err)
	}
	s := &Service{Projects: manager}
	first := importFixture(t, s, p.UUID, fixture())
	again := importFixture(t, s, p.UUID, fixture())
	if first.Imported != 3 || first.Replayed || !again.Replayed || again.Imported != 0 || again.ImportUUID != first.ImportUUID {
		t.Fatalf("retry after process exit was not atomic/idempotent: first=%+v again=%+v", first, again)
	}
	history, err := s.History(t.Context(), p.UUID, first.ThreadUUID, "", "", 100)
	if err != nil || len(history.Items) != 3 {
		t.Fatalf("reopened history=%+v %v", history, err)
	}
	t.Log("writer terminated after two uncommitted items; project reopened, no partial rows, retry imported three items once")
}

func pauseArchiveTransaction(t *testing.T, base string) {
	t.Helper()
	// The SQL function and trigger exist only in this child test process and
	// its synthetic project. Production code has no pause/fault-injection hook.
	if err := sqlite.RegisterScalarFunction("pause_cindy_archive_import", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		fmt.Println(transactionPaused)
		time.Sleep(10 * time.Second)
		return nil, errors.New("parent did not terminate the paused test writer")
	}); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(base, "child-data")
	app, err := appstore.Open(data, config.SQLiteDSN(filepath.Join(data, "app.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	manager := project.NewManager(app)
	defer manager.Close()
	root := filepath.Join(base, "project")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := manager.InitAt(t.Context(), root, project.CreateInput{Name: "Synthetic interrupted archive", GenerationLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.WithStore(t.Context(), p.UUID, func(store *project.Store) error {
		return store.DB().Exec(`CREATE TRIGGER pause_archive_import BEFORE INSERT ON cindy_session_items
			WHEN (SELECT COUNT(*) FROM cindy_session_items)=2
			BEGIN SELECT pause_cindy_archive_import(); END`).Error
	}); err != nil {
		t.Fatal(err)
	}
	s := &Service{Projects: manager}
	_, _ = s.Import(t.Context(), p.UUID, selected, bytes.NewReader(rawFixture(t, fixture())))
	t.Fatal("archive writer reached the end instead of being terminated mid-transaction")
}
