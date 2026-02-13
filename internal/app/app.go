package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/gitx"
	"bb-project/internal/state"
)

type App struct {
	Paths    state.Paths
	Stdout   io.Writer
	Stderr   io.Writer
	Verbose  bool
	Now      func() time.Time
	Hostname func() (string, error)
	Getwd    func() (string, error)
	Git      gitx.Runner

	IsInteractiveTerminal func() bool
	RunConfigWizard       ConfigWizardRunner
}

type InitOptions struct {
	Project string
	Catalog string
	Public  bool
	Push    bool
	HTTPS   bool
}

type ScanOptions struct {
	IncludeCatalogs []string
	AllowPush       bool
}

type SyncOptions struct {
	IncludeCatalogs []string
	Push            bool
	Notify          bool
	DryRun          bool
}

type FixOptions struct {
	IncludeCatalogs []string
	Project         string
	Action          string
	CommitMessage   string
}

func New(paths state.Paths, stdout io.Writer, stderr io.Writer) *App {
	nowFn := func() time.Time {
		if v := strings.TrimSpace(os.Getenv("BB_NOW")); v != "" {
			if ts, err := time.Parse(time.RFC3339, v); err == nil {
				return ts.UTC()
			}
		}
		return time.Now().UTC()
	}

	return &App{
		Paths:   paths,
		Stdout:  stdout,
		Stderr:  stderr,
		Verbose: true,
		Now:     nowFn,
		Hostname: func() (string, error) {
			return os.Hostname()
		},
		Getwd:                 os.Getwd,
		Git:                   gitx.Runner{Now: nowFn},
		IsInteractiveTerminal: defaultIsInteractiveTerminal,
		RunConfigWizard:       runConfigWizardInteractive,
	}
}

func (a *App) SetVerbose(verbose bool) {
	a.Verbose = verbose
}

func (a *App) logf(format string, args ...any) {
	if !a.Verbose {
		return
	}
	fmt.Fprintf(a.Stderr, "bb: "+format+"\n", args...)
}

func (a *App) loadContext() (domain.ConfigFile, domain.MachineFile, error) {
	a.logf("loading config from %s", a.Paths.ConfigPath())
	cfg, err := state.LoadConfig(a.Paths)
	if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, err
	}
	if cfg.StateTransport.Mode != "external" {
		return domain.ConfigFile{}, domain.MachineFile{}, fmt.Errorf("unsupported state_transport.mode %q in v1", cfg.StateTransport.Mode)
	}

	hostname, err := a.Hostname()
	if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, err
	}
	fallbackMachineID := strings.TrimSpace(os.Getenv("BB_MACHINE_ID"))
	if fallbackMachineID == "" {
		fallbackMachineID = hostname
	}
	machineID, err := state.LoadOrCreateMachineID(a.Paths, fallbackMachineID)
	if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, err
	}
	a.logf("using machine id %q", machineID)

	machine, err := state.LoadMachine(a.Paths, machineID)
	if errors.Is(err, os.ErrNotExist) {
		a.logf("machine file missing, bootstrapping %s", a.Paths.MachinePath(machineID))
		machine = state.BootstrapMachine(machineID, hostname, a.Now())
		if err := state.SaveMachine(a.Paths, machine); err != nil {
			return domain.ConfigFile{}, domain.MachineFile{}, err
		}
	} else if err != nil {
		return domain.ConfigFile{}, domain.MachineFile{}, err
	}
	if machine.MachineID == "" {
		machine.MachineID = machineID
	}
	if machine.Hostname == "" {
		machine.Hostname = hostname
	}
	if machine.Version == 0 {
		machine.Version = 1
	}

	return cfg, machine, nil
}

