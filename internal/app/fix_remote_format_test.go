package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunSyncAutomaticRemoteAlignment(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		verifyOK    bool
		wantAligned bool
	}{
		{"success_persists_before_observation", true, true, true},
		{"verification_failure_reverts_and_continues", true, false, false},
		{"disabled_gate_does_not_mutate", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realGit, err := exec.LookPath("git")
			if err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			fakeGit := filepath.Join(bin, "git")
			verifyExit := 1
			if tt.verifyOK {
				verifyExit = 0
			}
			script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"ls-remote\" ]; then exit %d; fi\nexec %q \"$@\"\n", verifyExit, realGit)
			if err := os.WriteFile(fakeGit, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			paths := state.NewPaths(t.TempDir())
			app := New(paths, io.Discard, io.Discard)
			now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
			app.Now = func() time.Time { return now }
			root := filepath.Join(paths.Home, "software")
			repo := filepath.Join(root, "demo")
			if err := os.MkdirAll(repo, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := app.Git.InitRepo(repo); err != nil {
				t.Fatal(err)
			}
			previous := "https://github.com/you/demo.git"
			if err := app.Git.AddOrigin(repo, previous); err != nil {
				t.Fatal(err)
			}
			cfg := state.DefaultConfig()
			cfg.Sync.AutoAlignRemoteFormat = tt.enabled
			cfg.Sync.FetchPrune = false
			cfg.GitHub.PreferredRemoteURLTemplate = "git@${org}.github.com:${org}/${repo}.git"
			if err := state.SaveConfig(paths, cfg); err != nil {
				t.Fatal(err)
			}
			machine := state.BootstrapMachine("test-machine", "test-machine", now)
			machine.DefaultCatalog = "software"
			machine.Catalogs = []domain.Catalog{{Name: "software", Root: root, RepoPathDepth: 1}}
			if err := state.SaveMachine(paths, machine); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(paths.MachineIDPath()), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(paths.MachineIDPath(), []byte("test-machine\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			code, runErr := app.runSync(SyncOptions{})
			if runErr != nil || code != 0 {
				t.Fatalf("runSync = code %d err %v", code, runErr)
			}
			raw, err := app.Git.RemoteURLRaw(repo, "origin")
			if err != nil {
				t.Fatal(err)
			}
			want := previous
			if tt.wantAligned {
				want = "git@you.github.com:you/demo.git"
			}
			if raw != want {
				t.Fatalf("origin = %q, want %q", raw, want)
			}
			mf, err := state.LoadMachine(paths, "test-machine")
			if err != nil {
				t.Fatal(err)
			}
			if len(mf.Repos) != 1 {
				t.Fatalf("repos = %d", len(mf.Repos))
			}
			hasWarning := slices.Contains(mf.Repos[0].Warnings, domain.ReasonRemoteFormatMismatch)
			if hasWarning == tt.wantAligned {
				t.Fatalf("warning present = %t, aligned = %t", hasWarning, tt.wantAligned)
			}
		})
	}
}

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
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Git.RunGit(repoPath, "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Git.RunGit(repoPath, "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Git.RunGit(repoPath, "config", "branch.main.remote", "origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Git.RunGit(repoPath, "config", "branch.main.merge", "refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Git.RunGit(repoPath, "update-ref", "refs/remotes/origin/main", "HEAD"); err != nil {
		t.Fatal(err)
	}
	reachable := filepath.Join(t.TempDir(), "reachable.git")
	if _, err := app.Git.RunGit(t.TempDir(), "init", "--bare", reachable); err != nil {
		t.Fatalf("init reachable remote: %v", err)
	}
	if _, err := app.Git.RunGit(repoPath, "config", "url."+reachable+".insteadOf", "git@you.github.com:you/demo.git"); err != nil {
		t.Fatalf("configure URL rewrite: %v", err)
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
	if !slices.Contains(rec.Warnings, domain.ReasonRemoteFormatMismatch) {
		t.Fatalf("warnings = %v, want %q", rec.Warnings, domain.ReasonRemoteFormatMismatch)
	}
	if rec.State != domain.RepoStateSynced {
		t.Fatalf("state = %q, warning must not affect tier; reasons=%v", rec.State, rec.Reasons)
	}
}

func TestAlignRemoteFormatVerifiedRevertsOnVerificationFailure(t *testing.T) {
	t.Parallel()
	app := New(state.NewPaths(t.TempDir()), io.Discard, io.Discard)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := app.Git.InitRepo(repo); err != nil {
		t.Fatal(err)
	}
	previous := "https://github.com/you/demo.git"
	if err := app.Git.AddOrigin(repo, previous); err != nil {
		t.Fatal(err)
	}
	if err := app.alignRemoteFormatVerified(repo, "origin", previous, filepath.Join(t.TempDir(), "missing.git")); err == nil {
		t.Fatal("expected verification failure")
	}
	got, err := app.Git.RemoteURLRaw(repo, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if got != previous {
		t.Fatalf("origin = %q, want byte-identical %q", got, previous)
	}
}

func TestManualAlignRemoteFormatVerificationFailureRestoresOrigin(t *testing.T) {
	t.Parallel()
	app := New(state.NewPaths(t.TempDir()), io.Discard, io.Discard)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := app.Git.InitRepo(repo); err != nil {
		t.Fatal(err)
	}
	previous := "https://github.com/you/demo.git"
	if err := app.Git.AddOrigin(repo, previous); err != nil {
		t.Fatal(err)
	}
	target := fixRepoState{Record: domain.MachineRepoRecord{Path: repo, OriginURL: previous, Warnings: []domain.UnsyncableReason{domain.ReasonRemoteFormatMismatch}}}
	cfg := state.DefaultConfig()
	cfg.GitHub.PreferredRemoteURLTemplate = "git@${org}.invalid:${org}/${repo}.git"
	err := app.executeFixAction(cfg, target, FixActionAlignRemoteFormat, fixApplyOptions{}, nil)
	if err == nil {
		t.Fatal("expected verification failure")
	}
	got, readErr := app.Git.RemoteURLRaw(repo, "origin")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got != previous {
		t.Fatalf("origin = %q, want %q", got, previous)
	}
}

func TestAlignRemoteFormatsBeforeObservationHonorsDisabledGate(t *testing.T) {
	t.Parallel()
	app := New(state.NewPaths(t.TempDir()), io.Discard, io.Discard)
	root := t.TempDir()
	repo := filepath.Join(root, "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := app.Git.InitRepo(repo); err != nil {
		t.Fatal(err)
	}
	previous := "https://github.com/you/demo.git"
	if err := app.Git.AddOrigin(repo, previous); err != nil {
		t.Fatal(err)
	}
	cfg := state.DefaultConfig()
	cfg.Sync.AutoAlignRemoteFormat = false
	cfg.GitHub.PreferredRemoteURLTemplate = "git@${org}.github.com:${org}/${repo}.git"
	if err := app.alignRemoteFormatsBeforeObservation(cfg, []domain.Catalog{{Name: "software", Root: root, RepoPathDepth: 1}}, false); err != nil {
		t.Fatal(err)
	}
	got, err := app.Git.RepoOrigin(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != previous {
		t.Fatalf("origin mutated with gate disabled: %q", got)
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
	reachable := filepath.Join(t.TempDir(), "reachable.git")
	if _, err := app.Git.RunGit(t.TempDir(), "init", "--bare", reachable); err != nil {
		t.Fatalf("init reachable remote: %v", err)
	}
	if _, err := app.Git.RunGit(repoPath, "config", "url."+reachable+".insteadOf", "git@you.github.com:you/demo.git"); err != nil {
		t.Fatalf("configure URL rewrite: %v", err)
	}

	rec := domain.MachineRepoRecord{
		RepoKey:   "software/demo",
		Name:      "demo",
		Catalog:   "software",
		Path:      repoPath,
		OriginURL: "https://github.com/you/demo.git",
		State:     domain.RepoStateSynced,
		Warnings:  []domain.UnsyncableReason{domain.ReasonRemoteFormatMismatch},
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
	if got := updated.Record.OriginURL; got != reachable {
		t.Fatalf("updated effective origin = %q, want reachable rewrite %q", got, reachable)
	}
	origin, err := app.Git.RunGit(repoPath, "config", "--get", "remote.origin.url")
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
