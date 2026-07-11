package domain

import "fmt"

type RepoStateResult struct {
	State   RepoSyncState
	Reasons []UnsyncableReason
}

func ReasonTier(reason UnsyncableReason) RepoSyncState {
	switch reason {
	case ReasonDirtyTracked, ReasonDirtyUntracked, ReasonOperationInProgress,
		ReasonMissingUpstream, ReasonMissingOrigin, ReasonPushPolicyBlocked:
		return RepoStateWip
	case ReasonDiverged, ReasonPushAccessBlocked, ReasonSyncConflict, ReasonPushFailed,
		ReasonPullFailed, ReasonCheckoutFailed, ReasonSyncProbeFailed,
		ReasonTargetPathNonRepo, ReasonTargetPathRepoMismatch:
		return RepoStateBlocked
	case ReasonCloneRequired, ReasonCatalogNotMapped, ReasonCatalogMismatch:
		return RepoStatePending
	default:
		panic(fmt.Sprintf("reason %q has no declared tier", reason))
	}
}

func EvaluateRepoState(state ObservedRepoState, autoPush bool, cliPush bool) RepoStateResult {
	reasons := make([]UnsyncableReason, 0, 8)
	if state.OriginURL == "" {
		reasons = append(reasons, ReasonMissingOrigin)
	}
	if state.OperationInProgress != "" && state.OperationInProgress != OperationNone {
		reasons = append(reasons, ReasonOperationInProgress)
	}
	if state.HasDirtyTracked {
		reasons = append(reasons, ReasonDirtyTracked)
	}
	if state.IncludeUntrackedRule && state.HasUntracked {
		reasons = append(reasons, ReasonDirtyUntracked)
	}
	if state.Upstream == "" {
		reasons = append(reasons, ReasonMissingUpstream)
	}
	if state.Diverged {
		reasons = append(reasons, ReasonDiverged)
	}
	if state.Ahead > 0 {
		if state.PushAccess == PushAccessReadOnly {
			reasons = append(reasons, ReasonPushAccessBlocked)
		} else if !(autoPush || cliPush) {
			reasons = append(reasons, ReasonPushPolicyBlocked)
		}
	}
	return RepoStateResult{State: DeriveRepoState(reasons), Reasons: reasons}
}

func DeriveRepoState(reasons []UnsyncableReason) RepoSyncState {
	state := RepoStateSynced
	for _, reason := range reasons {
		switch ReasonTier(reason) {
		case RepoStateBlocked:
			return RepoStateBlocked
		case RepoStatePending:
			if state != RepoStatePending {
				state = RepoStatePending
			}
		case RepoStateWip:
			if state == RepoStateSynced {
				state = RepoStateWip
			}
		}
	}
	return state
}
