package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
		{"repository_started", "software/web", intPointer(0), intPointer(2)},
		{"progress", "software/web", intPointer(0), intPointer(2)},
		{"progress", "software/web", intPointer(0), intPointer(2)},
		{"repository_finished", "software/api", intPointer(1), intPointer(2)},
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
	if events[8].Result != "failure" || !strings.Contains(events[8].Error, "pull_failed") {
		t.Fatalf("api finish = %+v", events[8])
	}
}

func TestTargetedCloneRequiredEmitsFailureLifecycleWhenAutoCloneDisabled(t *testing.T) {
	events, _, code, err := runCloneSync(t, false, false, true)
	if err != nil || code != 0 {
		t.Fatalf("sync = code %d err %v", code, err)
	}
	assertTargetedCloneLifecycle(t, events, "failure")
}

func TestTargetedCloneRequiredEmitsSuccessfulCloneLifecycle(t *testing.T) {
	events, target, code, err := runCloneSync(t, true, true, true)
	if err != nil || code != 0 {
		t.Fatalf("sync = code %d err %v", code, err)
	}
	assertTargetedCloneLifecycle(t, events, "success")
	if _, err := os.Stat(filepath.Join(target, "README.md")); err != nil {
		t.Fatalf("cloned repository unavailable: %v", err)
	}
}

func TestGlobalCloneReconciliationCompletesBeforeRepositoryFinish(t *testing.T) {
	events, target, code, err := runCloneSync(t, true, true, false)
	if err != nil || code != 0 {
		t.Fatalf("sync = code %d err %v", code, err)
	}
	assertTargetedCloneLifecycle(t, events, "success")
	finishedIndex := len(events) - 2
	cloneIndex := -1
	for i, event := range events {
		if event.Phase == "clone" {
			cloneIndex = i
		}
	}
	if cloneIndex < 0 || cloneIndex >= finishedIndex {
		t.Fatalf("clone/finish order = %+v", events)
	}
	if _, err := os.Stat(filepath.Join(target, "README.md")); err != nil {
		t.Fatal(err)
	}
}