func (a *App) RunInit(opts InitOptions) error {
	a.logf("init: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
		a.logf("init: released global lock")
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return err
	}

	targetCatalog, err := resolveInitCatalog(machine, opts.Catalog)
	if err != nil {
		return err
	}
	a.logf("init: selected catalog %q (%s)", targetCatalog.Name, targetCatalog.Root)
	targetPath, projectName, err := a.resolveInitTarget(machine, targetCatalog, opts.Project)
	if err != nil {
		return err
	}
	a.logf("init: target project %q at %s", projectName, targetPath)

	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		return err
	}

	if !a.Git.IsGitRepo(targetPath) {
		a.logf("init: running git init")
		if err := a.Git.InitRepo(targetPath); err != nil {
			return fmt.Errorf("git init failed: %w", err)
		}
	} else {
		a.logf("init: existing git repository detected")
	}

	visibility := domain.VisibilityPrivate
	if opts.Public {
		visibility = domain.VisibilityPublic
	}
	remoteProtocol := cfg.GitHub.RemoteProtocol
	if opts.HTTPS {
		remoteProtocol = "https"
	}
	owner := strings.TrimSpace(cfg.GitHub.Owner)
	if owner == "" {
		return errors.New("github.owner is required; run 'bb config' and set github.owner")
	}

	expectedOrigin, expectedRepoID, err := a.expectedOrigin(owner, projectName, remoteProtocol)
	if err != nil {
		return err
	}

	origin, err := a.Git.RepoOrigin(targetPath)
	if err != nil {
		return err
	}
	if origin != "" {
		a.logf("init: found existing origin %s", origin)
		originID, err := domain.NormalizeOriginToRepoID(origin)
		if err != nil {
			return fmt.Errorf("invalid existing origin: %w", err)
		}
		if originID != expectedRepoID {
			return fmt.Errorf("conflicting origin: existing %q does not match expected %q", origin, expectedOrigin)
		}
	} else {
		a.logf("init: creating remote repository for %s/%s", owner, projectName)
		createdOrigin, err := a.createRemoteRepo(owner, projectName, visibility, remoteProtocol, targetPath)
		if err != nil {
			return err
		}
		a.logf("init: setting origin to %s", createdOrigin)
		if err := a.Git.AddOrigin(targetPath, createdOrigin); err != nil {
			return fmt.Errorf("set origin failed: %w", err)
		}
		origin = createdOrigin
	}

	repoID, err := domain.NormalizeOriginToRepoID(origin)
	if err != nil {
		return fmt.Errorf("normalize origin: %w", err)
	}

	repoMeta, created, err := a.ensureRepoMetadata(cfg, repoID, projectName, origin, visibility, targetCatalog.Name)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(a.Stdout, "registered repo metadata for %s\n", repoID)
		a.logf("init: repo metadata created for %s", repoID)
	} else {
		a.logf("init: repo metadata already exists for %s", repoID)
	}

	branch, _ := a.Git.CurrentBranch(targetPath)
	headSHA, _ := a.Git.HeadSHA(targetPath)
	upstream, _ := a.Git.Upstream(targetPath)
	if headSHA != "" && upstream == "" {
		if repoMeta.AutoPush || opts.Push {
			if branch == "" {
				branch = "main"
			}
			a.logf("init: pushing %s and setting upstream", branch)
			if err := a.Git.PushUpstream(targetPath, branch); err != nil {
				return fmt.Errorf("initial push failed: %w", err)
			}
		} else {
			a.logf("init: leaving local commits unpushed (auto-push disabled)")
		}
	}

	a.logf("init: scanning and publishing observed state")
	if _, err := a.scanAndPublish(cfg, &machine, ScanOptions{IncludeCatalogs: nil}); err != nil {
		return err
	}
	a.logf("init: completed successfully")

	return nil
}

func resolveInitCatalog(machine domain.MachineFile, explicit string) (domain.Catalog, error) {
	if explicit != "" {
		for _, c := range machine.Catalogs {
			if c.Name == explicit {
				return c, nil
			}
		}
		return domain.Catalog{}, fmt.Errorf("invalid catalog %q", explicit)
	}
	if machine.DefaultCatalog == "" {
		return domain.Catalog{}, errors.New("default catalog is not configured")
	}
	for _, c := range machine.Catalogs {
		if c.Name == machine.DefaultCatalog {
			return c, nil
		}
	}
	return domain.Catalog{}, fmt.Errorf("default catalog %q is not configured", machine.DefaultCatalog)
}

