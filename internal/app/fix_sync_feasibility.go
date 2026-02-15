package app

import "bb-project/internal/domain"

type fixSyncFeasibility struct {
	Checked     bool
	RebaseClean bool
	MergeClean  bool
}

func (f fixSyncFeasibility) cleanFor(strategy FixSyncStrategy) bool {
	switch normalizeFixSyncStrategy(strategy) {
	case FixSyncStrategyMerge:
		return f.MergeClean
	default:
		return f.RebaseClean
	}
}

func appendUniqueUnsyncableReason(reasons []domain.UnsyncableReason, reason domain.UnsyncableReason) []domain.UnsyncableReason {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
