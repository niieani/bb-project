package app

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestStageCommitPushWithPublishBranchRenamesAndPublishesToNewBranch(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 15, 16, 0, 0, 0, time.UTC) }

	remotePath := filepath.Join(t.TempDir(), "remote.git")
	if _, err := app.Git.RunGit("", "init", "--bare", remotePath); err != nil {
		t.Fatalf("init remote failed: %v", err)
	}

	repoPath := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := app.Git.AddOrigin(repoPath, remotePath); err != nil {
		t.Fatalf("add origin failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	if err := app.Git.PushUpstreamWithPreferredRemote(repoPath, "main", "origin"); err != nil {
		t.Fatalf("initial push failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatalf("write feature file failed: %v", err)
	}

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:            "api",
			Path:            repoPath,
			OriginURL:       remotePath,
			Branch:          "main",
			Upstream:        "origin/main",
			HasDirtyTracked: true,
		},
		Meta: &domain.RepoMetadataFile{
			RepoKey:         "software/api",
			Name:            "api",
			OriginURL:       remotePath,
			PreferredRemote: "origin",
		},
	}

	if err := app.executeFixAction(domain.ConfigFile{}, target, FixActionStageCommitPush, fixApplyOptions{
		Interactive:        true,
		CommitMessage:      "publish safe branch",
		ForkBranchRenameTo: "feature/safe-publish",
	}, nil); err != nil {
		t.Fatalf("execute stage-commit-push failed: %v", err)
	}

	branch, err := app.Git.CurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "feature/safe-publish" {
		t.Fatalf("current branch = %q, want %q", branch, "feature/safe-publish")
	}

	upstream, err := app.Git.Upstream(repoPath)
	if err != nil {
		t.Fatalf("upstream failed: %v", err)
	}
	if upstream != "origin/feature/safe-publish" {
		t.Fatalf("upstream = %q, want %q", upstream, "origin/feature/safe-publish")
	}

	if _, err := app.Git.RunGit(repoPath, "--git-dir", remotePath, "rev-parse", "refs/heads/feature/safe-publish"); err != nil {
		t.Fatalf("expected remote safe branch to exist: %v", err)
	}
}

func TestPushWithPublishBranchRenamesAndPublishesToNewBranch(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 15, 16, 30, 0, 0, time.UTC) }

	remotePath := filepath.Join(t.TempDir(), "remote.git")
	if _, err := app.Git.RunGit("", "init", "--bare", remotePath); err != nil {
		t.Fatalf("init remote failed: %v", err)
	}

	repoPath := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := app.Git.AddOrigin(repoPath, remotePath); err != nil {
		t.Fatalf("add origin failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	if err := app.Git.PushUpstreamWithPreferredRemote(repoPath, "main", "origin"); err != nil {
		t.Fatalf("initial push failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "ahead.txt"), []byte("ahead\n"), 0o644); err != nil {
		t.Fatalf("write ahead file failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add ahead failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "ahead"); err != nil {
		t.Fatalf("git commit ahead failed: %v", err)
	}

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:      "api",
			Path:      repoPath,
			OriginURL: remotePath,
			Branch:    "main",
			Upstream:  "origin/main",
			Ahead:     1,
		},
		Meta: &domain.RepoMetadataFile{
			RepoKey:         "software/api",
			Name:            "api",
			OriginURL:       remotePath,
			PreferredRemote: "origin",
		},
	}

	if err := app.executeFixAction(domain.ConfigFile{}, target, FixActionPush, fixApplyOptions{
		Interactive:        true,
		ForkBranchRenameTo: "feature/safe-publish",
	}, nil); err != nil {
		t.Fatalf("execute push failed: %v", err)
	}

	branch, err := app.Git.CurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "feature/safe-publish" {
		t.Fatalf("current branch = %q, want %q", branch, "feature/safe-publish")
	}

	upstream, err := app.Git.Upstream(repoPath)
	if err != nil {
		t.Fatalf("upstream failed: %v", err)
	}
	if upstream != "origin/feature/safe-publish" {
		t.Fatalf("upstream = %q, want %q", upstream, "origin/feature/safe-publish")
	}

	if _, err := app.Git.RunGit(repoPath, "--git-dir", remotePath, "rev-parse", "refs/heads/feature/safe-publish"); err != nil {
		t.Fatalf("expected remote safe branch to exist: %v", err)
	}
}
