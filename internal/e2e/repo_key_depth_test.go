package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepoKeyDepthCases(t *testing.T) {
	t.Parallel()

	t.Run("TC-DEPTH-001 depth2 converges owner/repo path", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, rootA, rootB := setupTwoMachines(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		setCatalogRepoPathDepth(t, mA, "software", 2)
		setCatalogRepoPathDepth(t, mB, "software", 2)

		if err := os.MkdirAll(filepath.Join(rootA, "openai"), 0o755); err != nil {
			t.Fatalf("mkdir nested root on A: %v", err)
		}
		repoA, remote := createRepoWithOrigin(t, mA, rootA, "openai/codex", now)
		repoB := filepath.Join(rootB, "openai", "codex")
		if err := os.MkdirAll(filepath.Dir(repoB), 0o755); err != nil {
			t.Fatalf("mkdir nested root on B: %v", err)
		}
		mB.MustRunGit(now, "", "clone", remote, repoB)
		mB.MustRunGit(now, repoB, "checkout", "-B", "main", "--track", "origin/main")

		if out, err := mA.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		mA.MustRunGit(now, repoA, "checkout", "-b", "feature/depth")
		mA.MustWriteFile(filepath.Join(repoA, "depth.txt"), "depth\n")
		mA.MustRunGit(now, repoA, "add", "depth.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "depth branch")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/depth")
		if out, err := mA.RunBB(now.Add(3*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(4*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		if got := gitCurrentBranch(t, mB, repoB, now); got != "feature/depth" {
			t.Fatalf("expected depth2 repo to converge, got %q", got)
		}
	})

	t.Run("TC-DEPTH-002 depth mismatch ignored by discovery", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		if err := os.MkdirAll(filepath.Join(catalogRoot, "owner"), 0o755); err != nil {
			t.Fatalf("mkdir nested root: %v", err)
		}
		_, _ = createRepoWithOrigin(t, m, catalogRoot, "owner/repo", now)

		if out, err := m.RunBB(now.Add(1*time.Minute), "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}
		mf := loadMachineFile(t, m)
		if len(mf.Repos) != 0 {
			t.Fatalf("expected nested depth-2 repo to be ignored for depth-1 catalog, got %d records", len(mf.Repos))
		}
	})

	t.Run("TC-DEPTH-003 init validates project depth", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		setCatalogRepoPathDepth(t, m, "software", 2)

		out, err := m.RunBB(now.Add(1*time.Minute), "init", "demo")
		if err == nil {
			t.Fatalf("expected init to fail for invalid depth, output=%s", out)
		}
		if !strings.Contains(out, "project path must match catalog layout depth 2") {
			t.Fatalf("unexpected error output: %s", out)
		}

		if out, err := m.RunBB(now.Add(2*time.Minute), "init", "team/demo"); err != nil {
			t.Fatalf("expected init to succeed for depth-2 project: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(catalogRoot, "team", "demo", ".git")); err != nil {
			t.Fatalf("expected git repo at depth-2 init path: %v", err)
		}
	})
}
