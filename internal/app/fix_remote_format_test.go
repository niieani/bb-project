package app

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestObserveRepoMarksRemoteFormatMismatch(t *testing.T) {
	t.Parallel()

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	cfg.GitHub.RemoteProtocol = "ssh"
	cfg.GitHub.PreferredRemoteURLTemplate = "git@${org}.github.com:${org}/${repo}.git"

	app := New(state.NewPaths(t.TempDir()), io.Discard, io.Discard)
	repoPath := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := app.Git.AddOrigin(repoPath, "https://github.com/you/demo.git"); err != nil {
		t.Fatalf("add origin: %v", err)
	}

	rec, err := app.observeRepo(cfg, discoveredRepo{
		Catalog: domain.Catalog{Name: "software", Root: filepath.Dir(repoPath), RepoPathDepth: 1},
		Path:    repoPath,
		Name:    "demo",
		RepoKey: "",
	}, false)
	if err != nil {
		t.Fatalf("observeRepo returned error: %v", err)
	}
	if !slices.Contains(rec.UnsyncableReasons, domain.ReasonRemoteFormatMismatch) {
		t.Fatalf("unsyncable reasons = %v, want %q", rec.UnsyncableReasons, domain.ReasonRemoteFormatMismatch)
	}
}

func TestApplyFixActionAlignRemoteFormatRewritesRemoteAndMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 16, 14, 0, 0, 0, time.UTC)
	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "fix-remote-format-host", nil }

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	cfg.GitHub.RemoteProtocol = "ssh"
	cfg.GitHub.PreferredRemoteURLTemplate = "git@${org}.github.com:${org}/${repo}.git"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	catalogRoot := filepath.Join(paths.Home, "software")
	repoPath := filepath.Join(catalogRoot, "demo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := app.Git.AddOrigin(repoPath, "https://github.com/you/demo.git"); err != nil {
		t.Fatalf("add origin: %v", err)
	}

	rec := domain.MachineRepoRecord{
		RepoKey:           "software/demo",
		Name:              "demo",
		Catalog:           "software",
		Path:              repoPath,
		OriginURL:         "https://github.com/you/demo.git",
		Syncable:          false,
		UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonRemoteFormatMismatch},
	}
	rec.StateHash = domain.ComputeStateHash(rec)

	machine := state.BootstrapMachine("fix-remote-format-host", "fix-remote-format-host", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: catalogRoot, RepoPathDepth: 1}}
	machine.Repos = []domain.MachineRepoRecord{rec}
	machine.LastScanAt = now
	machine.LastScanCatalogs = []string{"software"}
	machine.UpdatedAt = now
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	meta := domain.RepoMetadataFile{
		RepoKey:          "software/demo",
		Name:             "demo",
		OriginURL:        "https://github.com/you/demo.git",
		Visibility:       domain.VisibilityPrivate,
		PreferredCatalog: "software",
		AutoPush:         domain.AutoPushModeDisabled,
		PushAccess:       domain.PushAccessReadWrite,
	}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	updated, err := app.applyFixActionWithObserver(
		[]string{"software"},
		repoPath,
		FixActionAlignRemoteFormat,
		fixApplyOptions{Interactive: false},
		nil,
	)
	if err != nil {
		t.Fatalf("applyFixActionWithObserver returned error: %v", err)
	}

	wantOrigin := "git@you.github.com:you/demo.git"
	if got := updated.Record.OriginURL; got != wantOrigin {
		t.Fatalf("updated record origin = %q, want %q", got, wantOrigin)
	}
	origin, err := app.Git.RepoOrigin(repoPath)
	if err != nil {
		t.Fatalf("read origin: %v", err)
	}
	if origin != wantOrigin {
		t.Fatalf("git origin = %q, want %q", origin, wantOrigin)
	}

	updatedMeta, err := state.LoadRepoMetadata(paths, "software/demo")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if updatedMeta.OriginURL != wantOrigin {
		t.Fatalf("metadata origin = %q, want %q", updatedMeta.OriginURL, wantOrigin)
	}
	if updatedMeta.PushAccess != domain.PushAccessUnknown {
		t.Fatalf("metadata push access = %q, want %q", updatedMeta.PushAccess, domain.PushAccessUnknown)
	}
}
