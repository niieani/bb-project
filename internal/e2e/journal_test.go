package e2e

import (
	"bb-project/internal/domain"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncWritesJournalVisibleInLog(t *testing.T) {
	t.Parallel()
	_, m, root := setupSingleMachine(t)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	createRepoWithOrigin(t, m, root, "api", now)
	if out, err := m.RunBB(now, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	out, err := m.RunBB(now, "log", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"event": "sync_run"`) || !strings.Contains(out, `"machine": "machine-a"`) {
		t.Fatalf("log=%s", out)
	}
}

func TestJournalMergesMachinesAndWriteFailureDoesNotFailSync(t *testing.T) {
	t.Parallel()
	h, mA, mB, repoA, _, now := bootstrapRepoAcrossTwoMachines(t)
	mA.MustRunGit(now, repoA, "checkout", "-b", "feature/journal")
	mA.MustRunGit(now, repoA, "push", "-u", "origin", "feature/journal")
	mA.MustWriteFile(filepath.Join(repoA, "journal.txt"), "event\n")
	mA.MustRunGit(now, repoA, "add", "journal.txt")
	mA.MustRunGit(now, repoA, "commit", "-m", "journal event")
	if out, err := mA.RunBB(now.Add(2*time.Minute), "sync", "--push"); err != nil {
		t.Fatalf("push sync: %v\n%s", err, out)
	}
	h.ExternalSync("a-machine", "b-machine")
	if out, err := mB.RunBB(now.Add(3*time.Minute), "sync"); err != nil {
		t.Fatalf("converge sync: %v\n%s", err, out)
	}
	h.ExternalSync("a-machine", "b-machine")
	out, err := mA.RunBB(now.Add(4*time.Minute), "log", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var events []domain.JournalEvent
	if err := json.Unmarshal([]byte(out), &events); err != nil {
		t.Fatalf("decode log: %v\n%s", err, out)
	}
	for i := 1; i < len(events); i++ {
		if events[i].At.After(events[i-1].At) {
			t.Fatalf("events not newest first at %d: %+v", i, events)
		}
	}
	if !strings.Contains(out, `"machine": "a-machine"`) || !strings.Contains(out, `"machine": "b-machine"`) {
		t.Fatalf("merged log=%s", out)
	}
	for _, event := range []string{"sync_run", "cloned", "pushed", "converged"} {
		if !strings.Contains(out, `"event": "`+event+`"`) {
			t.Fatalf("missing %s: %s", event, out)
		}
	}
	_, m, root := setupSingleMachine(t)
	createRepoWithOrigin(t, m, root, "broken-journal", now)
	m.MustWriteFile(filepath.Join(m.ConfigRoot(), "journal"), "not a directory")
	out, err = m.RunBB(now, "sync")
	if err != nil {
		t.Fatalf("sync failed because journal failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "journal: append failed") {
		t.Fatalf("missing journal failure log: %s", out)
	}
}
