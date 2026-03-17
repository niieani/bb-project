package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/gitx"
	"bb-project/internal/state"
)

type cloneOutcome struct {
	Record domain.MachineRepoRecord
	Noop   bool
}

type cloneRepoSpec struct {
	CloneURL string
	Owner    string
	RepoName string
}

type registeredCloneTarget struct {
	RepoKey      string
	CatalogName  string
	RelativePath string
	OriginURL    string
}

func (a *App) runClone(opts CloneOptions) (int, error) {
	a.logf("clone: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths, "clone")
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("clone: released global lock")
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}

	_, err = a.runCloneLocked(cfg, &machine, opts)
	if err != nil {
		return 2, err
	}
	return 0, nil
}

func (a *App) runCloneLocked(cfg domain.ConfigFile, machine *domain.MachineFile, opts CloneOptions) (cloneOutcome, error) {
	registered, err := resolveRegisteredCloneTarget(a.Paths, cfg, opts.Repo, a.Getenv)
	if err != nil {
		return cloneOutcome{}, err
	}

	specInput := opts.Repo
	if registered != nil {
		if strings.TrimSpace(registered.OriginURL) == "" {
			return cloneOutcome{}, fmt.Errorf("registered repo %q is missing origin_url; cannot clone", registered.RepoKey)
		}
		specInput = registered.OriginURL
	}

	spec, err := parseCloneRepoSpec(cfg, specInput, a.Getenv)
	if err != nil {
		return cloneOutcome{}, err
	}

	targetCatalog, err := resolveCloneCatalog(*machine, cfg, opts.Catalog, registered)
	if err != nil {
		return cloneOutcome{}, err
	}

	repoKey, relativePath, repoName, err := resolveCloneTarget(targetCatalog, spec, opts.As, registered)
	if err != nil {
		return cloneOutcome{}, err
	}
	targetPath := filepath.Join(targetCatalog.Root, filepath.FromSlash(relativePath))

	if existing, found := findExistingRepoByOrigin(*machine, a.Git, spec.CloneURL); found {
		fmt.Fprintf(a.Stdout, "repository already exists as %s at %s\n", existing.Name, existing.Path)
		return cloneOutcome{Record: existing, Noop: true}, nil
	}

	pathConflictReason, err := validateTargetPath(a.Git, targetPath, spec.CloneURL, "")
	if err != nil {
		return cloneOutcome{}, err
	}
	if pathConflictReason != "" {
		if strings.TrimSpace(opts.As) == "" {
			return cloneOutcome{}, fmt.Errorf("target path %s conflicts; pass --as to choose a different target path", targetPath)
		}
		return cloneOutcome{}, fmt.Errorf("target path %s conflicts (%s)", targetPath, pathConflictReason)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return cloneOutcome{}, err
	}

	cloneShallow, cloneFilter, cloneOnly := resolveCloneTransportOptions(cfg, targetCatalog.Name, opts)
	if err := a.Git.CloneWithOptions(gitx.CloneOptions{
		Origin:  spec.CloneURL,
		Path:    targetPath,
		Shallow: cloneShallow,
		Filter:  cloneFilter,
		Only:    cloneOnly,
		Stdout:  a.Stdout,
		Stderr:  a.Stderr,
	}); err != nil {
		return cloneOutcome{}, fmt.Errorf("clone failed: %w", err)
	}

	record, err := a.observeRepo(cfg, discoveredRepo{
		Catalog: targetCatalog,
		Path:    targetPath,
		Name:    repoName,
		RepoKey: repoKey,
	}, false)
	if err != nil {
		return cloneOutcome{}, err
	}
	upsertMachineRepoRecord(machine, record)
	machine.UpdatedAt = a.Now()
	if err := state.SaveMachine(a.Paths, *machine); err != nil {
		return cloneOutcome{}, err
	}

	fmt.Fprintf(a.Stdout, "cloned %s to %s\n", repoKey, targetPath)
	return cloneOutcome{Record: record}, nil
}

