package domain

import (
	"testing"
	"time"
)

func TestSelectWinner(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Minute)

	recs := []MachineRepoRecordWithMachine{
		{MachineID: "m2", Record: MachineRepoRecord{Syncable: true, ObservedAt: t0, Branch: "main"}},
		{MachineID: "m1", Record: MachineRepoRecord{Syncable: true, ObservedAt: t1, Branch: "feature/x"}},
		{MachineID: "m3", Record: MachineRepoRecord{Syncable: false, ObservedAt: t1.Add(1 * time.Minute), Branch: "ignored"}},
	}

	winner, ok := SelectWinner(recs)
	if !ok {
		t.Fatal("expected winner")
	}
	if winner.MachineID != "m1" {
		t.Fatalf("winner machine = %q, want %q", winner.MachineID, "m1")
	}
	if winner.Record.Branch != "feature/x" {
		t.Fatalf("winner branch = %q", winner.Record.Branch)
	}
}

func TestSelectWinnerTieBreakByMachineID(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	recs := []MachineRepoRecordWithMachine{
		{MachineID: "z-machine", Record: MachineRepoRecord{Syncable: true, ObservedAt: ts, Branch: "main"}},
		{MachineID: "a-machine", Record: MachineRepoRecord{Syncable: true, ObservedAt: ts, Branch: "dev"}},
	}

	winner, ok := SelectWinner(recs)
	if !ok {
		t.Fatal("expected winner")
	}
	if winner.MachineID != "a-machine" {
		t.Fatalf("winner machine = %q, want %q", winner.MachineID, "a-machine")
	}
}
