package app

import "bb-project/internal/domain"

type fixSyncProbeOutcome string

const (
	fixSyncProbeUnknown  fixSyncProbeOutcome = "unknown"
	fixSyncProbeClean    fixSyncProbeOutcome = "clean"
	fixSyncProbeConflict fixSyncProbeOutcome = "conflict"
	fixSyncProbeFailed   fixSyncProbeOutcome = "probe_failed"
)

type fixSyncFeasibility struct {
	Checked       bool
	RebaseOutcome fixSyncProbeOutcome
	MergeOutcome  fixSyncProbeOutcome
}

func (f fixSyncFeasibility) outcomeFor(strategy FixSyncStrategy) fixSyncProbeOutcome {
	switch normalizeFixSyncStrategy(strategy) {
	case FixSyncStrategyMerge:
		return f.MergeOutcome
	default:
		return f.RebaseOutcome
	}
}

func (f fixSyncFeasibility) cleanFor(strategy FixSyncStrategy) bool {
	return f.outcomeFor(strategy) == fixSyncProbeClean
}

func (f fixSyncFeasibility) conflictFor(strategy FixSyncStrategy) bool {
	return f.outcomeFor(strategy) == fixSyncProbeConflict
}

func (f fixSyncFeasibility) probeFailedFor(strategy FixSyncStrategy) bool {
	return f.outcomeFor(strategy) == fixSyncProbeFailed
}

func (f fixSyncFeasibility) canAttemptFor(strategy FixSyncStrategy) bool {
	switch f.outcomeFor(strategy) {
	case fixSyncProbeClean, fixSyncProbeFailed:
		return true
	default:
		return false
	}
}

func (f fixSyncFeasibility) allStrategiesConflict() bool {
	return f.RebaseOutcome == fixSyncProbeConflict && f.MergeOutcome == fixSyncProbeConflict
}

func (f fixSyncFeasibility) defaultStrategyConflictWithoutCleanFallback() bool {
	return f.RebaseOutcome == fixSyncProbeConflict &&
		f.RebaseOutcome != fixSyncProbeClean &&
		f.MergeOutcome != fixSyncProbeClean
}

func (f fixSyncFeasibility) anyProbeFailed() bool {
	return f.RebaseOutcome == fixSyncProbeFailed || f.MergeOutcome == fixSyncProbeFailed
}

func (f *fixSyncFeasibility) setOutcome(strategy FixSyncStrategy, outcome fixSyncProbeOutcome) {
	if outcome == "" {
		outcome = fixSyncProbeUnknown
	}
	switch normalizeFixSyncStrategy(strategy) {
	case FixSyncStrategyMerge:
		f.MergeOutcome = outcome
	default:
		f.RebaseOutcome = outcome
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
