package app

import (
	"io"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestEnsureFromWinnersMarksCatalogMismatchForPreviousRepoKey(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 11, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }

	softwareRoot := filepath.Join(home, "software")
	referencesRoot := filepath.Join(home, "references")
	machine := state.BootstrapMachine("local", "local", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: softwareRoot, RepoPathDepth: 1},
		{Name: "references", Root: referencesRoot, RepoPathDepth: 1},
	}
	machine.Repos = []domain.MachineRepoRecord{
		{
			RepoKey:   "software/api",
			Name:      "api",
			Catalog:   "software",
			Path:      filepath.Join(softwareRoot, "api"),
			OriginURL: "https://github.com/you/api.git",
			Syncable:  true,
		},
	}

	meta := domain.RepoMetadataFile{
		RepoKey:          "references/api",
		PreviousRepoKeys: []string{"software/api"},
		Name:             "api",
		OriginURL:        "https://github.com/you/api.git",
	}
	allMachines := []domain.MachineFile{
		{
			MachineID: "remote",
			Repos: []domain.MachineRepoRecord{{
				RepoKey:   "references/api",
				Name:      "api",
				Catalog:   "references",
				Path:      "/remote/references/api",
				OriginURL: "https://github.com/you/api.git",
				Branch:    "main",
				Syncable:  true,
			}},
		},
	}

	selectedCatalogMap := map[string]domain.Catalog{
		"software":   machine.Catalogs[0],
		"references": machine.Catalogs[1],
	}

	err := app.ensureFromWinners(
		domain.ConfigFile{},
		&machine,
		allMachines,
		[]domain.RepoMetadataFile{meta},
		selectedCatalogMap,
		nil,
		SyncOptions{},
	)
	if err != nil {
		t.Fatalf("ensureFromWinners error: %v", err)
	}

	rec := machine.Repos[0]
	if rec.Syncable {
		t.Fatal("expected mismatch repo to be unsyncable")
	}
	if !slices.Contains(rec.UnsyncableReasons, domain.ReasonCatalogMismatch) {
		t.Fatalf("unsyncable reasons = %v, want catalog_mismatch", rec.UnsyncableReasons)
	}
	if rec.ExpectedRepoKey != "references/api" {
		t.Fatalf("expected_repo_key = %q, want references/api", rec.ExpectedRepoKey)
	}
	if rec.ExpectedCatalog != "references" {
		t.Fatalf("expected_catalog = %q, want references", rec.ExpectedCatalog)
	}
	if rec.ExpectedPath != filepath.Join(referencesRoot, "api") {
		t.Fatalf("expected_path = %q, want %q", rec.ExpectedPath, filepath.Join(referencesRoot, "api"))
	}
}

func TestEnsureFromWinnersSkipsCloneRequiredForMovedRepoWhenNeverCloned(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 11, 5, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }

	referencesRoot := filepath.Join(home, "references")
	machine := state.BootstrapMachine("local", "local", now)
	machine.DefaultCatalog = "references"
	machine.Catalogs = []domain.Catalog{{Name: "references", Root: referencesRoot, RepoPathDepth: 1}}
	machine.Repos = nil

	meta := domain.RepoMetadataFile{
		RepoKey:          "references/api",
		PreviousRepoKeys: []string{"software/api"},
		Name:             "api",
		OriginURL:        "https://github.com/you/api.git",
	}
	allMachines := []domain.MachineFile{{
		MachineID: "remote",
		Repos: []domain.MachineRepoRecord{{
			RepoKey:   "references/api",
			Name:      "api",
			Catalog:   "references",
			Path:      "/remote/references/api",
			OriginURL: "https://github.com/you/api.git",
			Branch:    "main",
			Syncable:  true,
		}},
	}}

	selectedCatalogMap := map[string]domain.Catalog{"references": machine.Catalogs[0]}

	err := app.ensureFromWinners(
		domain.ConfigFile{},
		&machine,
		allMachines,
		[]domain.RepoMetadataFile{meta},
		selectedCatalogMap,
		nil,
		SyncOptions{},
	)
	if err != nil {
		t.Fatalf("ensureFromWinners error: %v", err)
	}
	if len(machine.Repos) != 0 {
		t.Fatalf("repos = %v, want none", machine.Repos)
	}
}

func TestEnsureFromWinnersMarksCatalogNotMappedWhenMoveTargetCatalogMissing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 11, 10, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }

	softwareRoot := filepath.Join(home, "software")
	machine := state.BootstrapMachine("local", "local", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: softwareRoot, RepoPathDepth: 1}}
	machine.Repos = []domain.MachineRepoRecord{{
		RepoKey:   "software/api",
		Name:      "api",
		Catalog:   "software",
		Path:      filepath.Join(softwareRoot, "api"),
		OriginURL: "https://github.com/you/api.git",
		Syncable:  true,
	}}

	meta := domain.RepoMetadataFile{
		RepoKey:          "references/api",
		PreviousRepoKeys: []string{"software/api"},
		Name:             "api",
		OriginURL:        "https://github.com/you/api.git",
	}
	allMachines := []domain.MachineFile{{
		MachineID: "remote",
		Repos: []domain.MachineRepoRecord{{
			RepoKey:   "references/api",
			Name:      "api",
			Catalog:   "references",
			Path:      "/remote/references/api",
			OriginURL: "https://github.com/you/api.git",
			Branch:    "main",
			Syncable:  true,
		}},
	}}

	err := app.ensureFromWinners(
		domain.ConfigFile{},
		&machine,
		allMachines,
		[]domain.RepoMetadataFile{meta},
		map[string]domain.Catalog{"software": machine.Catalogs[0]},
		nil,
		SyncOptions{},
	)
	if err != nil {
		t.Fatalf("ensureFromWinners error: %v", err)
	}

	rec := machine.Repos[0]
	if !slices.Contains(rec.UnsyncableReasons, domain.ReasonCatalogMismatch) {
		t.Fatalf("unsyncable reasons = %v, want catalog_mismatch", rec.UnsyncableReasons)
	}
	if !slices.Contains(rec.UnsyncableReasons, domain.ReasonCatalogNotMapped) {
		t.Fatalf("unsyncable reasons = %v, want catalog_not_mapped", rec.UnsyncableReasons)
	}
	if rec.ExpectedRepoKey != "references/api" {
		t.Fatalf("expected_repo_key = %q, want references/api", rec.ExpectedRepoKey)
	}
	if rec.ExpectedCatalog != "references" {
		t.Fatalf("expected_catalog = %q, want references", rec.ExpectedCatalog)
	}
}
