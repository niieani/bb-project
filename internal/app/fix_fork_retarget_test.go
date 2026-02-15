package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestForkAndRetargetFromFixWithBranchRenameRenamesBeforePush(t *testing.T) {
	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 15, 15, 0, 0, 0, time.UTC) }

	repoPath := filepath.Join(t.TempDir(), "bun")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	if err := app.Git.AddOrigin(repoPath, "git@github.com:oven-sh/bun.git"); err != nil {
		t.Fatalf("add origin failed: %v", err)
	}

	repoKey := "software/bun"
	meta := domain.RepoMetadataFile{
		RepoKey:             repoKey,
		Name:                "bun",
		OriginURL:           "git@github.com:oven-sh/bun.git",
		PreferredCatalog:    "software",
		PreferredRemote:     "origin",
		PushAccess:          domain.PushAccessReadOnly,
		AutoPush:            domain.AutoPushModeDisabled,
		BranchFollowEnabled: true,
	}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("save metadata failed: %v", err)
	}

	remoteRoot := filepath.Join(t.TempDir(), "remotes")
	t.Setenv("BB_TEST_REMOTE_ROOT", remoteRoot)

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			RepoKey:   repoKey,
			Name:      "bun",
			Path:      repoPath,
			OriginURL: "git@github.com:oven-sh/bun.git",
			Branch:    "main",
		},
		Meta: &meta,
	}
	cfg := domain.ConfigFile{}
	cfg.GitHub.Owner = "acme"
	cfg.GitHub.RemoteProtocol = "ssh"

	if err := app.forkAndRetargetFromFix(cfg, target, fixApplyOptions{
		Interactive:        true,
		ForkBranchRenameTo: "feature/acme-bun",
	}, nil, nil); err != nil {
		t.Fatalf("forkAndRetargetFromFix failed: %v", err)
	}

	branch, err := app.Git.CurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "feature/acme-bun" {
		t.Fatalf("current branch = %q, want %q", branch, "feature/acme-bun")
	}

	upstream, err := app.Git.Upstream(repoPath)
	if err != nil {
		t.Fatalf("upstream failed: %v", err)
	}
	if upstream != "acme/feature/acme-bun" {
		t.Fatalf("upstream = %q, want %q", upstream, "acme/feature/acme-bun")
	}

	remotePath := filepath.Join(remoteRoot, "acme", "bun.git")
	if _, err := app.Git.RunGit(repoPath, "--git-dir", remotePath, "rev-parse", "refs/heads/feature/acme-bun"); err != nil {
		t.Fatalf("expected remote branch to exist after renamed push: %v", err)
	}
	if _, err := app.Git.RunGit(repoPath, "--git-dir", remotePath, "rev-parse", "refs/heads/main"); err == nil {
		t.Fatal("did not expect remote default branch to be force-updated when renamed push is used")
	}

	updatedMeta, err := state.LoadRepoMetadata(paths, repoKey)
	if err != nil {
		t.Fatalf("load updated metadata failed: %v", err)
	}
	if got := strings.TrimSpace(updatedMeta.PreferredRemote); got != "acme" {
		t.Fatalf("preferred_remote = %q, want %q", got, "acme")
	}
}