func (a *App) resolveInitTarget(machine domain.MachineFile, targetCatalog domain.Catalog, project string) (path string, name string, err error) {
	if project != "" {
		project = strings.TrimSpace(project)
		return filepath.Join(targetCatalog.Root, project), filepath.Base(project), nil
	}

	cwd, err := a.Getwd()
	if err != nil {
		return "", "", err
	}

	for _, c := range machine.Catalogs {
		if targetCatalog.Name != "" && c.Name != targetCatalog.Name {
			continue
		}
		rel, ok := pathUnderRoot(cwd, c.Root)
		if !ok {
			continue
		}
		parts := splitPath(rel)
		if len(parts) == 0 {
			break
		}
		name = parts[0]
		return filepath.Join(c.Root, name), name, nil
	}
	return "", "", errors.New("current directory is outside configured catalogs")
}

func pathUnderRoot(path, root string) (string, bool) {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(cleanPath); err == nil {
		cleanPath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(cleanRoot); err == nil {
		cleanRoot = resolved
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return "", true
	}
	if strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}

func splitPath(p string) []string {
	parts := strings.Split(filepath.Clean(p), string(filepath.Separator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (a *App) expectedOrigin(owner, repo, protocol string) (origin string, repoID string, err error) {
	if fakeRoot := strings.TrimSpace(os.Getenv("BB_TEST_REMOTE_ROOT")); fakeRoot != "" {
		origin = filepath.Join(fakeRoot, owner, repo+".git")
		repoID, err = domain.NormalizeOriginToRepoID(origin)
		return origin, repoID, err
	}
	if protocol == "https" {
		origin = fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	} else {
		origin = fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
	}
	repoID, err = domain.NormalizeOriginToRepoID(origin)
	return origin, repoID, err
}

func (a *App) createRemoteRepo(owner, repo string, visibility domain.Visibility, protocol string, repoPath string) (string, error) {
	if fakeRoot := strings.TrimSpace(os.Getenv("BB_TEST_REMOTE_ROOT")); fakeRoot != "" {
		remotePath := filepath.Join(fakeRoot, owner, repo+".git")
		a.logf("init: using test remote backend at %s", remotePath)
		if err := os.MkdirAll(filepath.Dir(remotePath), 0o755); err != nil {
			return "", err
		}
		if _, err := os.Stat(remotePath); errors.Is(err, os.ErrNotExist) {
			cmd := exec.Command("git", "init", "--bare", remotePath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("create fake remote: %w: %s", err, string(out))
			}
		}
		return remotePath, nil
	}

	name := fmt.Sprintf("%s/%s", owner, repo)
	visibilityFlag := "--private"
	if visibility == domain.VisibilityPublic {
		visibilityFlag = "--public"
	}
	args := []string{"repo", "create", name, visibilityFlag}
	a.logf("init: running gh %s", strings.Join(args, " "))
	cmd := exec.Command("gh", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh repo create failed: %w: %s", err, string(out))
	}

	if protocol == "https" {
		return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), nil
	}
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo), nil
}

func (a *App) ensureRepoMetadata(cfg domain.ConfigFile, repoID, name, origin string, visibility domain.Visibility, preferredCatalog string) (domain.RepoMetadataFile, bool, error) {
	meta, err := state.LoadRepoMetadata(a.Paths, repoID)
	created := false
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return domain.RepoMetadataFile{}, false, err
		}
		meta = domain.RepoMetadataFile{
			Version:             1,
			RepoID:              repoID,
			Name:                name,
			OriginURL:           origin,
			Visibility:          visibility,
			PreferredCatalog:    preferredCatalog,
			BranchFollowEnabled: true,
		}
		switch visibility {
		case domain.VisibilityPrivate:
			meta.AutoPush = cfg.Sync.DefaultAutoPushPrivate
		case domain.VisibilityPublic:
			meta.AutoPush = cfg.Sync.DefaultAutoPushPublic
		default:
			meta.AutoPush = false
		}
		created = true
	} else {
		if meta.Name == "" {
			meta.Name = name
		}
		if meta.OriginURL == "" {
			meta.OriginURL = origin
		}
		if meta.Visibility == "" {
			meta.Visibility = visibility
		}
		if meta.PreferredCatalog == "" {
			meta.PreferredCatalog = preferredCatalog
		}
		if !meta.BranchFollowEnabled {
			// keep explicit false; default is true only when missing on creation
		}
	}
	if err := state.SaveRepoMetadata(a.Paths, meta); err != nil {
		return domain.RepoMetadataFile{}, false, err
	}
	a.logf("state: wrote repo metadata %s", state.RepoMetaPath(a.Paths, repoID))
	return meta, created, nil
}

