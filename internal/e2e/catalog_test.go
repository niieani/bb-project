package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/testharness"
)

func setupTwoCatalogMachine(t *testing.T) (*testharness.Harness, *testharness.Machine, string, string) {
	t.Helper()
	h := testharness.NewHarness(t)
	softwareRoot := filepath.Join(h.Root, "machines", "a", "catalogs", "software")
	referencesRoot := filepath.Join(h.Root, "machines", "a", "catalogs", "references")
	m := h.AddMachine("machine-a", map[string]string{
		"software":   softwareRoot,
		"references": referencesRoot,
	}, "software")
	return h, m, softwareRoot, referencesRoot
}

func TestCatalogCases(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	t.Run("TC-CATALOG-001", func(t *testing.T) {
		t.Parallel()
		_, m, softwareRoot, _ := setupTwoCatalogMachine(t)
		if out, err := m.RunBB(now, "init", "api"); err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(softwareRoot, "api", ".git")); err != nil {
			t.Fatalf("expected repo in default catalog: %v", err)
		}
	})

	t.Run("TC-CATALOG-002", func(t *testing.T) {
		t.Parallel()
		_, m, _, referencesRoot := setupTwoCatalogMachine(t)
		if out, err := m.RunBB(now, "init", "api", "--catalog", "references"); err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(referencesRoot, "api", ".git")); err != nil {
			t.Fatalf("expected repo in references catalog: %v", err)
		}
	})

	t.Run("TC-CATALOG-003", func(t *testing.T) {
		t.Parallel()
		_, m, softwareRoot, referencesRoot := setupTwoCatalogMachine(t)
		createRepoWithOrigin(t, m, softwareRoot, "s-repo", now)
		createRepoWithOrigin(t, m, referencesRoot, "r-repo", now)

		if out, err := m.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync failed: %v\n%s", err, out)
		}
		mf := loadMachineFile(t, m)
		if len(mf.Repos) != 2 {
			t.Fatalf("expected both catalogs processed, got %d repos", len(mf.Repos))
		}
	})

	t.Run("TC-CATALOG-004", func(t *testing.T) {
		t.Parallel()
		_, m, softwareRoot, referencesRoot := setupTwoCatalogMachine(t)
		createRepoWithOrigin(t, m, softwareRoot, "s-repo", now)
		createRepoWithOrigin(t, m, referencesRoot, "r-repo", now)

		if out, err := m.RunBB(now.Add(1*time.Minute), "sync", "--include-catalog", "references", "--include-catalog", "software", "--include-catalog", "references"); err != nil {
			t.Fatalf("sync failed: %v\n%s", err, out)
		}
		mf := loadMachineFile(t, m)
		if len(mf.Repos) != 2 {
			t.Fatalf("expected union of includes, got %d repos", len(mf.Repos))
		}
	})

	t.Run("TC-CATALOG-005", func(t *testing.T) {
		t.Parallel()
		h := testharness.NewHarness(t)
		rootA := filepath.Join(h.Root, "machines", "a", "catalogs", "software")
		refsA := filepath.Join(h.Root, "machines", "a", "catalogs", "references")
		rootB := filepath.Join(h.Root, "machines", "b", "catalogs", "software")
		mA := h.AddMachine("a-machine", map[string]string{"software": rootA, "references": refsA}, "software")
		mB := h.AddMachine("b-machine", map[string]string{"software": rootB}, "software")

		if out, err := mA.RunBB(now, "init", "api", "--catalog", "references"); err != nil {
			t.Fatalf("init on A failed: %v\n%s", err, out)
		}
		repoA := filepath.Join(refsA, "api")
		mA.MustWriteFile(filepath.Join(repoA, "README.md"), "hello\n")
		mA.MustRunGit(now, repoA, "add", "README.md")
		mA.MustRunGit(now, repoA, "commit", "-m", "bootstrap")
		mA.MustRunGit(now, repoA, "push", "-u", "origin", "main")
		if out, err := mA.RunBB(now.Add(30*time.Second), "sync"); err != nil {
			t.Fatalf("sync on A failed: %v\n%s", err, out)
		}

		h.ExternalSync("a-machine", "b-machine")
		if out, err := mB.RunBB(now.Add(1*time.Minute), "sync"); err != nil {
			t.Fatalf("sync on B failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(rootB, "api", ".git")); err != nil {
			t.Fatalf("expected fallback to default catalog on B: %v", err)
		}
	})

	t.Run("TC-CATALOG-006", func(t *testing.T) {
		t.Parallel()
		_, m, _, _ := setupTwoCatalogMachine(t)
		out, err := m.RunBB(now, "init", "api", "--catalog", "missing")
		if err == nil {
			t.Fatalf("expected init catalog validation failure, output=%s", out)
		}

		out, err = m.RunBB(now.Add(1*time.Minute), "sync", "--include-catalog", "missing")
		if err == nil {
			t.Fatalf("expected sync catalog validation failure, output=%s", out)
		}
	})
}
