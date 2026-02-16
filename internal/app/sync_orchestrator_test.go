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
			Catalog:           "software",
			Syncable:          false,
			UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonCloneRequired},
		},
	}

	if anyUnsyncableInSelectedCatalogs(repos, selected) {
		t.Fatal("expected clone_required to be non-blocking for sync exit semantics")
	}
}

func TestAnyUnsyncableInSelectedCatalogsBlocksOnTraditionalReasons(t *testing.T) {
	t.Parallel()

	selected := map[string]domain.Catalog{
		"software": {Name: "software"},
	}
	repos := []domain.MachineRepoRecord{
		{
			Catalog:           "software",
			Syncable:          false,
			UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
		},
	}

	if !anyUnsyncableInSelectedCatalogs(repos, selected) {
		t.Fatal("expected dirty_tracked to remain blocking")
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
