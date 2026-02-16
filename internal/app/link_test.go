package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunLinkAnchorsAtRepoRoot(t *testing.T) {
	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	catalogRoot := filepath.Join(home, "catalogs", "software")
	projectPath := filepath.Join(catalogRoot, "project")
	referencePath := filepath.Join(catalogRoot, "reference")

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: catalogRoot, RepoPathDepth: 1},
	}
	machine.Repos = []domain.MachineRepoRecord{
		{
			RepoKey: "software/reference",
			Name:    "reference",
			Catalog: "software",
			Path:    referencePath,
		},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	app := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }
	if err := os.MkdirAll(catalogRoot, 0o755); err != nil {
		t.Fatalf("mkdir catalog root: %v", err)
	}

	if _, err := app.Git.RunGit(catalogRoot, "init", "-b", "main", projectPath); err != nil {
		t.Fatalf("init project repo: %v", err)
	}
	if err := os.MkdirAll(referencePath, 0o755); err != nil {
		t.Fatalf("mkdir reference path: %v", err)
	}
	if _, err := app.Git.RunGit(catalogRoot, "init", "-b", "main", referencePath); err != nil {
		t.Fatalf("init reference repo: %v", err)
	}

	app.Getwd = func() (string, error) {
		return filepath.Join(projectPath, "subdir"), nil
	}
	if err := os.MkdirAll(filepath.Join(projectPath, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	code, err := app.RunLink(LinkOptions{Selector: "reference"})
	if err != nil {
		t.Fatalf("RunLink error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	linkPath := filepath.Join(projectPath, "references", "reference")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("expected link created: %v", err)
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if filepath.IsAbs(target) {
		t.Fatalf("expected relative symlink target, got %q", target)
	}
}

func TestRunLinkAutoCloneRemote(t *testing.T) {
	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	remoteRoot := filepath.Join(home, "remotes")
	setupCloneTestRemote(t, remoteRoot, "openai", "codex")
	t.Setenv("BB_TEST_REMOTE_ROOT", remoteRoot)

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	cfg.Clone.DefaultCatalog = "references"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	catalogRoot := filepath.Join(home, "catalogs", "software")
	projectPath := filepath.Join(catalogRoot, "project")

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: catalogRoot, RepoPathDepth: 1},
		{Name: "references", Root: filepath.Join(home, "catalogs", "references"), RepoPathDepth: 2},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	app := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }
	if err := os.MkdirAll(catalogRoot, 0o755); err != nil {
		t.Fatalf("mkdir catalog root: %v", err)
	}
	if _, err := app.Git.RunGit(catalogRoot, "init", "-b", "main", projectPath); err != nil {
		t.Fatalf("init project repo: %v", err)
	}
	app.Getwd = func() (string, error) { return projectPath, nil }

	code, err := app.RunLink(LinkOptions{Selector: "openai/codex"})
	if err != nil {
		t.Fatalf("RunLink error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	clonedPath := filepath.Join(home, "catalogs", "references", "openai", "codex")
	if _, err := os.Stat(filepath.Join(clonedPath, ".git")); err != nil {
		t.Fatalf("expected cloned repo: %v", err)
	}

	linkPath := filepath.Join(projectPath, "references", "codex")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("expected link created: %v", err)
	}
}
