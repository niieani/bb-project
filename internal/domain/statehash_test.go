package domain

import (
	"testing"
	"time"
)

func TestComputeStateHashStable(t *testing.T) {
	t.Parallel()

	a := MachineRepoRecord{
		Branch:              "main",
		HeadSHA:             "1",
		Upstream:            "origin/main",
		RemoteHeadSHA:       "1",
		Ahead:               0,
		Behind:              0,
		Diverged:            false,
		HasDirtyTracked:     false,
		HasUntracked:        false,
		OperationInProgress: OperationNone,
		Syncable:            true,
		UnsyncableReasons:   nil,
	}
	b := a

	ha := ComputeStateHash(a)
	hb := ComputeStateHash(b)
	if ha == "" || hb == "" {
		t.Fatal("expected non-empty hashes")
	}
	if ha != hb {
		t.Fatalf("expected stable hash; got %q vs %q", ha, hb)
	}

	b.Behind = 1
	if ComputeStateHash(a) == ComputeStateHash(b) {
		t.Fatal("expected hash to change when state fields change")
	}
}

func TestUpdateObservedAt(t *testing.T) {
	t.Parallel()

	prevTime := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	now := prevTime.Add(1 * time.Hour)

	record := MachineRepoRecord{StateHash: "sha256:old", ObservedAt: prevTime}
	current := MachineRepoRecord{StateHash: "sha256:old"}

	out := UpdateObservedAt(record, current, now)
	if !out.ObservedAt.Equal(prevTime) {
		t.Fatalf("ObservedAt changed on same hash: got %v want %v", out.ObservedAt, prevTime)
	}

	current.StateHash = "sha256:new"
	out = UpdateObservedAt(record, current, now)
	if !out.ObservedAt.Equal(now) {
		t.Fatalf("ObservedAt did not update for changed hash: got %v want %v", out.ObservedAt, now)
	}
}
