package app

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunRepoMoveMovesRepoAndRewritesMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 10, 0, 0, 0, time.UTC)
	app, paths, oldPath, newPath, oldRepoKey := setupRepoMoveFixture(t, now)

	code, err := app.RunRepoMove(RepoMoveOptions{
		Selector:      oldRepoKey,
		TargetCatalog: "references",
	})
	if err != nil {
		t.Fatalf("RunRepoMove error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newPath, ".git")); err != nil {
		t.Fatalf("new repo path missing git dir: %v", err)
	}

	newRepoKey := "references/api"
	meta, err := state.LoadRepoMetadata(paths, newRepoKey)
	if err != nil {
		t.Fatalf("load moved metadata: %v", err)
	}
	if meta.RepoKey != newRepoKey {
		t.Fatalf("meta.repo_key = %q, want %q", meta.RepoKey, newRepoKey)
	}
	if meta.PreferredCatalog != "references" {
		t.Fatalf("meta.preferred_catalog = %q, want references", meta.PreferredCatalog)
	}
	if !slices.Contains(meta.PreviousRepoKeys, oldRepoKey) {
		t.Fatalf("meta.previous_repo_keys = %v, want contains %q", meta.PreviousRepoKeys, oldRepoKey)
	}

	if _, err := os.Stat(state.RepoMetaPath(paths, oldRepoKey)); !os.IsNotExist(err) {
		t.Fatalf("old metadata still exists or stat failed: %v", err)
	}

	machine, err := state.LoadMachine(paths, "repo-move-host")
	if err != nil {
		t.Fatalf("load machine: %v", err)
	}
	if len(machine.Repos) != 1 {
		t.Fatalf("machine repo count = %d, want 1", len(machine.Repos))
	}
	rec := machine.Repos[0]
	if rec.RepoKey != newRepoKey {
		t.Fatalf("record.repo_key = %q, want %q", rec.RepoKey, newRepoKey)
	}
	if rec.Catalog != "references" {
		t.Fatalf("record.catalog = %q, want references", rec.Catalog)
	}
	if filepath.Clean(rec.Path) != filepath.Clean(newPath) {
		t.Fatalf("record.path = %q, want %q", rec.Path, newPath)
	}
}

func TestRunRepoMoveAppendsPreviousRepoKeys(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 10, 5, 0, 0, time.UTC)
	app, paths, _, _, oldRepoKey := setupRepoMoveFixture(t, now)

	meta, err := state.LoadRepoMetadata(paths, oldRepoKey)
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	meta.PreviousRepoKeys = []string{"legacy/api", oldRepoKey, "legacy/api"}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("seed metadata previous keys: %v", err)
	}

	code, err := app.RunRepoMove(RepoMoveOptions{Selector: oldRepoKey, TargetCatalog: "references"})
	if err != nil {
		t.Fatalf("RunRepoMove error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	moved, err := state.LoadRepoMetadata(paths, "references/api")
	if err != nil {
		t.Fatalf("load moved metadata: %v", err)
	}
	if got, want := moved.PreviousRepoKeys, []string{"legacy/api", "software/api"}; !slices.Equal(got, want) {
		t.Fatalf("previous_repo_keys = %v, want %v", got, want)
	}
}

func TestRunRepoMoveRejectsTargetPathConflict(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 10, 10, 0, 0, time.UTC)
	app, _, oldPath, newPath, oldRepoKey := setupRepoMoveFixture(t, now)

	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("mkdir conflict path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newPath, "README.txt"), []byte("conflict\n"), 0o644); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}

	code, err := app.RunRepoMove(RepoMoveOptions{Selector: oldRepoKey, TargetCatalog: "references"})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldPath, ".git")); err != nil {
		t.Fatalf("old path should remain intact: %v", err)
	}
}

