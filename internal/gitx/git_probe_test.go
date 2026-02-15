package gitx

import (
	"os"
	"path/filepath"
	"testing"

	"bb-project/internal/domain"
)

type gitProbeFixture struct {
	root       string
	repoPath   string
	remotePath string
}

func TestProbePushAccessIgnoresPrePushHook(t *testing.T) {
	t.Parallel()

	runner := Runner{}
	fx := newGitProbeFixture(t, runner)
	prePushHook := filepath.Join(fx.repoPath, ".git", "hooks", "pre-push")
	mustWriteExecutableHook(t, prePushHook, "#!/bin/sh\nexit 77\n")

	if _, err := runner.RunGit(fx.repoPath, "push", "--dry-run", "--porcelain", "origin", "HEAD:refs/heads/main"); err == nil {
		t.Fatal("expected baseline dry-run push to fail when pre-push hook exits non-zero")
	}

	access, remote, err := runner.ProbePushAccess(fx.repoPath, "origin")
	if err != nil {
		t.Fatalf("ProbePushAccess() error = %v, want nil", err)
	}
	if access != domain.PushAccessReadWrite {
		t.Fatalf("ProbePushAccess() access = %q, want %q", access, domain.PushAccessReadWrite)
	}
	if remote != "origin" {
		t.Fatalf("ProbePushAccess() remote = %q, want %q", remote, "origin")
	}
}

func TestProbeSyncWithUpstreamDetectsMergeConflictFromStdout(t *testing.T) {
	t.Parallel()

	runner := Runner{}
	fx := newGitProbeFixture(t, runner)

	remoteClonePath := filepath.Join(fx.root, "remote-clone")
	if _, err := runner.RunGit(fx.root, "clone", fx.remotePath, remoteClonePath); err != nil {
		t.Fatalf("clone remote: %v", err)
	}
	if _, err := runner.RunGit(remoteClonePath, "checkout", "-B", "main", "--track", "origin/main"); err != nil {
		t.Fatalf("checkout tracked main in remote clone: %v", err)
	}
	mustWriteFile(t, filepath.Join(remoteClonePath, "shared.txt"), "remote change\n")
	if err := runner.AddAll(remoteClonePath); err != nil {
		t.Fatalf("stage remote change: %v", err)
	}
	if err := runner.Commit(remoteClonePath, "remote change"); err != nil {
		t.Fatalf("commit remote change: %v", err)
	}
	if _, err := runner.RunGit(remoteClonePath, "push", "origin", "main"); err != nil {
		t.Fatalf("push remote change: %v", err)
	}

	mustWriteFile(t, filepath.Join(fx.repoPath, "shared.txt"), "local change\n")
	if err := runner.AddAll(fx.repoPath); err != nil {
		t.Fatalf("stage local change: %v", err)
	}
	if err := runner.Commit(fx.repoPath, "local change"); err != nil {
		t.Fatalf("commit local change: %v", err)
	}
	if _, err := runner.RunGit(fx.repoPath, "fetch", "origin"); err != nil {
		t.Fatalf("fetch origin: %v", err)
	}

	outcome, err := runner.ProbeSyncWithUpstream(fx.repoPath, "origin/main", "merge")
	if err != nil {
		t.Fatalf("ProbeSyncWithUpstream() error = %v, want nil", err)
	}
	if outcome != SyncProbeOutcomeConflict {
		t.Fatalf("ProbeSyncWithUpstream() outcome = %q, want %q", outcome, SyncProbeOutcomeConflict)
	}
}

func TestProbeSyncWithUpstreamIgnoresWorktreeHooks(t *testing.T) {
	t.Parallel()

	runner := Runner{}
	fx := newGitProbeFixture(t, runner)

	postCheckoutHook := filepath.Join(fx.repoPath, ".git", "hooks", "post-checkout")
	mustWriteExecutableHook(t, postCheckoutHook, "#!/bin/sh\nexit 66\n")
	baselineWorktreePath := filepath.Join(fx.root, "baseline-worktree")
	if _, err := runner.RunGit(fx.repoPath, "worktree", "add", "--detach", baselineWorktreePath, "HEAD"); err == nil {
		t.Fatal("expected baseline worktree add to fail when post-checkout hook exits non-zero")
	}

	outcome, err := runner.ProbeSyncWithUpstream(fx.repoPath, "origin/main", "merge")
	if err != nil {
		t.Fatalf("ProbeSyncWithUpstream() error = %v, want nil", err)
	}
	if outcome != SyncProbeOutcomeClean {
		t.Fatalf("ProbeSyncWithUpstream() outcome = %q, want %q", outcome, SyncProbeOutcomeClean)
	}
}

func newGitProbeFixture(t *testing.T, runner Runner) gitProbeFixture {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	remotePath := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	if _, err := runner.RunGit(root, "init", "--bare", "--initial-branch=main", remotePath); err != nil {
		t.Fatalf("init bare remote: %v", err)
	}
	if _, err := runner.RunGit(repoPath, "init", "-b", "main"); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoPath, "shared.txt"), "base\n")
	if err := runner.AddAll(repoPath); err != nil {
		t.Fatalf("stage initial content: %v", err)
	}
	if err := runner.Commit(repoPath, "initial commit"); err != nil {
		t.Fatalf("commit initial content: %v", err)
	}
	if err := runner.AddRemote(repoPath, "origin", remotePath); err != nil {
		t.Fatalf("add origin remote: %v", err)
	}
	if _, err := runner.RunGit(repoPath, "push", "-u", "origin", "main"); err != nil {
		t.Fatalf("push initial commit: %v", err)
	}
	return gitProbeFixture{
		root:       root,
		repoPath:   repoPath,
		remotePath: remotePath,
	}
}

func mustWriteExecutableHook(t *testing.T, path string, content string) {
	t.Helper()

	mustWriteFile(t, path, content)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod hook %q: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}
