package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestOverviewPlainAndJSONContract(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	cfg := state.DefaultConfig()
	cfg.Overview.MachineStaleDays = 3
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}
	local := state.BootstrapMachine("local", "local", now)
	local.Repos = []domain.MachineRepoRecord{{RepoKey: "software/clean", State: domain.RepoStateSynced}, {RepoKey: "software/work", State: domain.RepoStateSynced}, {RepoKey: "software/blocked", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked, domain.ReasonDiverged}, LastActivityAt: now.Add(-2 * time.Hour)}}
	local.UpdatedAt = now
	remote := state.BootstrapMachine("remote", "remote", now.Add(-3*24*time.Hour))
	remote.Repos = []domain.MachineRepoRecord{{RepoKey: "software/clean", State: domain.RepoStateSynced}, {RepoKey: "software/work", State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-6 * time.Hour)}}
	remote.UpdatedAt = now.Add(-3 * 24 * time.Hour)
	if err := state.SaveMachine(paths, local); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveMachine(paths, remote); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(state.NewPaths(paths.Home).LocalStateRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MachineIDPath(), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	a := New(paths, &out, &bytes.Buffer{})
	a.Now = func() time.Time { return now }
	code, err := a.RunOverview(OverviewOptions{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	plain := out.String()
	wantPlain := "remote last published 3d ago — its data may be stale.\nsoftware/blocked   here: blocked (diverged · 2h ago)   remote: — (not cloned)\nsoftware/work   here: synced   remote: wip (dirty_tracked · 6h ago)\nsynced everywhere: 1 repos (--all to list)\n"
	if plain != wantPlain {
		t.Fatalf("plain:\n%s\nwant:\n%s", plain, wantPlain)
	}
	out.Reset()
	code, err = a.RunOverview(OverviewOptions{JSON: true})
	if err != nil || code != 0 {
		t.Fatal(err)
	}
	var matrix OverviewMatrix
	if err := json.Unmarshal(out.Bytes(), &matrix); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(matrix.Machines) != 2 || !matrix.Machines[0].Here || len(matrix.Repos) != 3 || matrix.SyncedEverywhere != 1 {
		t.Fatalf("matrix=%+v", matrix)
	}
	if !strings.Contains(out.String(), `"last_activity_at"`) {
		t.Fatalf("contract missing timestamps: %s", out.String())
	}
	var contract map[string]any
	if err := json.Unmarshal(out.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, contract, []string{"machines", "repos", "synced_everywhere", "warnings"})
	machine := contract["machines"].([]any)[0].(map[string]any)
	assertJSONKeys(t, machine, []string{"here", "id", "published", "stale", "updated_at"})
	repo := contract["repos"].([]any)[0].(map[string]any)
	assertJSONKeys(t, repo, []string{"cells", "repo_key", "synced_everywhere"})
	cell := repo["cells"].([]any)[0].(map[string]any)
	assertJSONKeys(t, cell, []string{"last_activity_at", "machine_id", "present", "reasons", "state", "warnings"})
	for _, rawRepo := range contract["repos"].([]any) {
		for _, rawCell := range rawRepo.(map[string]any)["cells"].([]any) {
			cell := rawCell.(map[string]any)
			if cell["reasons"] == nil || cell["warnings"] == nil {
				t.Fatalf("overview array fields must encode as arrays, cell=%#v", cell)
			}
		}
	}
	out.Reset()
	if code, err := a.RunOverview(OverviewOptions{All: true}); err != nil || code != 0 {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "software/clean") {
		t.Fatalf("--all did not expand synced row: %s", out.String())
	}
	out.Reset()
	if code, err := a.RunOverview(OverviewOptions{IncludeCatalogs: []string{"references"}}); err != nil || code != 0 {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "software/") {
		t.Fatalf("catalog filter leaked rows: %s", out.String())
	}
}

func TestOverviewAlwaysZeroWithBlockedAndDoesNotWrite(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	if err := state.SaveConfig(paths, state.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	m := state.BootstrapMachine("m", "m", time.Now())
	m.Repos = []domain.MachineRepoRecord{{RepoKey: "software/x", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}}}
	if err := state.SaveMachine(paths, m); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(paths.MachinePath("m"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	code, err := a.RunOverview(OverviewOptions{})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	after, _ := os.Stat(paths.MachinePath("m"))
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("overview wrote machine state")
	}
}

func TestOverviewMissingLocalPublicationStillHasHereColumnWithoutCreatingConfig(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	remote := state.BootstrapMachine("remote", "remote", time.Now())
	remote.Repos = []domain.MachineRepoRecord{{RepoKey: "software/x", State: domain.RepoStateSynced}}
	if err := state.SaveMachine(paths, remote); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.LocalStateRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MachineIDPath(), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	a := New(paths, &out, &bytes.Buffer{})
	if code, err := a.RunOverview(OverviewOptions{All: true}); err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "here: — (not cloned)") {
		t.Fatalf("missing here column: %s", out.String())
	}
	if strings.Contains(out.String(), "last published") {
		t.Fatalf("unpublished local rendered stale age: %s", out.String())
	}
	if _, err := os.Stat(paths.ConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("overview created config: %v", err)
	}
}

func TestOverviewFailsOnMalformedRepoMetadata(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	if err := state.SaveConfig(paths, state.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.RepoDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.RepoDir(), state.RepoMetaFileName("software/x")), []byte("version: [broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	if code, err := a.RunOverview(OverviewOptions{}); err == nil || code != 2 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestOverviewConfigRejectsNonPositiveStaleDays(t *testing.T) {
	t.Parallel()
	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	cfg.Overview.MachineStaleDays = 0
	if err := validateConfigForSave(cfg); err == nil || !strings.Contains(err.Error(), "overview.machine_stale_days") {
		t.Fatalf("err=%v", err)
	}
}

func TestOverviewRejectsInvalidRuntimeStaleDays(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	cfg := state.DefaultConfig()
	cfg.Overview.MachineStaleDays = 0
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	if code, err := a.RunOverview(OverviewOptions{}); err == nil || code != 2 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func assertJSONKeys(t *testing.T, value map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(value))
	for k := range value {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("keys=%v want=%v", got, want)
	}
}

func TestOverviewSurfacesOldPublisherWarning(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	old := state.BootstrapMachine("old", "old", time.Now())
	old.Version = 1
	if err := state.SaveYAML(paths.MachinePath("old"), old); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	a := New(paths, &out, &bytes.Buffer{})
	if code, err := a.RunOverview(OverviewOptions{JSON: true}); err != nil || code != 0 {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "machine old publishes old-format state version 1") {
		t.Fatalf("warning missing: %s", out.String())
	}
}

func TestOverviewCatalogFilterIncludesMetadataOnlyRepo(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	for _, key := range []string{"software/app", "references/lib"} {
		meta := domain.RepoMetadataFile{Version: 1, RepoKey: key, Name: key}
		if err := state.SaveRepoMetadata(paths, meta); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	a := New(paths, &out, &bytes.Buffer{})
	if code, err := a.RunOverview(OverviewOptions{JSON: true, IncludeCatalogs: []string{"references"}}); err != nil || code != 0 {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "references/lib") || strings.Contains(out.String(), "software/app") {
		t.Fatalf("filtered JSON: %s", out.String())
	}
}
