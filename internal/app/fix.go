package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

const (
	FixActionIgnore          = "ignore"
	FixActionAbortOperation  = "abort-operation"
	FixActionCreateProject   = "create-project"
	FixActionForkAndRetarget = "fork-and-retarget"
	FixActionPush            = "push"
	FixActionStageCommitPush = "stage-commit-push"
	FixActionPullFFOnly      = "pull-ff-only"
	FixActionSetUpstreamPush = "set-upstream-push"
	FixActionEnableAutoPush  = "enable-auto-push"

	DefaultFixCommitMessage = "bb: checkpoint local changes before sync"
)

var errFixActionNotEligible = errors.New("fix action not eligible")

type fixRepoState struct {
	Record           domain.MachineRepoRecord
	Meta             *domain.RepoMetadataFile
	Risk             fixRiskSnapshot
	IsDefaultCatalog bool
}

type fixApplyOptions struct {
	Interactive             bool
	CommitMessage           string
	CreateProjectName       string
	CreateProjectVisibility domain.Visibility
	GenerateGitignore       bool
	GitignorePatterns       []string
}

type fixIneligibleError struct {
	Action string
	Reason string
}

func (e *fixIneligibleError) Error() string {
	if e == nil {
		return errFixActionNotEligible.Error()
	}
	if strings.TrimSpace(e.Reason) == "" {
		return errFixActionNotEligible.Error()
	}
	return fmt.Sprintf("%s: %s", errFixActionNotEligible.Error(), e.Reason)
}

func (e *fixIneligibleError) Unwrap() error {
	return errFixActionNotEligible
}

func (a *App) runFix(opts FixOptions) (int, error) {
	if strings.TrimSpace(opts.Project) == "" && strings.TrimSpace(opts.Action) == "" {
		if a.IsInteractiveTerminal == nil || !a.IsInteractiveTerminal() {
			return 2, errors.New("bb fix requires an interactive terminal")
		}
		return a.runFixInteractiveWithMutedLogs(opts.IncludeCatalogs, opts.NoRefresh)
	}

	refreshMode := scanRefreshIfStale
	if opts.NoRefresh {
		refreshMode = scanRefreshNever
	}
	repos, err := a.loadFixRepos(opts.IncludeCatalogs, refreshMode)
	if err != nil {
		return 2, err
	}

	target, err := resolveFixTarget(opts.Project, repos)
	if err != nil {
		return 2, err
	}

	eligibility := fixEligibilityContext{Interactive: false, Risk: target.Risk}
	eligible := eligibleFixActions(target.Record, target.Meta, eligibility)
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
		if reason := ineligibleFixReason(action, eligibility); reason != "" {
			fmt.Fprintln(a.Stdout, reason)
		}
		return 1, nil
	}

	updated, err := a.applyFixAction(opts.IncludeCatalogs, target.Record.Path, action, fixApplyOptions{
		Interactive:   false,
		CommitMessage: opts.CommitMessage,
	})
	if errors.Is(err, errFixActionNotEligible) {
		var ineligibleErr *fixIneligibleError
		if errors.As(err, &ineligibleErr) && strings.TrimSpace(ineligibleErr.Reason) != "" {
			fmt.Fprintln(a.Stdout, ineligibleErr.Reason)
		}
		return 1, nil
	}
	if err != nil {
		return 2, err
	}

	fmt.Fprintf(a.Stdout, "applied %s to %s\n", action, updated.Record.Name)
	a.renderFixStatus(updated.Record, eligibleFixActions(updated.Record, updated.Meta, fixEligibilityContext{
		Interactive: false,
		Risk:        updated.Risk,
	}))
	if updated.Record.Syncable {
		return 0, nil
	}
	return 1, nil
}

