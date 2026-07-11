package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestStatusSummaryAndDoctorTierGroups(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	paths := state.NewPaths(t.TempDir())
	t.Setenv("BB_MACHINE_ID", "machine-a")
	cfg := state.DefaultConfig()
	cfg.Sync.ScanFreshnessSeconds = 3600
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}
	m := state.BootstrapMachine("machine-a", "host", now)
	m.DefaultCatalog = "software"
	m.Catalogs = []domain.Catalog{{Name: "software", Root: t.TempDir()}}
	m.LastScanAt = now
	m.LastScanCatalogs = []string{"software"}
	m.Repos = []domain.MachineRepoRecord{
		{Name: "blocked", Catalog: "software", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}},
		{Name: "stale", Catalog: "software", State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-25 * time.Hour)},
		{Name: "fresh", Catalog: "software", State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-time.Hour)},
		{Name: "pending", Catalog: "software", State: domain.RepoStatePending, Reasons: []domain.UnsyncableReason{domain.ReasonCloneRequired}},
		{Name: "synced", Catalog: "software", State: domain.RepoStateSynced},
		{Name: "warn", Catalog: "software", State: domain.RepoStateSynced, Warnings: []domain.UnsyncableReason{domain.ReasonRemoteFormatMismatch}},
	}
	if err := state.SaveMachine(paths, m); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	a := New(paths, &out, &bytes.Buffer{})
	a.Now = func() time.Time { return now }
	a.Hostname = func() (string, error) { return "host", nil }
	code, err := a.RunStatus(false, nil)
	if err != nil || code != 0 {
		t.Fatalf("status code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "6 repos: 2 synced · 1 pending · 2 wip · 1 blocked · 1 warnings\n") {
		t.Fatalf("status:\n%s", out.String())
	}
	out.Reset()
	code, err = a.RunDoctor(nil)
	if err != nil || code != 1 {
		t.Fatalf("doctor code=%d err=%v", code, err)
	}
	want := "blocked:\n  blocked: [diverged]\nstale wip:\n  stale: [dirty_tracked]\npending:\n  pending: [clone_required]\nwarnings:\n  warn: [] [remote_format_mismatch]\n"
	if out.String() != want {
		t.Fatalf("doctor:\n%s\nwant:\n%s", out.String(), want)
	}
}
