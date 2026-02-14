package e2e

import (
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/testharness"
)

func bootstrapRepoAcrossTwoMachines(t *testing.T) (*testharness.Harness, *testharness.Machine, *testharness.Machine, string, string, time.Time) {
	t.Helper()
	h, mA, mB, rootA, rootB := setupTwoMachines(t)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	if out, err := mA.RunBB(now, "init", "api"); err != nil {
		t.Fatalf("init on A failed: %v\n%s", err, out)
	}
	repoA := filepath.Join(rootA, "api")
	mA.MustWriteFile(filepath.Join(repoA, "README.md"), "hello\n")
	mA.MustRunGit(now, repoA, "add", "README.md")
	mA.MustRunGit(now, repoA, "commit", "-m", "bootstrap")
	mA.MustRunGit(now, repoA, "push", "-u", "origin", "main")
	if out, err := mA.RunBB(now.Add(30*time.Second), "sync"); err != nil {
		t.Fatalf("sync on A after bootstrap commit failed: %v\n%s", err, out)
	}
	h.ExternalSync("a-machine", "b-machine")
	if out, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
		t.Fatalf("sync on B failed: %v\n%s", err, out)
	}
	return h, mA, mB, filepath.Join(rootA, "api"), filepath.Join(rootB, "api"), now
}

