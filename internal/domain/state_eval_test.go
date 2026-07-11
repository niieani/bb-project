package domain

import "testing"

func TestReasonTier(t *testing.T) {
	t.Parallel()
	tests := map[UnsyncableReason]RepoSyncState{
		ReasonDirtyTracked: RepoStateWip, ReasonDirtyUntracked: RepoStateWip,
		ReasonOperationInProgress: RepoStateWip, ReasonMissingUpstream: RepoStateWip,
		ReasonMissingOrigin: RepoStateWip, ReasonPushPolicyBlocked: RepoStateWip,
		ReasonDiverged: RepoStateBlocked, ReasonPushAccessBlocked: RepoStateBlocked,
		ReasonSyncConflict: RepoStateBlocked, ReasonPushFailed: RepoStateBlocked,
		ReasonPullFailed: RepoStateBlocked, ReasonCheckoutFailed: RepoStateBlocked,
		ReasonSyncProbeFailed: RepoStateBlocked, ReasonTargetPathNonRepo: RepoStateBlocked,
		ReasonTargetPathRepoMismatch: RepoStateBlocked, ReasonCloneRequired: RepoStatePending,
		ReasonCatalogNotMapped: RepoStatePending, ReasonCatalogMismatch: RepoStatePending,
	}
	for reason, want := range tests {
		reason, want := reason, want
		t.Run(string(reason), func(t *testing.T) {
			t.Parallel()
			if got := ReasonTier(reason); got != want {
				t.Fatalf("ReasonTier(%q) = %q, want %q", reason, got, want)
			}
		})
	}
}

func TestReasonTierPanicsOnUnknown(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	ReasonTier("new_reason_without_declared_tier")
}

func TestEvaluateRepoStatePrecedence(t *testing.T) {
	t.Parallel()
	base := ObservedRepoState{OriginURL: "git@example.test:o/r.git", Upstream: "origin/main"}
	tests := []struct {
		name     string
		state    ObservedRepoState
		autoPush bool
		want     RepoSyncState
	}{
		{"synced", base, true, RepoStateSynced},
		{"wip", func() ObservedRepoState { s := base; s.HasDirtyTracked = true; return s }(), true, RepoStateWip},
		{"blocked_over_wip", func() ObservedRepoState { s := base; s.HasDirtyTracked = true; s.Diverged = true; return s }(), true, RepoStateBlocked},
		{"push_policy_wip", func() ObservedRepoState { s := base; s.Ahead = 1; return s }(), false, RepoStateWip},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EvaluateRepoState(tt.state, tt.autoPush, false)
			if got.State != tt.want {
				t.Fatalf("state = %q, want %q; reasons=%v", got.State, tt.want, got.Reasons)
			}
		})
	}
}

func TestDeriveRepoStatePrecedence(t *testing.T) {
	t.Parallel()
	if got := DeriveRepoState([]UnsyncableReason{ReasonDirtyTracked, ReasonCloneRequired}); got != RepoStatePending {
		t.Fatalf("pending must outrank wip: %q", got)
	}
	if got := DeriveRepoState([]UnsyncableReason{ReasonCloneRequired, ReasonDiverged}); got != RepoStateBlocked {
		t.Fatalf("blocked must outrank pending: %q", got)
	}
}
