package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	return domain.ConfigFile{
		Version:        1,
		StateTransport: domain.StateTransport{Mode: "external"},
		GitHub:         domain.GitHubConfig{Owner: "", DefaultVisibility: "private", RemoteProtocol: "ssh"},
		Sync: domain.SyncConfig{
			AutoDiscover:            true,
			IncludeUntrackedAsDirty: true,
			DefaultAutoPushPrivate:  true,
			DefaultAutoPushPublic:   false,
			FetchPrune:              true,
			PullFFOnly:              true,
		},
		Notify: domain.NotifyConfig{Enabled: true, Dedupe: true, ThrottleMinutes: 60},
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

	var cfg domain.ConfigFile
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
	if cfg.Notify.ThrottleMinutes == 0 {
		cfg.Notify.ThrottleMinutes = 60
	}
	return cfg, nil
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
	m.Version = 1
	m.UpdatedAt = m.UpdatedAt.UTC()
	return SaveYAML(paths.MachinePath(m.MachineID), m)
}

func BootstrapMachine(machineID, hostname string, now time.Time) domain.MachineFile {
	return domain.MachineFile{
		Version:        1,
		MachineID:      machineID,
		Hostname:       hostname,
		DefaultCatalog: "",
		Catalogs:       nil,
		UpdatedAt:      now.UTC(),
		Repos:          nil,
	}
}

func RepoMetaFileName(repoID string) string {
	replacer := strings.NewReplacer("/", "__", ":", "_", "\\", "_", "?", "_", "*", "_")
	return replacer.Replace(repoID) + ".yaml"
}

func RepoMetaPath(paths Paths, repoID string) string {
	return filepath.Join(paths.RepoDir(), RepoMetaFileName(repoID))
}

func SaveRepoMetadata(paths Paths, repo domain.RepoMetadataFile) error {
	repo.Version = 1
	return SaveYAML(RepoMetaPath(paths, repo.RepoID), repo)
}

func LoadRepoMetadata(paths Paths, repoID string) (domain.RepoMetadataFile, error) {
	path := RepoMetaPath(paths, repoID)
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
		if repo.RepoID == "" {
			continue
		}
		out = append(out, repo)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RepoID < out[j].RepoID })
	return out, nil
}

func LoadAllMachineFiles(paths Paths) ([]domain.MachineFile, error) {
	dir := paths.MachineDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]domain.MachineFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		var m domain.MachineFile
		if err := LoadYAML(filepath.Join(dir, e.Name()), &m); err != nil {
			return nil, fmt.Errorf("parse machine file %s: %w", e.Name(), err)
		}
		if m.MachineID == "" {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func LoadNotifyCache(paths Paths) (domain.NotifyCacheFile, error) {
	cachePath := paths.NotifyCachePath()
	if _, err := os.Stat(cachePath); errors.Is(err, os.ErrNotExist) {
		return domain.NotifyCacheFile{Version: 1, LastSent: map[string]domain.NotifyCacheEntry{}}, nil
	}
	var cache domain.NotifyCacheFile
	if err := LoadYAML(cachePath, &cache); err != nil {
		return domain.NotifyCacheFile{}, fmt.Errorf("parse %s: %w", cachePath, err)
	}
	if cache.LastSent == nil {
		cache.LastSent = map[string]domain.NotifyCacheEntry{}
	}
	if cache.Version == 0 {
		cache.Version = 1
	}
	return cache, nil
}

func SaveNotifyCache(paths Paths, cache domain.NotifyCacheFile) error {
	cache.Version = 1
	if cache.LastSent == nil {
		cache.LastSent = map[string]domain.NotifyCacheEntry{}
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

func AcquireLock(paths Paths) (*Lock, error) {
	if err := EnsureDir(paths.LocalStateRoot()); err != nil {
		return nil, err
	}
	path := paths.LockPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("another bb process holds the lock")
		}
		return nil, err
	}
	_, _ = f.WriteString(fmt.Sprintf("pid=%d\n", os.Getpid()))
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
