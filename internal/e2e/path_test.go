package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/testharness"
)

func setupSourceRepoForClone(t *testing.T) (*testharness.Harness, *testharness.Machine, *testharness.Machine, string, string, time.Time) {
	t.Helper()
	h, mA, mB, rootA, rootB := setupTwoMachines(t)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	if out, err := mA.RunBB(now, "init", "api"); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	repoA := filepath.Join(rootA, "api")
	mA.MustWriteFile(filepath.Join(repoA, "README.md"), "hello\n")
	mA.MustRunGit(now, repoA, "add", "README.md")
	mA.MustRunGit(now, repoA, "commit", "-m", "bootstrap")
	mA.MustRunGit(now, repoA, "push", "-u", "origin", "main")
	if out, err := mA.RunBB(now.Add(30*time.Second), "sync"); err != nil {
		t.Fatalf("sync on A failed: %v\n%s", err, out)
	}
	h.ExternalSync("a-machine", "b-machine")
	return h, mA, mB, filepath.Join(rootA, "api"), filepath.Join(rootB, "api"), now
}

func TestPathCases(t *testing.T) {
	t.Parallel()
	t.Run("TC-PATH-001", func(t *testing.T) {
		t.Parallel()
		_, _, mB, _, targetPath, now := setupSourceRepoForClone(t)
		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		if out, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(targetPath, ".git")); err != nil {
			t.Fatalf("expected clone into empty target, stat .git: %v", err)
		}
	})

	t.Run("TC-PATH-002", func(t *testing.T) {
		t.Parallel()
		_, _, mB, _, targetPath, now := setupSourceRepoForClone(t)
		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		marker := filepath.Join(targetPath, "note.txt")
		if err := os.WriteFile(marker, []byte("keep\n"), 0o644); err != nil {
			t.Fatalf("write marker: %v", err)
		}
		if _, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err == nil {
			t.Fatal("expected path conflict failure")
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonTargetPathNonRepo) {
			t.Fatalf("expected target_path_nonempty_not_repo, got %v", rec.UnsyncableReasons)
		}
	})

	t.Run("TC-PATH-003", func(t *testing.T) {
		t.Parallel()
		_, _, mB, _, targetPath, now := setupSourceRepoForClone(t)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatalf("mkdir parent: %v", err)
		}
		mB.MustRunGit(now, filepath.Dir(targetPath), "init", "-b", "main", targetPath)
		mB.MustRunGit(now, targetPath, "remote", "add", "origin", "/tmp/other.git")

		if _, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err == nil {
			t.Fatal("expected repo mismatch failure")
		}
		mf := loadMachineFile(t, mB)
		found := false
		for _, rec := range mf.Repos {
			if containsReason(rec.UnsyncableReasons, domain.ReasonTargetPathRepoMismatch) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected target_path_repo_mismatch, got repos=%+v", mf.Repos)
		}
	})

	t.Run("TC-PATH-004", func(t *testing.T) {
		t.Parallel()
		_, mA, mB, repoA, targetPath, now := setupSourceRepoForClone(t)
		origin := stringTrim(mA.MustRunGit(now, repoA, "remote", "get-url", "origin"))
		mB.MustRunGit(now, "", "clone", origin, targetPath)
		mB.MustRunGit(now, targetPath, "checkout", "-B", "main", "--track", "origin/main")

		if out, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			mf := loadMachineFile(t, mB)
			t.Fatalf("sync failed: %v\n%s\nrepos=%+v", err, out, mf.Repos)
		}
		if _, err := os.Stat(filepath.Join(targetPath, ".git")); err != nil {
			t.Fatalf("expected adopted repo to remain valid: %v", err)
		}
	})

	t.Run("TC-PATH-005", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		_, remote := createRepoWithOrigin(t, m, catalogRoot, "one", now)
		repo2 := filepath.Join(catalogRoot, "two")
		m.MustRunGit(now, catalogRoot, "clone", remote, repo2)
		m.MustRunGit(now, repo2, "checkout", "-B", "main", "--track", "origin/main")

		if out, err := m.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("expected sync to succeed for duplicate origin with different repo paths: %v\n%s", err, out)
		}
		mf := loadMachineFile(t, m)
		repoByPath := map[string]domain.MachineRepoRecord{}
		for _, rec := range mf.Repos {
			repoByPath[rec.Path] = rec
		}
		onePath := filepath.Join(catalogRoot, "one")
		if _, ok := repoByPath[onePath]; !ok {
			t.Fatalf("missing repo record for %s", onePath)
		}
		if _, ok := repoByPath[repo2]; !ok {
			t.Fatalf("missing repo record for %s", repo2)
		}
		if !repoByPath[onePath].Syncable || !repoByPath[repo2].Syncable {
			t.Fatalf("expected both repos to remain syncable, got one=%t two=%t", repoByPath[onePath].Syncable, repoByPath[repo2].Syncable)
		}
	})

	t.Run("TC-PATH-006", func(t *testing.T) {
		t.Parallel()
		_, _, mB, _, targetPath, now := setupSourceRepoForClone(t)
		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		marker := filepath.Join(targetPath, "note.txt")
		if err := os.WriteFile(marker, []byte("keep\n"), 0o644); err != nil {
			t.Fatalf("write marker: %v", err)
		}
		_, _ = mB.RunBB(now.Add(1*time.Minute), "sync")
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("expected existing files to remain untouched: %v", err)
		}
		if _, err := os.Stat(filepath.Join(targetPath, ".git")); err == nil {
			t.Fatal("expected no clone on path conflict")
		}
	})
}
