package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestApplyFixActionWithObserverSkipsWholeCatalogRefreshForStaleSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 15, 12, 0, 0, 0, time.UTC)
	app, paths := newFixApplyTestApp(t, now)

	catalogRoot := filepath.Join(paths.Home, "catalog")
	apiPath := filepath.Join(catalogRoot, "api")
	webPath := filepath.Join(catalogRoot, "web")
	for _, gitDir := range []string{
		filepath.Join(apiPath, ".git"),
		filepath.Join(webPath, ".git"),
	} {
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", gitDir, err)
		}
	}

	apiRecord := fixApplyTestRepoRecord("software/api", "api", "software", apiPath, "https://github.com/you/api.git")
	webRecord := fixApplyTestRepoRecord("software/web", "web", "software", webPath, "https://github.com/you/web.git")
	writeFixApplyTestMachine(t, paths, now.Add(-2*time.Minute), catalogRoot, apiRecord, webRecord)

	writeFixApplyTestMetadata(t, paths, apiRecord.RepoKey, apiRecord.Name, apiRecord.OriginURL)
	writeFixApplyTestMetadata(t, paths, webRecord.RepoKey, webRecord.Name, webRecord.OriginURL)

	var mu sync.Mutex
	observedPaths := make([]string, 0, 4)
	app.observeRepoHook = func(_ domain.ConfigFile, repo discoveredRepo, _ bool) (domain.MachineRepoRecord, error) {
		mu.Lock()
		observedPaths = append(observedPaths, filepath.Clean(repo.Path))
		mu.Unlock()

		if filepath.Clean(repo.Path) == filepath.Clean(webPath) {
			return domain.MachineRepoRecord{}, errors.New("web repository should not be observed during targeted apply")
		}
		return apiRecord, nil
	}

	updated, err := app.applyFixActionWithObserver(
		[]string{"software"},
		apiPath,
		FixActionEnableAutoPush,
		fixApplyOptions{Interactive: true},
		nil,
	)
	if err != nil {
		t.Fatalf("applyFixActionWithObserver failed: %v", err)
	}
	if filepath.Clean(updated.Record.Path) != filepath.Clean(apiPath) {
		t.Fatalf("updated repo path = %q, want %q", updated.Record.Path, apiPath)
	}

	meta, err := state.LoadRepoMetadata(paths, apiRecord.RepoKey)
	if err != nil {
		t.Fatalf("load repo metadata failed: %v", err)
	}
	if meta.AutoPush != domain.AutoPushModeIncludeDefaultBranch {
		t.Fatalf("auto_push = %q, want %q", meta.AutoPush, domain.AutoPushModeIncludeDefaultBranch)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, observed := range observedPaths {
		if observed == filepath.Clean(webPath) {
			t.Fatalf("unexpected non-target repo observation: %v", observedPaths)
		}
	}
}

func TestApplyFixActionWithObserverLoadsOnlyTargetRepoState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 15, 12, 30, 0, 0, time.UTC)
	app, paths := newFixApplyTestApp(t, now)

	catalogRoot := filepath.Join(paths.Home, "catalog")
	apiPath := filepath.Join(catalogRoot, "api")
	webPath := filepath.Join(catalogRoot, "web")
	for _, repoPath := range []string{apiPath, webPath} {
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", repoPath, err)
		}
	}

	apiRecord := fixApplyTestRepoRecord("software/api", "api", "software", apiPath, "https://github.com/you/api.git")
	webRecord := fixApplyTestRepoRecord("software/web", "web", "software", webPath, "https://github.com/you/web.git")
	writeFixApplyTestMachine(t, paths, now, catalogRoot, apiRecord, webRecord)

	writeFixApplyTestMetadata(t, paths, apiRecord.RepoKey, apiRecord.Name, apiRecord.OriginURL)
	if err := os.MkdirAll(paths.RepoDir(), 0o755); err != nil {
		t.Fatalf("mkdir repo metadata dir failed: %v", err)
	}
	if err := os.WriteFile(state.RepoMetaPath(paths, webRecord.RepoKey), []byte("repo_key: [\n"), 0o644); err != nil {
		t.Fatalf("write invalid metadata failed: %v", err)
	}

	app.observeRepoHook = func(_ domain.ConfigFile, repo discoveredRepo, _ bool) (domain.MachineRepoRecord, error) {
		if filepath.Clean(repo.Path) == filepath.Clean(webPath) {
			return domain.MachineRepoRecord{}, errors.New("web repository should not be observed during targeted apply")
		}
		return apiRecord, nil
	}

	updated, err := app.applyFixActionWithObserver(
		[]string{"software"},
		apiPath,
		FixActionEnableAutoPush,
		fixApplyOptions{Interactive: true},
		nil,
	)
	if err != nil {
		t.Fatalf("applyFixActionWithObserver failed: %v", err)
	}
	if filepath.Clean(updated.Record.Path) != filepath.Clean(apiPath) {
		t.Fatalf("updated repo path = %q, want %q", updated.Record.Path, apiPath)
	}
}

