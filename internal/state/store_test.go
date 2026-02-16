package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
)

func TestAcquireLock(t *testing.T) {
	t.Run("active lock blocks", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		lock, err := AcquireLock(paths)
		if err != nil {
			t.Fatalf("first lock: %v", err)
		}
		defer func() { _ = lock.Release() }()

		_, err = AcquireLock(paths)
		if err == nil {
			t.Fatal("expected second lock acquire to fail")
		}
		if !strings.Contains(err.Error(), "another bb process holds the lock") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stale lock with dead pid is recovered", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		if err := EnsureDir(paths.LocalStateRoot()); err != nil {
			t.Fatalf("ensure local state root: %v", err)
		}
		hostname, err := os.Hostname()
		if err != nil {
			t.Fatalf("hostname: %v", err)
		}
		lockBody := fmt.Sprintf(
			"pid=%d\nhostname=%s\ncreated_at=%s\n",
			99999999,
			hostname,
			time.Now().UTC().Format(time.RFC3339),
		)
		if err := os.WriteFile(paths.LockPath(), []byte(lockBody), 0o644); err != nil {
			t.Fatalf("write stale lock: %v", err)
		}

		lock, err := AcquireLock(paths)
		if err != nil {
			t.Fatalf("expected stale lock recovery, got: %v", err)
		}
		_ = lock.Release()
	})

	t.Run("old corrupt lock is recovered", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		if err := EnsureDir(paths.LocalStateRoot()); err != nil {
			t.Fatalf("ensure local state root: %v", err)
		}
		lockPath := paths.LockPath()
		if err := os.WriteFile(lockPath, []byte("held\n"), 0o644); err != nil {
			t.Fatalf("write lock: %v", err)
		}
		old := time.Now().Add(-25 * time.Hour)
		if err := os.Chtimes(lockPath, old, old); err != nil {
			t.Fatalf("chtimes lock: %v", err)
		}

		lock, err := AcquireLock(paths)
		if err != nil {
			t.Fatalf("expected stale corrupt lock recovery, got: %v", err)
		}
		_ = lock.Release()
	})

	t.Run("recent corrupt lock still blocks", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		if err := EnsureDir(paths.LocalStateRoot()); err != nil {
			t.Fatalf("ensure local state root: %v", err)
		}
		lockPath := paths.LockPath()
		if err := os.WriteFile(lockPath, []byte("held\n"), 0o644); err != nil {
			t.Fatalf("write lock: %v", err)
		}
		now := time.Now()
		if err := os.Chtimes(lockPath, now, now); err != nil {
			t.Fatalf("chtimes lock: %v", err)
		}

		_, err := AcquireLock(paths)
		if err == nil {
			t.Fatal("expected recent corrupt lock to block")
		}
		if !strings.Contains(err.Error(), "another bb process holds the lock") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestLockFilePayload(t *testing.T) {
	t.Parallel()

	paths := NewPaths(t.TempDir())
	lock, err := AcquireLock(paths)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	raw, err := os.ReadFile(filepath.Clean(paths.LockPath()))
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "pid=") {
		t.Fatalf("expected pid in lock file, got: %q", text)
	}
	if !strings.Contains(text, "hostname=") {
		t.Fatalf("expected hostname in lock file, got: %q", text)
	}
	if !strings.Contains(text, "created_at=") {
		t.Fatalf("expected created_at in lock file, got: %q", text)
	}
}

func TestRepoMetaFileNameUsesRepoKey(t *testing.T) {
	t.Parallel()

	got := RepoMetaFileName("software/openai/codex")
	if got != "software__openai__codex.yaml" {
		t.Fatalf("RepoMetaFileName() = %q, want %q", got, "software__openai__codex.yaml")
	}
}

func TestLoadAllRepoMetadataSkipsMissingRepoKeyAndSortsByRepoKey(t *testing.T) {
	t.Parallel()

	paths := NewPaths(t.TempDir())
	if err := EnsureDir(paths.RepoDir()); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	if err := SaveRepoMetadata(paths, domain.RepoMetadataFile{
		RepoKey:   "software/zeta",
		OriginURL: "https://github.com/you/zeta.git",
		Name:      "zeta",
	}); err != nil {
		t.Fatalf("save repo zeta: %v", err)
	}
	if err := SaveRepoMetadata(paths, domain.RepoMetadataFile{
		RepoKey:   "software/alpha",
		OriginURL: "https://github.com/you/alpha.git",
		Name:      "alpha",
	}); err != nil {
		t.Fatalf("save repo alpha: %v", err)
	}

	legacyPath := filepath.Join(paths.RepoDir(), "legacy.yaml")
	legacyYAML := strings.TrimSpace(`
version: 1
repo_id: github.com/you/legacy
name: legacy
`) + "\n"
	if err := os.WriteFile(legacyPath, []byte(legacyYAML), 0o644); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}

	repos, err := LoadAllRepoMetadata(paths)
	if err != nil {
		t.Fatalf("LoadAllRepoMetadata: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("repo count = %d, want 2", len(repos))
	}
	if repos[0].RepoKey != "software/alpha" || repos[1].RepoKey != "software/zeta" {
		t.Fatalf("unexpected repo key order: %q, %q", repos[0].RepoKey, repos[1].RepoKey)
	}
}

