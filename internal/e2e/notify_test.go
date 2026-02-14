package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNotifyCases(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	t.Run("TC-NOTIFY-001", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "api", now)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "dirty\n")

		out, err := m.RunBB(now.Add(1*time.Minute), "sync", "--notify")
		if err == nil {
			t.Fatalf("expected unsyncable sync, output=%s", out)
		}
		if !strings.Contains(out, "notify api") {
			t.Fatalf("expected notification output, got: %s", out)
		}
	})

	t.Run("TC-NOTIFY-002", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "api", now)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "dirty\n")

		_, _ = m.RunBB(now.Add(1*time.Minute), "sync", "--notify")
		out, err := m.RunBB(now.Add(2*time.Minute), "sync", "--notify")
		if err == nil {
			t.Fatalf("expected unsyncable sync, output=%s", out)
		}
		if strings.Contains(out, "notify api") {
			t.Fatalf("expected deduped notification (no duplicate), got: %s", out)
		}
	})

	t.Run("TC-NOTIFY-003", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "api", now)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "dirty\n")

		_, _ = m.RunBB(now.Add(1*time.Minute), "sync", "--notify")
		m.MustRunGit(now, repoPath, "checkout", "--", "README.md")
		m.MustWriteFile(filepath.Join(repoPath, "scratch.txt"), "untracked\n")

		out, err := m.RunBB(now.Add(2*time.Minute), "sync", "--notify")
		if err == nil {
			t.Fatalf("expected unsyncable sync, output=%s", out)
		}
		if strings.Contains(out, "notify api") {
			t.Fatalf("expected throttle suppression for changed fingerprint inside window, got: %s", out)
		}
	})

	t.Run("TC-NOTIFY-004", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		_, _ = createRepoWithOrigin(t, m, catalogRoot, "api", now)
		out, err := m.RunBB(now.Add(1*time.Minute), "sync", "--notify")
		if err != nil {
			t.Fatalf("expected syncable run, got err=%v output=%s", err, out)
		}
		if strings.Contains(out, "notify ") {
			t.Fatalf("expected no notifications for syncable repos, got: %s", out)
		}
	})

	t.Run("TC-NOTIFY-005", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoOne := filepath.Join(catalogRoot, "api-one")
		repoTwo := filepath.Join(catalogRoot, "api-two")
		m.MustRunGit(now, catalogRoot, "init", "-b", "main", repoOne)
		m.MustRunGit(now, catalogRoot, "init", "-b", "main", repoTwo)

		out, err := m.RunBB(now.Add(1*time.Minute), "sync", "--notify")
		if err == nil {
			t.Fatalf("expected unsyncable sync, output=%s", out)
		}
		if !strings.Contains(out, "notify api-one") {
			t.Fatalf("expected notification for api-one, got: %s", out)
		}
		if !strings.Contains(out, "notify api-two") {
			t.Fatalf("expected notification for api-two, got: %s", out)
		}
	})

	t.Run("TC-NOTIFY-006", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "api", now)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "dirty\n")

		_, _ = m.RunBB(now.Add(1*time.Minute), "sync", "--notify")
		m.MustRunGit(now, repoPath, "checkout", "--", "README.md")
		m.MustWriteFile(filepath.Join(repoPath, "scratch.txt"), "untracked\n")

		out, err := m.RunBB(now.Add(62*time.Minute), "sync", "--notify")
		if err == nil {
			t.Fatalf("expected unsyncable sync, output=%s", out)
		}
		if !strings.Contains(out, "notify api") {
			t.Fatalf("expected notification after throttle window expires, got: %s", out)
		}
	})

	t.Run("TC-NOTIFY-007", func(t *testing.T) {
		t.Parallel()
		_, m, catalogRoot := setupSingleMachine(t)
		cfg := strings.Replace(m.MustReadFile(m.ConfigPath()), "throttle_minutes: 60", "throttle_minutes: 0", 1)
		m.MustWriteFile(m.ConfigPath(), cfg)
		repoPath, _ := createRepoWithOrigin(t, m, catalogRoot, "api", now)
		m.MustWriteFile(filepath.Join(repoPath, "README.md"), "dirty\n")

		_, _ = m.RunBB(now.Add(1*time.Minute), "sync", "--notify")
		m.MustRunGit(now, repoPath, "checkout", "--", "README.md")
		m.MustWriteFile(filepath.Join(repoPath, "scratch.txt"), "untracked\n")

		out, err := m.RunBB(now.Add(2*time.Minute), "sync", "--notify")
		if err == nil {
			t.Fatalf("expected unsyncable sync, output=%s", out)
		}
		if !strings.Contains(out, "notify api") {
			t.Fatalf("expected notification when throttle disabled, got: %s", out)
		}
	})
}
