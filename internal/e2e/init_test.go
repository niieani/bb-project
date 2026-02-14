package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/testharness"
)

var fixedNow = time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

func setupSingleMachine(t *testing.T) (*testharness.Harness, *testharness.Machine, string) {
	t.Helper()
	h := testharness.NewHarness(t)
	catalogRoot := filepath.Join(h.Root, "machines", "a", "catalogs", "software")
	m := h.AddMachine("machine-a", map[string]string{"software": catalogRoot}, "software")
	return h, m, catalogRoot
}

func firstRepoMetadataPath(t *testing.T, m *testharness.Machine) string {
	t.Helper()
	entries, err := os.ReadDir(m.ReposDir())
	if err != nil {
		t.Fatalf("read repos dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one repo metadata file")
	}
	return filepath.Join(m.ReposDir(), entries[0].Name())
}

func TestInitCases(t *testing.T) {
	t.Parallel()
	t.Run("TC-INIT-001", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)

		out, err := m.RunBB(fixedNow, "init", "demo")
		if err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}

		repoPath := filepath.Join(catalogRoot, "demo")
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
			t.Fatalf("expected git repo: %v", err)
		}

		origin := strings.TrimSpace(m.MustRunGit(fixedNow, repoPath, "remote", "get-url", "origin"))
		if origin == "" {
			t.Fatal("expected origin url")
		}

		meta := m.MustReadFile(firstRepoMetadataPath(t, m))
		if !strings.Contains(meta, "repo_id:") || !strings.Contains(meta, "name: demo") {
			t.Fatalf("unexpected metadata:\n%s", meta)
		}
	})

	t.Run("TC-INIT-002", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath := filepath.Join(catalogRoot, "demo")
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "hello")

		out, err := m.RunBB(fixedNow, "init", "demo")
		if err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
			t.Fatalf("expected git repo: %v", err)
		}
	})

	t.Run("TC-INIT-003", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath := filepath.Join(catalogRoot, "demo")
		if out, err := m.RunBB(fixedNow, "init", "demo"); err != nil {
			t.Fatalf("first init failed: %v\n%s", err, out)
		}
		firstOrigin := strings.TrimSpace(m.MustRunGit(fixedNow, repoPath, "remote", "get-url", "origin"))
		if out, err := m.RunBB(fixedNow, "init", "demo"); err != nil {
			t.Fatalf("second init failed: %v\n%s", err, out)
		}
		secondOrigin := strings.TrimSpace(m.MustRunGit(fixedNow, repoPath, "remote", "get-url", "origin"))
		if firstOrigin != secondOrigin {
			t.Fatalf("origin changed across idempotent init: %q vs %q", firstOrigin, secondOrigin)
		}
	})

	t.Run("TC-INIT-004", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath := filepath.Join(catalogRoot, "demo")
		if _, err := m.RunGit(fixedNow, catalogRoot, "init", "-b", "main", repoPath); err != nil {
			t.Fatalf("git init failed: %v", err)
		}
		m.MustRunGit(fixedNow, repoPath, "remote", "add", "origin", "git@github.com:other/demo.git")

		out, err := m.RunBB(fixedNow, "init", "demo")
		if err == nil {
			t.Fatalf("expected init failure, output: %s", out)
		}
		if !strings.Contains(out, "conflicting origin") {
			t.Fatalf("expected conflicting origin message, got: %s", out)
		}
	})

	t.Run("TC-INIT-005", func(t *testing.T) {
		t.Parallel()
		_, m, _ := setupSingleMachine(t)
		if out, err := m.RunBB(fixedNow, "init", "demo", "--public"); err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		meta := m.MustReadFile(firstRepoMetadataPath(t, m))
		if !strings.Contains(meta, "visibility: public") || !strings.Contains(meta, "auto_push: false") {
			t.Fatalf("expected public auto_push=false, got:\n%s", meta)
		}
	})

	t.Run("TC-INIT-006", func(t *testing.T) {
		t.Parallel()
		_, m, _ := setupSingleMachine(t)
		if out, err := m.RunBB(fixedNow, "init", "demo"); err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		meta := m.MustReadFile(firstRepoMetadataPath(t, m))
		if !strings.Contains(meta, "visibility: private") || !strings.Contains(meta, "auto_push: true") {
			t.Fatalf("expected private auto_push=true, got:\n%s", meta)
		}
	})

	t.Run("TC-INIT-007", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath := filepath.Join(catalogRoot, "demo")
		m.MustRunGit(fixedNow, catalogRoot, "init", "-b", "main", repoPath)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "hello")
		m.MustRunGit(fixedNow, repoPath, "add", "README.md")
		m.MustRunGit(fixedNow, repoPath, "commit", "-m", "initial")

		if out, err := m.RunBB(fixedNow, "init", "demo"); err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}

		if _, err := m.RunGit(fixedNow, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err != nil {
			t.Fatalf("expected upstream to be set: %v", err)
		}
	})

	t.Run("TC-INIT-008", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath := filepath.Join(catalogRoot, "demo")
		m.MustRunGit(fixedNow, catalogRoot, "init", "-b", "main", repoPath)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "hello")
		m.MustRunGit(fixedNow, repoPath, "add", "README.md")
		m.MustRunGit(fixedNow, repoPath, "commit", "-m", "initial")

		if out, err := m.RunBB(fixedNow, "init", "demo", "--public"); err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}

		if _, err := m.RunGit(fixedNow, repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
			t.Fatal("expected no upstream without --push for public repos")
		}
	})

	t.Run("TC-INIT-009", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		cwd := filepath.Join(catalogRoot, "demo", "nested")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		out, err := m.RunBBAt(fixedNow, cwd, "init")
		if err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(catalogRoot, "demo", ".git")); err != nil {
			t.Fatalf("expected inferred project root git repo: %v", err)
		}
	})

	t.Run("TC-INIT-010", func(t *testing.T) {
		t.Parallel()
		_, m, _ := setupSingleMachine(t)
		out, err := m.RunBBAt(fixedNow, m.Home, "init")
		if err == nil {
			t.Fatalf("expected init failure outside catalog, output: %s", out)
		}
		if !strings.Contains(out, "outside configured catalogs") {
			t.Fatalf("expected explicit outside catalogs message, got: %s", out)
		}
	})

	t.Run("TC-INIT-011", func(t *testing.T) {
		t.Parallel()
		_, m, _ := setupSingleMachine(t)
		cfg := strings.Replace(m.MustReadFile(m.ConfigPath()), "owner: you", "owner: \"\"", 1)
		m.MustWriteFile(m.ConfigPath(), cfg)

		out, err := m.RunBB(fixedNow, "init", "demo")
		if err == nil {
			t.Fatalf("expected init failure with blank owner, output: %s", out)
		}
		if !strings.Contains(out, "github.owner is required") {
			t.Fatalf("expected owner validation message, got: %s", out)
		}
	})
}
