package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/gitx"
	"bb-project/internal/state"
)

func (a *App) runLink(opts LinkOptions) (int, error) {
	a.logf("link: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("link: released global lock")
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}

	cwd, err := a.Getwd()
	if err != nil {
		return 2, err
	}
	anchor, err := resolveLinkAnchor(a.Git, cwd)
	if err != nil {
		return 2, err
	}

	target, found, err := a.resolveProjectOrRepoSelector(cfg, &machine, opts.Selector, resolveProjectOrRepoSelectorOptions{
		AllowClone: true,
		Catalog:    opts.Catalog,
	})
	if err != nil {
		return 2, err
	}
	if !found {
		return 2, fmt.Errorf("selector %q could not be resolved", opts.Selector)
	}

	targetDir, err := resolveLinkTargetDir(anchor, cfg, opts)
	if err != nil {
		return 2, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return 2, err
	}

	linkName := strings.TrimSpace(opts.As)
	if linkName == "" {
		linkName = strings.TrimSpace(target.Name)
	}
	if linkName == "" {
		linkName = filepath.Base(strings.TrimSpace(target.Path))
	}
	if linkName == "" {
		return 2, errors.New("cannot determine link name")
	}
	linkPath := filepath.Join(targetDir, linkName)

	absolute := opts.Absolute || cfg.Link.Absolute
	linkTarget := strings.TrimSpace(target.Path)
	if linkTarget == "" {
		return 2, fmt.Errorf("target path is empty for selector %q", opts.Selector)
	}
	if !absolute {
		rel, err := filepath.Rel(filepath.Dir(linkPath), linkTarget)
		if err != nil {
			return 2, err
		}
		linkTarget = rel
	}

	if err := ensureSymlink(linkPath, linkTarget, target.Path); err != nil {
		return 2, err
	}
	fmt.Fprintf(a.Stdout, "linked %s -> %s\n", linkPath, target.Path)
	return 0, nil
}

type resolveProjectOrRepoSelectorOptions struct {
	AllowClone bool
	Catalog    string
}

func (a *App) resolveProjectOrRepoSelector(cfg domain.ConfigFile, machine *domain.MachineFile, selector string, opts resolveProjectOrRepoSelectorOptions) (domain.MachineRepoRecord, bool, error) {
	target, found, err := resolveLocalProjectSelector(machine.Repos, selector)
	if err != nil {
		return domain.MachineRepoRecord{}, false, err
	}
	if found {
		return target, true, nil
	}

	if spec, err := parseCloneRepoSpec(cfg, selector, a.Getenv); err == nil {
		if existing, ok := findExistingRepoByOrigin(*machine, a.Git, spec.CloneURL); ok {
			return existing, true, nil
		}
	}

	if !opts.AllowClone {
		return domain.MachineRepoRecord{}, false, nil
	}

	outcome, err := a.runCloneLocked(cfg, machine, CloneOptions{
		Repo:    selector,
		Catalog: opts.Catalog,
	})
	if err != nil {
		return domain.MachineRepoRecord{}, false, err
	}
	return outcome.Record, true, nil
}

func resolveLinkAnchor(gitRunner gitx.Runner, cwd string) (string, error) {
	cwd = filepath.Clean(cwd)
	topLevel, err := gitRunner.RunGit(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return cwd, nil
	}
	topLevel = strings.TrimSpace(topLevel)
	if topLevel == "" {
		return cwd, nil
	}
	return filepath.Clean(topLevel), nil
}

func resolveLinkTargetDir(anchor string, cfg domain.ConfigFile, opts LinkOptions) (string, error) {
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		dir = strings.TrimSpace(cfg.Link.TargetDir)
	}
	if dir == "" {
		return "", errors.New("link target directory is not configured; set link.target_dir or pass --dir")
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	return filepath.Join(anchor, filepath.Clean(dir)), nil
}

func resolveLocalProjectSelector(records []domain.MachineRepoRecord, selector string) (domain.MachineRepoRecord, bool, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return domain.MachineRepoRecord{}, false, errors.New("selector is required")
	}

	byRepoKey := make([]domain.MachineRepoRecord, 0, 2)
	for _, rec := range records {
		if strings.TrimSpace(rec.RepoKey) == selector {
			byRepoKey = append(byRepoKey, rec)
		}
	}
	if len(byRepoKey) == 1 {
		return byRepoKey[0], true, nil
	}
	if len(byRepoKey) > 1 {
		return domain.MachineRepoRecord{}, false, fmt.Errorf("selector %q is ambiguous", selector)
	}

	if catalog, project, ok := splitCatalogProjectSelector(selector); ok {
		candidate := catalog + "/" + project
		matches := make([]domain.MachineRepoRecord, 0, 2)
		for _, rec := range records {
			if strings.TrimSpace(rec.RepoKey) == candidate {
				matches = append(matches, rec)
			}
		}
		if len(matches) == 1 {
			return matches[0], true, nil
		}
		if len(matches) > 1 {
			return domain.MachineRepoRecord{}, false, fmt.Errorf("selector %q is ambiguous", selector)
		}
	}

	byName := make([]domain.MachineRepoRecord, 0, 2)
	for _, rec := range records {
		if strings.TrimSpace(rec.Name) == selector {
			byName = append(byName, rec)
		}
	}
	if len(byName) == 1 {
		return byName[0], true, nil
	}
	if len(byName) > 1 {
		paths := make([]string, 0, len(byName))
		for _, rec := range byName {
			paths = append(paths, rec.Path)
		}
		sort.Strings(paths)
		return domain.MachineRepoRecord{}, false, fmt.Errorf("selector %q is ambiguous; matches: %s", selector, strings.Join(paths, ", "))
	}

	return domain.MachineRepoRecord{}, false, nil
}

func splitCatalogProjectSelector(selector string) (catalog string, project string, ok bool) {
	if strings.Contains(selector, "://") {
		return "", "", false
	}
	if strings.Count(selector, ":") != 1 {
		return "", "", false
	}
	catalog, project, ok = strings.Cut(selector, ":")
	if !ok {
		return "", "", false
	}
	catalog = strings.TrimSpace(catalog)
	project = strings.TrimSpace(project)
	if catalog == "" || project == "" {
		return "", "", false
	}
	return catalog, project, true
}

func ensureSymlink(linkPath string, linkTarget string, resolvedTargetPath string) error {
	info, err := os.Lstat(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return os.Symlink(linkTarget, linkPath)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("link path already exists and is not a symlink: %s", linkPath)
	}
	currentTarget, err := os.Readlink(linkPath)
	if err != nil {
		return err
	}
	currentResolved := currentTarget
	if !filepath.IsAbs(currentResolved) {
		currentResolved = filepath.Join(filepath.Dir(linkPath), currentResolved)
	}
	currentResolved = filepath.Clean(currentResolved)
	if filepath.Clean(resolvedTargetPath) == currentResolved {
		return nil
	}
	return fmt.Errorf("link path already exists with different target: %s", linkPath)
}