func runCloneSync(t *testing.T, autoClone, validOrigin, targeted bool) ([]OperationEvent, string, int, error) {
	t.Helper()
	home := t.TempDir()
	paths := state.NewPaths(home)
	root := filepath.Join(home, "software")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	autoCloneValue := autoClone
	local := state.BootstrapMachine("local", "local", now)
	local.DefaultCatalog = "software"
	local.Catalogs = []domain.Catalog{{Name: "software", Root: root, RepoPathDepth: 1, AutoCloneOnSync: &autoCloneValue}}
	local.Repos = []domain.MachineRepoRecord{{RepoKey: "software/api", Name: "api", Catalog: "software", Path: filepath.Join(root, "api"), State: domain.RepoStatePending, Reasons: []domain.UnsyncableReason{domain.ReasonCloneRequired}}}
	if err := state.SaveMachine(paths, local); err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	a := New(paths, stdout, &bytes.Buffer{})
	origin := filepath.Join(home, "api.git")
	if validOrigin {
		if _, err := a.Git.RunGit(home, "init", "--bare", origin); err != nil {
			t.Fatal(err)
		}
		seed := filepath.Join(home, "seed")
		if err := os.MkdirAll(seed, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := a.Git.InitRepo(seed); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("api\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := a.Git.AddAll(seed); err != nil {
			t.Fatal(err)
		}
		if err := a.Git.Commit(seed, "initial"); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Git.RunGit(seed, "branch", "-M", "main"); err != nil {
			t.Fatal(err)
		}
		if err := a.Git.AddOrigin(seed, origin); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Git.RunGit(seed, "push", "-u", "origin", "main"); err != nil {
			t.Fatal(err)
		}
	}
	remote := state.BootstrapMachine("remote", "remote", now)
	remote.Repos = []domain.MachineRepoRecord{{RepoKey: "software/api", Name: "api", Catalog: "software", Path: "/remote/api", OriginURL: origin, Branch: "main", State: domain.RepoStateSynced}}
	if err := state.SaveMachine(paths, remote); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveRepoMetadata(paths, domain.RepoMetadataFile{RepoKey: "software/api", Name: "api", OriginURL: origin}); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveConfig(paths, state.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BB_MACHINE_ID", "local")
	opts := SyncOptions{EventsJSON: true}
	if targeted {
		opts.Repository = "software/api"
	}
	code, err := a.runSync(opts)
	var events []OperationEvent
	decoder := json.NewDecoder(stdout)
	for decoder.More() {
		var event OperationEvent
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events, filepath.Join(root, "api"), code, err
}

func assertTargetedCloneLifecycle(t *testing.T, events []OperationEvent, result string) {
	t.Helper()
	if len(events) < 7 || events[0].Event != "operation_started" || events[1].Phase != "discover" {
		t.Fatalf("events = %+v", events)
	}
	foundClone := false
	for _, event := range events {
		if event.Repository == "software/api" && event.Phase == "clone" {
			foundClone = true
		}
	}
	if !foundClone {
		t.Fatalf("missing clone progress: %+v", events)
	}
	finished := events[len(events)-2]
	if finished.Event != "repository_finished" || finished.Repository != "software/api" || finished.Result != result || finished.Completed == nil || *finished.Completed != 1 || finished.Total == nil || *finished.Total != 1 {
		t.Fatalf("finish = %+v", finished)
	}
	if events[len(events)-1].Event != "operation_finished" {
		t.Fatalf("terminal event = %+v", events[len(events)-1])
	}
}

func intPointer(value int) *int { return &value }

func equalIntPointers(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestObserveHardErrorEmitsAttributedRepositoryFailure(t *testing.T) {
	stdout := &bytes.Buffer{}
	a := New(state.NewPaths(t.TempDir()), stdout, &bytes.Buffer{})
	repoPath := t.TempDir()
	progress := newSyncOperationProgress(a, true, 1)
	_, _, err := a.observePhase(domain.ConfigFile{}, []discoveredRepo{{RepoKey: "software/api", Name: "api", Path: repoPath, Catalog: domain.Catalog{Name: "software"}}}, nil, SyncOptions{EventsJSON: true, progress: progress})
	if err == nil {
		t.Fatal("expected observation error")
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
	finished := events[len(events)-1]
	if finished.Event != "repository_finished" || finished.Repository != "software/api" || finished.Result != "failure" || finished.Error == "" || finished.Completed == nil || *finished.Completed != 1 {
		t.Fatalf("finish = %+v", finished)
	}
}

func TestReconcileHardErrorEmitsAttributedRepositoryFailure(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "software")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "api")
	if err := os.Symlink(target, target); err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	a := New(state.NewPaths(home), stdout, &bytes.Buffer{})
	progress := newSyncOperationProgress(a, true, 1)
	meta := domain.RepoMetadataFile{RepoKey: "software/api", Name: "api", OriginURL: "https://example.com/api.git"}
	machine := domain.MachineFile{MachineID: "local"}
	remote := domain.MachineFile{MachineID: "remote", Repos: []domain.MachineRepoRecord{{RepoKey: "software/api", OriginURL: meta.OriginURL, Branch: "main", State: domain.RepoStateSynced}}}
	err := a.ensureFromWinners(domain.ConfigFile{}, &machine, []domain.MachineFile{remote}, []domain.RepoMetadataFile{meta}, map[string]domain.Catalog{"software": {Name: "software", Root: root}}, nil, SyncOptions{EventsJSON: true, progress: progress})
	if err == nil {
		t.Fatal("expected reconciliation error")
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
	finished := events[len(events)-1]
	if finished.Event != "repository_finished" || finished.Repository != "software/api" || finished.Result != "failure" || finished.Error == "" || finished.Completed == nil || *finished.Completed != 1 {
		t.Fatalf("finish = %+v", finished)
	}
}

func TestPlannedSyncRepositoriesExcludesSkippedMetadata(t *testing.T) {
	discovered := []discoveredRepo{{RepoKey: "software/local"}}
	metas := []domain.RepoMetadataFile{
		{RepoKey: "software/current", OriginURL: "current.git", PreviousRepoKeys: []string{"software/historical"}},
		{RepoKey: "software/historical", OriginURL: "old.git"},
		{RepoKey: "software/empty-origin"},
		{RepoKey: "software/no-winner", OriginURL: "missing.git"},
	}
	machines := []domain.MachineFile{{MachineID: "remote", Repos: []domain.MachineRepoRecord{{RepoKey: "software/current", State: domain.RepoStateSynced}}}}
	got := plannedSyncRepositories(discovered, machines, metas, map[string]domain.Catalog{"software": {Name: "software"}}, "")
	if want := []string{"software/current", "software/local"}; !slices.Equal(got, want) {
		t.Fatalf("planned = %v, want %v", got, want)
	}
	stdout := &bytes.Buffer{}
	a := New(state.NewPaths(t.TempDir()), stdout, &bytes.Buffer{})
	machine := domain.MachineFile{MachineID: "local"}
	noWinner := domain.RepoMetadataFile{RepoKey: "software/no-winner", OriginURL: "missing.git"}
	progress := newSyncOperationProgress(a, true, 0)
	if err := a.ensureFromWinners(domain.ConfigFile{}, &machine, nil, []domain.RepoMetadataFile{noWinner}, map[string]domain.Catalog{"software": {Name: "software", Root: t.TempDir()}}, nil, SyncOptions{EventsJSON: true, progress: progress}); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || progress.completed != 0 {
		t.Fatalf("skipped metadata emitted phantom lifecycle: %q", stdout.String())
	}
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
