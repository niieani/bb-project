package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/state"
	"bb-project/internal/testharness"
)

type twoMachineRoots struct {
	aSoftware   string
	aReferences string
	bSoftware   string
	bReferences string
}

func setupTwoMachinesWithReferences(t *testing.T) (*testharness.Harness, *testharness.Machine, *testharness.Machine, twoMachineRoots) {
	t.Helper()

	h := testharness.NewHarness(t)
	roots := twoMachineRoots{
		aSoftware:   filepath.Join(h.Root, "machines", "a", "catalogs", "software"),
		aReferences: filepath.Join(h.Root, "machines", "a", "catalogs", "references"),
		bSoftware:   filepath.Join(h.Root, "machines", "b", "catalogs", "software"),
		bReferences: filepath.Join(h.Root, "machines", "b", "catalogs", "references"),
	}
	mA := h.AddMachine("a-machine", map[string]string{"software": roots.aSoftware, "references": roots.aReferences}, "software")
	mB := h.AddMachine("b-machine", map[string]string{"software": roots.bSoftware, "references": roots.bReferences}, "software")
	return h, mA, mB, roots
}

func configureMoveHook(t *testing.T, m *testharness.Machine, hook string) {
	t.Helper()

	paths := state.NewPaths(m.Home)
	cfg, err := state.LoadConfig(paths)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Move.PostHooks = []string{hook}
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func TestCatalogMovePropagatesAsMismatchAndFixesWithMoveToCatalog(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 12, 0, 0, 0, time.UTC)
	h, mA, mB, roots := setupTwoMachinesWithReferences(t)

	_, _ = createRepoWithOrigin(t, mA, roots.aSoftware, "api", now)
	if out, err := mA.RunBB(now, "scan"); err != nil {
		t.Fatalf("machine A scan failed: %v\n%s", err, out)
	}
	h.ExternalSync("a-machine", "b-machine")

	if out, err := mB.RunBB(now.Add(30*time.Second), "clone", "you/api", "--catalog", "software"); err != nil {
		t.Fatalf("machine B clone failed: %v\n%s", err, out)
	}
	h.ExternalSync("b-machine", "a-machine")

	if out, err := mA.RunBB(now.Add(1*time.Minute), "repo", "move", "software/api", "--catalog", "references"); err != nil {
		t.Fatalf("machine A repo move failed: %v\n%s", err, out)
	}
	h.ExternalSync("b-machine", "a-machine")

	if out, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
		t.Fatalf("machine B sync failed: %v\n%s", err, out)
	}

	listOut, listErr := mB.RunBB(now.Add(3*time.Minute), "fix", "software/api")
	if listErr == nil {
		t.Fatalf("expected fix list mode to report unsyncable mismatch, output=%s", listOut)
	}
	if !strings.Contains(listOut, "catalog_mismatch") {
		t.Fatalf("expected catalog_mismatch reason in fix output, got: %s", listOut)
	}
	if !strings.Contains(listOut, "move-to-catalog") {
		t.Fatalf("expected move-to-catalog action in fix output, got: %s", listOut)
	}

	moveOut, _ := mB.RunBB(now.Add(4*time.Minute), "fix", "software/api", "move-to-catalog")
	if !strings.Contains(moveOut, "applied move-to-catalog") {
		t.Fatalf("expected move-to-catalog apply confirmation, got: %s", moveOut)
	}

	oldPath := filepath.Join(roots.bSoftware, "api")
	newPath := filepath.Join(roots.bReferences, "api")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path should be removed on machine B: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newPath, ".git")); err != nil {
		t.Fatalf("new path missing git repo on machine B: %v", err)
	}

	machineB := loadMachineFile(t, mB)
	rec := findRepoRecordByName(t, machineB, "api")
	if rec.RepoKey != "references/api" {
		t.Fatalf("machine B repo key = %q, want references/api", rec.RepoKey)
	}
	if rec.Catalog != "references" {
		t.Fatalf("machine B catalog = %q, want references", rec.Catalog)
	}
}

