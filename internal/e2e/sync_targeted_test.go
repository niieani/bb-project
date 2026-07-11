package e2e

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTargetedSyncChangesOnlySelectedRepository(t *testing.T) {
	t.Parallel()
	h, machine, catalogRoot := setupSingleMachine(t)
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)

	apiPath, apiRemote := createRepoWithOrigin(t, machine, catalogRoot, "api", now)
	webPath, webRemote := createRepoWithOrigin(t, machine, catalogRoot, "web", now)
	if out, err := machine.RunBB(now.Add(time.Minute), "scan"); err != nil {
		t.Fatalf("initial scan: %v\n%s", err, out)
	}
	beforeWeb := findRepoRecordByName(t, loadMachineFile(t, machine), "web")

	pushRemoteChange(t, machine, apiRemote, filepath.Join(h.Root, "api-writer"), "api-remote.txt", now.Add(2*time.Minute))
	pushRemoteChange(t, machine, webRemote, filepath.Join(h.Root, "web-writer"), "web-remote.txt", now.Add(2*time.Minute))

	out, err := machine.RunBB(now.Add(3*time.Minute), "sync", "--repo", "software/api", "--events-json", "--quiet")
	if err != nil {
		t.Fatalf("targeted sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"repository":"software/api"`) || !strings.Contains(out, `"phase":"fetch"`) {
		t.Fatalf("targeted event stream missing repository progress:\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	events := make([]struct{ Event, Repository, Phase, Result, Error string }, 0, len(lines))
	for _, line := range lines {
		var event struct{ Event, Repository, Phase, Result, Error string }
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		events = append(events, event)
	}
	if len(events) < 6 || events[0].Event != "operation_started" || events[1].Event != "progress" || events[1].Phase != "discover" || events[2].Event != "repository_started" || events[len(events)-2].Event != "repository_finished" || events[len(events)-1].Event != "operation_finished" {
		t.Fatalf("unexpected lifecycle order: %#v", events)
	}
	if events[len(events)-2].Repository != "software/api" || events[len(events)-2].Result != "success" {
		t.Fatalf("repository result = %#v", events[len(events)-2])
	}
	if got := machine.MustReadFile(filepath.Join(apiPath, "api-remote.txt")); got != "api-remote\n" {
		t.Fatalf("selected repository content = %q", got)
	}
	if _, err := machine.RunGit(now, webPath, "cat-file", "-e", "origin/main:web-remote.txt"); err == nil {
		t.Fatal("unselected repository fetched remote state")
	}
	afterWeb := findRepoRecordByName(t, loadMachineFile(t, machine), "web")
	if !reflect.DeepEqual(afterWeb, beforeWeb) {
		t.Fatalf("unselected machine state changed\nbefore=%+v\nafter=%+v", beforeWeb, afterWeb)
	}
}

func pushRemoteChange(t *testing.T, machine interface {
	MustRunGit(time.Time, string, ...string) string
	MustWriteFile(string, string)
}, remote, clone, filename string, now time.Time) {
	t.Helper()
	machine.MustRunGit(now, filepath.Dir(clone), "clone", "--branch", "main", remote, clone)
	machine.MustWriteFile(filepath.Join(clone, filename), strings.TrimSuffix(filename, ".txt")+"\n")
	machine.MustRunGit(now, clone, "add", filename)
	machine.MustRunGit(now, clone, "commit", "-m", "remote change")
	machine.MustRunGit(now, clone, "push", "origin", "main")
}
