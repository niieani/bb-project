package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/testharness"
)

func TestScanCases(t *testing.T) {
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	t.Run("TC-SCAN-001", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		createRepoWithOrigin(t, m, catalogRoot, "demo", now)

		if out, err := m.RunBB(now, "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}

		mf := loadMachineFile(t, m)
		rec := findRepoRecordByName(t, mf, "demo")
		if rec.RepoID == "" {
			t.Fatal("expected discovered repo to have repo_id")
		}
	})

	t.Run("TC-SCAN-002", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		if err := os.MkdirAll(filepath.Join(catalogRoot, "not-a-repo"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if out, err := m.RunBB(now, "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}

		mf := loadMachineFile(t, m)
		if len(mf.Repos) != 0 {
			t.Fatalf("expected no discovered repos, got %d", len(mf.Repos))
		}
	})

	t.Run("TC-SCAN-003", func(t *testing.T) {
		h := testHarnessTwoCatalogs(t)
		now2 := now.Add(10 * time.Minute)
		createRepoWithOrigin(t, h.m, h.softwareRoot, "software-repo", now2)
		createRepoWithOrigin(t, h.m, h.referencesRoot, "references-repo", now2)

		if out, err := h.m.RunBB(now2, "scan", "--include-catalog", "references"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}

		mf := loadMachineFile(t, h.m)
		if len(mf.Repos) != 1 {
			t.Fatalf("expected one repo, got %d", len(mf.Repos))
		}
		if !strings.Contains(mf.Repos[0].Path, "references-repo") {
			t.Fatalf("expected only references repo, got path %q", mf.Repos[0].Path)
		}
	})

	t.Run("TC-SCAN-004", func(t *testing.T) {
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "demo", now)
		worktreePath := filepath.Join(catalogRoot, "demo-worktree")
		m.MustRunGit(now, repoPath, "worktree", "add", "-b", "worktree-branch", worktreePath)
		m.MustRunGit(now, worktreePath, "branch", "--set-upstream-to", "origin/main", "worktree-branch")

		if out, err := m.RunBB(now.Add(1*time.Minute), "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}

		mf := loadMachineFile(t, m)
		found := false
		for _, rec := range mf.Repos {
			if rec.Path == worktreePath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected worktree repo at %q to be discovered, got repos=%+v", worktreePath, mf.Repos)
		}
	})
}

type twoCatalogHarness struct {
	m              *testharness.Machine
	softwareRoot   string
	referencesRoot string
}

func testHarnessTwoCatalogs(t *testing.T) twoCatalogHarness {
	t.Helper()
	h := testharness.NewHarness(t)
	softwareRoot := filepath.Join(h.Root, "machines", "a", "catalogs", "software")
	referencesRoot := filepath.Join(h.Root, "machines", "a", "catalogs", "references")
	m := h.AddMachine("machine-a", map[string]string{
		"software":   softwareRoot,
		"references": referencesRoot,
	}, "software")
	return twoCatalogHarness{m: m, softwareRoot: softwareRoot, referencesRoot: referencesRoot}
}
