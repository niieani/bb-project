package e2e

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFixCases(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	t.Run("requires interactive terminal for table mode", func(t *testing.T) {
		t.Parallel()
		_, m, _ := setupSingleMachine(t)
		out, err := m.RunBB(now, "fix")
		if err == nil {
			t.Fatalf("expected bb fix to fail in non-interactive harness, output=%s", out)
		}
		if !strings.Contains(out, "requires an interactive terminal") {
			t.Fatalf("expected interactive terminal error, got: %s", out)
		}
	})

	t.Run("reuses fresh scan snapshot before list mode", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		_, _ = createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		if out, err := m.RunBB(now, "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}

		out, err := m.RunBB(now.Add(30*time.Second), "fix", "demo")
		if err != nil {
			t.Fatalf("fix list mode failed: %v\n%s", err, out)
		}
		if !strings.Contains(out, "snapshot is fresh") {
			t.Fatalf("expected freshness-skip log in output, got: %s", out)
		}
		if strings.Contains(out, "scan: discovered") {
			t.Fatalf("did not expect a rescan within freshness window, got: %s", out)
		}

		out, err = m.RunBB(now.Add(2*time.Minute), "fix", "demo")
		if err != nil {
			t.Fatalf("fix list mode after freshness window failed: %v\n%s", err, out)
		}
		if !strings.Contains(out, "scan: discovered") {
			t.Fatalf("expected refresh scan after freshness window, got: %s", out)
		}
	})

	t.Run("lists and applies push", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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

	t.Run("stage commit push validates push feasibility before mutating git state", func(t *testing.T) {
		t.Parallel()
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, remotePath := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		remoteClone := filepath.Join(h.Root, "remote-clones", "demo-behind")
		m.MustRunGit(now, catalogRoot, "clone", remotePath, remoteClone)
		m.MustRunGit(now, remoteClone, "checkout", "-B", "main", "--track", "origin/main")
		m.MustWriteFile(filepath.Join(remoteClone, "remote-ahead.txt"), "remote ahead\n")
		m.MustRunGit(now, remoteClone, "add", "remote-ahead.txt")
		m.MustRunGit(now, remoteClone, "commit", "-m", "remote ahead commit")
		m.MustRunGit(now, remoteClone, "push", "origin", "main")

		m.MustRunGit(now, repoPath, "fetch", "origin")
		beforeHead := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "HEAD"))

		m.MustWriteFile(filepath.Join(repoPath, "dirty.txt"), "local dirty\n")
		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo", "stage-commit-push", "--message=auto")
		if err == nil {
			t.Fatalf("expected stage-commit-push to be rejected when branch is behind upstream, output=%s", out)
		}
		if !strings.Contains(out, "behind upstream") {
			t.Fatalf("expected behind-upstream validation guidance, got: %s", out)
		}

		afterHead := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "HEAD"))
		if afterHead != beforeHead {
			t.Fatalf("HEAD changed despite pre-validation failure: before=%s after=%s", beforeHead, afterHead)
		}
		status := m.MustRunGit(now, repoPath, "status", "--porcelain")
		if !strings.Contains(status, "dirty.txt") {
			t.Fatalf("expected uncommitted file to remain unstaged after validation failure, status=%s", status)
		}
	})

	t.Run("sync with upstream action resolves clean diverged branches", func(t *testing.T) {
		t.Parallel()
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, remotePath := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		remoteClone := filepath.Join(h.Root, "remote-clones", "demo-diverged")
		m.MustRunGit(now, catalogRoot, "clone", remotePath, remoteClone)
		m.MustRunGit(now, remoteClone, "checkout", "-B", "main", "--track", "origin/main")
		m.MustWriteFile(filepath.Join(remoteClone, "remote.txt"), "remote commit\n")
		m.MustRunGit(now, remoteClone, "add", "remote.txt")
		m.MustRunGit(now, remoteClone, "commit", "-m", "remote commit")
		m.MustRunGit(now, remoteClone, "push", "origin", "main")

		m.MustWriteFile(filepath.Join(repoPath, "local.txt"), "local commit\n")
		m.MustRunGit(now, repoPath, "add", "local.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "local commit")
		m.MustRunGit(now, repoPath, "fetch", "origin")

		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected list mode exit 1 for diverged state, output=%s", out)
		}
		if !strings.Contains(out, "sync-with-upstream") {
			t.Fatalf("expected sync-with-upstream action in output, got: %s", out)
		}

		out, err = m.RunBB(now.Add(2*time.Minute), "fix", "demo", "sync-with-upstream")
		if !strings.Contains(out, "applied sync-with-upstream") {
			t.Fatalf("expected sync action apply confirmation, got err=%v output=%s", err, out)
		}

		counts := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-list", "--left-right", "--count", "@{u}...HEAD"))
		parts := strings.Fields(counts)
		if len(parts) != 2 {
			t.Fatalf("unexpected ahead/behind output: %q", counts)
		}
		if parts[0] != "0" {
			t.Fatalf("expected behind count to be 0 after sync-with-upstream, counts=%q", counts)
		}
	})

	t.Run("sync feasibility probe ignores local hooks and does not report false conflict", func(t *testing.T) {
		t.Parallel()
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, remotePath := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		remoteClone := filepath.Join(h.Root, "remote-clones", "demo-hooks")
		m.MustRunGit(now, catalogRoot, "clone", remotePath, remoteClone)
		m.MustRunGit(now, remoteClone, "checkout", "-B", "main", "--track", "origin/main")
		m.MustWriteFile(filepath.Join(remoteClone, "remote-hooks.txt"), "remote commit\n")
		m.MustRunGit(now, remoteClone, "add", "remote-hooks.txt")
		m.MustRunGit(now, remoteClone, "commit", "-m", "remote commit")
		m.MustRunGit(now, remoteClone, "push", "origin", "main")

		m.MustWriteFile(filepath.Join(repoPath, "local-hooks.txt"), "local commit\n")
		m.MustRunGit(now, repoPath, "add", "local-hooks.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "local commit")
		m.MustRunGit(now, repoPath, "fetch", "origin")

		preRebaseHook := filepath.Join(repoPath, ".git", "hooks", "pre-rebase")
		preMergeHook := filepath.Join(repoPath, ".git", "hooks", "pre-merge-commit")
		m.MustWriteFile(preRebaseHook, "#!/bin/sh\nexit 42\n")
		m.MustWriteFile(preMergeHook, "#!/bin/sh\nexit 43\n")
		if err := os.Chmod(preRebaseHook, 0o755); err != nil {
			t.Fatalf("chmod pre-rebase hook: %v", err)
		}
		if err := os.Chmod(preMergeHook, 0o755); err != nil {
			t.Fatalf("chmod pre-merge-commit hook: %v", err)
		}

		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected list mode exit 1 for diverged state, output=%s", out)
		}
		if !strings.Contains(out, "sync-with-upstream") {
			t.Fatalf("expected sync-with-upstream action in output, got: %s", out)
		}
		if strings.Contains(out, "sync_conflict_requires_manual_resolution") {
			t.Fatalf("did not expect false sync conflict reason from hook scripts, got: %s", out)
		}
		if strings.Contains(out, "sync feasibility") && strings.Contains(out, "probe failed") {
			t.Fatalf("did not expect noisy probe-failed log output for hook scripts, got: %s", out)
		}
	})

	t.Run("sync conflict reason is surfaced when neither rebase nor merge can be applied cleanly", func(t *testing.T) {
		t.Parallel()
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, remotePath := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustWriteFile(filepath.Join(repoPath, "conflict.txt"), "base\n")
		m.MustRunGit(now, repoPath, "add", "conflict.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "add baseline conflict file")
		m.MustRunGit(now, repoPath, "push", "origin", "main")

		remoteClone := filepath.Join(h.Root, "remote-clones", "demo-conflict")
		m.MustRunGit(now, catalogRoot, "clone", remotePath, remoteClone)
		m.MustRunGit(now, remoteClone, "checkout", "-B", "main", "--track", "origin/main")
		m.MustWriteFile(filepath.Join(remoteClone, "conflict.txt"), "remote change\n")
		m.MustRunGit(now, remoteClone, "add", "conflict.txt")
		m.MustRunGit(now, remoteClone, "commit", "-m", "remote conflict change")
		m.MustRunGit(now, remoteClone, "push", "origin", "main")

		m.MustWriteFile(filepath.Join(repoPath, "conflict.txt"), "local change\n")
		m.MustRunGit(now, repoPath, "add", "conflict.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "local conflict change")
		m.MustRunGit(now, repoPath, "fetch", "origin")

		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected list mode exit 1 for unsyncable diverged conflict state, output=%s", out)
		}
		if !strings.Contains(out, "sync_conflict_requires_manual_resolution") {
			t.Fatalf("expected sync conflict validation reason in output, got: %s", out)
		}
		if strings.Contains(out, "sync-with-upstream") {
			t.Fatalf("did not expect sync-with-upstream action when conflict is predicted, got: %s", out)
		}
	})

	t.Run("stage commit push blocked when uncommitted .env exists", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustWriteFile(filepath.Join(repoPath, ".env"), "TOKEN=secret\n")
		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected fix list mode to report unsyncable state, output=%s", out)
		}
		if strings.Contains(out, "stage-commit-push") {
			t.Fatalf("did not expect stage-commit-push action while .env is uncommitted, got: %s", out)
		}

		out, err = m.RunBB(now.Add(2*time.Minute), "fix", "demo", "stage-commit-push")
		if err == nil {
			t.Fatalf("expected stage-commit-push to be blocked when .env is uncommitted, output=%s", out)
		}
		if !strings.Contains(out, "secret-like") {
			t.Fatalf("expected secret-like block reason, got: %s", out)
		}
	})

	t.Run("non-interactive stage commit push blocked for node_modules when root gitignore missing", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustWriteFile(filepath.Join(repoPath, "node_modules", "left-pad", "index.js"), "module.exports = 1\n")
		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected fix list mode to report unsyncable state, output=%s", out)
		}
		if strings.Contains(out, "stage-commit-push") {
			t.Fatalf("did not expect stage-commit-push when .gitignore is missing and node_modules is dirty, got: %s", out)
		}

		out, err = m.RunBB(now.Add(2*time.Minute), "fix", "demo", "stage-commit-push")
		if err == nil {
			t.Fatalf("expected stage-commit-push to be blocked for noisy paths without .gitignore, output=%s", out)
		}
		if !strings.Contains(out, ".gitignore") {
			t.Fatalf("expected .gitignore guidance in output, got: %s", out)
		}
	})

	t.Run("set upstream push", func(t *testing.T) {
		t.Parallel()
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
		if strings.Contains(out, "create-project") {
			t.Fatalf("did not expect create-project when origin exists, got: %s", out)
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
		t.Parallel()
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
		t.Parallel()
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)
		secondaryRemote := filepath.Join(h.RemotesRoot, "you", "demo-secondary.git")
		m.MustRunGit(now, catalogRoot, "init", "--bare", secondaryRemote)
		m.MustRunGit(now, repoPath, "remote", "add", "upstream", secondaryRemote)
		if out, err := m.RunBB(now.Add(30*time.Second), "scan"); err != nil && out == "" {
			t.Fatalf("scan failed without output: %v", err)
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
		t.Parallel()
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
		if !strings.Contains(meta, "auto_push: include-default-branch") {
			t.Fatalf("expected auto_push=include-default-branch in repo metadata, got:\n%s", meta)
		}
	})

	t.Run("fork and retarget action for read-only remote", func(t *testing.T) {
		t.Parallel()
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustWriteFile(filepath.Join(repoPath, "ahead.txt"), "ahead\n")
		m.MustRunGit(now, repoPath, "add", "ahead.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "ahead")

		if out, err := m.RunBB(now.Add(30*time.Second), "scan"); err != nil && out == "" {
			t.Fatalf("scan failed without output: %v", err)
		}

		if out, err := m.RunBB(now.Add(1*time.Minute), "repo", "access-set", "demo", "--push-access=read_only"); err != nil {
			t.Fatalf("repo access-set failed: %v\n%s", err, out)
		}

		out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected list mode exit 1 for unsyncable state, output=%s", out)
		}
		if !strings.Contains(out, "fork-and-retarget") {
			t.Fatalf("expected fork-and-retarget action in output, got: %s", out)
		}
		if strings.Contains(out, "actions: push") {
			t.Fatalf("did not expect push action while remote is read-only, got: %s", out)
		}

		if out, err := m.RunBB(now.Add(3*time.Minute), "fix", "demo", "fork-and-retarget"); err != nil {
			t.Fatalf("fork-and-retarget failed: %v\n%s", err, out)
		}

		upstream := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"))
		if upstream != "you/main" {
			t.Fatalf("upstream = %q, want you/main", upstream)
		}
		remoteURL := strings.TrimSpace(m.MustRunGit(now, repoPath, "remote", "get-url", "you"))
		expectedRemote := filepath.Join(h.RemotesRoot, "you", "demo.git")
		if remoteURL != expectedRemote {
			t.Fatalf("fork remote url = %q, want %q", remoteURL, expectedRemote)
		}
	})

	t.Run("fork and retarget force-pushes when fork has advanced remotely", func(t *testing.T) {
		t.Parallel()
		h, m, catalogRoot := setupSingleMachine(t)
		repoPath, remotePath := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		remoteClone := filepath.Join(h.Root, "remote-clones", "demo-fork-stale")
		m.MustRunGit(now, catalogRoot, "clone", remotePath, remoteClone)
		m.MustRunGit(now, remoteClone, "checkout", "-B", "main", "--track", "origin/main")
		m.MustWriteFile(filepath.Join(remoteClone, "remote-only.txt"), "remote only\n")
		m.MustRunGit(now, remoteClone, "add", "remote-only.txt")
		m.MustRunGit(now, remoteClone, "commit", "-m", "remote only commit")
		m.MustRunGit(now, remoteClone, "push", "origin", "main")

		m.MustWriteFile(filepath.Join(repoPath, "local-only.txt"), "local only\n")
		m.MustRunGit(now, repoPath, "add", "local-only.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "local only commit")
		localHead := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "HEAD"))

		if out, err := m.RunBB(now.Add(30*time.Second), "scan"); err != nil && out == "" {
			t.Fatalf("scan failed without output: %v", err)
		}
		if out, err := m.RunBB(now.Add(1*time.Minute), "repo", "access-set", "demo", "--push-access=read_only"); err != nil {
			t.Fatalf("repo access-set failed: %v\n%s", err, out)
		}

		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo", "fork-and-retarget"); err != nil {
			t.Fatalf("fork-and-retarget failed: %v\n%s", err, out)
		}

		remoteHead := strings.TrimSpace(m.MustRunGit(now, catalogRoot, "--git-dir", remotePath, "rev-parse", "refs/heads/main"))
		if remoteHead != localHead {
			t.Fatalf("remote HEAD = %q, want %q after force push", remoteHead, localHead)
		}
	})

	t.Run("fork and retarget writes metadata before push so failure does not loop action", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, remotePath := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		m.MustWriteFile(filepath.Join(repoPath, "ahead.txt"), "ahead\n")
		m.MustRunGit(now, repoPath, "add", "ahead.txt")
		m.MustRunGit(now, repoPath, "commit", "-m", "ahead")

		if out, err := m.RunBB(now.Add(30*time.Second), "scan"); err != nil && out == "" {
			t.Fatalf("scan failed without output: %v", err)
		}
		if out, err := m.RunBB(now.Add(1*time.Minute), "repo", "access-set", "demo", "--push-access=read_only"); err != nil {
			t.Fatalf("repo access-set failed: %v\n%s", err, out)
		}

		if err := filepath.WalkDir(remotePath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return os.Chmod(path, 0o555)
			}
			return os.Chmod(path, 0o444)
		}); err != nil {
			t.Fatalf("chmod remote read-only: %v", err)
		}
		defer func() {
			_ = filepath.WalkDir(remotePath, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if d.IsDir() {
					_ = os.Chmod(path, 0o755)
					return nil
				}
				_ = os.Chmod(path, 0o644)
				return nil
			})
		}()

		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo", "fork-and-retarget"); err == nil {
			t.Fatalf("expected fork-and-retarget push failure, output=%s", out)
		}

		meta := m.MustReadFile(firstRepoMetadataPath(t, m))
		if !strings.Contains(meta, "preferred_remote: you") {
			t.Fatalf("expected preferred_remote=you after failed push, got:\n%s", meta)
		}
		if !strings.Contains(meta, "push_access: unknown") {
			t.Fatalf("expected push_access=unknown after failed push, got:\n%s", meta)
		}

		out, err := m.RunBB(now.Add(3*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected fix list mode to remain unsyncable, output=%s", out)
		}
		if strings.Contains(out, "fork-and-retarget") {
			t.Fatalf("did not expect fork-and-retarget action after metadata retarget, got: %s", out)
		}
	})

	t.Run("create project from missing origin", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath := filepath.Join(catalogRoot, "demo")
		m.MustRunGit(now, catalogRoot, "init", "-b", "main", repoPath)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "hello\n")
		m.MustRunGit(now, repoPath, "add", "README.md")
		m.MustRunGit(now, repoPath, "commit", "-m", "init local")

		out, err := m.RunBB(now.Add(1*time.Minute), "fix", "demo")
		if err == nil {
			t.Fatalf("expected unsyncable list mode before create-project, output=%s", out)
		}
		if !strings.Contains(out, "create-project") {
			t.Fatalf("expected create-project action in output, got: %s", out)
		}

		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", "demo", "create-project"); err != nil {
			t.Fatalf("create-project failed: %v\n%s", err, out)
		}

		origin := strings.TrimSpace(m.MustRunGit(now, repoPath, "remote", "get-url", "origin"))
		if origin == "" {
			t.Fatal("expected origin to be configured by create-project")
		}
		upstream := strings.TrimSpace(m.MustRunGit(now, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"))
		if upstream != "origin/main" {
			t.Fatalf("upstream = %q, want origin/main", upstream)
		}
	})

	t.Run("project selector accepts repo_key and path", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		if out, err := m.RunBB(now.Add(1*time.Minute), "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, m), "demo")

		if out, err := m.RunBB(now.Add(2*time.Minute), "fix", rec.RepoKey); err != nil {
			t.Fatalf("bb fix by repo_key failed: %v\n%s", err, out)
		}
		if out, err := m.RunBB(now.Add(3*time.Minute), "fix", repoPath); err != nil {
			t.Fatalf("bb fix by path failed: %v\n%s", err, out)
		}
	})
}
