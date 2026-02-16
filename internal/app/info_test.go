package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunInfoByProjectSelector(t *testing.T) {
	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: filepath.Join(home, "catalogs", "software"), RepoPathDepth: 1},
	}
	machine.Repos = []domain.MachineRepoRecord{
		{
			RepoKey:   "software/codex",
			Name:      "codex",
			Catalog:   "software",
			Path:      filepath.Join(home, "catalogs", "software", "codex"),
			OriginURL: "git@github.com:openai/codex.git",
			Branch:    "main",
			Upstream:  "origin/main",
			Ahead:     2,
			Behind:    1,
		},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}
	if err := os.MkdirAll(machine.Repos[0].Path, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}

	var stdout bytes.Buffer
	app := New(paths, &stdout, &bytes.Buffer{})
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }
	if _, err := app.Git.RunGit(filepath.Dir(machine.Repos[0].Path), "init", "-b", "main", machine.Repos[0].Path); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	code, err := app.RunInfo(InfoOptions{Selector: "codex"})
	if err != nil {
		t.Fatalf("RunInfo error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Project: codex") {
		t.Fatalf("expected project line, got: %q", out)
	}
	if !strings.Contains(out, "Repo Key: software/codex") {
		t.Fatalf("expected repo key line, got: %q", out)
	}
}

func TestRunInfoResolvesRepoSelectorByOrigin(t *testing.T) {
	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	remoteRoot := filepath.Join(home, "remotes")
	remotePath := setupCloneTestRemote(t, remoteRoot, "openai", "codex")
	t.Setenv("BB_TEST_REMOTE_ROOT", remoteRoot)

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "references"
	machine.Catalogs = []domain.Catalog{
		{Name: "references", Root: filepath.Join(home, "catalogs", "references"), RepoPathDepth: 2},
	}
	machine.Repos = []domain.MachineRepoRecord{
		{
			RepoKey:   "references/custom/name",
			Name:      "custom-name",
			Catalog:   "references",
			Path:      filepath.Join(home, "catalogs", "references", "custom", "name"),
			OriginURL: "file://" + remotePath,
		},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}
	if err := os.MkdirAll(machine.Repos[0].Path, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}

	var stdout bytes.Buffer
	app := New(paths, &stdout, &bytes.Buffer{})
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }
	if _, err := app.Git.RunGit(filepath.Dir(machine.Repos[0].Path), "init", "-b", "main", machine.Repos[0].Path); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	code, err := app.RunInfo(InfoOptions{Selector: "openai/codex"})
	if err != nil {
		t.Fatalf("RunInfo error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Project: custom-name") {
		t.Fatalf("expected resolved project name, got: %q", out)
	}
	if !strings.Contains(out, "Repo Key: references/custom/name") {
		t.Fatalf("expected resolved repo key, got: %q", out)
	}
}

func TestRunInfoMissingReturnsOne(t *testing.T) {
	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: filepath.Join(home, "catalogs", "software"), RepoPathDepth: 1},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	var stdout bytes.Buffer
	app := New(paths, &stdout, &bytes.Buffer{})
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }

	code, err := app.RunInfo(InfoOptions{Selector: "openai/codex"})
	if err != nil {
		t.Fatalf("RunInfo error: %v", err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if out := stdout.String(); !strings.Contains(out, "not found locally") {
		t.Fatalf("expected not found message, got: %q", out)
	}
}

func TestRunInfoRecordMissingCloneReturnsOne(t *testing.T) {
	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: filepath.Join(home, "catalogs", "software"), RepoPathDepth: 1},
	}
	machine.Repos = []domain.MachineRepoRecord{
		{
			RepoKey: "software/codex",
			Name:    "codex",
			Catalog: "software",
			Path:    filepath.Join(home, "catalogs", "software", "codex"),
		},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	var stdout bytes.Buffer
	app := New(paths, &stdout, &bytes.Buffer{})
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }

	code, err := app.RunInfo(InfoOptions{Selector: "codex"})
	if err != nil {
		t.Fatalf("RunInfo error: %v", err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if out := stdout.String(); !strings.Contains(out, "is not cloned locally") {
		t.Fatalf("expected local-missing message, got: %q", out)
	}
}
