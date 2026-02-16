package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestFixActionStashStagedOnlyStashesOnlyStagedChanges(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 16, 10, 0, 0, 0, time.UTC) }

	repoPath := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "staged.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write staged base failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "unstaged.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write unstaged base failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add base failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit base failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "staged.txt"), []byte("base\nstaged\n"), 0o644); err != nil {
		t.Fatalf("write staged update failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add staged update failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "unstaged.txt"), []byte("base\nunstaged\n"), 0o644); err != nil {
		t.Fatalf("write unstaged update failed: %v", err)
	}

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:            "api",
			Path:            repoPath,
			Branch:          "main",
			HasDirtyTracked: true,
		},
	}
	if err := app.executeFixAction(domain.ConfigFile{}, target, FixActionStash, fixApplyOptions{
		Interactive:          true,
		StashMessage:         "stash staged only",
		StashIncludeUnstaged: boolPtr(false),
	}, nil); err != nil {
		t.Fatalf("execute stash staged-only failed: %v", err)
	}

	statusOut, statusErr := app.Git.RunGit(repoPath, "status", "--porcelain")
	status := mustTrimOutput(t, statusOut, statusErr)
	hasStaged := false
	hasUnstaged := false
	for _, line := range strings.Split(status, "\n") {
		trimmed := strings.TrimSpace(line)
		parts := strings.Fields(trimmed)
		if len(parts) == 0 {
			continue
		}
		path := parts[len(parts)-1]
		switch path {
		case "staged.txt":
			hasStaged = true
		case "unstaged.txt":
			hasUnstaged = true
		}
	}
	if hasStaged {
		t.Fatalf("did not expect staged file to remain dirty after staged-only stash, status=%q", status)
	}
	if !hasUnstaged {
		t.Fatalf("expected unstaged file to remain dirty after staged-only stash, status=%q", status)
	}

	stashListOut, stashListErr := app.Git.RunGit(repoPath, "stash", "list")
	stashList := mustTrimOutput(t, stashListOut, stashListErr)
	if !strings.Contains(stashList, "stash staged only") {
		t.Fatalf("expected staged-only stash message in stash list, got %q", stashList)
	}
}

func TestFixActionStashStagedAndUnstagedStagesAllBeforeStash(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 16, 10, 10, 0, 0, time.UTC) }

	repoPath := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write tracked base failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add base failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit base failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "tracked.txt"), []byte("base\ntracked-change\n"), 0o644); err != nil {
		t.Fatalf("write tracked update failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "untracked.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatalf("write untracked file failed: %v", err)
	}

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:            "api",
			Path:            repoPath,
			Branch:          "main",
			HasDirtyTracked: true,
			HasUntracked:    true,
		},
	}
	if err := app.executeFixAction(domain.ConfigFile{}, target, FixActionStash, fixApplyOptions{
		Interactive:          true,
		StashMessage:         "stash all dirty changes",
		StashIncludeUnstaged: boolPtr(true),
	}, nil); err != nil {
		t.Fatalf("execute stash staged+unstaged failed: %v", err)
	}

	statusOut, statusErr := app.Git.RunGit(repoPath, "status", "--porcelain")
	status := mustTrimOutput(t, statusOut, statusErr)
	if status != "" {
		t.Fatalf("expected clean worktree after staged+unstaged stash, status=%q", status)
	}

	stashListOut, stashListErr := app.Git.RunGit(repoPath, "stash", "list")
	stashList := mustTrimOutput(t, stashListOut, stashListErr)
	if !strings.Contains(stashList, "stash all dirty changes") {
		t.Fatalf("expected staged+unstaged stash message in stash list, got %q", stashList)
	}
}

func TestFixActionStashStagedOnlyFailsWithoutAnyStagedChanges(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 16, 10, 20, 0, 0, time.UTC) }

	repoPath := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write tracked base failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add base failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit base failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "tracked.txt"), []byte("base\nchange\n"), 0o644); err != nil {
		t.Fatalf("write tracked update failed: %v", err)
	}

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:            "api",
			Path:            repoPath,
			Branch:          "main",
			HasDirtyTracked: true,
		},
	}
	err := app.executeFixAction(domain.ConfigFile{}, target, FixActionStash, fixApplyOptions{
		Interactive:          true,
		StashMessage:         "stash staged only",
		StashIncludeUnstaged: boolPtr(false),
	}, nil)
	if err == nil {
		t.Fatal("expected staged-only stash to fail when there are no staged changes")
	}
	if !strings.Contains(err.Error(), "no staged changes") {
		t.Fatalf("expected no-staged-changes guidance, got %v", err)
	}
}
