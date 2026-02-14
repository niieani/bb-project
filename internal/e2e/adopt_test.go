package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
)

func TestAdoptCases(t *testing.T) {
	t.Parallel()
	t.Run("TC-ADOPT-001", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, rootA, rootB := setupTwoMachines(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		repoA, remote := createRepoWithOrigin(t, mA, rootA, "api", now)
		repoB := filepath.Join(rootB, "api")
		mB.MustRunGit(now, "", "clone", remote, repoB)
		mB.MustRunGit(now, repoB, "checkout", "-B", "main", "--track", "origin/main")

		if out, err := mA.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		mA.MustRunGit(now, repoA, "checkout", "-b", "feature/adopt")
		mA.MustWriteFile(filepath.Join(repoA, "adopt.txt"), "adopt\n")
		mA.MustRunGit(now, repoA, "add", "adopt.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "adopt branch")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/adopt")
		if out, err := mA.RunBB(now.Add(3*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(4*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		if got := gitCurrentBranch(t, mB, repoB, now); got != "feature/adopt" {
			t.Fatalf("expected adopted repo to converge on feature/adopt, got %q", got)
		}
	})

	t.Run("TC-ADOPT-002", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, rootA, rootB := setupTwoMachines(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		repoA, remote := createRepoWithOrigin(t, mA, rootA, "api", now)
		repoB := filepath.Join(rootB, "custom", "api-local")
		if err := os.MkdirAll(filepath.Dir(repoB), 0o755); err != nil {
			t.Fatalf("mkdir custom dir: %v", err)
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

		mA.MustRunGit(now, repoA, "checkout", "-b", "feature/adopt-2")
		mA.MustWriteFile(filepath.Join(repoA, "adopt2.txt"), "adopt2\n")
		mA.MustRunGit(now, repoA, "add", "adopt2.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "adopt branch 2")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/adopt-2")
		if out, err := mA.RunBB(now.Add(3*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(4*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		if got := gitCurrentBranch(t, mB, repoB, now); got != "feature/adopt-2" {
			t.Fatalf("expected custom path repo to converge, got %q", got)
		}
		if _, err := os.Stat(filepath.Join(rootB, "api")); !os.IsNotExist(err) {
			t.Fatalf("expected no additional clone at default path, stat err=%v", err)
		}
	})

	t.Run("TC-ADOPT-003", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, rootA, rootB := setupTwoMachines(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		_, _ = createRepoWithOrigin(t, mA, rootA, "api", now)
		if out, err := mA.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")

		targetPath := filepath.Join(rootB, "api")
		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		marker := filepath.Join(targetPath, "keep.txt")
		if err := os.WriteFile(marker, []byte("keep\n"), 0o644); err != nil {
			t.Fatalf("write marker: %v", err)
		}

		if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
			t.Fatal("expected path conflict unsyncable")
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonTargetPathNonRepo) {
			t.Fatalf("expected target_path_nonempty_not_repo, got %v", rec.UnsyncableReasons)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("expected existing marker to remain: %v", err)
		}
	})
}