func resolveCloneCatalog(machine domain.MachineFile, cfg domain.ConfigFile, explicit string, registered *registeredCloneTarget) (domain.Catalog, error) {
	name := strings.TrimSpace(explicit)
	if registered != nil {
		expected := strings.TrimSpace(registered.CatalogName)
		if expected == "" {
			return domain.Catalog{}, fmt.Errorf("registered repo %q is missing catalog information", registered.RepoKey)
		}
		if name != "" && name != expected {
			return domain.Catalog{}, fmt.Errorf("registered repo %q belongs to catalog %q, not %q", registered.RepoKey, expected, name)
		}
		name = expected
	}
	if name == "" {
		name = strings.TrimSpace(cfg.Clone.DefaultCatalog)
	}
	if name == "" {
		return domain.Catalog{}, errors.New("clone catalog is not configured; set clone.default_catalog or pass --catalog")
	}
	catalog, ok := domain.FindCatalog(machine, name)
	if !ok {
		return domain.Catalog{}, fmt.Errorf("catalog %q is not configured on this machine; run `bb catalog list`", name)
	}
	return catalog, nil
}

func resolveCloneTarget(catalog domain.Catalog, spec cloneRepoSpec, as string, registered *registeredCloneTarget) (repoKey string, relativePath string, repoName string, err error) {
	trimmedAs := strings.TrimSpace(as)
	if trimmedAs != "" {
		repoKey, relativePath, repoName, ok := domain.DeriveRepoKeyFromRelative(catalog, trimmedAs)
		if !ok {
			return "", "", "", fmt.Errorf("clone target path must match catalog layout depth %d", domain.EffectiveRepoPathDepth(catalog))
		}
		return repoKey, relativePath, repoName, nil
	}
	if registered != nil {
		repoKey, relativePath, repoName, ok := domain.DeriveRepoKeyFromRelative(catalog, registered.RelativePath)
		if !ok {
			return "", "", "", fmt.Errorf("registered repo %q target path %q does not match catalog layout depth %d", registered.RepoKey, registered.RelativePath, domain.EffectiveRepoPathDepth(catalog))
		}
		return repoKey, relativePath, repoName, nil
	}

	switch domain.EffectiveRepoPathDepth(catalog) {
	case 1:
		if strings.TrimSpace(spec.RepoName) == "" {
			return "", "", "", errors.New("cannot derive repository name from input; pass --as")
		}
		repoKey, relativePath, repoName, ok := domain.DeriveRepoKeyFromRelative(catalog, spec.RepoName)
		if !ok {
			return "", "", "", fmt.Errorf("clone target path must match catalog layout depth %d", domain.EffectiveRepoPathDepth(catalog))
		}
		return repoKey, relativePath, repoName, nil
	case 2:
		if strings.TrimSpace(spec.Owner) == "" || strings.TrimSpace(spec.RepoName) == "" {
			return "", "", "", errors.New("cannot derive owner/repo path from input; pass --as")
		}
		relative := spec.Owner + "/" + spec.RepoName
		repoKey, relativePath, repoName, ok := domain.DeriveRepoKeyFromRelative(catalog, relative)
		if !ok {
			return "", "", "", fmt.Errorf("clone target path must match catalog layout depth %d", domain.EffectiveRepoPathDepth(catalog))
		}
		return repoKey, relativePath, repoName, nil
	default:
		return "", "", "", fmt.Errorf("unsupported catalog layout depth %d", domain.EffectiveRepoPathDepth(catalog))
	}
}

