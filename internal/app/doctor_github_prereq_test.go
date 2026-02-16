package app

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunDoctorWarnsWhenGitHubCLIIsMissing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	a, stdout := newDoctorGitHubPrereqTestApp(t, now, "you", nil)
	a.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + file, nil
	}

	code, err := a.RunDoctor(nil)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	output := stdout.String()
	if !strings.Contains(output, "warning: GitHub operations require gh") {
		t.Fatalf("expected gh missing warning, got:\n%s", output)
	}
	if !strings.Contains(output, "brew install gh") {
		t.Fatalf("expected install instruction, got:\n%s", output)
	}
	if !strings.Contains(output, "gh auth login") {
		t.Fatalf("expected auth instruction, got:\n%s", output)
	}
}

func TestRunDoctorWarnsWhenGitHubCLIIsUnauthenticated(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	a, stdout := newDoctorGitHubPrereqTestApp(t, now, "you", nil)
	a.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "/usr/local/bin/gh", nil
		}
		return "", errors.New("not found")
	}
	a.RunCommand = func(name string, args ...string) (string, error) {
		if name != "gh" {
			t.Fatalf("unexpected command %q", name)
		}
		if len(args) != 2 || args[0] != "auth" || args[1] != "status" {
			t.Fatalf("unexpected args: %#v", args)
		}
		return "not logged into any GitHub hosts", errors.New("exit status 1")
	}

	code, err := a.RunDoctor(nil)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	output := stdout.String()
	if !strings.Contains(output, "warning: gh is installed but not authenticated") {
		t.Fatalf("expected auth warning, got:\n%s", output)
	}
	if !strings.Contains(output, "gh auth login") {
		t.Fatalf("expected login instruction, got:\n%s", output)
	}
}

func TestRunDoctorSkipsGitHubCLIWarningWhenGitHubNotNeeded(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	repos := []domain.MachineRepoRecord{
		{
			Name:      "demo",
			Catalog:   "software",
			Path:      filepath.Join(t.TempDir(), "software", "demo"),
			OriginURL: "https://gitlab.com/acme/demo.git",
			Syncable:  true,
		},
	}
	a, stdout := newDoctorGitHubPrereqTestApp(t, now, "", repos)
	a.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + file, nil
	}

	code, err := a.RunDoctor(nil)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(stdout.String(), "gh auth login") {
		t.Fatalf("did not expect GitHub warning when GitHub is not configured/used, got:\n%s", stdout.String())
	}
}

func newDoctorGitHubPrereqTestApp(t *testing.T, now time.Time, owner string, repos []domain.MachineRepoRecord) (*App, *bytes.Buffer) {
	t.Helper()

	home := t.TempDir()
	paths := state.NewPaths(home)

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = owner
	cfg.Sync.ScanFreshnessSeconds = 300
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("host-a", "host-a", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{
		{Name: "software", Root: filepath.Join(home, "software")},
	}
	machine.Repos = append([]domain.MachineRepoRecord(nil), repos...)
	machine.LastScanAt = now
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := New(paths, stdout, stderr)
	a.Now = func() time.Time { return now }
	a.Hostname = func() (string, error) { return "host-a", nil }
	return a, stdout
}
