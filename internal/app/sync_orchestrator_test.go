package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunSyncEmitsOrderedCountedRepositoryEvents(t *testing.T) {
	home := t.TempDir()
	paths := state.NewPaths(home)
	root := filepath.Join(home, "software")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	a := New(paths, stdout, &bytes.Buffer{})
	for _, name := range []string{"api", "web"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := a.Git.InitRepo(path); err != nil {
			t.Fatal(err)
		}
		if err := a.Git.AddOrigin(path, filepath.Join(home, "missing", name+".git")); err != nil {
			t.Fatal(err)
		}
	}
	cfg := state.DefaultConfig()
	cfg.Sync.FetchPrune = true
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	machine := state.BootstrapMachine("test-machine", "test-machine", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: root, RepoPathDepth: 1}}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BB_MACHINE_ID", "test-machine")

	code, err := a.runSync(SyncOptions{EventsJSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want attention exit 1", code)
	}
	var events []OperationEvent
	decoder := json.NewDecoder(stdout)
	for decoder.More() {
		var event OperationEvent
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	want := []struct {
		event, repository string
		completed, total  *int
	}{
		{"operation_started", "", nil, nil},
		{"progress", "", intPointer(0), intPointer(2)},
		{"repository_started", "software/api", intPointer(0), intPointer(2)},
		{"progress", "software/api", intPointer(0), intPointer(2)},
		{"progress", "software/api", intPointer(0), intPointer(2)},
		{"repository_finished", "software/api", intPointer(1), intPointer(2)},
		{"repository_started", "software/web", intPointer(1), intPointer(2)},
		{"progress", "software/web", intPointer(1), intPointer(2)},
		{"progress", "software/web", intPointer(1), intPointer(2)},
		{"repository_finished", "software/web", intPointer(2), intPointer(2)},
		{"operation_finished", "", intPointer(2), intPointer(2)},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v", events)
	}
	for i, expected := range want {
		got := events[i]
		if got.Event != expected.event || got.Repository != expected.repository || !equalIntPointers(got.Completed, expected.completed) || !equalIntPointers(got.Total, expected.total) {
			t.Fatalf("event[%d] = %+v, want %+v", i, got, expected)
		}
	}
	if events[5].Result != "failure" || !strings.Contains(events[5].Error, "pull_failed") {
		t.Fatalf("api finish = %+v", events[5])
	}
}

func intPointer(value int) *int { return &value }

func equalIntPointers(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestAnyUnsyncableInSelectedCatalogsIgnoresNonBlockingReasons(t *testing.T) {
	t.Parallel()

	selected := map[string]domain.Catalog{
		"software": {Name: "software"},
	}
	repos := []domain.MachineRepoRecord{
		{
			Catalog: "software",
			State:   domain.RepoStatePending,
			Reasons: []domain.UnsyncableReason{domain.ReasonCloneRequired, domain.ReasonCatalogMismatch},
		},
	}

	if anyUnsyncableInSelectedCatalogs(repos, selected) {
		t.Fatal("expected clone_required to be non-blocking for sync exit semantics")
	}
}

func TestLoadSyncReconcileInputsSkipsV1Publisher(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	now := time.Now()
	old := state.BootstrapMachine("old-mac", "old-mac", now)
	old.Version = 1
	if err := state.SaveYAML(paths.MachinePath(old.MachineID), old); err != nil {
		t.Fatal(err)
	}
	current := state.BootstrapMachine("new-mac", "new-mac", now)
	if err := state.SaveMachine(paths, current); err != nil {
		t.Fatal(err)
	}
	machines, _, err := loadSyncReconcileInputs(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 || machines[0].MachineID != "new-mac" {
		t.Fatalf("machines = %+v", machines)
	}
}

func TestAnyUnsyncableInSelectedCatalogsIgnoresWipReasons(t *testing.T) {
	t.Parallel()

	selected := map[string]domain.Catalog{
		"software": {Name: "software"},
	}
	repos := []domain.MachineRepoRecord{
		{
			Catalog: "software",
			State:   domain.RepoStateWip,
			Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
		},
	}

	if anyUnsyncableInSelectedCatalogs(repos, selected) {
		t.Fatal("expected dirty_tracked wip not to block")
	}
}

func TestAnyUnsyncableInScopeOnlyChecksSelectedRepository(t *testing.T) {
	t.Parallel()
	catalogs := map[string]domain.Catalog{"software": {Name: "software"}}
	repos := []domain.MachineRepoRecord{
		{RepoKey: "software/blocked", Catalog: "software", State: domain.RepoStateBlocked},
		{RepoKey: "software/ready", Catalog: "software", State: domain.RepoStatePending},
	}
	if anyUnsyncableInScope(repos, catalogs, "software/ready") {
		t.Fatal("unselected blocked repository affected targeted result")
	}
	if !anyUnsyncableInScope(repos, catalogs, "software/blocked") {
		t.Fatal("selected blocked repository was ignored")
	}
}

func TestOperationEventsAreJSONLines(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	a := New(state.NewPaths(t.TempDir()), stdout, &bytes.Buffer{})
	a.emitOperationEvent(true, OperationEvent{Event: "progress", Operation: "sync", Repository: "software/api", Phase: "fetch", Message: "Fetching origin"})
	if got := stdout.String(); !strings.HasSuffix(got, "\n") || !strings.Contains(got, `"repository":"software/api"`) || !strings.Contains(got, `"phase":"fetch"`) {
		t.Fatalf("event = %q", got)
	}
}

func TestRepositoryFailureDetailIncludesReasons(t *testing.T) {
	t.Parallel()
	got := repositoryFailureDetail([]domain.MachineRepoRecord{{RepoKey: "software/api", Reasons: []domain.UnsyncableReason{domain.ReasonPullFailed}}}, "software/api")
	if got != "repository remains blocked: pull_failed" {
		t.Fatalf("detail = %q", got)
	}
}

func TestRunSyncIncludeCatalogWarnsWhenCatalogOnlyExistsRemotely(t *testing.T) {
	home := t.TempDir()
	paths := state.NewPaths(home)
	now := time.Date(2026, 2, 16, 11, 0, 0, 0, time.UTC)

	cfg := state.DefaultConfig()
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	localMachine := state.BootstrapMachine("local", "host-local", now)
	localMachine.DefaultCatalog = "software"
	localMachine.Catalogs = []domain.Catalog{
		{Name: "software", Root: filepath.Join(home, "software")},
	}
	if err := state.SaveMachine(paths, localMachine); err != nil {
		t.Fatalf("save local machine: %v", err)
	}

	remoteMachine := state.BootstrapMachine("remote", "host-remote", now)
	remoteMachine.DefaultCatalog = "references"
	remoteMachine.Catalogs = []domain.Catalog{
		{Name: "references", Root: "/Volumes/Projects/References"},
	}
	if err := state.SaveMachine(paths, remoteMachine); err != nil {
		t.Fatalf("save remote machine: %v", err)
	}

	t.Setenv("BB_MACHINE_ID", "local")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := New(paths, stdout, stderr)
	app.Now = func() time.Time { return now }

	code, err := app.runSync(SyncOptions{
		IncludeCatalogs: []string{"references"},
	})
	if err == nil {
		t.Fatalf("expected sync to fail catalog selection, code=%d", code)
	}
	if !strings.Contains(err.Error(), "known on other machines") {
		t.Fatalf("expected remote-known catalog hint, err=%v", err)
	}
	if !strings.Contains(err.Error(), "bb config") {
		t.Fatalf("expected bb config remediation in error, err=%v", err)
	}
}