func TestDefaultConfigSetsSchedulerInterval(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Scheduler.IntervalMinutes != 60 {
		t.Fatalf("scheduler interval = %d, want 60", cfg.Scheduler.IntervalMinutes)
	}
}

func TestDefaultConfigCloneLinkDefaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Clone.DefaultCatalog != "" {
		t.Fatalf("clone.default_catalog = %q, want empty", cfg.Clone.DefaultCatalog)
	}
	if cfg.Clone.Shallow {
		t.Fatal("clone.shallow = true, want false")
	}
	if cfg.Clone.Filter != "" {
		t.Fatalf("clone.filter = %q, want empty", cfg.Clone.Filter)
	}
	preset, ok := cfg.Clone.Presets["references"]
	if !ok {
		t.Fatal("clone.presets.references missing")
	}
	if preset.Shallow == nil || !*preset.Shallow {
		t.Fatal("clone.presets.references.shallow missing or false")
	}
	if preset.Filter == nil || *preset.Filter != "blob:none" {
		t.Fatalf("clone.presets.references.filter = %#v, want %q", preset.Filter, "blob:none")
	}
	if got := cfg.Clone.CatalogPreset["references"]; got != "references" {
		t.Fatalf("clone.catalog_preset.references = %q, want %q", got, "references")
	}
	if cfg.Link.TargetDir != "references" {
		t.Fatalf("link.target_dir = %q, want %q", cfg.Link.TargetDir, "references")
	}
	if cfg.Link.Absolute {
		t.Fatal("link.absolute = true, want false")
	}
	if !cfg.Integrations.Lumen.Enabled {
		t.Fatal("integrations.lumen.enabled = false, want true")
	}
	if !cfg.Integrations.Lumen.ShowInstallTip {
		t.Fatal("integrations.lumen.show_install_tip = false, want true")
	}
	if cfg.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty {
		t.Fatal("integrations.lumen.auto_generate_commit_message_when_empty = true, want false")
	}
}

func TestLoadConfigAllowsEmptyCloneCatalogPresetOverride(t *testing.T) {
	t.Parallel()

	paths := NewPaths(t.TempDir())
	cfg := DefaultConfig()
	cfg.GitHub.Owner = "you"
	cfg.Clone.CatalogPreset = map[string]string{}
	if err := SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := LoadConfig(paths)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Clone.CatalogPreset) != 0 {
		t.Fatalf("clone.catalog_preset = %#v, want empty map", loaded.Clone.CatalogPreset)
	}
}

func TestLoadConfigLumenIntegrationDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	t.Run("missing integration fields keep defaults", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		raw := strings.TrimSpace(`
version: 1
state_transport:
  mode: external
github:
  owner: alice
  default_visibility: private
  remote_protocol: ssh
clone:
  default_catalog: ""
  shallow: false
  filter: ""
  presets: {}
  catalog_preset: {}
link:
  target_dir: references
  absolute: false
sync:
  auto_discover: true
  include_untracked_as_dirty: true
  default_auto_push_private: true
  default_auto_push_public: false
  fetch_prune: true
  pull_ff_only: true
  scan_freshness_seconds: 60
scheduler:
  interval_minutes: 60
notify:
  enabled: true
  dedupe: true
  throttle_minutes: 60
`) + "\n"
		if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
			t.Fatalf("mkdir config root: %v", err)
		}
		if err := os.WriteFile(paths.ConfigPath(), []byte(raw), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		cfg, err := LoadConfig(paths)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if !cfg.Integrations.Lumen.Enabled {
			t.Fatal("integrations.lumen.enabled = false, want true")
		}
		if !cfg.Integrations.Lumen.ShowInstallTip {
			t.Fatal("integrations.lumen.show_install_tip = false, want true")
		}
		if cfg.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty {
			t.Fatal("integrations.lumen.auto_generate_commit_message_when_empty = true, want false")
		}
	})

	t.Run("explicit false values persist", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		cfg := DefaultConfig()
		cfg.GitHub.Owner = "alice"
		cfg.Integrations.Lumen.Enabled = false
		cfg.Integrations.Lumen.ShowInstallTip = false
		cfg.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty = true
		if err := SaveConfig(paths, cfg); err != nil {
			t.Fatalf("save config: %v", err)
		}

		loaded, err := LoadConfig(paths)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if loaded.Integrations.Lumen.Enabled {
			t.Fatal("integrations.lumen.enabled = true, want false")
		}
		if loaded.Integrations.Lumen.ShowInstallTip {
			t.Fatal("integrations.lumen.show_install_tip = true, want false")
		}
		if !loaded.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty {
			t.Fatal("integrations.lumen.auto_generate_commit_message_when_empty = false, want true")
		}
	})
}