func TestRunRepoMoveDryRunDoesNotMutate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 10, 15, 0, 0, time.UTC)
	app, paths, oldPath, newPath, oldRepoKey := setupRepoMoveFixture(t, now)

	code, err := app.RunRepoMove(RepoMoveOptions{
		Selector:      oldRepoKey,
		TargetCatalog: "references",
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("RunRepoMove error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if _, err := os.Stat(filepath.Join(oldPath, ".git")); err != nil {
		t.Fatalf("old path should remain: %v", err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("new path should not exist in dry-run: %v", err)
	}
	if _, err := state.LoadRepoMetadata(paths, oldRepoKey); err != nil {
		t.Fatalf("old metadata missing in dry-run: %v", err)
	}
	if _, err := state.LoadRepoMetadata(paths, "references/api"); !os.IsNotExist(err) {
		t.Fatalf("new metadata should not exist in dry-run: %v", err)
	}
}

func TestRunRepoMoveRunsPostMoveHooks(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 10, 20, 0, 0, time.UTC)
	app, paths, oldPath, newPath, oldRepoKey := setupRepoMoveFixture(t, now)

	hookOutput := filepath.Join(paths.Home, "hook-output.txt")
	cfg, err := state.LoadConfig(paths)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Move.PostHooks = []string{
		"echo \"$BB_MOVE_OLD_REPO_KEY|$BB_MOVE_NEW_REPO_KEY|$BB_MOVE_OLD_CATALOG|$BB_MOVE_NEW_CATALOG|$BB_MOVE_OLD_PATH|$BB_MOVE_NEW_PATH\" > " + hookOutput,
	}
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	code, err := app.RunRepoMove(RepoMoveOptions{Selector: oldRepoKey, TargetCatalog: "references"})
	if err != nil {
		t.Fatalf("RunRepoMove error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	raw, err := os.ReadFile(hookOutput)
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "|")
	if len(parts) != 6 {
		t.Fatalf("hook output parts = %d, want 6 (%q)", len(parts), string(raw))
	}
	if parts[0] != oldRepoKey || parts[1] != "references/api" {
		t.Fatalf("hook repo keys mismatch: %v", parts)
	}
	if parts[2] != "software" || parts[3] != "references" {
		t.Fatalf("hook catalogs mismatch: %v", parts)
	}
	if filepath.Clean(parts[4]) != filepath.Clean(oldPath) || filepath.Clean(parts[5]) != filepath.Clean(newPath) {
		t.Fatalf("hook paths mismatch: %v", parts)
	}
}

func TestRunRepoMoveNoHooksSkipsPostMoveHooks(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 10, 25, 0, 0, time.UTC)
	app, paths, _, _, oldRepoKey := setupRepoMoveFixture(t, now)

	hookOutput := filepath.Join(paths.Home, "hook-output.txt")
	cfg, err := state.LoadConfig(paths)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Move.PostHooks = []string{"echo called > " + hookOutput}
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	code, err := app.RunRepoMove(RepoMoveOptions{
		Selector:      oldRepoKey,
		TargetCatalog: "references",
		NoHooks:       true,
	})
	if err != nil {
		t.Fatalf("RunRepoMove error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if _, err := os.Stat(hookOutput); !os.IsNotExist(err) {
		t.Fatalf("hook output should not exist: %v", err)
	}
}

func TestRunRepoMoveFailsWhenTargetCatalogUnknown(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 10, 30, 0, 0, time.UTC)
	app, _, _, _, oldRepoKey := setupRepoMoveFixture(t, now)

	code, err := app.RunRepoMove(RepoMoveOptions{Selector: oldRepoKey, TargetCatalog: "unknown"})
	if err == nil {
		t.Fatal("expected unknown catalog error")
	}
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func setupRepoMoveFixture(t *testing.T, now time.Time) (*App, state.Paths, string, string, string) {
	t.Helper()

	home := t.TempDir()
	paths := state.NewPaths(home)

	softwareRoot := filepath.Join(home, "catalogs", "software")
	referencesRoot := filepath.Join(home, "catalogs", "references")
	if err := os.MkdirAll(softwareRoot, 0o755); err != nil {
		t.Fatalf("mkdir software root: %v", err)
	}
	if err := os.MkdirAll(referencesRoot, 0o755); err != nil {
		t.Fatalf("mkdir references root: %v", err)
	}

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("repo-move-host", "repo-move-host", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: softwareRoot, RepoPathDepth: 1},
		{Name: "references", Root: referencesRoot, RepoPathDepth: 1},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	app := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "repo-move-host", nil }

	oldPath := filepath.Join(softwareRoot, "api")
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	if err := app.Git.InitRepo(oldPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	remotePath := filepath.Join(home, "remotes", "you", "api.git")
	if err := os.MkdirAll(filepath.Dir(remotePath), 0o755); err != nil {
		t.Fatalf("mkdir remote parent: %v", err)
	}
	if _, err := app.Git.RunGit(home, "init", "--bare", remotePath); err != nil {
		t.Fatalf("init bare remote: %v", err)
	}
	if err := app.Git.AddOrigin(oldPath, remotePath); err != nil {
		t.Fatalf("add origin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := app.Git.AddAll(oldPath); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := app.Git.Commit(oldPath, "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	oldRepoKey := "software/api"
	rec, err := app.observeRepo(cfg, discoveredRepo{
		Catalog: domain.Catalog{Name: "software", Root: softwareRoot, RepoPathDepth: 1},
		Path:    oldPath,
		Name:    "api",
		RepoKey: oldRepoKey,
	}, false)
	if err != nil {
		t.Fatalf("observe repo: %v", err)
	}

	machine, err = state.LoadMachine(paths, "repo-move-host")
	if err != nil {
		t.Fatalf("load machine for record seed: %v", err)
	}
	machine.Repos = []domain.MachineRepoRecord{rec}
	machine.UpdatedAt = now
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine with record: %v", err)
	}

	return app, paths, oldPath, filepath.Join(referencesRoot, "api"), oldRepoKey
}
