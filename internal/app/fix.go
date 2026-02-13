package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

const (
	FixActionIgnore          = "ignore"
	FixActionAbortOperation  = "abort-operation"
	FixActionPush            = "push"
	FixActionStageCommitPush = "stage-commit-push"
	FixActionPullFFOnly      = "pull-ff-only"
	FixActionSetUpstreamPush = "set-upstream-push"
	FixActionEnableAutoPush  = "enable-auto-push"

	AutoFixCommitMessage = "bb: checkpoint local changes before sync"
)

var errFixActionNotEligible = errors.New("fix action not eligible")

type fixRepoState struct {
	Record domain.MachineRepoRecord
	Meta   *domain.RepoMetadataFile
}

func (a *App) runFix(opts FixOptions) (int, error) {
	if strings.TrimSpace(opts.Project) == "" && strings.TrimSpace(opts.Action) == "" {
		if a.IsInteractiveTerminal == nil || !a.IsInteractiveTerminal() {
			return 2, errors.New("bb fix requires an interactive terminal")
		}
		return a.runFixInteractive(opts.IncludeCatalogs, opts.NoRefresh)
	}

	repos, err := a.loadFixRepos(opts.IncludeCatalogs, !opts.NoRefresh)
	if err != nil {
		return 2, err
	}

	target, err := resolveFixTarget(opts.Project, repos)
	if err != nil {
		return 2, err
	}

	eligible := eligibleFixActions(target.Record, target.Meta)
	if strings.TrimSpace(opts.Action) == "" {
		a.renderFixStatus(target.Record, eligible)
		if target.Record.Syncable {
			return 0, nil
		}
		return 1, nil
	}

	action := strings.TrimSpace(opts.Action)
	if action == FixActionIgnore {
		return 2, errors.New("ignore action is interactive-only; use `bb fix`")
	}

	if !containsAction(eligible, action) {
		a.renderFixStatus(target.Record, eligible)
		fmt.Fprintf(a.Stdout, "action %q is not eligible for %s\n", action, target.Record.Name)
		return 1, nil
	}

	updated, err := a.applyFixAction(opts.IncludeCatalogs, target.Record.Path, action, opts.CommitMessage)
	if errors.Is(err, errFixActionNotEligible) {
		return 1, nil
	}
	if err != nil {
		return 2, err
	}

	fmt.Fprintf(a.Stdout, "applied %s to %s\n", action, updated.Record.Name)
	a.renderFixStatus(updated.Record, eligibleFixActions(updated.Record, updated.Meta))
	if updated.Record.Syncable {
		return 0, nil
	}
	return 1, nil
}

func (a *App) renderFixStatus(rec domain.MachineRepoRecord, actions []string) {
	fmt.Fprintf(a.Stdout, "repo: %s\n", rec.Name)
	fmt.Fprintf(a.Stdout, "path: %s\n", rec.Path)
	fmt.Fprintf(a.Stdout, "catalog: %s\n", rec.Catalog)
	fmt.Fprintf(a.Stdout, "syncable: %t\n", rec.Syncable)
	if len(rec.UnsyncableReasons) == 0 {
		fmt.Fprintln(a.Stdout, "reasons: none")
	} else {
		parts := make([]string, 0, len(rec.UnsyncableReasons))
		for _, r := range rec.UnsyncableReasons {
			parts = append(parts, string(r))
		}
		sort.Strings(parts)
		fmt.Fprintf(a.Stdout, "reasons: %s\n", strings.Join(parts, ", "))
	}
	if len(actions) == 0 {
		fmt.Fprintln(a.Stdout, "actions: none")
		return
	}
	fmt.Fprintf(a.Stdout, "actions: %s\n", strings.Join(actions, ", "))
}

func containsAction(actions []string, action string) bool {
	for _, candidate := range actions {
		if candidate == action {
			return true
		}
	}
	return false
}