func TestSyncBasicCases(t *testing.T) {
	t.Parallel()
	t.Run("TC-SYNC-001", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mA.MustRunGit(now, repoA, "checkout", "-b", "feature/x")
		mA.MustWriteFile(filepath.Join(repoA, "feature.txt"), "x\n")
		mA.MustRunGit(now, repoA, "add", "feature.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "feature x")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/x")
		if out, err := mA.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}

		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(3*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		if got := gitCurrentBranch(t, mB, repoB, now); got != "feature/x" {
			t.Fatalf("branch on B = %q, want feature/x", got)
		}
	})

	t.Run("TC-SYNC-002", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mA.MustRunGit(now, repoA, "checkout", "-b", "feature/y")
		mA.MustWriteFile(filepath.Join(repoA, "feature.txt"), "y\n")
		mA.MustRunGit(now, repoA, "add", "feature.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "feature y")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/y")
		if out, err := mA.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}

		mB.MustWriteFile(filepath.Join(repoB, "README.md"), "dirty tracked change\n")
		h.ExternalSync("a-machine", "b-machine")
		if _, err := mB.RunBB(now.Add(3*time.Minute), "sync"); err == nil {
			t.Fatal("expected sync to return unsyncable")
		}

		if got := gitCurrentBranch(t, mB, repoB, now); got != "main" {
			t.Fatalf("branch on B changed unexpectedly: %q", got)
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonDirtyTracked) {
			t.Fatalf("expected dirty_tracked reason, got %v", rec.UnsyncableReasons)
		}
	})

	t.Run("TC-SYNC-003", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mA.MustRunGit(now, repoA, "checkout", "-b", "feature/z")
		mA.MustWriteFile(filepath.Join(repoA, "feature.txt"), "z\n")
		mA.MustRunGit(now, repoA, "add", "feature.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "feature z")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/z")
		if out, err := mA.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}

		mB.MustWriteFile(filepath.Join(repoB, "scratch.txt"), "untracked\n")
		h.ExternalSync("a-machine", "b-machine")
		if _, err := mB.RunBB(now.Add(3*time.Minute), "sync"); err == nil {
			t.Fatal("expected sync to return unsyncable")
		}

		if got := gitCurrentBranch(t, mB, repoB, now); got != "main" {
			t.Fatalf("branch on B changed unexpectedly: %q", got)
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonDirtyUntracked) {
			t.Fatalf("expected dirty_untracked reason, got %v", rec.UnsyncableReasons)
		}
	})

	t.Run("TC-SYNC-004", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mA.MustRunGit(now, repoA, "checkout", "-b", "feature/clean")
		mA.MustWriteFile(filepath.Join(repoA, "feature.txt"), "clean\n")
		mA.MustRunGit(now, repoA, "add", "feature.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "feature clean")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/clean")
		if out, err := mA.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}

		mB.MustWriteFile(filepath.Join(repoB, "README.md"), "dirty tracked change\n")
		h.ExternalSync("a-machine", "b-machine")
		_, _ = mB.RunBB(now.Add(3*time.Minute), "sync")

		mB.MustRunGit(now, repoB, "checkout", "--", "README.md")
		if out, err := mA.RunBB(now.Add(4*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A refresh failed: %v\n%s", err, out)
		}
		if got := findRepoRecordByName(t, loadMachineFile(t, mA), "api").Branch; got != "feature/clean" {
			t.Fatalf("expected A to remain on feature/clean before final sync, got %q", got)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(5*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B after cleanup failed: %v\n%s", err, out)
		}
		if got := gitCurrentBranch(t, mB, repoB, now); got != "feature/clean" {
			rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
			t.Fatalf("branch on B = %q, want feature/clean (reasons=%v syncable=%t)", got, rec.UnsyncableReasons, rec.Syncable)
		}
	})

	t.Run("TC-SYNC-005", func(t *testing.T) {
		t.Parallel()
		_, mA, _, _, _, now := bootstrapRepoAcrossTwoMachines(t)
		if out, err := mA.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("first sync failed: %v\n%s", err, out)
		}
		mf1 := loadMachineFile(t, mA)
		rec1 := findRepoRecordByName(t, mf1, "api")

		if out, err := mA.RunBB(now.Add(10*time.Minute), "sync"); err != nil {
			t.Fatalf("second sync failed: %v\n%s", err, out)
		}
		mf2 := loadMachineFile(t, mA)
		rec2 := findRepoRecordByName(t, mf2, "api")
		if !rec1.ObservedAt.Equal(rec2.ObservedAt) {
			t.Fatalf("observed_at changed on no-op sync: %v -> %v", rec1.ObservedAt, rec2.ObservedAt)
		}
	})

	t.Run("TC-SYNC-006", func(t *testing.T) {
		t.Parallel()
		_, mA, _, repoA, _, now := bootstrapRepoAcrossTwoMachines(t)
		if out, err := mA.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("baseline sync failed: %v\n%s", err, out)
		}
		rec1 := findRepoRecordByName(t, loadMachineFile(t, mA), "api")

		mA.MustWriteFile(filepath.Join(repoA, "new.txt"), "new\n")
		mA.MustRunGit(now, repoA, "add", "new.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "new state")
		mA.MustRunGit(now, repoA, "push")

		if out, err := mA.RunBB(now.Add(20*time.Minute), "sync"); err != nil {
			t.Fatalf("sync after change failed: %v\n%s", err, out)
		}
		rec2 := findRepoRecordByName(t, loadMachineFile(t, mA), "api")
		if rec1.StateHash == rec2.StateHash {
			t.Fatalf("expected state_hash change, both=%q", rec1.StateHash)
		}
		if !rec2.ObservedAt.After(rec1.ObservedAt) {
			t.Fatalf("expected observed_at to advance, got %v <= %v", rec2.ObservedAt, rec1.ObservedAt)
		}
	})

	t.Run("TC-SYNC-012", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mA.MustWriteFile(filepath.Join(repoA, "pull.txt"), "pull\n")
		mA.MustRunGit(now, repoA, "add", "pull.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "pull me")
		mA.MustRunGit(now, repoA, "push")
		if out, err := mA.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}

		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(3*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		shaA := stringTrim(mA.MustRunGit(now, repoA, "rev-parse", "HEAD"))
		shaB := stringTrim(mB.MustRunGit(now, repoB, "rev-parse", "HEAD"))
		if shaA != shaB {
			t.Fatalf("expected B to fast-forward pull: A=%s B=%s", shaA, shaB)
		}
	})

	t.Run("TC-SYNC-013", func(t *testing.T) {
		t.Parallel()
		_, _, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mB.MustWriteFile(filepath.Join(repoB, "ahead.txt"), "ahead\n")
		mB.MustRunGit(now, repoB, "add", "ahead.txt")
		mB.MustRunGit(now, repoB, "commit", "-m", "ahead")

		if out, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
			t.Fatalf("sync failed: %v\n%s", err, out)
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if rec.Ahead != 0 {
			t.Fatalf("expected ahead=0 after auto-push, got %d", rec.Ahead)
		}
	})

	t.Run("TC-SYNC-013A", func(t *testing.T) {
		t.Parallel()
		_, _, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)
		privateBlocked := false
		setCatalogDefaultBranchAutoPushPolicy(t, mB, "software", &privateBlocked, nil)

		mB.MustWriteFile(filepath.Join(repoB, "ahead.txt"), "ahead\n")
		mB.MustRunGit(now, repoB, "add", "ahead.txt")
		mB.MustRunGit(now, repoB, "commit", "-m", "ahead")

		if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
			t.Fatal("expected sync unsyncable when default-branch auto-push is disabled for private repos")
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonPushPolicyBlocked) {
			t.Fatalf("expected push_policy_blocked reason, got %v", rec.UnsyncableReasons)
		}
	})

	t.Run("TC-SYNC-013B", func(t *testing.T) {
		t.Parallel()
		_, _, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mB.MustWriteFile(filepath.Join(repoB, "ahead.txt"), "ahead\n")
		mB.MustRunGit(now, repoB, "add", "ahead.txt")
		mB.MustRunGit(now, repoB, "commit", "-m", "ahead")

		if out, err := mB.RunBB(now.Add(2*time.Minute), "repo", "access-set", "api", "--push-access=read_only"); err != nil {
			t.Fatalf("repo access-set failed: %v\n%s", err, out)
		}

		if out, err := mB.RunBB(now.Add(3*time.Minute), "sync", "--push"); err == nil {
			t.Fatalf("expected sync unsyncable for read-only remote, output=%s", out)
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonPushAccessBlocked) {
			t.Fatalf("expected push_access_blocked reason, got %v", rec.UnsyncableReasons)
		}
		if rec.Ahead == 0 {
			t.Fatalf("expected ahead commits to remain when push access is read-only, got ahead=%d", rec.Ahead)
		}

		if out, err := mB.RunBB(now.Add(4*time.Minute), "repo", "policy", "api", "--auto-push=true"); err == nil {
			t.Fatalf("expected repo policy auto-push enable to fail for read-only remote, output=%s", out)
		}
	})

	t.Run("TC-SYNC-014", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, rootA, rootB := setupTwoMachines(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		if out, err := mA.RunBB(now, "init", "api", "--public"); err != nil {
			t.Fatalf("init public failed: %v\n%s", err, out)
		}
		repoA := filepath.Join(rootA, "api")
		mA.MustWriteFile(filepath.Join(repoA, "README.md"), "hello\n")
		mA.MustRunGit(now, repoA, "add", "README.md")
		mA.MustRunGit(now, repoA, "commit", "-m", "bootstrap")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "main")
		if out, err := mA.RunBB(now.Add(30*time.Second), "sync"); err != nil {
			t.Fatalf("sync on A after bootstrap failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}
		repoB := filepath.Join(rootB, "api")

		mB.MustWriteFile(filepath.Join(repoB, "public.txt"), "ahead\n")
		mB.MustRunGit(now, repoB, "add", "public.txt")
		mB.MustRunGit(now, repoB, "commit", "-m", "ahead public")

		if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
			t.Fatal("expected sync unsyncable due push policy")
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonPushPolicyBlocked) {
			t.Fatalf("expected push_policy_blocked reason, got %v", rec.UnsyncableReasons)
		}
	})

	t.Run("TC-SYNC-015", func(t *testing.T) {
		t.Parallel()
		h, mA, mB, _, rootB := setupTwoMachines(t)
		now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
		if out, err := mA.RunBB(now, "init", "api", "--public"); err != nil {
			t.Fatalf("init public failed: %v\n%s", err, out)
		}
		repoA := filepath.Join(h.Root, "machines", "a", "catalogs", "software", "api")
		mA.MustWriteFile(filepath.Join(repoA, "README.md"), "hello\n")
		mA.MustRunGit(now, repoA, "add", "README.md")
		mA.MustRunGit(now, repoA, "commit", "-m", "bootstrap")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "main")
		if out, err := mA.RunBB(now.Add(30*time.Second), "sync"); err != nil {
			t.Fatalf("sync on A after bootstrap failed: %v\n%s", err, out)
		}
		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}
		repoB := filepath.Join(rootB, "api")

		mB.MustWriteFile(filepath.Join(repoB, "public.txt"), "ahead\n")
		mB.MustRunGit(now, repoB, "add", "public.txt")
		mB.MustRunGit(now, repoB, "commit", "-m", "ahead public")

		if out, err := mB.RunBB(now.Add(2*time.Minute), "sync", "--push"); err != nil {
			t.Fatalf("sync --push failed: %v\n%s", err, out)
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if rec.Ahead != 0 {
			t.Fatalf("expected ahead=0 after sync --push, got %d", rec.Ahead)
		}
	})
}

func containsReason(reasons []domain.UnsyncableReason, want domain.UnsyncableReason) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}
