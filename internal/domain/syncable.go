package domain

func EvaluateSyncability(state ObservedRepoState, autoPush bool, cliPush bool) (bool, []UnsyncableReason) {
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

	if len(reasons) == 0 {
		return true, nil
	}
	return false, reasons
}