func eligibleFixActions(rec domain.MachineRepoRecord, meta *domain.RepoMetadataFile) []string {
	if rec.OperationInProgress != domain.OperationNone && rec.OperationInProgress != "" {
		return []string{FixActionAbortOperation}
	}

	actions := make([]string, 0, 5)
	if rec.OriginURL != "" && rec.Upstream != "" && rec.Ahead > 0 && !rec.Diverged {
		actions = append(actions, FixActionPush)
	}
	if rec.OriginURL != "" && (rec.HasDirtyTracked || rec.HasUntracked) && !rec.Diverged {
		actions = append(actions, FixActionStageCommitPush)
	}
	if rec.Upstream != "" && rec.Behind > 0 && rec.Ahead == 0 && !rec.Diverged && !rec.HasDirtyTracked && !rec.HasUntracked {
		actions = append(actions, FixActionPullFFOnly)
	}
	if rec.OriginURL != "" && rec.Upstream == "" && rec.Branch != "" && !rec.Diverged {
		actions = append(actions, FixActionSetUpstreamPush)
	}
	if meta != nil && !meta.AutoPush && strings.TrimSpace(rec.RepoID) != "" {
		actions = append(actions, FixActionEnableAutoPush)
	}
	return actions
}

func resolveFixTarget(selector string, repos []fixRepoState) (fixRepoState, error) {
	if len(repos) == 0 {
		return fixRepoState{}, errors.New("no repositories found for selected catalogs")
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return fixRepoState{}, errors.New("project is required")
	}

	selectorPath := filepath.Clean(selector)
	for _, repo := range repos {
		if filepath.Clean(repo.Record.Path) == selectorPath {
			return repo, nil
		}
	}

	for _, repo := range repos {
		if strings.EqualFold(strings.TrimSpace(repo.Record.RepoID), selector) {
			return repo, nil
		}
	}

	matches := make([]fixRepoState, 0, 2)
	for _, repo := range repos {
		if repo.Record.Name == selector {
			matches = append(matches, repo)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.Record.Path)
		}
		sort.Strings(paths)
		return fixRepoState{}, fmt.Errorf("project selector %q is ambiguous; matches: %s", selector, strings.Join(paths, ", "))
	}

	return fixRepoState{}, fmt.Errorf("project %q not found", selector)
}

func (a *App) loadFixRepos(includeCatalogs []string, refresh bool) ([]fixRepoState, error) {
	a.logf("fix: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("fix: released global lock")
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return nil, err
	}
	if refresh {
		if _, err := a.scanAndPublish(cfg, &machine, ScanOptions{IncludeCatalogs: includeCatalogs, AllowPush: false}); err != nil {
			return nil, err
		}
	} else {
		a.logf("fix: using existing machine snapshot without refresh")
	}

	metas, err := state.LoadAllRepoMetadata(a.Paths)
	if err != nil {
		return nil, err
	}
	metaByRepoID := make(map[string]*domain.RepoMetadataFile, len(metas))
	for i := range metas {
		meta := metas[i]
		metaByRepoID[meta.RepoID] = &meta
	}

	out := make([]fixRepoState, 0, len(machine.Repos))
	for _, rec := range machine.Repos {
		out = append(out, fixRepoState{
			Record: rec,
			Meta:   metaByRepoID[rec.RepoID],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Record.Syncable != out[j].Record.Syncable {
			return !out[i].Record.Syncable
		}
		if out[i].Record.Name != out[j].Record.Name {
			return out[i].Record.Name < out[j].Record.Name
		}
		return out[i].Record.Path < out[j].Record.Path
	})

	return out, nil
}

func (a *App) applyFixAction(includeCatalogs []string, repoPath string, action string, commitMessage string) (fixRepoState, error) {
	a.logf("fix: acquiring global lock for action %s", action)
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return fixRepoState{}, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("fix: released global lock for action %s", action)
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return fixRepoState{}, err
	}
	if _, err := a.scanAndPublish(cfg, &machine, ScanOptions{IncludeCatalogs: includeCatalogs, AllowPush: false}); err != nil {
		return fixRepoState{}, err
	}

	repos, err := a.loadFixReposUnlocked(machine)
	if err != nil {
		return fixRepoState{}, err
	}
	target, err := resolveFixTarget(repoPath, repos)
	if err != nil {
		return fixRepoState{}, err
	}
	eligible := eligibleFixActions(target.Record, target.Meta)
	if !containsAction(eligible, action) {
		return target, errFixActionNotEligible
	}

	if err := a.executeFixAction(cfg, target, action, commitMessage); err != nil {
		return fixRepoState{}, err
	}
	if _, err := a.scanAndPublish(cfg, &machine, ScanOptions{IncludeCatalogs: includeCatalogs, AllowPush: false}); err != nil {
		return fixRepoState{}, err
	}
	refreshedRepos, err := a.loadFixReposUnlocked(machine)
	if err != nil {
		return fixRepoState{}, err
	}
	return resolveFixTarget(target.Record.Path, refreshedRepos)
}

