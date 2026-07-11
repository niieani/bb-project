package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

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
