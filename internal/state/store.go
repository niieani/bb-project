package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"bb-project/internal/domain"
)

const (
	ConfigDirName   = ".config/bb-project"
	LocalStateDir   = ".local/state/bb-project"
	ConfigFileName  = "config.yaml"
	MachineDirName  = "machines"
	RepoDirName     = "repos"
	MachineIDFile   = "machine-id"
	LockFileName    = "lock"
	NotifyCacheName = "notify-cache.yaml"
)

type Paths struct {
	Home string
}

func NewPaths(home string) Paths {
	return Paths{Home: home}
}

func (p Paths) ConfigRoot() string {
	return filepath.Join(p.Home, ConfigDirName)
}

func (p Paths) LocalStateRoot() string {
	return filepath.Join(p.Home, LocalStateDir)
}

func (p Paths) ConfigPath() string {
	return filepath.Join(p.ConfigRoot(), ConfigFileName)
}

func (p Paths) RepoDir() string {
	return filepath.Join(p.ConfigRoot(), RepoDirName)
}

func (p Paths) MachineDir() string {
	return filepath.Join(p.ConfigRoot(), MachineDirName)
}

func (p Paths) MachinePath(machineID string) string {
	return filepath.Join(p.MachineDir(), machineID+".yaml")
}

func (p Paths) MachineIDPath() string {
	return filepath.Join(p.LocalStateRoot(), MachineIDFile)
}

func (p Paths) LockPath() string {
	return filepath.Join(p.LocalStateRoot(), LockFileName)
}

func (p Paths) NotifyCachePath() string {
	return filepath.Join(p.LocalStateRoot(), NotifyCacheName)
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func DefaultConfig() domain.ConfigFile {
	trueValue := true
	blobNone := "blob:none"
	return domain.ConfigFile{
		Version:        1,
		StateTransport: domain.StateTransport{Mode: "external"},
		GitHub:         domain.GitHubConfig{Owner: "", DefaultVisibility: "private", RemoteProtocol: "ssh"},
		Clone: domain.CloneConfig{
			DefaultCatalog: "",
			Shallow:        false,
			Filter:         "",
			Presets: map[string]domain.ClonePreset{
				"references": {
					Shallow: &trueValue,
					Filter:  &blobNone,
				},
			},
			CatalogPreset: map[string]string{
				"references": "references",
			},
		},
		Link: domain.LinkConfig{
			TargetDir: "references",
			Absolute:  false,
		},
		Sync: domain.SyncConfig{
			AutoDiscover:            true,
			AutoAlignRemoteFormat:   true,
			IncludeUntrackedAsDirty: true,
			DefaultAutoPushPrivate:  true,
			DefaultAutoPushPublic:   false,
			FetchPrune:              true,
			PullFFOnly:              true,
			ScanFreshnessSeconds:    60,
		},
		Move: domain.MoveConfig{
			PostHooks: []string{},
		},
		Scheduler: domain.SchedulerConfig{
			IntervalMinutes: 60,
		},
		Notify:   domain.NotifyConfig{Enabled: true, ThrottleMinutes: 60, QuietHours: 2, WIPStaleHours: 24},
		Overview: domain.OverviewConfig{MachineStaleDays: 3},
		Integrations: domain.Integrations{
			Lumen: domain.LumenIntegrationConfig{
				Enabled:                            true,
				ShowInstallTip:                     true,
				AutoGenerateCommitMessageWhenEmpty: false,
			},
		},
	}
}

func LoadConfig(paths Paths) (domain.ConfigFile, error) {
	cfgPath := paths.ConfigPath()
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		cfg := DefaultConfig()
		if err := SaveYAML(cfgPath, cfg); err != nil {
			return domain.ConfigFile{}, err
		}
		return cfg, nil
	}

	cfg := DefaultConfig()
	// Clear map defaults before unmarshal so explicit empty maps in YAML remain empty
	// instead of inheriting seeded defaults from the in-memory template.
	cfg.Clone.Presets = nil
	cfg.Clone.CatalogPreset = nil
	if err := LoadYAML(cfgPath, &cfg); err != nil {
		return domain.ConfigFile{}, fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.StateTransport.Mode == "" {
		cfg.StateTransport.Mode = "external"
	}
	if cfg.GitHub.DefaultVisibility == "" {
		cfg.GitHub.DefaultVisibility = "private"
	}
	if cfg.GitHub.RemoteProtocol == "" {
		cfg.GitHub.RemoteProtocol = "ssh"
	}
	if strings.TrimSpace(cfg.Link.TargetDir) == "" {
		cfg.Link.TargetDir = "references"
	}
	defaults := DefaultConfig()
	if cfg.Clone.Presets == nil {
		cfg.Clone.Presets = defaults.Clone.Presets
	}
	if cfg.Clone.CatalogPreset == nil {
		cfg.Clone.CatalogPreset = defaults.Clone.CatalogPreset
	}
	if cfg.Sync.ScanFreshnessSeconds < 0 {
		cfg.Sync.ScanFreshnessSeconds = 0
	}
	if cfg.Scheduler.IntervalMinutes <= 0 {
		cfg.Scheduler.IntervalMinutes = 60
	}
	if cfg.Move.PostHooks == nil {
		cfg.Move.PostHooks = []string{}
	}
	return cfg, nil
}

func SaveConfig(paths Paths, cfg domain.ConfigFile) error {
	cfg.Version = 1
	if strings.TrimSpace(cfg.StateTransport.Mode) == "" {
		cfg.StateTransport.Mode = "external"
	}
	return SaveYAML(paths.ConfigPath(), cfg)
}

func LoadMachine(paths Paths, machineID string) (domain.MachineFile, error) {
	path := paths.MachinePath(machineID)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return domain.MachineFile{}, os.ErrNotExist
	}
	var mf domain.MachineFile
	if err := LoadYAML(path, &mf); err != nil {
		return domain.MachineFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return mf, nil
}

func SaveMachine(paths Paths, m domain.MachineFile) error {
	m.Version = domain.Version
	m.UpdatedAt = m.UpdatedAt.UTC()
	return SaveYAML(paths.MachinePath(m.MachineID), m)
}

func BootstrapMachine(machineID, hostname string, now time.Time) domain.MachineFile {
	return domain.MachineFile{
		Version:        domain.Version,
		MachineID:      machineID,
		Hostname:       hostname,
		DefaultCatalog: "",
		Catalogs:       nil,
		UpdatedAt:      now.UTC(),
		Repos:          nil,
	}
}

func RepoMetaFileName(repoKey string) string {
	replacer := strings.NewReplacer("/", "__", ":", "_", "\\", "_", "?", "_", "*", "_")
	return replacer.Replace(repoKey) + ".yaml"
}

func RepoMetaPath(paths Paths, repoKey string) string {
	return filepath.Join(paths.RepoDir(), RepoMetaFileName(repoKey))
}

func SaveRepoMetadata(paths Paths, repo domain.RepoMetadataFile) error {
	repo.Version = 1
	if strings.TrimSpace(repo.RepoKey) == "" {
		return fmt.Errorf("repo_key is required")
	}
	return SaveYAML(RepoMetaPath(paths, repo.RepoKey), repo)
}

func LoadRepoMetadata(paths Paths, repoKey string) (domain.RepoMetadataFile, error) {
	path := RepoMetaPath(paths, repoKey)
	if _, err := os.Stat(path); err != nil {
		return domain.RepoMetadataFile{}, err
	}
	var repo domain.RepoMetadataFile
	if err := LoadYAML(path, &repo); err != nil {
		return domain.RepoMetadataFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return repo, nil
}

func LoadAllRepoMetadata(paths Paths) ([]domain.RepoMetadataFile, error) {
	dir := paths.RepoDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]domain.RepoMetadataFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		var repo domain.RepoMetadataFile
		if err := LoadYAML(filepath.Join(dir, e.Name()), &repo); err != nil {
			return nil, fmt.Errorf("parse repo metadata %s: %w", e.Name(), err)
		}
		if strings.TrimSpace(repo.RepoKey) == "" {
			continue
		}
		out = append(out, repo)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RepoKey < out[j].RepoKey })
	return out, nil
}

