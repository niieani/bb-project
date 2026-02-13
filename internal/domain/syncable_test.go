package domain

import (
	"reflect"
	"testing"
)

func TestEvaluateSyncability(t *testing.T) {
	t.Parallel()

	base := ObservedRepoState{
		OriginURL:            "git@github.com:you/project.git",
		Branch:               "main",
		HeadSHA:              "1111111111111111111111111111111111111111",
		Upstream:             "origin/main",
		RemoteHeadSHA:        "1111111111111111111111111111111111111111",
		OperationInProgress:  OperationNone,
		IncludeUntrackedRule: true,
	}

	tests := []struct {
		name         string
		state        ObservedRepoState
		autoPush     bool
		cliPush      bool
		wantSyncable bool
		wantReasons  []UnsyncableReason
	}{
		{name: "clean syncable", state: base, autoPush: true, wantSyncable: true},
		{name: "missing origin", state: func() ObservedRepoState { s := base; s.OriginURL = ""; return s }(), autoPush: true, wantReasons: []UnsyncableReason{ReasonMissingOrigin}},
		{name: "operation in progress", state: func() ObservedRepoState { s := base; s.OperationInProgress = OperationRebase; return s }(), autoPush: true, wantReasons: []UnsyncableReason{ReasonOperationInProgress}},
		{name: "dirty tracked", state: func() ObservedRepoState { s := base; s.HasDirtyTracked = true; return s }(), autoPush: true, wantReasons: []UnsyncableReason{ReasonDirtyTracked}},
		{name: "dirty untracked enforced", state: func() ObservedRepoState { s := base; s.HasUntracked = true; return s }(), autoPush: true, wantReasons: []UnsyncableReason{ReasonDirtyUntracked}},
		{name: "dirty untracked ignored when disabled", state: func() ObservedRepoState { s := base; s.HasUntracked = true; s.IncludeUntrackedRule = false; return s }(), autoPush: true, wantSyncable: true},
		{name: "missing upstream", state: func() ObservedRepoState { s := base; s.Upstream = ""; return s }(), autoPush: true, wantReasons: []UnsyncableReason{ReasonMissingUpstream}},
		{name: "diverged", state: func() ObservedRepoState { s := base; s.Diverged = true; return s }(), autoPush: true, wantReasons: []UnsyncableReason{ReasonDiverged}},
		{name: "ahead blocked by policy", state: func() ObservedRepoState { s := base; s.Ahead = 1; return s }(), autoPush: false, wantReasons: []UnsyncableReason{ReasonPushPolicyBlocked}},
		{name: "ahead allowed by cli flag", state: func() ObservedRepoState { s := base; s.Ahead = 1; return s }(), autoPush: false, cliPush: true, wantSyncable: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			syncable, reasons := EvaluateSyncability(tt.state, tt.autoPush, tt.cliPush)
			if syncable != tt.wantSyncable {
				t.Fatalf("syncable = %v, want %v (reasons=%v)", syncable, tt.wantSyncable, reasons)
			}

			if !reflect.DeepEqual(reasons, tt.wantReasons) {
				t.Fatalf("reasons = %v, want %v", reasons, tt.wantReasons)
			}
		})
	}
}
