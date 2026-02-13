package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestSyncEdgeCases(t *testing.T) {
	t.Run("TC-SYNC-007", func(t *testing.T) {
		h, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)
		tieTime := now.Add(10 * time.Minute)

		mA.MustRunGit(now, repoA, "checkout", "-b", "tie-a")
		mA.MustWriteFile(filepath.Join(repoA, "a.txt"), "a\n")
		mA.MustRunGit(now, repoA, "add", "a.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "tie a")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "tie-a")

		mB.MustRunGit(now, repoB, "checkout", "-b", "tie-b")
		mB.MustWriteFile(filepath.Join(repoB, "b.txt"), "b\n")
		mB.MustRunGit(now, repoB, "add", "b.txt")
		mB.MustRunGit(now, repoB, "commit", "-m", "tie b")
		mB.MustRunGit(now, repoB, "push", "-u", "origin", "tie-b")

		if out, err := mA.RunBB(tieTime, "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}
		if out, err := mB.RunBB(tieTime, "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}

		mfA := loadMachineFile(t, mA)
		mfB := loadMachineFile(t, mB)
		recA := findRepoRecordByName(t, mfA, "api")
		recB := findRepoRecordByName(t, mfB, "api")
		recA.ObservedAt = tieTime
		recB.ObservedAt = tieTime
		mfA.Repos = []domain.MachineRepoRecord{recA}
		mfB.Repos = []domain.MachineRepoRecord{recB}
		if err := state.SaveMachine(state.NewPaths(mA.Home), mfA); err != nil {
			t.Fatalf("save machine A: %v", err)
		}
		if err := state.SaveMachine(state.NewPaths(mB.Home), mfB); err != nil {
			t.Fatalf("save machine B: %v", err)
		}

		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(20*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}
		if got := gitCurrentBranch(t, mB, repoB, now); got != "tie-a" {
			t.Fatalf("expected tie-break winner branch tie-a, got %q", got)
		}
	})

	t.Run("TC-SYNC-008", func(t *testing.T) {
		h, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mA.MustRunGit(now, repoA, "checkout", "-b", "local-a")
		mB.MustRunGit(now, repoB, "checkout", "-b", "local-b")
		_, _ = mA.RunBB(now.Add(2*time.Minute), "sync")
		_, _ = mB.RunBB(now.Add(3*time.Minute), "sync")
		h.ExternalSync("a-machine", "b-machine")

		if _, err := mB.RunBB(now.Add(4*time.Minute), "sync"); err == nil {
			t.Fatal("expected unsyncable state with no winner")
		}
		if got := gitCurrentBranch(t, mB, repoB, now); got != "local-b" {
			t.Fatalf("expected branch unchanged with no winner, got %q", got)
		}
	})

	t.Run("TC-SYNC-009", func(t *testing.T) {
		_, mA, mB, repoA, repoB, now := bootstrapRepoAcrossTwoMachines(t)

		mB.MustWriteFile(filepath.Join(repoB, "b.txt"), "b\n")
		mB.MustRunGit(now, repoB, "add", "b.txt")
		mB.MustRunGit(now, repoB, "commit", "-m", "b commit")

		mA.MustWriteFile(filepath.Join(repoA, "a.txt"), "a\n")
		mA.MustRunGit(now, repoA, "add", "a.txt")
		mA.MustRunGit(now, repoA, "commit", "-m", "a commit")
		mA.MustRunGit(now, repoA, "push")

		if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
			t.Fatal("expected sync to fail due divergence")
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonDiverged) {
			t.Fatalf("expected diverged reason, got %v", rec.UnsyncableReasons)
		}
	})

	t.Run("TC-SYNC-010", func(t *testing.T) {
		tests := []struct {
			name   string
			marker string
			dir    bool
		}{
			{name: "merge", marker: "MERGE_HEAD"},
			{name: "rebase", marker: "rebase-apply", dir: true},
			{name: "cherry-pick", marker: "CHERRY_PICK_HEAD"},
			{name: "bisect", marker: "BISECT_LOG"},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				_, _, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)
				gitDir := filepath.Join(repoB, ".git", tt.marker)
				if tt.dir {
					if err := os.MkdirAll(gitDir, 0o755); err != nil {
						t.Fatalf("mkdir marker: %v", err)
					}
				} else {
					if err := os.WriteFile(gitDir, []byte("x\n"), 0o644); err != nil {
						t.Fatalf("write marker: %v", err)
					}
				}
				if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
					t.Fatal("expected unsyncable due operation in progress")
				}
				rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
				if !containsReason(rec.UnsyncableReasons, domain.ReasonOperationInProgress) {
					t.Fatalf("expected operation_in_progress, got %v", rec.UnsyncableReasons)
				}
			})
		}
	})

	t.Run("TC-SYNC-011", func(t *testing.T) {
		_, _, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)
		mB.MustRunGit(now, repoB, "checkout", "-b", "missing-upstream")
		if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
			t.Fatal("expected unsyncable due missing upstream")
		}
		rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
		if !containsReason(rec.UnsyncableReasons, domain.ReasonMissingUpstream) {
			t.Fatalf("expected missing_upstream, got %v", rec.UnsyncableReasons)
		}
	})

	t.Run("TC-SYNC-016", func(t *testing.T) {
		t.Run("pull_failed", func(t *testing.T) {
			_, _, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)
			mB.MustRunGit(now, repoB, "remote", "set-url", "origin", "/nonexistent/remote.git")
			if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
				t.Fatal("expected sync failure")
			}
			rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
			if !containsReason(rec.UnsyncableReasons, domain.ReasonPullFailed) {
				t.Fatalf("expected pull_failed, got %v", rec.UnsyncableReasons)
			}
		})

		t.Run("push_failed", func(t *testing.T) {
			_, _, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)
			remote := stringTrim(mB.MustRunGit(now, repoB, "remote", "get-url", "origin"))
			hookPath := filepath.Join(remote, "hooks", "pre-receive")
			if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
				t.Fatalf("write hook: %v", err)
			}

			mB.MustWriteFile(filepath.Join(repoB, "push-fail.txt"), "x\n")
			mB.MustRunGit(now, repoB, "add", "push-fail.txt")
			mB.MustRunGit(now, repoB, "commit", "-m", "push fail")

			if _, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err == nil {
				t.Fatal("expected sync failure")
			}
			rec := findRepoRecordByName(t, loadMachineFile(t, mB), "api")
			if !containsReason(rec.UnsyncableReasons, domain.ReasonPushFailed) {
				t.Fatalf("expected push_failed, got %v", rec.UnsyncableReasons)
			}
		})
	})

	t.Run("TC-SYNC-017", func(t *testing.T) {
		_, m, _ := setupSingleMachine(t)
		if out, err := m.RunBB(time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC), "scan"); err != nil && out == "" {
			// ignore; lock test only needs machine paths initialized
		}
		lockPath := filepath.Join(m.LocalStateRoot(), "lock")
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			t.Fatalf("mkdir lock dir: %v", err)
		}
		if err := os.WriteFile(lockPath, []byte("held\n"), 0o644); err != nil {
			t.Fatalf("write lock: %v", err)
		}

		_, err := m.RunBB(time.Date(2026, 2, 13, 20, 40, 0, 0, time.UTC), "sync")
		if err == nil {
			t.Fatal("expected lock failure")
		}
	})
}