func LoadAllMachineFiles(paths Paths) ([]domain.MachineFile, error) {
	files, _, err := LoadAllMachineFilesWithWarnings(paths)
	return files, err
}

func LoadAllMachineFilesWithWarnings(paths Paths) ([]domain.MachineFile, []string, error) {
	dir := paths.MachineDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	out := make([]domain.MachineFile, 0, len(entries))
	warnings := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		var m domain.MachineFile
		if err := LoadYAML(filepath.Join(dir, e.Name()), &m); err != nil {
			return nil, nil, fmt.Errorf("parse machine file %s: %w", e.Name(), err)
		}
		if m.MachineID == "" {
			continue
		}
		if m.Version != domain.Version {
			warnings = append(warnings, fmt.Sprintf("machine %s publishes old-format state version %d; update bb there", m.MachineID, m.Version))
			continue
		}
		out = append(out, m)
	}
	return out, warnings, nil
}

func LoadNotifyCache(paths Paths) (domain.NotifyCacheFile, error) {
	cachePath := paths.NotifyCachePath()
	if _, err := os.Stat(cachePath); errors.Is(err, os.ErrNotExist) {
		return domain.NotifyCacheFile{
			Version:          2,
			DeliveryFailures: map[string]domain.NotifyDeliveryFailure{},
		}, nil
	}
	var cache domain.NotifyCacheFile
	if err := LoadYAML(cachePath, &cache); err != nil {
		return domain.NotifyCacheFile{}, fmt.Errorf("parse %s: %w", cachePath, err)
	}
	if cache.Version < 2 {
		return domain.NotifyCacheFile{Version: 2, DeliveryFailures: map[string]domain.NotifyDeliveryFailure{}}, nil
	}
	if cache.DeliveryFailures == nil {
		cache.DeliveryFailures = map[string]domain.NotifyDeliveryFailure{}
	}
	cache.Version = 2
	return cache, nil
}

