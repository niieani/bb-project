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

func TestStageCommitPushWithPublishBranchCreatesAndPublishesNewBranch(t *testing.T) {
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
	mainBeforeOut, mainBeforeErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainBefore := mustTrimOutput(t, mainBeforeOut, mainBeforeErr)

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
	mainAfterOut, mainAfterErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainAfter := mustTrimOutput(t, mainAfterOut, mainAfterErr)
	if mainAfter != mainBefore {
		t.Fatalf("main ref changed: before=%s after=%s", mainBefore, mainAfter)
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

func TestPushWithPublishBranchCreatesAndPublishesNewBranch(t *testing.T) {
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
	mainBeforeOut, mainBeforeErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainBefore := mustTrimOutput(t, mainBeforeOut, mainBeforeErr)

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
	mainAfterOut, mainAfterErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainAfter := mustTrimOutput(t, mainAfterOut, mainAfterErr)
	if mainAfter != mainBefore {
		t.Fatalf("main ref changed: before=%s after=%s", mainBefore, mainAfter)
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

func TestStageCommitPushWithPublishBranchFromPartiallyStagedStateCommitsOnNewBranchOnly(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 15, 16, 15, 0, 0, time.UTC) }

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
	mainBeforeOut, mainBeforeErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainBefore := mustTrimOutput(t, mainBeforeOut, mainBeforeErr)

	notesPath := filepath.Join(repoPath, "notes.txt")
	if err := os.WriteFile(notesPath, []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("write notes staged content failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("stage initial notes content failed: %v", err)
	}
	if err := os.WriteFile(notesPath, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("write notes unstaged content failed: %v", err)
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
		CommitMessage:      "publish partially staged branch",
		ForkBranchRenameTo: "feature/partial-publish",
	}, nil); err != nil {
		t.Fatalf("execute stage-commit-push failed: %v", err)
	}

	branch, err := app.Git.CurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "feature/partial-publish" {
		t.Fatalf("current branch = %q, want %q", branch, "feature/partial-publish")
	}

	mainAfterOut, mainAfterErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainAfter := mustTrimOutput(t, mainAfterOut, mainAfterErr)
	if mainAfter != mainBefore {
		t.Fatalf("main ref changed: before=%s after=%s", mainBefore, mainAfter)
	}

	headNotesOut, headNotesErr := app.Git.RunGit(repoPath, "show", "HEAD:notes.txt")
	headNotes := mustTrimOutput(t, headNotesOut, headNotesErr)
	if headNotes != "line1\nline2" {
		t.Fatalf("published branch notes content = %q, want %q", headNotes, "line1\\nline2")
	}

	if _, err := app.Git.RunGit(repoPath, "show", "main:notes.txt"); err == nil {
		t.Fatal("did not expect notes.txt on main branch")
	}
}

func TestPublishNewBranchCreatesBranchBeforeCommitAndLeavesOriginalBranchUntouched(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 15, 17, 0, 0, 0, time.UTC) }

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

	mainBeforeOut, mainBeforeErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainBefore := mustTrimOutput(t, mainBeforeOut, mainBeforeErr)
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

	if err := app.executeFixAction(domain.ConfigFile{}, target, FixActionPublishNewBranch, fixApplyOptions{
		Interactive:        true,
		CommitMessage:      "publish isolated branch",
		ForkBranchRenameTo: "feature/isolated",
	}, nil); err != nil {
		t.Fatalf("execute publish-new-branch failed: %v", err)
	}

	branch, err := app.Git.CurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "feature/isolated" {
		t.Fatalf("current branch = %q, want %q", branch, "feature/isolated")
	}

	mainAfterOut, mainAfterErr := app.Git.RunGit(repoPath, "rev-parse", "main")
	mainAfter := mustTrimOutput(t, mainAfterOut, mainAfterErr)
	if mainAfter != mainBefore {
		t.Fatalf("main ref changed: before=%s after=%s", mainBefore, mainAfter)
	}

	upstream, err := app.Git.Upstream(repoPath)
	if err != nil {
		t.Fatalf("upstream failed: %v", err)
	}
	if upstream != "origin/feature/isolated" {
		t.Fatalf("upstream = %q, want %q", upstream, "origin/feature/isolated")
	}

	if _, err := app.Git.RunGit(repoPath, "--git-dir", remotePath, "rev-parse", "refs/heads/feature/isolated"); err != nil {
		t.Fatalf("expected remote isolated branch to exist: %v", err)
	}
}

func TestPublishNewBranchWithReturnSyncSwitchesBackAndFastForwardsOriginalBranch(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 15, 17, 30, 0, 0, time.UTC) }

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

	remoteClone := filepath.Join(t.TempDir(), "remote-clone")
	if err := app.Git.Clone(remotePath, remoteClone); err != nil {
		t.Fatalf("clone remote failed: %v", err)
	}
	if _, err := app.Git.RunGit(remoteClone, "checkout", "-B", "main", "--track", "origin/main"); err != nil {
		t.Fatalf("checkout remote clone main failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(remoteClone, "remote-ahead.txt"), []byte("remote ahead\n"), 0o644); err != nil {
		t.Fatalf("write remote ahead file failed: %v", err)
	}
	if err := app.Git.AddAll(remoteClone); err != nil {
		t.Fatalf("git add remote ahead failed: %v", err)
	}
	if err := app.Git.Commit(remoteClone, "remote ahead commit"); err != nil {
		t.Fatalf("git commit remote ahead failed: %v", err)
	}
	if _, err := app.Git.RunGit(remoteClone, "push", "origin", "main"); err != nil {
		t.Fatalf("push remote ahead failed: %v", err)
	}

	if err := app.Git.FetchPrune(repoPath); err != nil {
		t.Fatalf("fetch local repo failed: %v", err)
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
			Behind:          1,
		},
		Meta: &domain.RepoMetadataFile{
			RepoKey:         "software/api",
			Name:            "api",
			OriginURL:       remotePath,
			PreferredRemote: "origin",
		},
	}

	if err := app.executeFixAction(domain.ConfigFile{Sync: domain.SyncConfig{FetchPrune: true}}, target, FixActionPublishNewBranch, fixApplyOptions{
		Interactive:                   true,
		CommitMessage:                 "auto",
		ForkBranchRenameTo:            "feature/safe-sync",
		ReturnToOriginalBranchAndSync: true,
	}, nil); err != nil {
		t.Fatalf("execute publish-new-branch with return/sync failed: %v", err)
	}

	branch, err := app.Git.CurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "main" {
		t.Fatalf("current branch = %q, want %q", branch, "main")
	}

	countsOut, countsErr := app.Git.RunGit(repoPath, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	counts := mustTrimOutput(t, countsOut, countsErr)
	if counts != "0\t0" {
		t.Fatalf("ahead/behind after return/sync = %q, want 0\\t0", counts)
	}

	if _, err := app.Git.RunGit(repoPath, "--git-dir", remotePath, "rev-parse", "refs/heads/feature/safe-sync"); err != nil {
		t.Fatalf("expected remote publish branch to exist: %v", err)
	}
}

func TestPublishNewBranchRequiresExplicitTargetBranch(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 15, 18, 0, 0, 0, time.UTC) }

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
	}

	err := app.executeFixAction(domain.ConfigFile{}, target, FixActionPublishNewBranch, fixApplyOptions{
		Interactive: true,
	}, nil)
	if err == nil {
		t.Fatal("expected publish-new-branch to fail without explicit publish target")
	}
}

func mustTrimOutput(t *testing.T, out string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("git command failed: %v", err)
	}
	return strings.TrimSpace(out)
}