func TestCatalogMoveNoopOnMachineWithoutLocalClone(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 12, 30, 0, 0, time.UTC)
	h, mA, mB, roots := setupTwoMachinesWithReferences(t)

	_, _ = createRepoWithOrigin(t, mA, roots.aSoftware, "api", now)
	if out, err := mA.RunBB(now, "scan"); err != nil {
		t.Fatalf("machine A scan failed: %v\n%s", err, out)
	}
	h.ExternalSync("a-machine", "b-machine")

	if out, err := mA.RunBB(now.Add(1*time.Minute), "repo", "move", "software/api", "--catalog", "references"); err != nil {
		t.Fatalf("machine A repo move failed: %v\n%s", err, out)
	}
	h.ExternalSync("b-machine", "a-machine")

	if out, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
		t.Fatalf("machine B sync failed: %v\n%s", err, out)
	}

	machineB := loadMachineFile(t, mB)
	if len(machineB.Repos) != 0 {
		t.Fatalf("expected no repos on machine B (never cloned), got: %+v", machineB.Repos)
	}
}

func TestCatalogMoveHooksRunOnInitiatorAndFixingMachine(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 13, 0, 0, 0, time.UTC)
	h, mA, mB, roots := setupTwoMachinesWithReferences(t)

	configureMoveHook(t, mA, "echo \"$BB_MACHINE_ID:$BB_MOVE_NEW_REPO_KEY\" >> \"$HOME/move-hooks.log\"")
	configureMoveHook(t, mB, "echo \"$BB_MACHINE_ID:$BB_MOVE_NEW_REPO_KEY\" >> \"$HOME/move-hooks.log\"")
	h.ExternalSync("a-machine", "b-machine")

	_, _ = createRepoWithOrigin(t, mA, roots.aSoftware, "api", now)
	if out, err := mA.RunBB(now, "scan"); err != nil {
		t.Fatalf("machine A scan failed: %v\n%s", err, out)
	}
	h.ExternalSync("a-machine", "b-machine")

	if out, err := mB.RunBB(now.Add(30*time.Second), "clone", "you/api", "--catalog", "software"); err != nil {
		t.Fatalf("machine B clone failed: %v\n%s", err, out)
	}
	h.ExternalSync("b-machine", "a-machine")

	if out, err := mA.RunBB(now.Add(1*time.Minute), "repo", "move", "software/api", "--catalog", "references"); err != nil {
		t.Fatalf("machine A repo move failed: %v\n%s", err, out)
	}

	aHookLog := filepath.Join(mA.Home, "move-hooks.log")
	aLog, err := os.ReadFile(aHookLog)
	if err != nil {
		t.Fatalf("read machine A hook log: %v", err)
	}
	if !strings.Contains(string(aLog), "a-machine:references/api") {
		t.Fatalf("unexpected machine A hook output: %s", string(aLog))
	}

	h.ExternalSync("b-machine", "a-machine")
	if out, err := mB.RunBB(now.Add(2*time.Minute), "sync"); err != nil {
		t.Fatalf("machine B sync failed: %v\n%s", err, out)
	}
	moveOut, _ := mB.RunBB(now.Add(3*time.Minute), "fix", "software/api", "move-to-catalog")
	if !strings.Contains(moveOut, "applied move-to-catalog") {
		t.Fatalf("expected move-to-catalog apply confirmation, got: %s", moveOut)
	}

	bHookLog := filepath.Join(mB.Home, "move-hooks.log")
	bLog, err := os.ReadFile(bHookLog)
	if err != nil {
		t.Fatalf("read machine B hook log: %v", err)
	}
	if !strings.Contains(string(bLog), "b-machine:references/api") {
		t.Fatalf("unexpected machine B hook output: %s", string(bLog))
	}
}
