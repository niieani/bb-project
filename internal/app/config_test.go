package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestValidateConfigForSaveRequiresOwner(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "  "

	err := validateConfigForSave(cfg)
	if err == nil {
		t.Fatal("expected owner validation error")
	}
	if !strings.Contains(err.Error(), "github.owner") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConfigAppliesChanges(t *testing.T) {
	home := t.TempDir()
	paths := state.NewPaths(home)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "old-owner"
	if err := state.SaveYAML(paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("machine-a", "host-a", now.Add(-time.Hour))
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: filepath.Join(home, "software")}}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.Now = func() time.Time { return now }
	a.IsInteractiveTerminal = func() bool { return true }

	wantCfg := cfg
	wantCfg.GitHub.Owner = "new-owner"
	wantCfg.Notify.ThrottleMinutes = 5
	missingRoot := filepath.Join(home, "references")
	wantMachine := machine
	wantMachine.Catalogs = append(wantMachine.Catalogs, domain.Catalog{Name: "references", Root: missingRoot})
	wantMachine.DefaultCatalog = "references"

	a.RunConfigWizard = func(input ConfigWizardInput) (ConfigWizardResult, error) {
		return ConfigWizardResult{
			Applied:                   true,
			CreateMissingCatalogRoots: true,
			Config:                    wantCfg,
			Machine:                   wantMachine,
		}, nil
	}

	if err := a.RunConfig(); err != nil {
		t.Fatalf("RunConfig failed: %v", err)
	}

	gotCfg, err := state.LoadConfig(paths)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if gotCfg.GitHub.Owner != "new-owner" {
		t.Fatalf("owner = %q, want %q", gotCfg.GitHub.Owner, "new-owner")
	}
	if gotCfg.Notify.ThrottleMinutes != 5 {
		t.Fatalf("throttle = %d, want 5", gotCfg.Notify.ThrottleMinutes)
	}

	gotMachine, err := state.LoadMachine(paths, "machine-a")
	if err != nil {
		t.Fatalf("load machine: %v", err)
	}
	if gotMachine.DefaultCatalog != "references" {
		t.Fatalf("default catalog = %q, want references", gotMachine.DefaultCatalog)
	}
	if len(gotMachine.Catalogs) != 2 {
		t.Fatalf("catalog count = %d, want 2", len(gotMachine.Catalogs))
	}
	if gotMachine.UpdatedAt != now {
		t.Fatalf("updated_at = %s, want %s", gotMachine.UpdatedAt, now)
	}
	if _, err := os.Stat(missingRoot); err != nil {
		t.Fatalf("expected created missing root: %v", err)
	}
}

func TestRunConfigCancelMakesNoChanges(t *testing.T) {
	home := t.TempDir()
	paths := state.NewPaths(home)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "owner-before"
	if err := state.SaveYAML(paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	machine := state.BootstrapMachine("machine-a", "host-a", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: filepath.Join(home, "software")}}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	beforeCfg, err := os.ReadFile(paths.ConfigPath())
	if err != nil {
		t.Fatalf("read before config: %v", err)
	}
	beforeMachine, err := os.ReadFile(paths.MachinePath("machine-a"))
	if err != nil {
		t.Fatalf("read before machine: %v", err)
	}

	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.IsInteractiveTerminal = func() bool { return true }
	a.RunConfigWizard = func(input ConfigWizardInput) (ConfigWizardResult, error) {
		return ConfigWizardResult{Applied: false}, nil
	}

	if err := a.RunConfig(); err != nil {
		t.Fatalf("RunConfig failed: %v", err)
	}

	afterCfg, err := os.ReadFile(paths.ConfigPath())
	if err != nil {
		t.Fatalf("read after config: %v", err)
	}
	afterMachine, err := os.ReadFile(paths.MachinePath("machine-a"))
	if err != nil {
		t.Fatalf("read after machine: %v", err)
	}

	if !bytes.Equal(beforeCfg, afterCfg) {
		t.Fatal("config changed on cancel")
	}
	if !bytes.Equal(beforeMachine, afterMachine) {
		t.Fatal("machine file changed on cancel")
	}
}

func TestRunConfigDetectsExternalConfigChange(t *testing.T) {
	home := t.TempDir()
	paths := state.NewPaths(home)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "owner-before"
	if err := state.SaveYAML(paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	machine := state.BootstrapMachine("machine-a", "host-a", time.Now().UTC())
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: filepath.Join(home, "software")}}
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.IsInteractiveTerminal = func() bool { return true }
	a.RunConfigWizard = func(input ConfigWizardInput) (ConfigWizardResult, error) {
		changed := input.Config
		changed.GitHub.Owner = "external-change"
		if err := state.SaveYAML(paths.ConfigPath(), changed); err != nil {
			t.Fatalf("save changed config: %v", err)
		}
		input.Config.GitHub.Owner = "wizard-change"
		return ConfigWizardResult{
			Applied:                   true,
			CreateMissingCatalogRoots: true,
			Config:                    input.Config,
			Machine:                   input.Machine,
		}, nil
	}

	err := a.RunConfig()
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConfigNonInteractiveFails(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	paths := state.NewPaths(home)
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.IsInteractiveTerminal = func() bool { return false }

	err := a.RunConfig()
	if err == nil {
		t.Fatal("expected non-interactive error")
	}
	if !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMachineForSaveRejectsInvalidRepoPathDepth(t *testing.T) {
	t.Parallel()

	machine := domain.MachineFile{
		DefaultCatalog: "software",
		Catalogs: []domain.Catalog{
			{Name: "software", Root: "/tmp/software", RepoPathDepth: 3},
		},
	}
	err := validateMachineForSave(machine)
	if err == nil {
		t.Fatal("expected validation error for invalid repo_path_depth")
	}
	if !strings.Contains(err.Error(), "repo_path_depth") {
		t.Fatalf("unexpected error: %v", err)
	}
}
