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

func TestRemoteAlignmentEventsAppearInFilteredLog(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name      string
		template  string
		wantEvent string
		wantError bool
	}{
		{name: "verified", wantEvent: "remote_aligned"},
		{name: "reverted", template: "/definitely-missing/${repo}.git", wantEvent: "remote_align_reverted", wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, m, root := setupSingleMachine(t)
			repo, remote := createRepoWithOrigin(t, m, root, "demo", now)
			githubURL := "https://github.com/you/demo.git"
			m.MustRunGit(now, repo, "remote", "set-url", "origin", githubURL)
			template := tt.template
			if tt.wantError {
				m.MustRunGit(now, repo, "config", "url."+remote+".insteadOf", githubURL)
			} else {
				template = remote
			}
			cfg := strings.Replace(m.MustReadFile(m.ConfigPath()), "  remote_protocol: ssh", "  remote_protocol: ssh\n  preferred_remote_url_template: \""+template+"\"", 1)
			m.MustWriteFile(m.ConfigPath(), cfg)
			if out, err := m.RunBB(now.Add(time.Minute), "sync"); err != nil && !tt.wantError {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			out, err := m.RunBB(now.Add(2*time.Minute), "log", "--repo", "demo", "--json")
			if err != nil {
				t.Fatalf("log: %v\n%s", err, out)
			}
			if !strings.Contains(out, `"event": "`+tt.wantEvent+`"`) {
				t.Fatalf("filtered log missing %s: %s", tt.wantEvent, out)
			}
			if tt.wantError && !strings.Contains(out, "verification failed") {
				t.Fatalf("revert event missing verification error: %s", out)
			}
		})
	}
}
