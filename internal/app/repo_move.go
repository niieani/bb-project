package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

type repoMoveResult struct {
	OldPath    string
	NewPath    string
	OldRepoKey string
	NewRepoKey string
}

func (a *App) runRepoMove(opts RepoMoveOptions) (int, error) {
	a.logf("repo move: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("repo move: released global lock")
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	if _, err := a.runRepoMoveLocked(cfg, &machine, opts); err != nil {
		return 2, err
	}
	return 0, nil
}

func (a *App) runRepoMoveLocked(cfg domain.ConfigFile, machine *domain.MachineFile, opts RepoMoveOptions) (repoMoveResult, error) {
	if machine == nil {
		return repoMoveResult{}, errors.New("machine snapshot is required")
	}
	selector := strings.TrimSpace(opts.Selector)
	if selector == "" {
		return repoMoveResult{}, errors.New("repo selector is required")
	}
	repo, found, err := resolveLocalProjectSelector(machine.Repos, selector)
	if err != nil {
		return repoMoveResult{}, err
	}
	if !found {
		return repoMoveResult{}, fmt.Errorf("repo %q not found", selector)
	}

	targetCatalogName := strings.TrimSpace(opts.TargetCatalog)
	if targetCatalogName == "" {
		return repoMoveResult{}, errors.New("target catalog is required")
	}
	targetCatalog, ok := domain.FindCatalog(*machine, targetCatalogName)
	if !ok {
		return repoMoveResult{}, fmt.Errorf("catalog %q is not configured on this machine; run `bb catalog list`", targetCatalogName)
	}

	targetRepoKey, targetRelative, targetName, err := resolveRepoMoveTarget(repo, targetCatalog, opts.As)
	if err != nil {
		return repoMoveResult{}, err
	}
	targetPath := filepath.Join(targetCatalog.Root, filepath.FromSlash(targetRelative))
	oldPath := filepath.Clean(strings.TrimSpace(repo.Path))
	if oldPath == "" {
		return repoMoveResult{}, errors.New("selected repo has empty path")
	}

	oldRepoKey := strings.TrimSpace(repo.RepoKey)
	if oldRepoKey == "" {
		oldRepoKey = targetRepoKey
	}

	preferredRemote := ""
	meta, hasMeta, err := loadRepoMetadataForMove(a.Paths, oldRepoKey)
	if err != nil {
		return repoMoveResult{}, err
	}
	if hasMeta {
		preferredRemote = strings.TrimSpace(meta.PreferredRemote)
	}

	pathConflictReason, err := validateTargetPath(a.Git, targetPath, repo.OriginURL, preferredRemote)
	if err != nil {
		return repoMoveResult{}, err
	}
	if pathConflictReason != "" && filepath.Clean(targetPath) != filepath.Clean(oldPath) {
		return repoMoveResult{}, fmt.Errorf("target path %s conflicts (%s)", targetPath, pathConflictReason)
	}

	if opts.DryRun {
		fmt.Fprintf(a.Stdout, "dry-run: move %s -> %s\n", oldPath, targetPath)
		fmt.Fprintf(a.Stdout, "dry-run: repo_key %s -> %s\n", oldRepoKey, targetRepoKey)
		return repoMoveResult{
			OldPath:    oldPath,
			NewPath:    targetPath,
			OldRepoKey: oldRepoKey,
			NewRepoKey: targetRepoKey,
		}, nil
	}

	if filepath.Clean(oldPath) != filepath.Clean(targetPath) {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return repoMoveResult{}, err
		}
		if err := moveDirectoryWithCrossDeviceFallback(oldPath, targetPath); err != nil {
			return repoMoveResult{}, fmt.Errorf("move repository directory: %w", err)
		}
	}

	updatedMeta := movedRepoMetadata(meta, hasMeta, cfg, repo, oldRepoKey, targetRepoKey, targetCatalog.Name, targetName)
	if err := state.SaveRepoMetadata(a.Paths, updatedMeta); err != nil {
		return repoMoveResult{}, err
	}
	if oldRepoKey != "" && oldRepoKey != targetRepoKey {
		if err := os.Remove(state.RepoMetaPath(a.Paths, oldRepoKey)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return repoMoveResult{}, err
		}
	}

	observed, err := a.observeRepo(cfg, discoveredRepo{
		Catalog: targetCatalog,
		Path:    targetPath,
		Name:    targetName,
		RepoKey: targetRepoKey,
	}, false)
	if err != nil {
		return repoMoveResult{}, err
	}

	machine.Repos = removeRepoRecordByPath(machine.Repos, oldPath)
	upsertMachineRepoRecord(machine, observed)
	machine.UpdatedAt = a.Now()
	if err := state.SaveMachine(a.Paths, *machine); err != nil {
		return repoMoveResult{}, err
	}

	env := moveHookEnvironment{
		OldRepoKey: oldRepoKey,
		NewRepoKey: targetRepoKey,
		OldCatalog: strings.TrimSpace(repo.Catalog),
		NewCatalog: targetCatalog.Name,
		OldPath:    oldPath,
		NewPath:    targetPath,
	}
	if !opts.NoHooks {
		if err := a.runPostMoveHooks(cfg.Move.PostHooks, targetPath, env); err != nil {
			return repoMoveResult{}, err
		}
	}

	fmt.Fprintf(a.Stdout, "moved %s to %s\n", oldRepoKey, targetRepoKey)
	return repoMoveResult{
		OldPath:    oldPath,
		NewPath:    targetPath,
		OldRepoKey: oldRepoKey,
		NewRepoKey: targetRepoKey,
	}, nil
}

