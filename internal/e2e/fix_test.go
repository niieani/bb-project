package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFixCases(t *testing.T) {
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	t.Run("requires interactive terminal for table mode", func(t *testing.T) {
		_, m, _ := setupSingleMachine(t)
		out, err := m.RunBB(now, "fix")
		if err == nil {
			t.Fatalf("expected bb fix to fail in non-interactive harness, output=%s", out)
		}
		if !strings.Contains(out, "requires an interactive terminal") {
			t.Fatalf("expected interactive terminal error, got: %s", out)
		}
	})

	t.Run("lists and applies push", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustWriteFile(filepath.Join(repoPath, "ahead.txt"), "ahead\n")
		m.MustRunGit(now, repoPath, "add", "ahead.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "ahead commit")

		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected list mode exit 1 for unsyncable state, output=%s", out)
		}
		if !strings.Contains(out, "actions:") || !strings.Contains(out, "push") {
			t.Fatalf("expected push action in output, got: %s", out)
		}

		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo", "push"); err != nil {
			t.Fatalf("fix push failed: %v\n%s", err, out)
		}

		if out := m.MustRunGit(now, repoPath, "status", "--porcelain", "--branch"); strings.Contains(out, "ahead ") {
			t.Fatalf("expected no ahead commits after fix push, status=%s", out)
		}
	})

	t.Run("stage commit push with auto message", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustWriteFile(filepath.Join(repoPath, "dirty.txt"), "dirty\n")
		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo", "stage-commit-push", "--message=auto")
		if err != nil {
			t.Fatalf("stage-commit-push failed: %v\n%s", err, out)
		}

		msg := strings.TrimSpace(m.MustRunGit(now, repoPath, "log", "-1", "--pretty=%s"))
		if msg != "bb: checkpoint local changes before sync" {
			t.Fatalf("commit message = %q, want %q", msg, "bb: checkpoint local changes before sync")
		}
		if out := m.MustRunGit(now, repoPath, "status", "--porcelain", "--branch"); strings.Contains(out, "ahead ") {
			t.Fatalf("expected no ahead commits after stage-commit-push, status=%s", out)
		}
	})

	t.Run("set upstream push", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustRunGit(now, repoPath, "checkout", "-b", "feature/local-only")
		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected list mode exit 1 when upstream missing, output=%s", out)
		}
		if !strings.Contains(out, "set-upstream-push") {
			t.Fatalf("expected set-upstream-push action in output, got: %s", out)
		}

		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo", "set-upstream-push"); err != nil {
			t.Fatalf("set-upstream-push failed: %v\n%s", err, out)
		}

		upstream := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"))
		if upstream != "origin/feature/local-only" {
			t.Fatalf("upstream = %q, want origin/feature/local-only", upstream)
		}
	})

	t.Run("set upstream push works with non-origin remote", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)
		m.MustRunGit(now, repoPath, "remote", "rename", "origin", "upstream")

		m.MustRunGit(now, repoPath, "checkout", "-b", "feature/upstream-only")
		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo", "set-upstream-push")
		if err != nil {
			t.Fatalf("set-upstream-push failed: %v\n%s", err, out)
		}

		upstream := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"))
		if upstream != "upstream/feature/upstream-only" {
			t.Fatalf("upstream = %q, want upstream/feature/upstream-only", upstream)
		}
	})

	t.Run("preferred remote override is honored", func(t *testing.T) {
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)
		secondaryRemote := filepath.Join(h.RemotesRoot, "you", "demo-secondary.git")
		m.MustRunGit(now, catalogRoot, "init", "--bare", secondaryRemote)
		m.MustRunGit(now, repoPath, "remote", "add", "upstream", secondaryRemote)
		if out, err := m.RunBB(now.Add(30*time.Second), "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}

		if out, err := m.RunBB(now.Add(1*time.Minute), "repo", "remote", "demo", "--preferred-remote=upstream"); err != nil {
			t.Fatalf("repo remote command failed: %v\n%s", err, out)
		}

		m.MustRunGit(now, repoPath, "checkout", "-b", "feature/preferred-remote")
		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo", "set-upstream-push"); err != nil {
			t.Fatalf("set-upstream-push failed: %v\n%s", err, out)
		}

		upstream := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"))
		if upstream != "upstream/feature/preferred-remote" {
			t.Fatalf("upstream = %q, want upstream/feature/preferred-remote", upstream)
		}
	})

	t.Run("enable auto push action", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		_, _ = createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		if out, err := m.RunBB(now.Add(1*time.Minute), "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}

		out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo")
		if err != nil {
			t.Fatalf("expected list mode success for syncable repo, got err=%v output=%s", err, out)
		}
		if !strings.Contains(out, "enable-auto-push") {
			t.Fatalf("expected enable-auto-push action in output, got: %s", out)
		}

		if out, err := m.RunBB(now.Add(3*time.Minute), "fix", "demo", "enable-auto-push"); err != nil {
			t.Fatalf("enable-auto-push failed: %v\n%s", err, out)
		}

		meta := m.MustReadFile(firstRepoMetadataPath(t, m))
		if !strings.Contains(meta, "auto_push: true") {
			t.Fatalf("expected auto_push=true in repo metadata, got:\n%s", meta)
		}
	})

	t.Run("project selector accepts repo_id and path", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		if out, err := m.RunBB(now.Add(1*time.Minute), "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, m), "demo")

		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", rec.RepoID); err != nil {
			t.Fatalf("bb fix by repo_id failed: %v\n%s", err, out)
		}
		if out, err := m.RunBB(now.Add(3*time.Minute), "fix", repoPath); err != nil {
			t.Fatalf("bb fix by path failed: %v\n%s", err, out)
		}
	})
}
