package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"bb-project/internal/app"
	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestOverviewTwoMachineRealGitGolden(t *testing.T) {
	t.Parallel()
	h, mA, mB, _, repoB, now := bootstrapRepoAcrossTwoMachines(t)
	dirty := filepath.Join(repoB, "README.md")
	mB.MustWriteFile(dirty, "remote work\n")
	touched := now.Add(-6 * time.Hour)
	for _, path := range []string{dirty, filepath.Join(repoB, ".git", "HEAD"), filepath.Join(repoB, ".git", "index")} {
		if err := os.Chtimes(path, touched, touched); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := mB.RunBB(now, "scan"); err != nil {
		t.Fatalf("scan B: %v\n%s", err, out)
	}
	pathsA, pathsB := state.NewPaths(mA.Home), state.NewPaths(mB.Home)
	machineA, err := state.LoadMachine(pathsA, "a-machine")
	if err != nil {
		t.Fatal(err)
	}
	machineB, err := state.LoadMachine(pathsB, "b-machine")
	if err != nil {
		t.Fatal(err)
	}
	for i := range machineA.Repos {
		if machineA.Repos[i].RepoKey == "software/api" {
			machineA.Repos[i].LastActivityAt = now.Add(-30 * time.Minute)
		}
	}
	for i := range machineB.Repos {
		if machineB.Repos[i].RepoKey == "software/api" {
			machineB.Repos[i].LastActivityAt = now.Add(-6 * time.Hour)
		}
	}
	machineA.Repos = append(machineA.Repos, domain.MachineRepoRecord{RepoKey: "software/blocked", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}, LastActivityAt: now.Add(-2 * time.Hour)}, domain.MachineRepoRecord{RepoKey: "software/clean", State: domain.RepoStateSynced}, domain.MachineRepoRecord{RepoKey: "references/only", State: domain.RepoStateSynced})
	machineB.Repos = append(machineB.Repos, domain.MachineRepoRecord{RepoKey: "software/clean", State: domain.RepoStateSynced}, domain.MachineRepoRecord{RepoKey: "references/only", State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-48 * time.Hour)})
	machineB.UpdatedAt = now.Add(-4 * 24 * time.Hour)
	if err := state.SaveMachine(pathsA, machineA); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveMachine(pathsB, machineB); err != nil {
		t.Fatal(err)
	}
	h.ExternalSync("b-machine", "a-machine")
	out, err := mA.RunBB(now, "overview")
	if err != nil {
		t.Fatal(err)
	}
	wantPlain := "b-machine last published 4d ago — its data may be stale.\nreferences/only   here: synced   b-machine: wip (dirty_tracked · 2d ago)\nsoftware/api   here: synced   b-machine: wip (dirty_tracked · 6h ago)\nsoftware/blocked   here: blocked (diverged · 2h ago)   b-machine: — (not cloned)\nsynced everywhere: 1 repos (--all to list)\n"
	if out != wantPlain {
		t.Fatalf("plain:\n%s\nwant:\n%s", out, wantPlain)
	}
	allOut, err := mA.RunBB(now, "overview", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(allOut, "software/clean   here: synced   b-machine: synced") {
		t.Fatalf("--all:\n%s", allOut)
	}
	filtered, err := mA.RunBB(now, "overview", "--include-catalog", "references")
	if err != nil {
		t.Fatal(err)
	}
	wantFiltered := "b-machine last published 4d ago — its data may be stale.\nreferences/only   here: synced   b-machine: wip (dirty_tracked · 2d ago)\nsynced everywhere: 0 repos (--all to list)\n"
	if filtered != wantFiltered {
		t.Fatalf("filter:\n%s\nwant:\n%s", filtered, wantFiltered)
	}
	jsonOut, err := mA.RunBB(now, "overview", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var matrix app.OverviewMatrix
	if err := json.Unmarshal([]byte(jsonOut), &matrix); err != nil {
		t.Fatalf("json: %v\n%s", err, jsonOut)
	}
	updatedA, updatedB := machineA.UpdatedAt, machineB.UpdatedAt
	empty := []domain.UnsyncableReason{}
	want := app.OverviewMatrix{Machines: []app.OverviewMachine{{ID: "a-machine", Here: true, Published: true, UpdatedAt: &updatedA}, {ID: "b-machine", Published: true, UpdatedAt: &updatedB, Stale: true}}, Repos: []app.OverviewRepo{
		{RepoKey: "references/only", Cells: []app.OverviewCell{{MachineID: "a-machine", Present: true, State: domain.RepoStateSynced}, {MachineID: "b-machine", Present: true, State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-48 * time.Hour)}}},
		{RepoKey: "software/api", Cells: []app.OverviewCell{{MachineID: "a-machine", Present: true, State: domain.RepoStateSynced, LastActivityAt: now.Add(-30 * time.Minute)}, {MachineID: "b-machine", Present: true, State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-6 * time.Hour)}}},
		{RepoKey: "software/blocked", Cells: []app.OverviewCell{{MachineID: "a-machine", Present: true, State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}, LastActivityAt: now.Add(-2 * time.Hour)}, {MachineID: "b-machine", Reasons: empty, Warnings: empty}}},
		{RepoKey: "software/clean", SyncedEverywhere: true, Cells: []app.OverviewCell{{MachineID: "a-machine", Present: true, State: domain.RepoStateSynced}, {MachineID: "b-machine", Present: true, State: domain.RepoStateSynced}}},
	}, SyncedEverywhere: 1, Warnings: []string{}}
	for repoIndex := range want.Repos {
		for cellIndex := range want.Repos[repoIndex].Cells {
			cell := &want.Repos[repoIndex].Cells[cellIndex]
			if cell.Reasons == nil {
				cell.Reasons = []domain.UnsyncableReason{}
			}
			if cell.Warnings == nil {
				cell.Warnings = []domain.UnsyncableReason{}
			}
		}
	}
	if !reflect.DeepEqual(matrix, want) {
		t.Fatalf("JSON matrix:\n%+v\nwant:\n%+v\nraw:\n%s", matrix, want, jsonOut)
	}
	rec := findRepoRecordByName(t, loadMachineFile(t, mA), "api")
	if rec.State != domain.RepoStateSynced {
		t.Fatalf("local state=%q", rec.State)
	}
}