func resolveRepoMoveTarget(repo domain.MachineRepoRecord, targetCatalog domain.Catalog, as string) (repoKey string, relative string, repoName string, err error) {
	trimmedAs := strings.TrimSpace(as)
	if trimmedAs != "" {
		repoKey, relative, repoName, ok := domain.DeriveRepoKeyFromRelative(targetCatalog, trimmedAs)
		if !ok {
			return "", "", "", fmt.Errorf("move target path must match catalog layout depth %d", domain.EffectiveRepoPathDepth(targetCatalog))
		}
		return repoKey, relative, repoName, nil
	}

	oldRepoKey := strings.TrimSpace(repo.RepoKey)
	if oldRepoKey == "" {
		return "", "", "", errors.New("cannot derive target path from repo without repo_key; pass --as")
	}
	_, oldRelative, _, parseErr := domain.ParseRepoKey(oldRepoKey)
	if parseErr != nil {
		return "", "", "", fmt.Errorf("cannot parse repo_key %q: %w", oldRepoKey, parseErr)
	}
	repoKey, relative, repoName, ok := domain.DeriveRepoKeyFromRelative(targetCatalog, oldRelative)
	if !ok {
		return "", "", "", fmt.Errorf("move target path must match catalog layout depth %d; pass --as", domain.EffectiveRepoPathDepth(targetCatalog))
	}
	return repoKey, relative, repoName, nil
}

func loadRepoMetadataForMove(paths state.Paths, repoKey string) (domain.RepoMetadataFile, bool, error) {
	repoKey = strings.TrimSpace(repoKey)
	if repoKey == "" {
		return domain.RepoMetadataFile{}, false, nil
	}
	meta, err := state.LoadRepoMetadata(paths, repoKey)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.RepoMetadataFile{}, false, nil
		}
		return domain.RepoMetadataFile{}, false, err
	}
	return meta, true, nil
}