func resolveCloneTransportOptions(cfg domain.ConfigFile, catalog string, opts CloneOptions) (shallow bool, filter string, only []string) {
	shallow = cfg.Clone.Shallow
	filter = strings.TrimSpace(cfg.Clone.Filter)

	if presetName := strings.TrimSpace(cfg.Clone.CatalogPreset[catalog]); presetName != "" {
		if preset, ok := cfg.Clone.Presets[presetName]; ok {
			if preset.Shallow != nil {
				shallow = *preset.Shallow
			}
			if preset.Filter != nil {
				filter = strings.TrimSpace(*preset.Filter)
			}
		}
	}

	if opts.ShallowSet {
		shallow = opts.Shallow
	}
	if opts.FilterSet {
		filter = strings.TrimSpace(opts.Filter)
	}

	seen := map[string]struct{}{}
	only = make([]string, 0, len(opts.Only))
	for _, raw := range opts.Only {
		pathSpec := strings.TrimSpace(raw)
		if pathSpec == "" {
			continue
		}
		if _, ok := seen[pathSpec]; ok {
			continue
		}
		seen[pathSpec] = struct{}{}
		only = append(only, pathSpec)
	}
	return shallow, filter, only
}

func resolveRegisteredCloneTarget(paths state.Paths, cfg domain.ConfigFile, input string, getenv func(string) string) (*registeredCloneTarget, error) {
	repos, err := state.LoadAllRepoMetadata(paths)
	if err != nil {
		return nil, err
	}

	idx, err := selectRegisteredCloneMetadataIndex(repos, input)
	if err != nil {
		return nil, err
	}
	if idx == -1 {
		spec, parseErr := parseCloneRepoSpec(cfg, input, getenv)
		if parseErr != nil {
			return nil, nil
		}
		idx, err = selectRepoMetadataByOrigin(repos, spec.CloneURL)
		if err != nil {
			return nil, err
		}
	}
	if idx == -1 {
		return nil, nil
	}

	repoKey := strings.TrimSpace(repos[idx].RepoKey)
	catalogName, relativePath, _, err := domain.ParseRepoKey(repoKey)
	if err != nil {
		return nil, fmt.Errorf("registered repo %q has invalid repo_key %q: %w", input, repoKey, err)
	}

	return &registeredCloneTarget{
		RepoKey:      repoKey,
		CatalogName:  catalogName,
		RelativePath: relativePath,
		OriginURL:    strings.TrimSpace(repos[idx].OriginURL),
	}, nil
}

func selectRegisteredCloneMetadataIndex(repos []domain.RepoMetadataFile, selector string) (int, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return -1, nil
	}
	if catalog, project, ok := splitCatalogProjectSelector(selector); ok {
		selector = catalog + "/" + project
	}
	return selectRepoMetadataIndex(repos, selector)
}

func selectRepoMetadataByOrigin(repos []domain.RepoMetadataFile, origin string) (int, error) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return -1, nil
	}
	matches := make([]int, 0, 2)
	for i, repo := range repos {
		if strings.TrimSpace(repo.OriginURL) == "" {
			continue
		}
		same, err := originsMatchNormalized(repo.OriginURL, origin)
		if err != nil || !same {
			continue
		}
		matches = append(matches, i)
	}
	if len(matches) == 0 {
		return -1, nil
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	keys := make([]string, 0, len(matches))
	for _, idx := range matches {
		keys = append(keys, repos[idx].RepoKey)
	}
	sort.Strings(keys)
	return -1, fmt.Errorf("repo origin %q is ambiguous; matches: %s", origin, strings.Join(keys, ", "))
}

func parseCloneRepoSpec(cfg domain.ConfigFile, input string, getenv func(string) string) (cloneRepoSpec, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return cloneRepoSpec{}, errors.New("repo input is required")
	}

	if owner, repo, ok := parseGitHubShorthand(raw); ok {
		return cloneRepoSpec{
			CloneURL: resolveGitHubCloneURL(cfg, owner, repo, false, getenv),
			Owner:    owner,
			RepoName: repo,
		}, nil
	}

	if owner, repo, ok := parseGitHubHTTPRepoLink(raw); ok {
		return cloneRepoSpec{
			CloneURL: resolveGitHubCloneURL(cfg, owner, repo, true, getenv),
			Owner:    owner,
			RepoName: repo,
		}, nil
	}

	identity, err := domain.NormalizeOriginIdentity(raw)
	if err != nil {
		return cloneRepoSpec{}, fmt.Errorf("invalid repo input %q", raw)
	}
	_, owner, repo := deriveIdentityOwnerRepo(identity)
	return cloneRepoSpec{
		CloneURL: raw,
		Owner:    owner,
		RepoName: repo,
	}, nil
}