func TestApplyFixActionMoveToCatalog(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 15, 13, 0, 0, 0, time.UTC)
	app, paths := newFixApplyTestApp(t, now)

	softwareRoot := filepath.Join(paths.Home, "software")
	referencesRoot := filepath.Join(paths.Home, "references")
	oldPath := filepath.Join(softwareRoot, "api")
	newPath := filepath.Join(referencesRoot, "api")

	for _, p := range []string{oldPath, referencesRoot} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}

	if err := app.Git.InitRepo(oldPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := app.Git.AddAll(oldPath); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := app.Git.Commit(oldPath, "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	rec := fixApplyTestRepoRecord("software/api", "api", "software", oldPath, "https://github.com/you/api.git")
	rec.Syncable = false
	rec.UnsyncableReasons = []domain.UnsyncableReason{domain.ReasonCatalogMismatch}
	rec.ExpectedRepoKey = "references/api"
	rec.ExpectedCatalog = "references"
	rec.ExpectedPath = newPath
	rec.StateHash = domain.ComputeStateHash(rec)

	machine := state.BootstrapMachine("fix-apply-host", "fix-apply-host", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: softwareRoot, RepoPathDepth: 1},
		{Name: "references", Root: referencesRoot, RepoPathDepth: 1},
	}
	machine.Repos = []domain.MachineRepoRecord{rec}
	machine.UpdatedAt = now
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	writeFixApplyTestMetadata(t, paths, rec.RepoKey, rec.Name, rec.OriginURL)

	updated, err := app.applyFixActionWithObserver(
		[]string{"software", "references"},
		oldPath,
		FixActionMoveToCatalog,
		fixApplyOptions{Interactive: true},
		nil,
	)
	if err != nil {
		t.Fatalf("applyFixActionWithObserver failed: %v", err)
	}
	if filepath.Clean(updated.Record.Path) != filepath.Clean(newPath) {
		t.Fatalf("updated path = %q, want %q", updated.Record.Path, newPath)
	}
	if updated.Record.RepoKey != "references/api" {
		t.Fatalf("updated repo key = %q, want %q", updated.Record.RepoKey, "references/api")
	}

	if _, err := os.Stat(filepath.Join(newPath, ".git")); err != nil {
		t.Fatalf("new path missing git metadata: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path should be removed after move: %v", err)
	}
}

func TestApplyFixActionMoveToCatalogRequiresExpectedTarget(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 15, 13, 30, 0, 0, time.UTC)
	app, paths := newFixApplyTestApp(t, now)

	catalogRoot := filepath.Join(paths.Home, "software")
	repoPath := filepath.Join(catalogRoot, "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	rec := fixApplyTestRepoRecord("software/api", "api", "software", repoPath, "https://github.com/you/api.git")
	rec.Syncable = false
	rec.UnsyncableReasons = []domain.UnsyncableReason{domain.ReasonCatalogMismatch}
	rec.StateHash = domain.ComputeStateHash(rec)
	writeFixApplyTestMachine(t, paths, now, catalogRoot, rec)
	writeFixApplyTestMetadata(t, paths, rec.RepoKey, rec.Name, rec.OriginURL)

	_, err := app.applyFixActionWithObserver(
		[]string{"software"},
		repoPath,
		FixActionMoveToCatalog,
		fixApplyOptions{Interactive: true},
		nil,
	)
	if err == nil {
		t.Fatal("expected move-to-catalog to be rejected without expected target fields")
	}
	if !strings.Contains(err.Error(), "expected target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newFixApplyTestApp(t *testing.T, now time.Time) (*App, state.Paths) {
	t.Helper()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return "fix-apply-host", nil }
	return app, paths
}

func fixApplyTestRepoRecord(repoKey string, name string, catalog string, path string, origin string) domain.MachineRepoRecord {
	rec := domain.MachineRepoRecord{
		RepoKey:   repoKey,
		Name:      name,
		Catalog:   catalog,
		Path:      path,
		OriginURL: origin,
		Branch:    "main",
		Syncable:  true,
	}
	rec.StateHash = domain.ComputeStateHash(rec)
	return rec
}

func writeFixApplyTestMachine(t *testing.T, paths state.Paths, lastScanAt time.Time, catalogRoot string, repos ...domain.MachineRepoRecord) {
	t.Helper()

	machine := state.BootstrapMachine("fix-apply-host", "fix-apply-host", lastScanAt)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: catalogRoot}}
	machine.LastScanAt = lastScanAt
	machine.LastScanCatalogs = []string{"software"}
	machine.UpdatedAt = lastScanAt
	machine.Repos = append([]domain.MachineRepoRecord(nil), repos...)
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine failed: %v", err)
	}
}

func writeFixApplyTestMetadata(t *testing.T, paths state.Paths, repoKey string, name string, origin string) {
	t.Helper()

	meta := domain.RepoMetadataFile{
		RepoKey:             repoKey,
		Name:                name,
		OriginURL:           origin,
		PreferredCatalog:    "software",
		AutoPush:            domain.AutoPushModeDisabled,
		PushAccess:          domain.PushAccessReadWrite,
		BranchFollowEnabled: true,
	}
	if err := state.SaveRepoMetadata(paths, meta); err != nil {
		t.Fatalf("save repo metadata failed: %v", err)
	}
}
