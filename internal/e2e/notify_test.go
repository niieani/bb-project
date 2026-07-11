package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWipRepoDoesNotTriggerLegacyImmediateNotification(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	_, m, catalogRoot := setupSingleMachine(t)
	repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "api", now)
	m.MustWriteFile(filepath.Join(repoPath, "README.md"), "dirty\n")
	if err := os.Chtimes(filepath.Join(repoPath, "README.md"), now, now); err != nil {
		t.Fatal(err)
	}
	out, err := m.RunBB(now.Add(time.Minute), "sync", "--notify")
	if err != nil {
		t.Fatalf("wip sync failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "notify ") {
		t.Fatalf("active wip must not notify immediately: %s", out)
	}
}

func TestStaleWipEntersSingleDigest(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	_, m, root := setupSingleMachine(t)
	cfg := strings.Replace(m.MustReadFile(m.ConfigPath()), "wip_stale_hours: 24", "wip_stale_hours: 0", 1)
	m.MustWriteFile(m.ConfigPath(), cfg)
	repo, _ := createRepoWithOrigin(t, m, root, "api", now)
	dirty := filepath.Join(repo, "README.md")
	m.MustWriteFile(dirty, "dirty\n")
	stale := now.Add(-25 * time.Hour)
	for _, p := range []string{dirty, filepath.Join(repo, ".git", "HEAD"), filepath.Join(repo, ".git", "index")} {
		if err := os.Chtimes(p, stale, stale); err != nil {
			t.Fatal(err)
		}
	}
	out, err := m.RunBB(now.Add(48*time.Hour), "sync", "--notify")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "notify 1 repo(s) need attention:") || !strings.Contains(out, "api: dirty_tracked") {
		t.Fatalf("digest = %s", out)
	}
	logOut, err := m.RunBB(now.Add(49*time.Hour), "log", "--repo", "api", "--json")
	if err != nil {
		t.Fatalf("log: %v\n%s", err, logOut)
	}
	if !strings.Contains(logOut, `"event": "notified"`) || !strings.Contains(logOut, `"repo_key": "software/api"`) {
		t.Fatalf("filtered log missing notified event: %s", logOut)
	}
}
