package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
	"bb-project/internal/testharness"
)

func setupTwoMachines(t *testing.T) (*testharness.Harness, *testharness.Machine, *testharness.Machine, string, string) {
	t.Helper()
	h := testharness.NewHarness(t)
	rootA := filepath.Join(h.Root, "machines", "a", "catalogs", "software")
	rootB := filepath.Join(h.Root, "machines", "b", "catalogs", "software")
	mA := h.AddMachine("a-machine", map[string]string{"software": rootA}, "software")
	mB := h.AddMachine("b-machine", map[string]string{"software": rootB}, "software")
	return h, mA, mB, rootA, rootB
}

func loadMachineFile(t *testing.T, m *testharness.Machine) domain.MachineFile {
	t.Helper()
	mf, err := state.LoadMachine(state.NewPaths(m.Home), m.ID)
	if err != nil {
		t.Fatalf("load machine %s: %v", m.ID, err)
	}
	return mf
}

func findRepoRecordByName(t *testing.T, mf domain.MachineFile, name string) domain.MachineRepoRecord {
	t.Helper()
	for _, r := range mf.Repos {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("repo %q not found in machine file", name)
	return domain.MachineRepoRecord{}
}

func setCatalogDefaultBranchAutoPushPolicy(t *testing.T, m *testharness.Machine, catalogName string, private *bool, public *bool) {
	t.Helper()
	paths := state.NewPaths(m.Home)
	mf, err := state.LoadMachine(paths, m.ID)
	if err != nil {
		t.Fatalf("load machine %s: %v", m.ID, err)
	}
	found := false
	for i := range mf.Catalogs {
		if mf.Catalogs[i].Name != catalogName {
			continue
		}
		mf.Catalogs[i].AllowAutoPushDefaultBranchPrivate = private
		mf.Catalogs[i].AllowAutoPushDefaultBranchPublic = public
		found = true
	}
	if !found {
		t.Fatalf("catalog %q not found on machine %s", catalogName, m.ID)
	}
	if err := state.SaveMachine(paths, mf); err != nil {
		t.Fatalf("save machine %s: %v", m.ID, err)
	}
}

func setCatalogRepoPathDepth(t *testing.T, m *testharness.Machine, catalogName string, depth int) {
	t.Helper()
	paths := state.NewPaths(m.Home)
	mf, err := state.LoadMachine(paths, m.ID)
	if err != nil {
		t.Fatalf("load machine %s: %v", m.ID, err)
	}
	found := false
	for i := range mf.Catalogs {
		if mf.Catalogs[i].Name != catalogName {
			continue
		}
		mf.Catalogs[i].RepoPathDepth = depth
		found = true
	}
	if !found {
		t.Fatalf("catalog %q not found on machine %s", catalogName, m.ID)
	}
	if err := state.SaveMachine(paths, mf); err != nil {
		t.Fatalf("save machine %s: %v", m.ID, err)
	}
}

func setCatalogAutoCloneOnSync(t *testing.T, m *testharness.Machine, catalogName string, enabled bool) {
	t.Helper()
	paths := state.NewPaths(m.Home)
	mf, err := state.LoadMachine(paths, m.ID)
	if err != nil {
		t.Fatalf("load machine %s: %v", m.ID, err)
	}
	found := false
	for i := range mf.Catalogs {
		if mf.Catalogs[i].Name != catalogName {
			continue
		}
		mf.Catalogs[i].AutoCloneOnSync = &enabled
		found = true
	}
	if !found {
		t.Fatalf("catalog %q not found on machine %s", catalogName, m.ID)
	}
	if err := state.SaveMachine(paths, mf); err != nil {
		t.Fatalf("save machine %s: %v", m.ID, err)
	}
}

func createRepoWithOrigin(t *testing.T, m *testharness.Machine, catalogRoot, name string, now time.Time) (path string, remotePath string) {
	t.Helper()
	path = filepath.Join(catalogRoot, name)
	remotePath = filepath.Join(m.Harness.RemotesRoot, "you", name+".git")
	if err := os.MkdirAll(filepath.Dir(remotePath), 0o755); err != nil {
		t.Fatalf("mkdir remote parent: %v", err)
	}
	m.MustRunGit(now, catalogRoot, "init", "-b", "main", path)
	m.MustRunGit(now, catalogRoot, "init", "--bare", remotePath)
	m.MustRunGit(now, path, "remote", "add", "origin", remotePath)
	m.MustWriteFile(filepath.Join(path, "README.md"), "hello\n")
	m.MustRunGit(now, path, "add", "README.md")
	m.MustRunGit(now, path, "commit", "-m", "init")
	m.MustRunGit(now, path, "push", "-u", "origin", "main")
	return path, remotePath
}

func gitCurrentBranch(t *testing.T, m *testharness.Machine, repoPath string, now time.Time) string {
	t.Helper()
	return stringTrim(m.MustRunGit(now, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))
}

func stringTrim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	return s
}