func movedRepoMetadata(
	existing domain.RepoMetadataFile,
	hasExisting bool,
	cfg domain.ConfigFile,
	repo domain.MachineRepoRecord,
	oldRepoKey string,
	newRepoKey string,
	newCatalog string,
	repoName string,
) domain.RepoMetadataFile {
	meta := existing
	if !hasExisting {
		meta = domain.RepoMetadataFile{
			RepoKey:             oldRepoKey,
			Name:                strings.TrimSpace(repo.Name),
			OriginURL:           strings.TrimSpace(repo.OriginURL),
			Visibility:          domain.VisibilityUnknown,
			PreferredCatalog:    strings.TrimSpace(repo.Catalog),
			PushAccess:          domain.PushAccessUnknown,
			BranchFollowEnabled: true,
			AutoPush:            domain.AutoPushModeDisabled,
		}
	}

	if strings.TrimSpace(meta.Name) == "" {
		meta.Name = strings.TrimSpace(repoName)
	}
	if strings.TrimSpace(meta.OriginURL) == "" {
		meta.OriginURL = strings.TrimSpace(repo.OriginURL)
	}
	if strings.TrimSpace(meta.RepoKey) == "" {
		meta.RepoKey = oldRepoKey
	}
	if meta.Visibility == "" {
		meta.Visibility = domain.VisibilityUnknown
	}
	if meta.PushAccess == "" {
		meta.PushAccess = domain.PushAccessUnknown
	}
	if !hasExisting {
		switch meta.Visibility {
		case domain.VisibilityPrivate:
			meta.AutoPush = domain.AutoPushModeFromEnabled(cfg.Sync.DefaultAutoPushPrivate)
		case domain.VisibilityPublic:
			meta.AutoPush = domain.AutoPushModeFromEnabled(cfg.Sync.DefaultAutoPushPublic)
		}
	}

	if oldRepoKey != "" && oldRepoKey != newRepoKey {
		meta.PreviousRepoKeys = append(meta.PreviousRepoKeys, oldRepoKey)
	}
	meta.RepoKey = newRepoKey
	meta.PreferredCatalog = newCatalog
	return normalizedRepoMetadata(meta)
}

func removeRepoRecordByPath(records []domain.MachineRepoRecord, path string) []domain.MachineRepoRecord {
	if len(records) == 0 {
		return nil
	}
	cleanTarget := filepath.Clean(path)
	out := make([]domain.MachineRepoRecord, 0, len(records))
	for _, rec := range records {
		if filepath.Clean(rec.Path) == cleanTarget {
			continue
		}
		out = append(out, rec)
	}
	return out
}

type moveHookEnvironment struct {
	OldRepoKey string
	NewRepoKey string
	OldCatalog string
	NewCatalog string
	OldPath    string
	NewPath    string
}

func (a *App) runPostMoveHooks(hooks []string, repoPath string, env moveHookEnvironment) error {
	for i, raw := range hooks {
		hook := strings.TrimSpace(raw)
		if hook == "" {
			continue
		}
		if err := runMoveHook(a, hook, repoPath, env); err != nil {
			return fmt.Errorf("post-move hook %d failed: %w", i+1, err)
		}
	}
	return nil
}

func runMoveHook(a *App, hook string, repoPath string, env moveHookEnvironment) error {
	shellName, shellArgs := moveHookShellCommand(hook)
	cmd := exec.Command(shellName, shellArgs...)
	cmd.Dir = repoPath
	cmd.Stdout = a.Stdout
	cmd.Stderr = a.Stderr
	cmd.Env = append(os.Environ(),
		"BB_MOVE_OLD_REPO_KEY="+env.OldRepoKey,
		"BB_MOVE_NEW_REPO_KEY="+env.NewRepoKey,
		"BB_MOVE_OLD_CATALOG="+env.OldCatalog,
		"BB_MOVE_NEW_CATALOG="+env.NewCatalog,
		"BB_MOVE_OLD_PATH="+env.OldPath,
		"BB_MOVE_NEW_PATH="+env.NewPath,
	)
	return cmd.Run()
}

func moveHookShellCommand(hook string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", hook}
	}
	return "sh", []string{"-lc", hook}
}

func moveDirectoryWithCrossDeviceFallback(src string, dst string) error {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return nil
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDeviceLinkError(err) {
		return err
	}

	if err := copyDirectory(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func isCrossDeviceLinkError(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return errors.Is(linkErr.Err, syscall.EXDEV)
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, syscall.EXDEV)
	}
	return errors.Is(err, syscall.EXDEV)
}

func copyDirectory(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		if d.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}
		return copyFile(path, targetPath)
	})
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
