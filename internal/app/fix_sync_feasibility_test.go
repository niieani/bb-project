package app

import "testing"

func TestFixSyncFeasibilityCanAttemptFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		feas       fixSyncFeasibility
		strategy   FixSyncStrategy
		canAttempt bool
	}{
		{
			name: "clean strategy is attemptable",
			feas: fixSyncFeasibility{
				Checked:       true,
				RebaseOutcome: fixSyncProbeClean,
			},
			strategy:   FixSyncStrategyRebase,
			canAttempt: true,
		},
		{
			name: "probe failed strategy is still attemptable",
			feas: fixSyncFeasibility{
				Checked:      true,
				MergeOutcome: fixSyncProbeFailed,
			},
			strategy:   FixSyncStrategyMerge,
			canAttempt: true,
		},
		{
			name: "conflict strategy is not attemptable",
			feas: fixSyncFeasibility{
				Checked:       true,
				RebaseOutcome: fixSyncProbeConflict,
			},
			strategy:   FixSyncStrategyRebase,
			canAttempt: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.feas.canAttemptFor(tt.strategy); got != tt.canAttempt {
				t.Fatalf("canAttemptFor(%s) = %v, want %v", tt.strategy, got, tt.canAttempt)
			}
		})
	}
}
