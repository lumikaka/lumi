package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"lumi/internal/config"
	"lumi/internal/dbmigrate"
	"lumi/internal/project"
)

const usage = `Usage:
  lumi_ctl migrate create <app|project> <snake_case_name>
  lumi_ctl migrate app up
  lumi_ctl migrate app down [steps]
  lumi_ctl migrate app version
  lumi_ctl migrate project up <absolute_project_root>
  lumi_ctl migrate project version <absolute_project_root>

Legacy aliases for app migrations:
  lumi_ctl migrate up
  lumi_ctl migrate down [steps]
  lumi_ctl migrate version
`

var migrationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

func main() {
	if err := run(os.Args[1:], os.Stdout, time.Now, filepath.Join("db", "migrations")); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "lumi_ctl: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer, now func() time.Time, migrationsDirectory string) error {
	if len(args) == 1 && args[0] == "--help" {
		_, err := io.WriteString(output, usage)
		return err
	}
	if len(args) < 2 || args[0] != "migrate" {
		return errors.New(usage)
	}

	if args[1] == "app" {
		return runAppMigration(args[2:], output)
	}
	if args[1] == "project" {
		return runProjectMigration(args[2:], output)
	}
	switch args[1] {
	case "create":
		if len(args) != 4 || (args[2] != "app" && args[2] != "project") {
			return fmt.Errorf("migrate create requires app or project and exactly one name\n%s", usage)
		}
		upPath, downPath, err := createMigration(filepath.Join(migrationsDirectory, args[2]), args[3], now())
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "created %s\ncreated %s\n", upPath, downPath)
		return err
	case "up", "down", "version":
		return runAppMigration(args[1:], output)
	default:
		return fmt.Errorf("unknown migrate command %q\n%s", args[1], usage)
	}
}

func runAppMigration(args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "up":
		if len(args) != 1 {
			return fmt.Errorf("migrate app up does not accept arguments\n%s", usage)
		}
		return withAppRunner(func(runner *dbmigrate.Runner) error {
			if err := runner.Up(); err != nil {
				if dbmigrate.IsNoChange(err) {
					_, writeErr := io.WriteString(output, "no migrations to apply\n")
					return writeErr
				}
				return fmt.Errorf("migrate up: %w", err)
			}
			_, err := io.WriteString(output, "migrations applied\n")
			return err
		})
	case "down":
		if len(args) > 2 {
			return fmt.Errorf("migrate app down accepts at most one step count\n%s", usage)
		}
		steps := 1
		if len(args) == 2 {
			parsed, err := strconv.Atoi(args[1])
			if err != nil || parsed <= 0 {
				return errors.New("migrate down steps must be a positive integer")
			}
			steps = parsed
		}
		return withAppRunner(func(runner *dbmigrate.Runner) error {
			if err := runner.Down(steps); err != nil {
				if dbmigrate.IsNoChange(err) {
					_, writeErr := io.WriteString(output, "no migrations to roll back\n")
					return writeErr
				}
				return fmt.Errorf("migrate down: %w", err)
			}
			_, err := fmt.Fprintf(output, "rolled back %d migration(s)\n", steps)
			return err
		})
	case "version":
		if len(args) != 1 {
			return fmt.Errorf("migrate app version does not accept arguments\n%s", usage)
		}
		return withAppRunner(func(runner *dbmigrate.Runner) error { return printVersion(output, runner) })
	default:
		return fmt.Errorf("unknown app migration command %q\n%s", args[0], usage)
	}
}

func runProjectMigration(args []string, output io.Writer) error {
	if len(args) != 2 || (args[0] != "up" && args[0] != "version") {
		return fmt.Errorf("project migrations accept up or version and one absolute project root\n%s", usage)
	}
	root := strings.TrimSpace(args[1])
	if !filepath.IsAbs(root) {
		return errors.New("project root must be an absolute path")
	}
	if args[0] == "up" {
		if err := project.MigrateDirectory(context.Background(), root); err != nil {
			return fmt.Errorf("migrate project: %w", err)
		}
		_, err := io.WriteString(output, "project migrations applied\n")
		return err
	}
	runner, err := dbmigrate.OpenProject(config.SQLiteDSN(filepath.Join(root, "project.sqlite")))
	if err != nil {
		return err
	}
	defer runner.Close()
	return printVersion(output, runner)
}

func printVersion(output io.Writer, runner *dbmigrate.Runner) error {
	version, dirty, applied, err := runner.Version()
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if dirty {
		return fmt.Errorf("migration version %d is dirty; manual recovery is required", version)
	}
	if !applied {
		_, err = io.WriteString(output, "version: none, dirty: false\n")
		return err
	}
	_, err = fmt.Fprintf(output, "version: %d, dirty: false\n", version)
	return err
}

func withAppRunner(action func(*dbmigrate.Runner) error) (err error) {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	runner, err := dbmigrate.OpenApp(cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, runner.Close()) }()
	return action(runner)
}

func createMigration(directory, name string, timestamp time.Time) (string, string, error) {
	if !migrationNamePattern.MatchString(name) {
		return "", "", errors.New("migration name must be lower snake_case")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", "", fmt.Errorf("create migration directory: %w", err)
	}
	version := timestamp.UTC().Format("20060102150405")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", "", fmt.Errorf("read migration directory: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), version+"_") {
			return "", "", fmt.Errorf("migration version %s already exists", version)
		}
	}

	base := version + "_" + name
	upPath := filepath.Join(directory, base+".up.sql")
	downPath := filepath.Join(directory, base+".down.sql")
	up := []byte("-- Add forward migration SQL here.\n-- golang-migrate wraps SQLite migrations in a transaction; do not add BEGIN or COMMIT.\n")
	down := []byte("-- Add rollback migration SQL here.\n-- golang-migrate wraps SQLite migrations in a transaction; do not add BEGIN or COMMIT.\n")
	if err := writeExclusive(upPath, up); err != nil {
		return "", "", err
	}
	if err := writeExclusive(downPath, down); err != nil {
		_ = os.Remove(upPath)
		return "", "", err
	}
	return upPath, downPath, nil
}

func writeExclusive(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