func parseGitHubShorthand(raw string) (owner string, repo string, ok bool) {
	if strings.Contains(raw, "://") || strings.Contains(raw, "@") || strings.HasPrefix(raw, "/") {
		return "", "", false
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", "", false
	}
	if strings.Contains(owner, ":") || strings.Contains(repo, ":") {
		return "", "", false
	}
	return owner, strings.TrimSuffix(repo, ".git"), true
}

func parseGitHubHTTPRepoLink(raw string) (owner string, repo string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", false
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if host != "github.com" && host != "www.github.com" {
		return "", "", false
	}
	parts := splitPath(strings.Trim(u.Path, "/"))
	if len(parts) < 2 {
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

func resolveGitHubCloneURL(cfg domain.ConfigFile, owner string, repo string, forceHTTPS bool, getenv func(string) string) string {
	if getenv != nil {
		if fakeRoot := strings.TrimSpace(getenv("BB_TEST_REMOTE_ROOT")); fakeRoot != "" {
			return "file://" + filepath.Join(fakeRoot, owner, repo+".git")
		}
	}
	protocol := strings.TrimSpace(cfg.GitHub.RemoteProtocol)
	template := strings.TrimSpace(cfg.GitHub.PreferredRemoteURLTemplate)
	if forceHTTPS && template == "" {
		protocol = "https"
	}
	url, err := githubRemoteURL(owner, repo, protocol, template)
	if err == nil {
		return url
	}
	if forceHTTPS || strings.EqualFold(protocol, "https") {
		return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	}
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
}

func deriveIdentityOwnerRepo(identity string) (host string, owner string, repo string) {
	host, path, ok := strings.Cut(identity, "/")
	if !ok {
		return strings.TrimSpace(identity), "", ""
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return strings.TrimSpace(host), "", ""
	}
	repo = strings.TrimSpace(parts[len(parts)-1])
	if len(parts) == 2 {
		owner = strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(host), owner, repo
}

func findExistingRepoByOrigin(machine domain.MachineFile, git gitx.Runner, expectedOrigin string) (domain.MachineRepoRecord, bool) {
	for _, rec := range machine.Repos {
		if strings.TrimSpace(rec.Path) == "" {
			continue
		}
		origin := strings.TrimSpace(rec.OriginURL)
		if origin == "" && git.IsGitRepo(rec.Path) {
			origin, _ = git.RepoOrigin(rec.Path)
		}
		if origin == "" {
			continue
		}
		matches, err := originsMatchNormalized(origin, expectedOrigin)
		if err != nil || !matches {
			continue
		}
		return rec, true
	}
	return domain.MachineRepoRecord{}, false
}

func upsertMachineRepoRecord(machine *domain.MachineFile, rec domain.MachineRepoRecord) {
	for i := range machine.Repos {
		if filepath.Clean(machine.Repos[i].Path) == filepath.Clean(rec.Path) {
			machine.Repos[i] = rec
			sort.Slice(machine.Repos, func(i, j int) bool {
				if repoRecordSortKey(machine.Repos[i]) == repoRecordSortKey(machine.Repos[j]) {
					return machine.Repos[i].Path < machine.Repos[j].Path
				}
				return repoRecordSortKey(machine.Repos[i]) < repoRecordSortKey(machine.Repos[j])
			})
			return
		}
	}
	machine.Repos = append(machine.Repos, rec)
	sort.Slice(machine.Repos, func(i, j int) bool {
		if repoRecordSortKey(machine.Repos[i]) == repoRecordSortKey(machine.Repos[j]) {
			return machine.Repos[i].Path < machine.Repos[j].Path
		}
		return repoRecordSortKey(machine.Repos[i]) < repoRecordSortKey(machine.Repos[j])
	})
}