func SaveNotifyCache(paths Paths, cache domain.NotifyCacheFile) error {
	cache.Version = 2
	if cache.DeliveryFailures == nil {
		cache.DeliveryFailures = map[string]domain.NotifyDeliveryFailure{}
	}
	return SaveYAML(paths.NotifyCachePath(), cache)
}

func LoadYAML(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, out); err != nil {
		return err
	}
	return nil
}

func SaveYAML(path string, in any) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	b, err := yaml.Marshal(in)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

type Lock struct {
	path string
	file *os.File
}

type LockInfo struct {
	PID       int
	Hostname  string
	CreatedAt time.Time
	Command   string
}

type LockHeldError struct {
	Info LockInfo
}

func (e *LockHeldError) Error() string {
	return "another bb process holds the lock"
}

func LockInfoFromError(err error) (LockInfo, bool) {
	var heldErr *LockHeldError
	if !errors.As(err, &heldErr) {
		return LockInfo{}, false
	}
	return heldErr.Info, true
}

func AcquireLock(paths Paths, command string) (*Lock, error) {
	if err := EnsureDir(paths.LocalStateRoot()); err != nil {
		return nil, err
	}
	path := paths.LockPath()
	lock, err := createLock(path, command)
	if err == nil {
		return lock, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	stale, err := lockIsStale(path, time.Now().UTC(), 24*time.Hour)
	if err != nil {
		return nil, err
	}
	if !stale {
		return nil, newLockHeldError(path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	lock, err = createLock(path, command)
	if err == nil {
		return lock, nil
	}
	if errors.Is(err, os.ErrExist) {
		return nil, newLockHeldError(path)
	}
	return nil, err
}

func createLock(path string, command string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if err := writeLockPayload(f, command); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &Lock{path: path, file: f}, nil
}

func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	return os.Remove(l.path)
}

func writeLockPayload(f *os.File, command string) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	command = strings.TrimSpace(command)
	payload := fmt.Sprintf(
		"pid=%d\nhostname=%s\ncreated_at=%s\n",
		os.Getpid(),
		hostname,
		time.Now().UTC().Format(time.RFC3339),
	)
	if command != "" {
		payload += fmt.Sprintf("command=%s\n", command)
	}
	_, err = f.WriteString(payload)
	return err
}

type lockMeta struct {
	PID       int
	Hostname  string
	CreatedAt time.Time
	Command   string
}

func parseLockMeta(content []byte) (lockMeta, error) {
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return lockMeta{}, fmt.Errorf("invalid lock line %q", line)
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	pidText, ok := values["pid"]
	if !ok || pidText == "" {
		return lockMeta{}, errors.New("missing lock pid")
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return lockMeta{}, fmt.Errorf("invalid lock pid %q", pidText)
	}

	hostname, ok := values["hostname"]
	if !ok || hostname == "" {
		return lockMeta{}, errors.New("missing lock hostname")
	}

	createdAtText, ok := values["created_at"]
	if !ok || createdAtText == "" {
		return lockMeta{}, errors.New("missing lock created_at")
	}
	createdAt, err := time.Parse(time.RFC3339, createdAtText)
	if err != nil {
		return lockMeta{}, fmt.Errorf("invalid lock created_at %q", createdAtText)
	}

	return lockMeta{
		PID:       pid,
		Hostname:  hostname,
		CreatedAt: createdAt,
		Command:   strings.TrimSpace(values["command"]),
	}, nil
}

func loadLockInfo(path string) (LockInfo, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return LockInfo{}, err
	}
	meta, err := parseLockMeta(content)
	if err != nil {
		return LockInfo{}, err
	}
	return LockInfo{
		PID:       meta.PID,
		Hostname:  meta.Hostname,
		CreatedAt: meta.CreatedAt,
		Command:   meta.Command,
	}, nil
}

func newLockHeldError(path string) error {
	info, err := loadLockInfo(path)
	if err != nil {
		return &LockHeldError{}
	}
	return &LockHeldError{Info: info}
}

func lockIsStale(path string, now time.Time, maxAge time.Duration) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	fileAge := now.Sub(info.ModTime())
	if fileAge < 0 {
		fileAge = 0
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	meta, err := parseLockMeta(content)
	if err != nil {
		return fileAge >= maxAge, nil
	}

	createdAge := now.Sub(meta.CreatedAt)
	if createdAge < 0 {
		createdAge = 0
	}
	if createdAge >= maxAge || fileAge >= maxAge {
		return true, nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return false, err
	}
	if strings.EqualFold(hostname, meta.Hostname) && !isProcessAlive(meta.PID) {
		return true, nil
	}

	return false, nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		// Windows has no signal-0 probe; rely on age fallback there.
		return true
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	return false
}

func LoadOrCreateMachineID(paths Paths, fallback string) (string, error) {
	if err := EnsureDir(paths.LocalStateRoot()); err != nil {
		return "", err
	}
	idPath := paths.MachineIDPath()
	if b, err := os.ReadFile(idPath); err == nil {
		id := strings.TrimSpace(string(b))
		if id != "" {
			return id, nil
		}
	}
	id := strings.TrimSpace(fallback)
	if id == "" {
		id = fmt.Sprintf("machine-%d", time.Now().Unix())
	}
	if err := os.WriteFile(idPath, []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}