type discoveredRepo struct {
	CatalogName string
	Path        string
	Name        string
}

func (a *App) scanAndPublish(cfg domain.ConfigFile, machine *domain.MachineFile, opts ScanOptions) (bool, error) {
	selected, err := domain.SelectCatalogs(*machine, opts.IncludeCatalogs)
	if err != nil {
		return false, err
	}
	a.logf("scan: selected %d catalog(s)", len(selected))

	discovered, err := discoverRepos(selected)
	if err != nil {
		return false, err
	}
	a.logf("scan: discovered %d git repo(s)", len(discovered))

	prev := map[string]domain.MachineRepoRecord{}
	for _, rec := range machine.Repos {
		prev[rec.RepoID+"|"+rec.Path] = rec
	}

	records := make([]domain.MachineRepoRecord, 0, len(discovered))
	unsyncable := false
	for _, repo := range discovered {
		a.logf("scan: observing repo at %s", repo.Path)
		rec, err := a.observeRepo(cfg, repo, opts.AllowPush)
		if err != nil {
			return false, err
		}
		old := prev[rec.RepoID+"|"+rec.Path]
		rec = domain.UpdateObservedAt(old, rec, a.Now())
		if !rec.Syncable {
			unsyncable = true
		}
		records = append(records, rec)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].RepoID == records[j].RepoID {
			return records[i].Path < records[j].Path
		}
		return records[i].RepoID < records[j].RepoID
	})
	machine.Repos = records
	machine.UpdatedAt = a.Now()

	if err := state.SaveMachine(a.Paths, *machine); err != nil {
		return false, err
	}
	a.logf("state: wrote machine file %s with %d repo record(s)", a.Paths.MachinePath(machine.MachineID), len(machine.Repos))
	return unsyncable, nil
}

