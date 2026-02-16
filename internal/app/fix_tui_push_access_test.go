package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestNewFixTUIModelReprobesUnknownPushAccessOnStartup(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 16, 12, 0, 0, 0, time.UTC)
	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "fix-tui-host", nil }

	cfg := state.DefaultConfig()
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	catalogRoot := filepath.Join(t.TempDir(), "catalogs", "software")
	repoPath := filepath.Join(catalogRoot, "demo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := app.Git.AddOrigin(repoPath, "git@niieani.github.com:acme/demo.git"); err != nil {
		t.Fatalf("add origin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	record := domain.MachineRepoRecord{
		RepoKey:         "software/demo",
		Name:            "demo",
		Catalog:         "software",
		Path:            repoPath,
		OriginURL:       "git@niieani.github.com:acme/demo.git",
		Branch:          "main",
		Upstream:        "origin/main",
		Syncable:        false,
		HasDirtyTracked: true,
		UnsyncableReasons: []domain.UnsyncableReason{
			domain.ReasonDirtyTracked,
		},
	}
	record.StateHash = domain.ComputeStateHash(record)

	machine := state.BootstrapMachine("fix-tui-host", "fix-tui-host", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: catalogRoot}}
	machine.LastScanAt = now
	machine.LastScanCatalogs = []string{"software"}
	machine.Repos = []domain.MachineRepoRecord{record}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	meta := domain.RepoMetadataFile{
		RepoKey:             "software/demo",
		Name:                "demo",
		OriginURL:           "git@niieani.github.com:acme/demo.git",
		PreferredCatalog:    "software",
		AutoPush:            domain.AutoPushModeDisabled,
		PushAccess:          domain.PushAccessUnknown,
		BranchFollowEnabled: true,
	}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("save repo metadata: %v", err)
	}

	app.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "/usr/bin/gh", nil
		}
		return "", fmt.Errorf("unexpected executable lookup for %q", file)
	}
	app.RunCommand = func(name string, args ...string) (string, error) {
		if name != "gh" {
			return "", fmt.Errorf("unexpected command %q", name)
		}
		want := []string{"repo", "view", "acme/demo", "--json", "viewerPermission"}
		if len(args) != len(want) {
			t.Fatalf("gh args len=%d, want %d (%v)", len(args), len(want), args)
		}
		for i := range want {
			if args[i] != want[i] {
				t.Fatalf("gh arg[%d]=%q, want %q (args=%v)", i, args[i], want[i], args)
			}
		}
		return `{"viewerPermission":"READ"}`, nil
	}

	model, err := newFixTUIModel(app, []string{"software"}, true)
	if err != nil {
		t.Fatalf("newFixTUIModel: %v", err)
	}

	updated, err := state.LoadRepoMetadata(paths, "software/demo")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if updated.PushAccess != domain.PushAccessReadOnly {
		t.Fatalf("push_access=%q, want %q", updated.PushAccess, domain.PushAccessReadOnly)
	}

	if len(model.repos) != 1 {
		t.Fatalf("repos loaded=%d, want 1", len(model.repos))
	}
	actions := eligibleFixActions(model.repos[0].Record, model.repos[0].Meta, fixEligibilityContext{
		Interactive: true,
	})
	if containsAction(actions, FixActionStageCommitPush) {
		t.Fatalf("did not expect %q when push_access is unknown/read-only at startup, got %v", FixActionStageCommitPush, actions)
	}
	if containsAction(actions, FixActionPublishNewBranch) {
		t.Fatalf("did not expect %q when push_access is unknown/read-only at startup, got %v", FixActionPublishNewBranch, actions)
	}
}