func (a *App) runFixInteractiveWithMutedLogs(includeCatalogs []string, noRefresh bool) (int, error) {
	runInteractive := a.runFixInteractive
	if a.runFixInteractiveFn != nil {
		runInteractive = a.runFixInteractiveFn
	}

	previousVerbose := a.isVerbose()
	a.SetVerbose(false)
	defer a.SetVerbose(previousVerbose)

	return runInteractive(includeCatalogs, noRefresh)
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

func eligibleFixActions(rec domain.MachineRepoRecord, meta *domain.RepoMetadataFile, ctx fixEligibilityContext) []string {
	if rec.OperationInProgress != domain.OperationNone && rec.OperationInProgress != "" {
		return []string{FixActionAbortOperation}
	}

	actions := make([]string, 0, 5)
	pushAllowed := true
	if meta != nil && strings.TrimSpace(rec.OriginURL) != "" && !pushAccessAllowsAutoPush(meta.PushAccess) {
		pushAllowed = false
	}
	if rec.OriginURL == "" {
		actions = append(actions, FixActionCreateProject)
	}
	if rec.OriginURL != "" && rec.Upstream != "" && rec.Ahead > 0 && !rec.Diverged && pushAllowed {
		actions = append(actions, FixActionPush)
	}
	if (rec.HasDirtyTracked || rec.HasUntracked) && !rec.Diverged &&
		(strings.TrimSpace(rec.OriginURL) == "" || pushAllowed) &&
		!ctx.Risk.hasSecretLikeChanges() &&
		!(ctx.Risk.hasNoisyChangesWithoutGitignore() && !ctx.Interactive) {
		actions = append(actions, FixActionStageCommitPush)
	}
	if rec.Upstream != "" && rec.Behind > 0 && rec.Ahead == 0 && !rec.Diverged && !rec.HasDirtyTracked && !rec.HasUntracked {
		actions = append(actions, FixActionPullFFOnly)
	}
	if rec.OriginURL != "" && rec.Upstream == "" && rec.Branch != "" && !rec.Diverged && pushAllowed {
		actions = append(actions, FixActionSetUpstreamPush)
	}
	if rec.OriginURL != "" && !pushAllowed && strings.TrimSpace(rec.RepoKey) != "" {
		actions = append(actions, FixActionForkAndRetarget)
	}
	if meta != nil && !meta.AutoPush && strings.TrimSpace(rec.RepoKey) != "" && pushAccessAllowsAutoPush(meta.PushAccess) {
		actions = append(actions, FixActionEnableAutoPush)
	}
	return actions
}

func ineligibleFixReason(action string, ctx fixEligibilityContext) string {
	if action != FixActionStageCommitPush {
		return ""
	}
	if ctx.Risk.hasSecretLikeChanges() {
		return fmt.Sprintf(
			"stage-commit-push is blocked: secret-like uncommitted files detected (%s)",
			strings.Join(ctx.Risk.SecretLikeChangedPaths, ", "),
		)
	}
	if ctx.Risk.hasNoisyChangesWithoutGitignore() && !ctx.Interactive {
		return "stage-commit-push is blocked: root .gitignore is missing and noisy uncommitted paths were detected; run interactive `bb fix` to review/generate .gitignore or add it manually"
	}
	return ""
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

	repoKeyMatches := make([]fixRepoState, 0, 2)
	for _, repo := range repos {
		if strings.EqualFold(strings.TrimSpace(repo.Record.RepoKey), selector) {
			repoKeyMatches = append(repoKeyMatches, repo)
		}
	}
	if len(repoKeyMatches) == 1 {
		return repoKeyMatches[0], nil
	}
	if len(repoKeyMatches) > 1 {
		paths := make([]string, 0, len(repoKeyMatches))
		for _, match := range repoKeyMatches {
			paths = append(paths, match.Record.Path)
		}
		sort.Strings(paths)
		return fixRepoState{}, fmt.Errorf("project selector %q is ambiguous; matches: %s", selector, strings.Join(paths, ", "))
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

func (a *App) loadFixRepos(includeCatalogs []string, refreshMode scanRefreshMode) ([]fixRepoState, error) {
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
	if err := a.refreshMachineSnapshotLocked(cfg, &machine, includeCatalogs, refreshMode); err != nil {
		return nil, err
	}
	if refreshMode == scanRefreshNever {
		a.logf("fix: using existing machine snapshot without refresh")
	}

	metas, err := state.LoadAllRepoMetadata(a.Paths)
	if err != nil {
		return nil, err
	}
	metaByRepoKey := make(map[string]*domain.RepoMetadataFile, len(metas))
	for i := range metas {
		meta := metas[i]
		metaByRepoKey[meta.RepoKey] = &meta
	}

	out := make([]fixRepoState, 0, len(machine.Repos))
	for _, rec := range machine.Repos {
		risk, riskErr := collectFixRiskSnapshot(rec.Path, a.Git)
		if riskErr != nil && !errors.Is(riskErr, os.ErrNotExist) {
			a.logf("fix: risk scan failed for %s: %v", rec.Path, riskErr)
		}
		out = append(out, fixRepoState{
			Record:           rec,
			Meta:             metaByRepoKey[rec.RepoKey],
			Risk:             risk,
			IsDefaultCatalog: rec.Catalog == machine.DefaultCatalog,
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

func (a *App) applyFixAction(includeCatalogs []string, repoPath string, action string, opts fixApplyOptions) (fixRepoState, error) {
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
	if err := a.refreshMachineSnapshotLocked(cfg, &machine, includeCatalogs, scanRefreshIfStale); err != nil {
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
	eligibility := fixEligibilityContext{
		Interactive: opts.Interactive,
		Risk:        target.Risk,
	}
	eligible := eligibleFixActions(target.Record, target.Meta, eligibility)
	if !containsAction(eligible, action) {
		return target, &fixIneligibleError{
			Action: action,
			Reason: ineligibleFixReason(action, eligibility),
		}
	}

	if err := a.executeFixAction(cfg, target, action, opts); err != nil {
		return fixRepoState{}, err
	}
	if err := a.refreshMachineSnapshotLocked(cfg, &machine, includeCatalogs, scanRefreshAlways); err != nil {
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
	metaByRepoKey := make(map[string]*domain.RepoMetadataFile, len(metas))
	for i := range metas {
		meta := metas[i]
		metaByRepoKey[meta.RepoKey] = &meta
	}

	out := make([]fixRepoState, 0, len(machine.Repos))
	for _, rec := range machine.Repos {
		risk, riskErr := collectFixRiskSnapshot(rec.Path, a.Git)
		if riskErr != nil && !errors.Is(riskErr, os.ErrNotExist) {
			a.logf("fix: risk scan failed for %s: %v", rec.Path, riskErr)
		}
		out = append(out, fixRepoState{
			Record:           rec,
			Meta:             metaByRepoKey[rec.RepoKey],
			Risk:             risk,
			IsDefaultCatalog: rec.Catalog == machine.DefaultCatalog,
		})
	}
	return out, nil
}

func (a *App) executeFixAction(cfg domain.ConfigFile, target fixRepoState, action string, opts fixApplyOptions) error {
	path := target.Record.Path
	preferredRemote := ""
	if target.Meta != nil {
		preferredRemote = strings.TrimSpace(target.Meta.PreferredRemote)
	}
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
	case FixActionCreateProject:
		if opts.GenerateGitignore && len(opts.GitignorePatterns) > 0 {
			if err := writeOrAppendGitignore(path, opts.GitignorePatterns); err != nil {
				return err
			}
		}
		return a.createProjectFromFix(cfg, target, opts.CreateProjectName, opts.CreateProjectVisibility)
	case FixActionForkAndRetarget:
		return a.forkAndRetargetFromFix(cfg, target)
	case FixActionStageCommitPush:
		if opts.GenerateGitignore && len(opts.GitignorePatterns) > 0 {
			if err := writeOrAppendGitignore(path, opts.GitignorePatterns); err != nil {
				return err
			}
		}
		msg := strings.TrimSpace(opts.CommitMessage)
		if msg == "" || msg == "auto" {
			msg = DefaultFixCommitMessage
		}
		if err := a.Git.AddAll(path); err != nil {
			return err
		}
		if err := a.Git.Commit(path, msg); err != nil {
			return err
		}
		if strings.TrimSpace(target.Record.OriginURL) == "" {
			return nil
		}
		if target.Record.Upstream == "" {
			branch := target.Record.Branch
			if branch == "" {
				branch, _ = a.Git.CurrentBranch(path)
			}
			if strings.TrimSpace(branch) == "" {
				return errors.New("cannot determine branch for upstream push")
			}
			return a.Git.PushUpstreamWithPreferredRemote(path, branch, preferredRemote)
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
		return a.Git.PushUpstreamWithPreferredRemote(path, branch, preferredRemote)
	case FixActionEnableAutoPush:
		if strings.TrimSpace(target.Record.RepoKey) == "" {
			return errors.New("repo_key is required for enable-auto-push")
		}
		meta, err := state.LoadRepoMetadata(a.Paths, target.Record.RepoKey)
		if err != nil {
			return err
		}
		meta.AutoPush = true
		return state.SaveRepoMetadata(a.Paths, meta)
	default:
		return fmt.Errorf("unknown fix action %q", action)
	}
}

func (a *App) forkAndRetargetFromFix(cfg domain.ConfigFile, target fixRepoState) error {
	if target.Meta == nil {
		return errors.New("repo metadata is required for fork-and-retarget")
	}
	owner := strings.TrimSpace(cfg.GitHub.Owner)
	if owner == "" {
		return errors.New("github.owner is required; run 'bb config' and set github.owner")
	}

	repoPath := target.Record.Path
	originURL := strings.TrimSpace(target.Record.OriginURL)
	if originURL == "" {
		return errors.New("origin URL is required for fork-and-retarget")
	}

	forkRemoteName := strings.TrimSpace(owner)
	if forkRemoteName == "" {
		return errors.New("fork remote name is required")
	}

	forkURL, err := a.ensureForkRemoteRepo(originURL, owner, cfg.GitHub.RemoteProtocol, repoPath)
	if err != nil {
		return err
	}

	remoteExists := false
	remoteNames, err := a.Git.RemoteNames(repoPath)
	if err == nil {
		for _, name := range remoteNames {
			if name == forkRemoteName {
				remoteExists = true
				break
			}
		}
	}
	if remoteExists {
		if err := a.Git.SetRemoteURL(repoPath, forkRemoteName, forkURL); err != nil {
			return err
		}
	} else {
		if err := a.Git.AddRemote(repoPath, forkRemoteName, forkURL); err != nil {
			return err
		}
	}

	branch := strings.TrimSpace(target.Record.Branch)
	if branch == "" {
		branch, _ = a.Git.CurrentBranch(repoPath)
	}
	if strings.TrimSpace(branch) == "" {
		return errors.New("cannot determine branch for fork-and-retarget")
	}
	if err := a.Git.PushUpstreamWithPreferredRemote(repoPath, branch, forkRemoteName); err != nil {
		return err
	}

	meta := *target.Meta
	meta.PreferredRemote = forkRemoteName
	meta.PushAccess = domain.PushAccessUnknown
	meta.PushAccessCheckedAt = time.Time{}
	meta.PushAccessCheckedRemote = ""
	meta.PushAccessManualOverride = false

	updated, _, err := a.probeAndUpdateRepoPushAccess(repoPath, target.Record.OriginURL, meta, true)
	if err != nil {
		return err
	}
	if err := state.SaveRepoMetadata(a.Paths, updated); err != nil {
		return err
	}
	return nil
}

func (a *App) createProjectFromFix(cfg domain.ConfigFile, target fixRepoState, projectNameOverride string, visibilityOverride domain.Visibility) error {
	owner := strings.TrimSpace(cfg.GitHub.Owner)
	if owner == "" {
		return errors.New("github.owner is required; run 'bb config' and set github.owner")
	}
	if strings.TrimSpace(target.Record.Catalog) == "" {
		return errors.New("catalog is required for create-project")
	}

	projectName := strings.TrimSpace(projectNameOverride)
	if projectName == "" {
		projectName = strings.TrimSpace(target.Record.Name)
	}
	if projectName == "" {
		projectName = filepath.Base(target.Record.Path)
	}
	if strings.TrimSpace(projectName) == "" {
		return errors.New("project name is required for create-project")
	}

	visibility := resolveCreateProjectVisibility(cfg, visibilityOverride)
	expectedOrigin, err := a.expectedOrigin(owner, projectName, cfg.GitHub.RemoteProtocol)
	if err != nil {
		return err
	}

	origin, err := a.Git.RepoOrigin(target.Record.Path)
	if err != nil {
		return err
	}
	if origin != "" {
		matches, err := originsMatchNormalized(origin, expectedOrigin)
		if err != nil {
			return fmt.Errorf("invalid existing origin: %w", err)
		}
		if !matches {
			return fmt.Errorf("conflicting origin: existing %q does not match expected %q", origin, expectedOrigin)
		}
	} else {
		createdOrigin, err := a.createRemoteRepo(owner, projectName, visibility, cfg.GitHub.RemoteProtocol, target.Record.Path)
		if err != nil {
			return err
		}
		if err := a.Git.AddOrigin(target.Record.Path, createdOrigin); err != nil {
			return fmt.Errorf("set origin failed: %w", err)
		}
		origin = createdOrigin
	}

	if strings.TrimSpace(target.Record.RepoKey) == "" {
		return errors.New("repo_key is required for create-project")
	}
	meta, _, err := a.ensureRepoMetadata(cfg, target.Record.RepoKey, projectName, origin, visibility, target.Record.Catalog)
	if err != nil {
		return err
	}

	branch, _ := a.Git.CurrentBranch(target.Record.Path)
	headSHA, _ := a.Git.HeadSHA(target.Record.Path)
	upstream, _ := a.Git.Upstream(target.Record.Path)
	if headSHA != "" && upstream == "" {
		if branch == "" {
			branch = "main"
		}
		if err := a.Git.PushUpstreamWithPreferredRemote(target.Record.Path, branch, meta.PreferredRemote); err != nil {
			return fmt.Errorf("initial push failed: %w", err)
		}
	}

	return nil
}

func resolveCreateProjectVisibility(cfg domain.ConfigFile, override domain.Visibility) domain.Visibility {
	if override == domain.VisibilityPrivate || override == domain.VisibilityPublic {
		return override
	}
	switch strings.ToLower(strings.TrimSpace(cfg.GitHub.DefaultVisibility)) {
	case string(domain.VisibilityPublic):
		return domain.VisibilityPublic
	default:
		return domain.VisibilityPrivate
	}
}

func writeGeneratedGitignore(repoPath string, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	cleaned := make([]string, 0, len(patterns))
	seen := map[string]struct{}{}
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		cleaned = append(cleaned, pattern)
	}
	if len(cleaned) == 0 {
		return nil
	}
	sort.Strings(cleaned)
	content := "# Generated by bb fix\n" + strings.Join(cleaned, "\n") + "\n"
	return os.WriteFile(gitignorePath, []byte(content), 0o644)
}

func writeOrAppendGitignore(repoPath string, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	if _, err := os.Stat(gitignorePath); err != nil {
		if os.IsNotExist(err) {
			return writeGeneratedGitignore(repoPath, patterns)
		}
		return err
	}

	raw, err := os.ReadFile(gitignorePath)
	if err != nil {
		return err
	}
	missing := collectMissingGitignorePatterns(repoPath, patterns)
	if len(missing) == 0 {
		return nil
	}

	content := string(raw)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "# Added by bb fix\n" + strings.Join(missing, "\n") + "\n"
	return os.WriteFile(gitignorePath, []byte(content), 0o644)
}
