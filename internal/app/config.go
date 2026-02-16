package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

type ConfigWizardInput struct {
	Config         domain.ConfigFile
	Machine        domain.MachineFile
	ConfigPath     string
	MachinePath    string
	LumenAvailable bool
}

type ConfigWizardResult struct {
	Applied                   bool
	CreateMissingCatalogRoots bool
	Config                    domain.ConfigFile
	Machine                   domain.MachineFile
}

type ConfigWizardRunner func(ConfigWizardInput) (ConfigWizardResult, error)

type fileSnapshot struct {
	Exists bool
	Data   []byte
}

func defaultIsInteractiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func (a *App) RunConfig() error {
	if a.IsInteractiveTerminal == nil || !a.IsInteractiveTerminal() {
		return errors.New("bb config requires an interactive terminal")
	}
	if a.RunConfigWizard == nil {
		return errors.New("config wizard is not configured")
	}

	cfg, machine, machineID, err := a.loadConfigAndMachineForConfig()
	if err != nil {
		return err
	}
	configPath := a.Paths.ConfigPath()
	machinePath := a.Paths.MachinePath(machineID)

	configSnapshot, err := snapshotFile(configPath)
	if err != nil {
		return err
	}
	machineSnapshot, err := snapshotFile(machinePath)
	if err != nil {
		return err
	}

	result, err := a.RunConfigWizard(ConfigWizardInput{
		Config:         cfg,
		Machine:        machine,
		ConfigPath:     configPath,
		MachinePath:    machinePath,
		LumenAvailable: a.isLumenAvailableForConfigWizard(),
	})
	if err != nil {
		return err
	}
	if !result.Applied {
		return nil
	}

	if err := validateConfigForSave(result.Config); err != nil {
		return err
	}
	if err := validateMachineForSave(result.Machine); err != nil {
		return err
	}

	missingRoots := missingCatalogRoots(result.Machine.Catalogs)
	if len(missingRoots) > 0 && !result.CreateMissingCatalogRoots {
		return fmt.Errorf("missing catalog root directories: %s", strings.Join(missingRoots, ", "))
	}

	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	if err := assertFileUnchanged(configPath, configSnapshot); err != nil {
		return fmt.Errorf("config file changed on disk; rerun bb config: %w", err)
	}
	if err := assertFileUnchanged(machinePath, machineSnapshot); err != nil {
		return fmt.Errorf("machine file changed on disk; rerun bb config: %w", err)
	}

	if result.CreateMissingCatalogRoots {
		for _, root := range missingRoots {
			if err := os.MkdirAll(root, 0o755); err != nil {
				return fmt.Errorf("create catalog root %s: %w", root, err)
			}
		}
	}

	result.Machine.Version = 1
	result.Machine.MachineID = machineID
	if strings.TrimSpace(result.Machine.Hostname) == "" {
		result.Machine.Hostname = machine.Hostname
	}
	result.Machine.Repos = machine.Repos
	result.Machine.UpdatedAt = a.Now()

	if err := state.SaveConfig(a.Paths, result.Config); err != nil {
		return err
	}
	if err := state.SaveMachine(a.Paths, result.Machine); err != nil {
		return err
	}
	return nil
}

func (a *App) isLumenAvailableForConfigWizard() bool {
	lookPath := a.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	_, err := lookPath("lumen")
	return err == nil
}

func (a *App) loadConfigAndMachineForConfig() (domain.ConfigFile, domain.MachineFile, string, error) {
	cfg, err := state.LoadConfig(a.Paths)
	if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, "", err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}

	hostname, err := a.Hostname()
	if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, "", err
	}
	fallbackMachineID := strings.TrimSpace(os.Getenv("BB_MACHINE_ID"))
	if fallbackMachineID == "" {
		fallbackMachineID = hostname
	}
	machineID, err := state.LoadOrCreateMachineID(a.Paths, fallbackMachineID)
	if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, "", err
	}
	machine, err := state.LoadMachine(a.Paths, machineID)
	if errors.Is(err, os.ErrNotExist) {
		machine = state.BootstrapMachine(machineID, hostname, a.Now())
	} else if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, "", err
	}
	if machine.Version == 0 {
		machine.Version = 1
	}
	if machine.MachineID == "" {
		machine.MachineID = machineID
	}
	if machine.Hostname == "" {
		machine.Hostname = hostname
	}
	if machine.Catalogs == nil {
		machine.Catalogs = []domain.Catalog{}
	}
	if machine.Repos == nil {
		machine.Repos = []domain.MachineRepoRecord{}
	}
	return cfg, machine, machineID, nil
}

