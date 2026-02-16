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

func TestRunCloneGitHubHTTPLink(t *testing.T) {
	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	remoteRoot := filepath.Join(home, "remotes")
	remotePath := setupCloneTestRemote(t, remoteRoot, "openai", "codex")
	t.Setenv("BB_TEST_REMOTE_ROOT", remoteRoot)

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	cfg.Clone.DefaultCatalog = "references"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "references", Root: filepath.Join(home, "catalogs", "references"), RepoPathDepth: 2},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	var stdout bytes.Buffer
	app := New(paths, &stdout, &bytes.Buffer{})
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }

	code, err := app.RunClone(CloneOptions{Repo: "https://github.com/openai/codex"})
	if err != nil {
		t.Fatalf("RunClone error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	targetPath := filepath.Join(home, "catalogs", "references", "openai", "codex")
	if _, err := os.Stat(filepath.Join(targetPath, ".git")); err != nil {
		t.Fatalf("expected cloned repo: %v", err)
	}
	origin, err := app.Git.RepoOrigin(targetPath)
	if err != nil {
		t.Fatalf("repo origin: %v", err)
	}
	if ok, err := originsMatchNormalized(origin, remotePath); err != nil || !ok {
		t.Fatalf("origin mismatch: got=%q expected=%q err=%v", origin, remotePath, err)
	}
}

func TestRunCloneCatalogPresetMapping(t *testing.T) {
	tests := []struct {
		name          string
		catalogPreset map[string]string
		wantShallow   bool
		wantFilterSet bool
	}{
		{
			name:          "preset mapping absent does not apply preset",
			catalogPreset: map[string]string{},
			wantShallow:   false,
			wantFilterSet: false,
		},
		{
			name: "preset mapping applies preset",
			catalogPreset: map[string]string{
				"references": "references",
			},
			wantShallow:   true,
			wantFilterSet: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
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
			cfg.Clone.CatalogPreset = tt.catalogPreset
			if err := state.SaveConfig(paths, cfg); err != nil {
				t.Fatalf("save config: %v", err)
			}

			machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
			machine.DefaultCatalog = "software"
			machine.Catalogs = []domain.Catalog{
				{Name: "references", Root: filepath.Join(home, "catalogs", "references"), RepoPathDepth: 2},
			}
			if err := state.SaveMachine(paths, machine); err != nil {
				t.Fatalf("save machine: %v", err)
			}

			app := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
			app.Now = func() time.Time { return now }
			app.Hostname = func() (string, error) { return "host-a", nil }

			code, err := app.RunClone(CloneOptions{Repo: "openai/codex"})
			if err != nil {
				t.Fatalf("RunClone error: %v", err)
			}
			if code != 0 {
				t.Fatalf("code = %d, want 0", code)
			}

			targetPath := filepath.Join(home, "catalogs", "references", "openai", "codex")
			shallowRaw, err := app.Git.RunGit(targetPath, "rev-parse", "--is-shallow-repository")
			if err != nil {
				t.Fatalf("rev-parse shallow: %v", err)
			}
			gotShallow := strings.TrimSpace(shallowRaw) == "true"
			if !gotShallow {
				count, err := app.Git.RunGit(targetPath, "rev-list", "--count", "HEAD")
				if err != nil {
					t.Fatalf("rev-list count: %v", err)
				}
				if tt.wantShallow && strings.TrimSpace(count) != "1" {
					t.Fatalf("expected shallow history depth 1, got count=%q", count)
				}
				if !tt.wantShallow && strings.TrimSpace(count) == "1" {
					t.Fatalf("unexpected shallow depth with count=%q", count)
				}
			} else if !tt.wantShallow {
				t.Fatalf("is shallow = true, want false")
			}
			filter, err := app.Git.RunGit(targetPath, "config", "--get", "remote.origin.partialclonefilter")
			if tt.wantFilterSet {
				if err != nil {
					t.Fatalf("expected partialclonefilter: %v", err)
				}
				if filter != "blob:none" {
					t.Fatalf("partialclonefilter = %q, want %q", filter, "blob:none")
				}
				return
			}
			if err == nil {
				t.Fatalf("unexpected partialclonefilter = %q", filter)
			}
		})
	}
}

func TestRunCloneExistingOriginNoopReportsLocation(t *testing.T) {
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

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "references", Root: filepath.Join(home, "catalogs", "references"), RepoPathDepth: 2},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	var stdout bytes.Buffer
	app := New(paths, &stdout, &bytes.Buffer{})
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "host-a", nil }

	if code, err := app.RunClone(CloneOptions{Repo: "openai/codex"}); err != nil || code != 0 {
		t.Fatalf("first RunClone failed code=%d err=%v", code, err)
	}
	stdout.Reset()

	if code, err := app.RunClone(CloneOptions{Repo: "openai/codex", As: "alt/codex"}); err != nil || code != 0 {
		t.Fatalf("second RunClone failed code=%d err=%v", code, err)
	}
	out := stdout.String()
	if !strings.Contains(out, "already exists") {
		t.Fatalf("expected noop notice, got: %s", out)
	}
	if !strings.Contains(out, filepath.Join("references", "openai", "codex")) {
		t.Fatalf("expected location in notice, got: %s", out)
	}
}

func setupCloneTestRemote(t *testing.T, remoteRoot string, owner string, repo string) string {
	t.Helper()

	r := New(state.NewPaths(t.TempDir()), &bytes.Buffer{}, &bytes.Buffer{}).Git
	remotePath := filepath.Join(remoteRoot, owner, repo+".git")
	if err := os.MkdirAll(filepath.Dir(remotePath), 0o755); err != nil {
		t.Fatalf("mkdir remote parent: %v", err)
	}
	if _, err := r.RunGit(remoteRoot, "init", "--bare", remotePath); err != nil {
		t.Fatalf("init bare remote: %v", err)
	}
	if _, err := r.RunGit(remotePath, "config", "uploadpack.allowFilter", "true"); err != nil {
		t.Fatalf("set allowFilter: %v", err)
	}
	if _, err := r.RunGit(remotePath, "config", "uploadpack.allowAnySHA1InWant", "true"); err != nil {
		t.Fatalf("set allowAnySHA1InWant: %v", err)
	}
	workPath := filepath.Join(remoteRoot, owner, repo+"-work")
	if _, err := r.RunGit(remoteRoot, "clone", remotePath, workPath); err != nil {
		t.Fatalf("clone remote into work path: %v", err)
	}
	if _, err := r.RunGit(workPath, "checkout", "-B", "main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if _, err := r.RunGit(workPath, "add", "README.md"); err != nil {
		t.Fatalf("git add README: %v", err)
	}
	if _, err := r.RunGit(workPath, "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit init: %v", err)
	}
	if _, err := r.RunGit(workPath, "push", "-u", "origin", "main"); err != nil {
		t.Fatalf("git push origin main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workPath, "CHANGELOG.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write CHANGELOG: %v", err)
	}
	if _, err := r.RunGit(workPath, "add", "CHANGELOG.md"); err != nil {
		t.Fatalf("git add CHANGELOG: %v", err)
	}
	if _, err := r.RunGit(workPath, "commit", "-m", "second"); err != nil {
		t.Fatalf("git commit second: %v", err)
	}
	if _, err := r.RunGit(workPath, "push", "origin", "main"); err != nil {
		t.Fatalf("git push second: %v", err)
	}
	if _, err := r.RunGit(remotePath, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		t.Fatalf("set remote HEAD: %v", err)
	}
	return remotePath
}
