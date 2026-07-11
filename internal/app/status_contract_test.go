package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestStatusJSONStableContract(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	paths := state.NewPaths(t.TempDir())
	t.Setenv("BB_MACHINE_ID", "machine-a")
	cfg := state.DefaultConfig()
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}
	local := statusContractMachine(now, "machine-a", []domain.MachineRepoRecord{
		statusContractRepo("software/synced", "synced", domain.RepoStateSynced, nil, nil, now.Add(-time.Hour)),
		statusContractRepo("software/warning", "warning", domain.RepoStateSynced, nil, []domain.UnsyncableReason{domain.ReasonRemoteFormatMismatch}, now.Add(-2*time.Hour)),
		statusContractRepo("software/blocked", "blocked", domain.RepoStateBlocked, []domain.UnsyncableReason{domain.ReasonDiverged}, nil, now.Add(-3*time.Hour)),
		statusContractRepo("software/recent", "recent", domain.RepoStateBlocked, []domain.UnsyncableReason{domain.ReasonPushFailed}, nil, now.Add(-time.Hour)),
		statusContractRepo("software/stale", "stale", domain.RepoStateWip, []domain.UnsyncableReason{domain.ReasonDirtyTracked}, nil, now.Add(-25*time.Hour)),
		statusContractRepo("software/pending", "pending", domain.RepoStatePending, []domain.UnsyncableReason{domain.ReasonCloneRequired}, nil, time.Time{}),
	})
	remoteOnly := statusContractRepo("references/remote-only", "remote-only", domain.RepoStateBlocked, []domain.UnsyncableReason{domain.ReasonDiverged}, nil, now.Add(-48*time.Hour))
	remoteOnly.Catalog = "references"
	remote := statusContractMachine(now, "machine-b", []domain.MachineRepoRecord{
		statusContractRepo("software/remote", "remote", domain.RepoStateWip, []domain.UnsyncableReason{domain.ReasonDirtyUntracked}, nil, now.Add(-48*time.Hour)),
		remoteOnly,
	})
	for _, machine := range []domain.MachineFile{local, remote} {
		if err := state.SaveMachine(paths, machine); err != nil {
			t.Fatal(err)
		}
	}
	lastSync := domain.JournalEvent{At: now.Add(-30 * time.Minute), Machine: "machine-a", Event: "sync_run", Detail: "synced=2 pending=1 wip=1 blocked=1 duration=2s"}
	if err := state.AppendJournal(paths, lastSync, cfg.Journal.MaxEntries); err != nil {
		t.Fatal(err)
	}

	got := runStatusJSONContract(t, paths, now)
	want := readJSONDocument(t, filepath.Join("..", "..", "fixtures", "status", "mixed.json"))
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("status contract mismatch\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}

func TestStatusJSONNoJournalUsesExplicitNull(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	paths := state.NewPaths(t.TempDir())
	t.Setenv("BB_MACHINE_ID", "machine-a")
	if err := state.SaveConfig(paths, state.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	machine := statusContractMachine(now, "machine-a", []domain.MachineRepoRecord{
		statusContractRepo("software/api", "api", domain.RepoStateSynced, nil, nil, now),
	})
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatal(err)
	}
	got := runStatusJSONContract(t, paths, now)
	want := readJSONDocument(t, filepath.Join("..", "..", "fixtures", "status", "all-synced.json"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status contract = %#v, want %#v", got, want)
	}
}

func TestLatestSyncRunSelectsNewestLocalSync(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	for _, event := range []domain.JournalEvent{
		{At: now.Add(-3 * time.Hour), Machine: "machine-a", Event: "sync_run", Detail: "old"},
		{At: now.Add(-2 * time.Hour), Machine: "machine-a", Event: "pushed", Detail: "not a run"},
		{At: now.Add(-time.Hour), Machine: "machine-a", Event: "sync_run", Detail: "new"},
	} {
		if err := state.AppendJournal(paths, event, 10); err != nil {
			t.Fatal(err)
		}
	}
	got, err := latestSyncRun(paths, "machine-a")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Detail != "new" || !got.At.Equal(now.Add(-time.Hour)) {
		t.Fatalf("latest sync = %#v", got)
	}
}

func TestFleetAttentionFingerprintDeterministicAndEligibilityBoundaries(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	cfg := domain.AttentionConfig{ThrottleMinutes: 17, QuietHours: 2, WIPStaleHours: 24}
	records := []domain.MachineRepoRecordWithMachine{
		{MachineID: "b", Record: statusContractRepo("software/stale", "stale", domain.RepoStateWip, []domain.UnsyncableReason{domain.ReasonDirtyTracked}, nil, now.Add(-24*time.Hour))},
		{MachineID: "a", Record: statusContractRepo("software/recent", "recent", domain.RepoStateBlocked, []domain.UnsyncableReason{domain.ReasonDiverged}, nil, now.Add(-time.Hour))},
		{MachineID: "a", Record: statusContractRepo("software/pending", "pending", domain.RepoStatePending, []domain.UnsyncableReason{domain.ReasonCloneRequired}, nil, time.Time{})},
	}
	first := buildFleetAttention(records, now, cfg)
	if first.ThrottleMinutes != 17 {
		t.Fatalf("throttle_minutes = %d, want 17", first.ThrottleMinutes)
	}
	reversed := []domain.MachineRepoRecordWithMachine{records[2], records[1], records[0]}
	second := buildFleetAttention(reversed, now, cfg)
	if first.Fingerprint == "" || first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprints first=%q second=%q", first.Fingerprint, second.Fingerprint)
	}
	eligibility := map[string]bool{}
	for _, item := range first.Items {
		eligibility[item.RepoKey] = item.Eligible
	}
	if !eligibility["software/stale"] || eligibility["software/recent"] || eligibility["software/pending"] {
		t.Fatalf("eligibility = %#v", eligibility)
	}
	changed := append([]domain.MachineRepoRecordWithMachine(nil), records...)
	changed[0].Record.Reasons = []domain.UnsyncableReason{domain.ReasonDirtyUntracked}
	if got := buildFleetAttention(changed, now, cfg).Fingerprint; got == first.Fingerprint {
		t.Fatalf("fingerprint did not change: %q", got)
	}
	secondaryReasonChange := append([]domain.MachineRepoRecordWithMachine(nil), records...)
	secondaryReasonChange[0].Record.Reasons = []domain.UnsyncableReason{domain.ReasonDirtyTracked, domain.ReasonDirtyUntracked}
	if got := buildFleetAttention(secondaryReasonChange, now, cfg).Fingerprint; got == first.Fingerprint {
		t.Fatalf("fingerprint ignored secondary reason change: %q", got)
	}
}

func TestAttentionEligibilityExactBoundaries(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	cfg := domain.AttentionConfig{QuietHours: 2, WIPStaleHours: 24}
	tests := []struct {
		name     string
		state    domain.RepoSyncState
		activity time.Time
		want     bool
	}{
		{name: "blocked just inside quiet period", state: domain.RepoStateBlocked, activity: now.Add(-2*time.Hour + time.Second), want: false},
		{name: "blocked at quiet boundary", state: domain.RepoStateBlocked, activity: now.Add(-2 * time.Hour), want: true},
		{name: "wip just before stale boundary", state: domain.RepoStateWip, activity: now.Add(-24*time.Hour + time.Second), want: false},
		{name: "wip at stale boundary", state: domain.RepoStateWip, activity: now.Add(-24 * time.Hour), want: true},
		{name: "unknown blocked activity", state: domain.RepoStateBlocked, activity: time.Time{}, want: true},
		{name: "unknown wip activity", state: domain.RepoStateWip, activity: time.Time{}, want: true},
		{name: "pending excluded", state: domain.RepoStatePending, activity: time.Time{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := statusContractRepo("software/repo", "repo", tt.state, []domain.UnsyncableReason{domain.ReasonDirtyTracked}, nil, tt.activity)
			if got := isAttentionEligible(repo, now, cfg); got != tt.want {
				t.Fatalf("eligible = %t, want %t", got, tt.want)
			}
		})
	}
}

func runStatusJSONContract(t *testing.T, paths state.Paths, now time.Time) map[string]any {
	t.Helper()
	var out bytes.Buffer
	a := New(paths, &out, &bytes.Buffer{})
	a.Now = func() time.Time { return now }
	code, err := a.RunStatus(true, nil)
	if err != nil || code != 0 {
		t.Fatalf("status code=%d err=%v", code, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode status: %v\n%s", err, out.String())
	}
	return payload
}

func readJSONDocument(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func statusContractMachine(now time.Time, id string, repos []domain.MachineRepoRecord) domain.MachineFile {
	m := state.BootstrapMachine(id, id+"-host", now)
	m.DefaultCatalog = "software"
	m.Catalogs = []domain.Catalog{{Name: "software", Root: "/tmp/" + id}}
	m.Repos = repos
	return m
}

func statusContractRepo(key, name string, repoState domain.RepoSyncState, reasons, warnings []domain.UnsyncableReason, activity time.Time) domain.MachineRepoRecord {
	return domain.MachineRepoRecord{RepoKey: key, Name: name, Catalog: "software", Path: "/repos/" + name, State: repoState, Reasons: reasons, Warnings: warnings, LastActivityAt: activity}
}