func validateConfigForSave(cfg domain.ConfigFile) error {
	owner := strings.TrimSpace(cfg.GitHub.Owner)
	if owner == "" {
		return errors.New("github.owner is required")
	}
	if cfg.StateTransport.Mode != "external" {
		return fmt.Errorf("state_transport.mode must be external")
	}
	if cfg.GitHub.DefaultVisibility != "private" && cfg.GitHub.DefaultVisibility != "public" {
		return fmt.Errorf("github.default_visibility must be private or public")
	}
	if cfg.GitHub.RemoteProtocol != "ssh" && cfg.GitHub.RemoteProtocol != "https" {
		return fmt.Errorf("github.remote_protocol must be ssh or https")
	}
	if cfg.Notify.ThrottleMinutes < 0 {
		return fmt.Errorf("notify.throttle_minutes must be >= 0")
	}
	if cfg.Scheduler.IntervalMinutes < 1 {
		return fmt.Errorf("scheduler.interval_minutes must be >= 1")
	}
	targetDir := strings.TrimSpace(cfg.Link.TargetDir)
	if targetDir == "" {
		return fmt.Errorf("link.target_dir is required")
	}
	targetDir = filepath.ToSlash(filepath.Clean(targetDir))
	if targetDir == ".." || strings.HasPrefix(targetDir, "../") || strings.Contains(targetDir, "/../") {
		return fmt.Errorf("link.target_dir must not contain path traversal")
	}
	for catalog, preset := range cfg.Clone.CatalogPreset {
		catalog = strings.TrimSpace(catalog)
		if catalog == "" {
			return fmt.Errorf("clone.catalog_preset keys must be non-empty")
		}
		preset = strings.TrimSpace(preset)
		if preset == "" {
			return fmt.Errorf("clone.catalog_preset[%q] must be non-empty", catalog)
		}
		if _, ok := cfg.Clone.Presets[preset]; !ok {
			return fmt.Errorf("clone.catalog_preset[%q] references undefined preset %q", catalog, preset)
		}
	}
	return nil
}

func validateMachineForSave(machine domain.MachineFile) error {
	if len(machine.Catalogs) == 0 {
		return errors.New("at least one catalog is required")
	}
	seen := map[string]struct{}{}
	for _, c := range machine.Catalogs {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return errors.New("catalog name is required")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate catalog name %q", name)
		}
		seen[name] = struct{}{}
		root := strings.TrimSpace(c.Root)
		if root == "" {
			return fmt.Errorf("catalog %q root is required", name)
		}
		if !filepath.IsAbs(root) {
			return fmt.Errorf("catalog %q root must be an absolute path", name)
		}
		if err := domain.ValidateRepoPathDepth(c.RepoPathDepth); err != nil {
			return fmt.Errorf("catalog %q %w", name, err)
		}
	}
	if strings.TrimSpace(machine.DefaultCatalog) == "" {
		return errors.New("default catalog is required")
	}
	if _, ok := seen[machine.DefaultCatalog]; !ok {
		return fmt.Errorf("default catalog %q is not configured", machine.DefaultCatalog)
	}
	return nil
}

func missingCatalogRoots(catalogs []domain.Catalog) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, c := range catalogs {
		root := strings.TrimSpace(c.Root)
		if root == "" {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			out = append(out, root)
		}
		seen[root] = struct{}{}
	}
	sort.Strings(out)
	return out
}

func snapshotFile(path string) (fileSnapshot, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{Exists: false}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{Exists: true, Data: b}, nil
}

func assertFileUnchanged(path string, snapshot fileSnapshot) error {
	current, err := snapshotFile(path)
	if err != nil {
		return err
	}
	if current.Exists != snapshot.Exists {
		return fmt.Errorf("file existence changed")
	}
	if !bytes.Equal(current.Data, snapshot.Data) {
		return fmt.Errorf("file contents changed")
	}
	return nil
}