func (a *App) loadFixReposUnlocked(machine domain.MachineFile) ([]fixRepoState, error) {
	metas, err := state.LoadAllRepoMetadata(a.Paths)
	if err != nil {
		return nil, err
	}
	metaByRepoID := make(map[string]*domain.RepoMetadataFile, len(metas))
	for i := range metas {
		meta := metas[i]
		metaByRepoID[meta.RepoID] = &meta
	}

	out := make([]fixRepoState, 0, len(machine.Repos))
	for _, rec := range machine.Repos {
		out = append(out, fixRepoState{
			Record: rec,
			Meta:   metaByRepoID[rec.RepoID],
		})
	}
	return out, nil
}

func (a *App) executeFixAction(cfg domain.ConfigFile, target fixRepoState, action string, commitMessage string) error {
	path := target.Record.Path
	switch action {
	case FixActionAbortOperation:
		switch target.Record.OperationInProgress {
		case domain.OperationMerge:
			return a.Git.MergeAbort(path)
		case domain.OperationRebase:
			return a.Git.RebaseAbort(path)
		case domain.OperationCherryPick:
			return a.Git.CherryPickAbort(path)
		case domain.OperationBisect:
			return a.Git.BisectReset(path)
		default:
			return fmt.Errorf("no operation in progress for %s", target.Record.Name)
		}
	case FixActionPush:
		return a.Git.Push(path)
	case FixActionStageCommitPush:
		msg := strings.TrimSpace(commitMessage)
		if msg == "" || msg == "auto" {
			msg = AutoFixCommitMessage
		}
		if err := a.Git.AddAll(path); err != nil {
			return err
		}
		if err := a.Git.Commit(path, msg); err != nil {
			return err
		}
		if target.Record.Upstream == "" {
			branch := target.Record.Branch
			if branch == "" {
				branch, _ = a.Git.CurrentBranch(path)
			}
			if strings.TrimSpace(branch) == "" {
				return errors.New("cannot determine branch for upstream push")
			}
			return a.Git.PushUpstream(path, branch)
		}
		return a.Git.Push(path)
	case FixActionPullFFOnly:
		if cfg.Sync.FetchPrune {
			if err := a.Git.FetchPrune(path); err != nil {
				return err
			}
		}
		return a.Git.PullFFOnly(path)
	case FixActionSetUpstreamPush:
		branch := target.Record.Branch
		if branch == "" {
			branch, _ = a.Git.CurrentBranch(path)
		}
		if strings.TrimSpace(branch) == "" {
			return errors.New("cannot determine branch for upstream push")
		}
		return a.Git.PushUpstream(path, branch)
	case FixActionEnableAutoPush:
		if strings.TrimSpace(target.Record.RepoID) == "" {
			return errors.New("repo_id is required for enable-auto-push")
		}
		meta, err := state.LoadRepoMetadata(a.Paths, target.Record.RepoID)
		if err != nil {
			return err
		}
		meta.AutoPush = true
		return state.SaveRepoMetadata(a.Paths, meta)
	default:
		return fmt.Errorf("unknown fix action %q", action)
	}
}
