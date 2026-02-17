package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunInitSkipsPostInitScan(t *testing.T) {
	app, _, catalogRoot := newInitTestApp(t)
	repoPath := filepath.Join(catalogRoot, "demo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := app.Git.InitRepo(repoPath); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := app.Git.AddOrigin(repoPath, "git@github.com:you/demo.git"); err != nil {
		t.Fatalf("git add origin: %v", err)
	}

	app.observeRepoHook = func(_ domain.ConfigFile, _ discoveredRepo, _ bool) (domain.MachineRepoRecord, error) {
		return domain.MachineRepoRecord{}, errors.New("unexpected post-init scan")
	}

	if err := app.RunInit(InitOptions{Project: "demo"}); err != nil {
		t.Fatalf("RunInit error: %v", err)
	}
}

func TestRunInitUsesAttachedCommandForFakeRemoteGitInit(t *testing.T) {
	app, _, _ := newInitTestApp(t)
	fakeRemoteRoot := t.TempDir()
	t.Setenv("BB_TEST_REMOTE_ROOT", fakeRemoteRoot)

	type commandCall struct {
		dir  string
		name string
		args []string
	}
	var calls []commandCall
	app.RunCommandAttached = func(dir string, name string, args ...string) error {
		calls = append(calls, commandCall{
			dir:  dir,
			name: name,
			args: append([]string(nil), args...),
		})
		return nil
	}

	if err := app.RunInit(InitOptions{Project: "demo"}); err != nil {
		t.Fatalf("RunInit error: %v", err)
	}

	remotePath := filepath.Join(fakeRemoteRoot, "you", "demo.git")
	if len(calls) != 1 {
		t.Fatalf("attached command call count = %d, want 1", len(calls))
	}
	if calls[0].dir != "" {
		t.Fatalf("attached command dir = %q, want empty", calls[0].dir)
	}
	if calls[0].name != "git" {
		t.Fatalf("attached command name = %q, want %q", calls[0].name, "git")
	}
	wantArgs := []string{"init", "--bare", remotePath}
	if !slices.Equal(calls[0].args, wantArgs) {
		t.Fatalf("attached command args = %#v, want %#v", calls[0].args, wantArgs)
	}
}

func TestCreateRemoteRepoUsesAttachedCommandForGitHubCLI(t *testing.T) {
	app := New(state.NewPaths(t.TempDir()), os.Stdout, os.Stderr)
	app.SetVerbose(false)
	t.Setenv("PATH", "")
	t.Setenv("BB_TEST_REMOTE_ROOT", "")
	app.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "/usr/local/bin/gh", nil
		}
		return "", errors.New("not found")
	}
	app.RunCommand = func(name string, args ...string) (string, error) {
		if name != "gh" {
			t.Fatalf("unexpected command %q", name)
		}
		if !slices.Equal(args, []string{"auth", "status"}) {
			t.Fatalf("unexpected args: %#v", args)
		}
		return "logged in", nil
	}

	type commandCall struct {
		dir  string
		name string
		args []string
	}
	var calls []commandCall
	app.RunCommandAttached = func(dir string, name string, args ...string) error {
		calls = append(calls, commandCall{
			dir:  dir,
			name: name,
			args: append([]string(nil), args...),
		})
		return nil
	}

	repoPath := t.TempDir()
	origin, err := app.createRemoteRepo("you", "demo", domain.VisibilityPublic, "https", "", repoPath)
	if err != nil {
		t.Fatalf("createRemoteRepo error: %v", err)
	}
	if origin != "https://github.com/you/demo.git" {
		t.Fatalf("origin = %q, want %q", origin, "https://github.com/you/demo.git")
	}

	if len(calls) != 1 {
		t.Fatalf("attached command call count = %d, want 1", len(calls))
	}
	if calls[0].dir != repoPath {
		t.Fatalf("attached command dir = %q, want %q", calls[0].dir, repoPath)
	}
	if calls[0].name != "gh" {
		t.Fatalf("attached command name = %q, want %q", calls[0].name, "gh")
	}
	wantArgs := []string{"repo", "create", "you/demo", "--public"}
	if !slices.Equal(calls[0].args, wantArgs) {
		t.Fatalf("attached command args = %#v, want %#v", calls[0].args, wantArgs)
	}
}

func TestCreateRemoteRepoReturnsHelpfulErrorWhenGHMissing(t *testing.T) {
	app := New(state.NewPaths(t.TempDir()), os.Stdout, os.Stderr)
	app.SetVerbose(false)
	t.Setenv("BB_TEST_REMOTE_ROOT", "")
	app.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "", errors.New("not found")
		}
		return "", errors.New("not found")
	}

	_, err := app.createRemoteRepo("you", "demo", domain.VisibilityPrivate, "ssh", "", t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("expected login instruction in error, got: %v", err)
	}
}

func TestCreateRemoteRepoUsesGitHubRemoteTemplate(t *testing.T) {
	app := New(state.NewPaths(t.TempDir()), os.Stdout, os.Stderr)
	app.SetVerbose(false)
	t.Setenv("BB_TEST_REMOTE_ROOT", "")

	app.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "/usr/bin/gh", nil
		}
		return "", errors.New("not found")
	}
	app.RunCommand = func(name string, args ...string) (string, error) {
		if name != "gh" {
			return "", errors.New("unexpected command")
		}
		return "logged in", nil
	}
	app.RunCommandAttached = func(_ string, _ string, _ ...string) error {
		return nil
	}

	origin, err := app.createRemoteRepo(
		"niieani",
		"bb-project",
		domain.VisibilityPrivate,
		"https",
		"git@${org}.github.com:${org}/${repo}.git",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("createRemoteRepo error: %v", err)
	}
	if origin != "git@niieani.github.com:niieani/bb-project.git" {
		t.Fatalf("origin = %q, want %q", origin, "git@niieani.github.com:niieani/bb-project.git")
	}
}

func newInitTestApp(t *testing.T) (*App, state.Paths, string) {
	t.Helper()

	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	paths := state.NewPaths(t.TempDir())

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machineID := "machine-a"
	catalogRoot := filepath.Join(paths.Home, "catalogs", "software")
	machine := state.BootstrapMachine(machineID, machineID, now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: catalogRoot},
	}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	t.Setenv("BB_MACHINE_ID", machineID)
	app := New(paths, os.Stdout, os.Stderr)
	app.SetVerbose(false)
	app.Now = func() time.Time { return now }
	app.Hostname = func() (string, error) { return machineID, nil }

	return app, paths, catalogRoot
}
