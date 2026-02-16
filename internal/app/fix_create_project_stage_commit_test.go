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

func TestFixActionCreateProjectStagesAndCommitsDirtyFilesByDefaultBeforeInitialPush(t *testing.T) {
	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 16, 11, 0, 0, 0, time.UTC) }

	remoteRoot := t.TempDir()
	t.Setenv("BB_TEST_REMOTE_ROOT", remoteRoot)

	repoPath := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme failed: %v", err)
	}

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			RepoKey:         "software/demo",
			Name:            "demo",
			Path:            repoPath,
			Catalog:         "software",
			Branch:          "main",
			HasDirtyTracked: true,
		},
	}

	if err := app.executeFixAction(domain.ConfigFile{
		GitHub: domain.GitHubConfig{
			Owner:          "you",
			RemoteProtocol: "ssh",
		},
	}, target, FixActionCreateProject, fixApplyOptions{
		Interactive: true,
	}, nil); err != nil {
		t.Fatalf("execute create-project failed: %v", err)
	}

	subjectOut, subjectErr := app.Git.RunGit(repoPath, "log", "-1", "--pretty=%s")
	subject := mustTrimOutput(t, subjectOut, subjectErr)
	if subject != DefaultFixCreateProjectCommitMessage {
		t.Fatalf("create-project commit subject = %q, want %q", subject, DefaultFixCreateProjectCommitMessage)
	}
	statusOut, statusErr := app.Git.RunGit(repoPath, "status", "--porcelain")
	status := mustTrimOutput(t, statusOut, statusErr)
	if status != "" {
		t.Fatalf("expected clean working tree after default create-project stage/commit flow, status=%q", status)
	}

	remotePath := filepath.Join(remoteRoot, "you", "demo.git")
	lsTreeOut, lsTreeErr := app.Git.RunGit(repoPath, "--git-dir", remotePath, "ls-tree", "--name-only", "refs/heads/main")
	lsTree := mustTrimOutput(t, lsTreeOut, lsTreeErr)
	if !strings.Contains(lsTree, "README.md") {
		t.Fatalf("expected README.md to be present on remote after create-project push, ls-tree=%q", lsTree)
	}
}

func TestFixActionCreateProjectCanSkipStageCommitWhenToggleDisabled(t *testing.T) {
	paths := state.NewPaths(t.TempDir())
	app := New(paths, io.Discard, io.Discard)
	app.SetVerbose(false)
	app.Now = func() time.Time { return time.Date(2026, time.February, 16, 11, 10, 0, 0, time.UTC) }

	remoteRoot := t.TempDir()
	t.Setenv("BB_TEST_REMOTE_ROOT", remoteRoot)

	repoPath := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme failed: %v", err)
	}
	if err := app.Git.AddAll(repoPath); err != nil {
		t.Fatalf("git add readme failed: %v", err)
	}
	if err := app.Git.Commit(repoPath, "init local"); err != nil {
		t.Fatalf("git commit readme failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "LOCAL_ONLY.txt"), []byte("local only\n"), 0o644); err != nil {
		t.Fatalf("write local-only file failed: %v", err)
	}

	target := fixRepoState{
		Record: domain.MachineRepoRecord{
			RepoKey:         "software/demo",
			Name:            "demo",
			Path:            repoPath,
			Catalog:         "software",
			Branch:          "main",
			HasDirtyTracked: true,
		},
	}

	if err := app.executeFixAction(domain.ConfigFile{
		GitHub: domain.GitHubConfig{
			Owner:          "you",
			RemoteProtocol: "ssh",
		},
	}, target, FixActionCreateProject, fixApplyOptions{
		Interactive:              true,
		CreateProjectStageCommit: boolPtr(false),
	}, nil); err != nil {
		t.Fatalf("execute create-project failed: %v", err)
	}

	statusOut, statusErr := app.Git.RunGit(repoPath, "status", "--porcelain")
	status := mustTrimOutput(t, statusOut, statusErr)
	if !strings.Contains(status, "LOCAL_ONLY.txt") {
		t.Fatalf("expected LOCAL_ONLY.txt to remain uncommitted when stage/commit is disabled, status=%q", status)
	}

	remotePath := filepath.Join(remoteRoot, "you", "demo.git")
	lsTreeOut, lsTreeErr := app.Git.RunGit(repoPath, "--git-dir", remotePath, "ls-tree", "--name-only", "refs/heads/main")
	lsTree := mustTrimOutput(t, lsTreeOut, lsTreeErr)
	if strings.Contains(lsTree, "LOCAL_ONLY.txt") {
		t.Fatalf("did not expect LOCAL_ONLY.txt on remote when stage/commit is disabled, ls-tree=%q", lsTree)
	}
}
