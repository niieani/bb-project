package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWipRepoDoesNotTriggerLegacyImmediateNotification(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	_, m, catalogRoot := setupSingleMachine(t)
	repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "api", now)
	m.MustWriteFile(filepath.Join(repoPath, "README.md"), "dirty\n")
	out, err := m.RunBB(now.Add(time.Minute), "sync", "--notify")
	if err != nil {
		t.Fatalf("wip sync failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "notify ") {
		t.Fatalf("active wip must not notify immediately: %s", out)
	}
}
