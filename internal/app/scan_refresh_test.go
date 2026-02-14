package app

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestScanFreshnessWindow(t *testing.T) {
	cfg := state.DefaultConfig()
	if got := scanFreshnessWindow(cfg); got != time.Minute {
		t.Fatalf("scanFreshnessWindow(default) = %s, want %s", got, time.Minute)
	}

	cfg.Sync.ScanFreshnessSeconds = 15
	if got := scanFreshnessWindow(cfg); got != 15*time.Second {
		t.Fatalf("scanFreshnessWindow(15) = %s, want %s", got, 15*time.Second)
	}

	cfg.Sync.ScanFreshnessSeconds = 0
	if got := scanFreshnessWindow(cfg); got != 0 {
		t.Fatalf("scanFreshnessWindow(0) = %s, want 0", got)
	}

	cfg.Sync.ScanFreshnessSeconds = -10
	if got := scanFreshnessWindow(cfg); got != 0 {
		t.Fatalf("scanFreshnessWindow(-10) = %s, want 0", got)
	}
}

func TestShouldRefreshScanSnapshot(t *testing.T) {
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	selected := []domain.Catalog{{Name: "software"}}

	tests := []struct {
		name    string
		machine domain.MachineFile
		window  time.Duration
		want    bool
	}{
		{
			name:    "never scanned",
			machine: domain.MachineFile{},
			window:  time.Minute,
			want:    true,
		},
		{
			name: "fresh and catalogs covered",
			machine: domain.MachineFile{
				LastScanAt:       now.Add(-30 * time.Second),
				LastScanCatalogs: []string{"software", "references"},
			},
			window: time.Minute,
			want:   false,
		},
		{
			name: "stale age",
			machine: domain.MachineFile{
				LastScanAt:       now.Add(-2 * time.Minute),
				LastScanCatalogs: []string{"software"},
			},
			window: time.Minute,
			want:   true,
		},
		{
			name: "missing selected catalog from last scan",
			machine: domain.MachineFile{
				LastScanAt:       now.Add(-20 * time.Second),
				LastScanCatalogs: []string{"references"},
			},
			window: time.Minute,
			want:   true,
		},
		{
			name: "window disabled",
			machine: domain.MachineFile{
				LastScanAt:       now.Add(-5 * time.Second),
				LastScanCatalogs: []string{"software"},
			},
			window: 0,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRefreshScanSnapshot(tt.machine, selected, now, tt.window)
			if got != tt.want {
				t.Fatalf("shouldRefreshScanSnapshot() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestScanWorkerCount(t *testing.T) {
	old := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(old)

	if got := scanWorkerCount(0); got != 0 {
		t.Fatalf("scanWorkerCount(0) = %d, want 0", got)
	}
	if got := scanWorkerCount(1); got != 1 {
		t.Fatalf("scanWorkerCount(1) = %d, want 1", got)
	}
	if got := scanWorkerCount(3); got != 3 {
		t.Fatalf("scanWorkerCount(3) = %d, want 3", got)
	}
	if got := scanWorkerCount(10); got != 4 {
		t.Fatalf("scanWorkerCount(10) = %d, want 4", got)
	}
}

func TestScanAndPublishObservesReposInParallel(t *testing.T) {
	old := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(old)

	home := t.TempDir()
	catalogRoot := filepath.Join(home, "catalog")
	for _, rel := range []string{"api", "web", "ops"} {
		gitDir := filepath.Join(catalogRoot, rel, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", gitDir, err)
		}
	}

	paths := state.NewPaths(home)
	a := New(paths, io.Discard, io.Discard)
	a.SetVerbose(false)

	cfg := state.DefaultConfig()
	machine := state.BootstrapMachine("machine-a", "host-a", time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC))
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: catalogRoot}}
	machine.DefaultCatalog = "software"
	discovered, err := discoverRepos(machine.Catalogs)
	if err != nil {
		t.Fatalf("discoverRepos failed: %v", err)
	}
	if len(discovered) != 3 {
		t.Fatalf("discoverRepos() returned %d repos, want 3", len(discovered))
	}
	if workers := scanWorkerCount(len(discovered)); workers < 2 {
		t.Fatalf("scanWorkerCount(%d) = %d, want at least 2", len(discovered), workers)
	}

	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	a.observeRepoHook = func(_ domain.ConfigFile, repo discoveredRepo, _ bool) (domain.MachineRepoRecord, error) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()

		entered <- struct{}{}
		<-release

		mu.Lock()
		inFlight--
		mu.Unlock()

		return domain.MachineRepoRecord{
			RepoKey:   repo.RepoKey,
			Name:      repo.Name,
			Catalog:   repo.Catalog.Name,
			Path:      repo.Path,
			Syncable:  true,
			StateHash: "ok",
		}, nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := a.scanAndPublish(cfg, &machine, ScanOptions{})
		done <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("expected at least one observed repository")
	}
	time.Sleep(200 * time.Millisecond)
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scanAndPublish failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scanAndPublish did not complete")
	}

	if maxInFlight < 2 {
		t.Fatalf("max in-flight observations = %d, want at least 2", maxInFlight)
	}
}