func discoverRepos(catalogs []domain.Catalog) ([]discoveredRepo, error) {
	out := []discoveredRepo{}
	for _, c := range catalogs {
		if strings.TrimSpace(c.Root) == "" {
			continue
		}
		if _, err := os.Stat(c.Root); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(c.Root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			if isGitDir(path) {
				rel, _ := filepath.Rel(c.Root, path)
				name := filepath.Base(path)
				if rel == "." {
					name = filepath.Base(c.Root)
				}
				out = append(out, discoveredRepo{CatalogName: c.Name, Path: path, Name: name})
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func isGitDir(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func (a *App) observeRepo(cfg domain.ConfigFile, repo discoveredRepo, allowPush bool) (domain.MachineRepoRecord, error) {
	origin, err := a.Git.RepoOrigin(repo.Path)
	if err != nil {
		return domain.MachineRepoRecord{}, err
	}
	repoID := ""
	if origin != "" {
		repoID, _ = domain.NormalizeOriginToRepoID(origin)
	}
	if repoID != "" {
		_, _, err := a.ensureRepoMetadata(cfg, repoID, repo.Name, origin, domain.VisibilityUnknown, repo.CatalogName)
		if err != nil {
			return domain.MachineRepoRecord{}, err
		}
	}

	branch, _ := a.Git.CurrentBranch(repo.Path)
	head, _ := a.Git.HeadSHA(repo.Path)
	upstream, _ := a.Git.Upstream(repo.Path)
	remoteHead, _ := a.Git.RemoteHeadSHA(repo.Path)
	ahead, behind, diverged, _ := a.Git.AheadBehind(repo.Path)
	dirtyTracked, dirtyUntracked, _ := a.Git.Dirty(repo.Path)
	op := a.Git.Operation(repo.Path)

	autoPush := false
	if repoID != "" {
		if meta, err := state.LoadRepoMetadata(a.Paths, repoID); err == nil {
			autoPush = meta.AutoPush
		}
	}

	syncable, reasons := domain.EvaluateSyncability(domain.ObservedRepoState{
		OriginURL:            origin,
		Branch:               branch,
		HeadSHA:              head,
		Upstream:             upstream,
		RemoteHeadSHA:        remoteHead,
		Ahead:                ahead,
		Behind:               behind,
		Diverged:             diverged,
		HasDirtyTracked:      dirtyTracked,
		HasUntracked:         dirtyUntracked,
		OperationInProgress:  op,
		IncludeUntrackedRule: cfg.Sync.IncludeUntrackedAsDirty,
	}, autoPush, allowPush)

	rec := domain.MachineRepoRecord{
		RepoID:              repoID,
		Name:                repo.Name,
		Catalog:             repo.CatalogName,
		Path:                repo.Path,
		OriginURL:           origin,
		Branch:              branch,
		HeadSHA:             head,
		Upstream:            upstream,
		RemoteHeadSHA:       remoteHead,
		Ahead:               ahead,
		Behind:              behind,
		Diverged:            diverged,
		HasDirtyTracked:     dirtyTracked,
		HasUntracked:        dirtyUntracked,
		OperationInProgress: op,
		Syncable:            syncable,
		UnsyncableReasons:   reasons,
	}
	rec.StateHash = domain.ComputeStateHash(rec)
	a.logf("scan: repo=%s branch=%s syncable=%t ahead=%d behind=%d", repo.Path, rec.Branch, rec.Syncable, rec.Ahead, rec.Behind)
	return rec, nil
}

func (a *App) RunScan(opts ScanOptions) (int, error) {
	a.logf("scan: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("scan: released global lock")
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	a.logf("scan: start")
	unsyncable, err := a.scanAndPublish(cfg, &machine, opts)
	if err != nil {
		return 2, err
	}
	if unsyncable {
		a.logf("scan: completed with unsyncable repos")
		return 1, nil
	}
	a.logf("scan: completed successfully")
	return 0, nil
}

func (a *App) RunStatus(jsonOut bool, include []string) (int, error) {
	a.logf("status: loading state")
	_, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}

	selected, err := domain.SelectCatalogs(machine, include)
	if err != nil {
		return 2, err
	}
	allowed := map[string]struct{}{}
	for _, c := range selected {
		allowed[c.Name] = struct{}{}
	}

	if jsonOut {
		fmt.Fprintln(a.Stdout, "{")
		fmt.Fprintf(a.Stdout, "  \"machine_id\": %q,\n", machine.MachineID)
		fmt.Fprintln(a.Stdout, "  \"repos\": [")
		first := true
		for _, r := range machine.Repos {
			if _, ok := allowed[r.Catalog]; !ok {
				continue
			}
			if !first {
				fmt.Fprintln(a.Stdout, ",")
			}
			first = false
			fmt.Fprintf(a.Stdout, "    {\"repo_id\":%q,\"catalog\":%q,\"path\":%q,\"branch\":%q,\"syncable\":%t}", r.RepoID, r.Catalog, r.Path, r.Branch, r.Syncable)
		}
		fmt.Fprintln(a.Stdout)
		fmt.Fprintln(a.Stdout, "  ]")
		fmt.Fprintln(a.Stdout, "}")
		return 0, nil
	}

	for _, r := range machine.Repos {
		if _, ok := allowed[r.Catalog]; !ok {
			continue
		}
		fmt.Fprintf(a.Stdout, "%s %s %s syncable=%t\n", r.Name, r.Branch, r.Path, r.Syncable)
	}
	a.logf("status: reported %d repo(s)", len(machine.Repos))
	return 0, nil
}

func (a *App) RunDoctor(include []string) (int, error) {
	a.logf("doctor: loading state")
	_, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	selected, err := domain.SelectCatalogs(machine, include)
	if err != nil {
		return 2, err
	}
	allowed := map[string]struct{}{}
	for _, c := range selected {
		allowed[c.Name] = struct{}{}
	}
	unsyncable := false
	for _, r := range machine.Repos {
		if _, ok := allowed[r.Catalog]; !ok {
			continue
		}
		if !r.Syncable {
			unsyncable = true
			fmt.Fprintf(a.Stdout, "%s: %v\n", r.Name, r.UnsyncableReasons)
		}
	}
	if unsyncable {
		a.logf("doctor: found unsyncable repos")
		return 1, nil
	}
	a.logf("doctor: all checked repos are syncable")
	return 0, nil
}

func (a *App) RunCatalogAdd(name, root string) (int, error) {
	a.logf("catalog add: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("catalog add: released global lock")
	}()

	_, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	for _, c := range machine.Catalogs {
		if c.Name == name {
			return 2, fmt.Errorf("catalog %q already exists", name)
		}
	}
	machine.Catalogs = append(machine.Catalogs, domain.Catalog{Name: name, Root: root})
	if machine.DefaultCatalog == "" {
		machine.DefaultCatalog = name
	}
	machine.UpdatedAt = a.Now()
	if err := state.SaveMachine(a.Paths, machine); err != nil {
		return 2, err
	}
	a.logf("catalog add: added %q -> %s", name, root)
	return 0, nil
}

func (a *App) RunCatalogRM(name string) (int, error) {
	a.logf("catalog rm: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("catalog rm: released global lock")
	}()

	_, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	out := make([]domain.Catalog, 0, len(machine.Catalogs))
	for _, c := range machine.Catalogs {
		if c.Name == name {
			continue
		}
		out = append(out, c)
	}
	if len(out) == len(machine.Catalogs) {
		return 2, fmt.Errorf("catalog %q not found", name)
	}
	machine.Catalogs = out
	if machine.DefaultCatalog == name {
		machine.DefaultCatalog = ""
	}
	machine.UpdatedAt = a.Now()
	if err := state.SaveMachine(a.Paths, machine); err != nil {
		return 2, err
	}
	a.logf("catalog rm: removed %q", name)
	return 0, nil
}

func (a *App) RunCatalogDefault(name string) (int, error) {
	a.logf("catalog default: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("catalog default: released global lock")
	}()

	_, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	found := false
	for _, c := range machine.Catalogs {
		if c.Name == name {
			found = true
			break
		}
	}
	if !found {
		return 2, fmt.Errorf("catalog %q not found", name)
	}
	machine.DefaultCatalog = name
	machine.UpdatedAt = a.Now()
	if err := state.SaveMachine(a.Paths, machine); err != nil {
		return 2, err
	}
	a.logf("catalog default: set to %q", name)
	return 0, nil
}

func (a *App) RunCatalogList() (int, error) {
	a.logf("catalog list: loading state")
	_, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	for _, c := range machine.Catalogs {
		mark := ""
		if c.Name == machine.DefaultCatalog {
			mark = " (default)"
		}
		fmt.Fprintf(a.Stdout, "%s\t%s%s\n", c.Name, c.Root, mark)
	}
	a.logf("catalog list: reported %d catalog(s)", len(machine.Catalogs))
	return 0, nil
}

func (a *App) RunRepoPolicy(repoSelector string, autoPush bool) (int, error) {
	a.logf("repo policy: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("repo policy: released global lock")
	}()

	repos, err := state.LoadAllRepoMetadata(a.Paths)
	if err != nil {
		return 2, err
	}
	idx := -1
	for i, r := range repos {
		if r.RepoID == repoSelector || r.Name == repoSelector {
			if idx >= 0 {
				return 2, fmt.Errorf("repo selector %q is ambiguous", repoSelector)
			}
			idx = i
		}
	}
	if idx < 0 {
		return 2, fmt.Errorf("repo %q not found", repoSelector)
	}
	repos[idx].AutoPush = autoPush
	if err := state.SaveRepoMetadata(a.Paths, repos[idx]); err != nil {
		return 2, err
	}
	a.logf("repo policy: set auto_push=%t for %s", autoPush, repos[idx].RepoID)
	return 0, nil
}

func (a *App) RunEnsure(include []string) (int, error) {
	a.logf("ensure: delegating to sync")
	return a.RunSync(SyncOptions{IncludeCatalogs: include})
}

func (a *App) RunSync(opts SyncOptions) (int, error) {
	// Full convergence logic is implemented in sync.go to keep this file manageable.
	return a.runSync(opts)
}

func (a *App) RunFix(opts FixOptions) (int, error) {
	return a.runFix(opts)
}
