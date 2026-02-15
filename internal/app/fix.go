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
	"bb-project/internal/gitx"
	"bb-project/internal/state"
)

const (
	FixActionIgnore           = "ignore"
	FixActionAbortOperation   = "abort-operation"
	FixActionCreateProject    = "create-project"
	FixActionForkAndRetarget  = "fork-and-retarget"
	FixActionSyncWithUpstream = "sync-with-upstream"
	FixActionPush             = "push"
	FixActionStageCommitPush  = "stage-commit-push"
	FixActionPullFFOnly       = "pull-ff-only"
	FixActionSetUpstreamPush  = "set-upstream-push"
	FixActionEnableAutoPush   = "enable-auto-push"

	DefaultFixCommitMessage = "bb: checkpoint local changes before sync"
)

var errFixActionNotEligible = errors.New("fix action not eligible")

type fixRepoState struct {
	Record           domain.MachineRepoRecord
	Meta             *domain.RepoMetadataFile
	Risk             fixRiskSnapshot
	SyncFeasibility  fixSyncFeasibility
	IsDefaultCatalog bool
}

type fixApplyOptions struct {
	Interactive             bool
	CommitMessage           string
	SyncStrategy            FixSyncStrategy
	CreateProjectName       string
	CreateProjectVisibility domain.Visibility
	GenerateGitignore       bool
	GitignorePatterns       []string
}

type fixApplyStepStatus string

const (
	fixApplyStepRunning fixApplyStepStatus = "running"
	fixApplyStepDone    fixApplyStepStatus = "done"
	fixApplyStepFailed  fixApplyStepStatus = "failed"
	fixApplyStepSkipped fixApplyStepStatus = "skipped"
)

type fixApplyStepEvent struct {
	Entry  fixActionPlanEntry
	Status fixApplyStepStatus
	Err    error
}

type fixApplyStepObserver func(event fixApplyStepEvent)

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
	strategy := normalizeFixSyncStrategy(opts.SyncStrategy)

	eligibility := fixEligibilityContext{
		Interactive:     false,
		Risk:            target.Risk,
		SyncStrategy:    strategy,
		SyncFeasibility: target.SyncFeasibility,
	}
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
		if reason := ineligibleFixReason(action, target.Record, eligibility); reason != "" {
			fmt.Fprintln(a.Stdout, reason)
		}
		return 1, nil
	}

	updated, err := a.applyFixAction(opts.IncludeCatalogs, target.Record.Path, action, fixApplyOptions{
		Interactive:   false,
		CommitMessage: opts.CommitMessage,
		SyncStrategy:  strategy,
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
		Interactive:     false,
		Risk:            updated.Risk,
		SyncStrategy:    strategy,
		SyncFeasibility: updated.SyncFeasibility,
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

func containsUnsyncableReason(reasons []domain.UnsyncableReason, reason domain.UnsyncableReason) bool {
	for _, candidate := range reasons {
		if candidate == reason {
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
	if rec.OriginURL != "" &&
		rec.Upstream != "" &&
		rec.Diverged &&
		!rec.HasDirtyTracked &&
		!rec.HasUntracked &&
		ctx.SyncFeasibility.canAttemptFor(ctx.SyncStrategy) {
		actions = append(actions, FixActionSyncWithUpstream)
	}
	if rec.OriginURL != "" && rec.Upstream != "" && rec.Ahead > 0 && !rec.Diverged && pushAllowed {
		actions = append(actions, FixActionPush)
	}
	pushBeforeCommitAllowed := strings.TrimSpace(rec.OriginURL) == "" || strings.TrimSpace(rec.Upstream) == "" || (!rec.Diverged && rec.Behind == 0)
	if (rec.HasDirtyTracked || rec.HasUntracked) && !rec.Diverged &&
		pushBeforeCommitAllowed &&
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
	if meta != nil && strings.TrimSpace(rec.RepoKey) != "" && pushAccessAllowsAutoPush(meta.PushAccess) {
		mode := domain.NormalizeAutoPushMode(meta.AutoPush)
		if mode == domain.AutoPushModeDisabled || (mode == domain.AutoPushModeEnabled && containsUnsyncableReason(rec.UnsyncableReasons, domain.ReasonPushPolicyBlocked)) {
			actions = append(actions, FixActionEnableAutoPush)
		}
	}
	return actions
}

func ineligibleFixReason(action string, rec domain.MachineRepoRecord, ctx fixEligibilityContext) string {
	if action == FixActionStageCommitPush {
		if strings.TrimSpace(rec.OriginURL) != "" && strings.TrimSpace(rec.Upstream) != "" && (rec.Diverged || rec.Behind > 0) {
			return "stage-commit-push is blocked: branch is behind upstream, so push would be rejected; run sync-with-upstream first"
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
	if action == FixActionSyncWithUpstream {
		if !ctx.SyncFeasibility.Checked {
			return "sync-with-upstream is blocked: sync feasibility has not been validated"
		}
		if ctx.SyncFeasibility.conflictFor(ctx.SyncStrategy) {
			return fmt.Sprintf(
				"sync-with-upstream is blocked: %s (selected strategy: %s)",
				domain.ReasonSyncConflict,
				normalizeFixSyncStrategy(ctx.SyncStrategy),
			)
		}
		if ctx.SyncFeasibility.probeFailedFor(ctx.SyncStrategy) {
			return fmt.Sprintf(
				"sync-with-upstream is blocked: %s (selected strategy: %s)",
				domain.ReasonSyncProbeFailed,
				normalizeFixSyncStrategy(ctx.SyncStrategy),
			)
		}
		if !ctx.SyncFeasibility.cleanFor(ctx.SyncStrategy) {
			return fmt.Sprintf(
				"sync-with-upstream is blocked: sync strategy %s is not marked clean by feasibility validation",
				normalizeFixSyncStrategy(ctx.SyncStrategy),
			)
		}
		return ""
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
		rec, syncFeasibility := a.enrichFixSyncFeasibility(rec)
		risk, riskErr := collectFixRiskSnapshot(rec.Path, a.Git)
		if riskErr != nil && !errors.Is(riskErr, os.ErrNotExist) {
			a.logf("fix: risk scan failed for %s: %v", rec.Path, riskErr)
		}
		out = append(out, fixRepoState{
			Record:           rec,
			Meta:             metaByRepoKey[rec.RepoKey],
			Risk:             risk,
			SyncFeasibility:  syncFeasibility,
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

func (a *App) enrichFixSyncFeasibility(rec domain.MachineRepoRecord) (domain.MachineRepoRecord, fixSyncFeasibility) {
	feasibility := fixSyncFeasibility{
		RebaseOutcome: fixSyncProbeUnknown,
		MergeOutcome:  fixSyncProbeUnknown,
	}
	if strings.TrimSpace(rec.OriginURL) == "" ||
		strings.TrimSpace(rec.Upstream) == "" ||
		!rec.Diverged ||
		rec.HasDirtyTracked ||
		rec.HasUntracked ||
		(rec.OperationInProgress != "" && rec.OperationInProgress != domain.OperationNone) {
		return rec, feasibility
	}

	feasibility.Checked = true

	rebaseOutcome, err := a.Git.ProbeSyncWithUpstream(rec.Path, rec.Upstream, string(FixSyncStrategyRebase))
	if err != nil {
		a.logf("fix: sync feasibility rebase probe failed for %s: %v", rec.Path, err)
		feasibility.setOutcome(FixSyncStrategyRebase, fixSyncProbeFailed)
	} else {
		feasibility.setOutcome(FixSyncStrategyRebase, mapSyncProbeOutcome(rebaseOutcome))
	}

	mergeOutcome, err := a.Git.ProbeSyncWithUpstream(rec.Path, rec.Upstream, string(FixSyncStrategyMerge))
	if err != nil {
		a.logf("fix: sync feasibility merge probe failed for %s: %v", rec.Path, err)
		feasibility.setOutcome(FixSyncStrategyMerge, fixSyncProbeFailed)
	} else {
		feasibility.setOutcome(FixSyncStrategyMerge, mapSyncProbeOutcome(mergeOutcome))
	}

	if feasibility.defaultStrategyConflictWithoutCleanFallback() {
		rec.UnsyncableReasons = appendUniqueUnsyncableReason(rec.UnsyncableReasons, domain.ReasonSyncConflict)
		rec.Syncable = false
		rec.StateHash = domain.ComputeStateHash(rec)
	}
	if !feasibility.cleanFor(FixSyncStrategyRebase) &&
		!feasibility.cleanFor(FixSyncStrategyMerge) &&
		feasibility.anyProbeFailed() {
		rec.UnsyncableReasons = appendUniqueUnsyncableReason(rec.UnsyncableReasons, domain.ReasonSyncProbeFailed)
		rec.Syncable = false
		rec.StateHash = domain.ComputeStateHash(rec)
	}
	return rec, feasibility
}

func mapSyncProbeOutcome(in gitx.SyncProbeOutcome) fixSyncProbeOutcome {
	switch in {
	case gitx.SyncProbeOutcomeClean:
		return fixSyncProbeClean
	case gitx.SyncProbeOutcomeConflict:
		return fixSyncProbeConflict
	case gitx.SyncProbeOutcomeFailed:
		return fixSyncProbeFailed
	default:
		return fixSyncProbeUnknown
	}
}

func (a *App) applyFixAction(includeCatalogs []string, repoPath string, action string, opts fixApplyOptions) (fixRepoState, error) {
	return a.applyFixActionWithObserver(includeCatalogs, repoPath, action, opts, nil)
}

func (a *App) applyFixActionWithObserver(
	includeCatalogs []string,
	repoPath string,
	action string,
	opts fixApplyOptions,
	observer fixApplyStepObserver,
) (fixRepoState, error) {
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
		Interactive:     opts.Interactive,
		Risk:            target.Risk,
		SyncStrategy:    normalizeFixSyncStrategy(opts.SyncStrategy),
		SyncFeasibility: target.SyncFeasibility,
	}
	eligible := eligibleFixActions(target.Record, target.Meta, eligibility)
	if !containsAction(eligible, action) {
		return target, &fixIneligibleError{
			Action: action,
			Reason: ineligibleFixReason(action, target.Record, eligibility),
		}
	}

	planByID := fixActionPlanEntriesByID(
		fixActionExecutionPlanFor(action, buildFixActionPlanContext(cfg, target, opts)),
	)
	refreshEntry := lookupFixActionPlanEntry(planByID, fixActionPlanRevalidateStateID, fixActionPlanEntry{
		ID:      fixActionPlanRevalidateStateID,
		Command: false,
		Summary: "Revalidate repository status and syncability state.",
	})

	if err := a.executeFixAction(cfg, target, action, opts, observer); err != nil {
		return fixRepoState{}, err
	}

	if err := runFixApplyStep(observer, refreshEntry, func() error {
		if err := a.refreshFixRepoSnapshotLocked(cfg, &machine, target.Record.Path); err != nil {
			a.logf("fix: targeted revalidation failed for %s: %v; falling back to full scan", target.Record.Path, err)
			return a.refreshMachineSnapshotLocked(cfg, &machine, includeCatalogs, scanRefreshAlways)
		}
		return nil
	}); err != nil {
		return fixRepoState{}, err
	}

	updated, err := a.loadFixRepoByPathUnlocked(machine, target.Record.Path)
	if err == nil {
		return updated, nil
	}
	a.logf("fix: targeted state load failed for %s: %v; falling back to full state load", target.Record.Path, err)

	refreshedRepos, refreshErr := a.loadFixReposUnlocked(machine)
	if refreshErr != nil {
		return fixRepoState{}, refreshErr
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
		rec, syncFeasibility := a.enrichFixSyncFeasibility(rec)
		risk, riskErr := collectFixRiskSnapshot(rec.Path, a.Git)
		if riskErr != nil && !errors.Is(riskErr, os.ErrNotExist) {
			a.logf("fix: risk scan failed for %s: %v", rec.Path, riskErr)
		}
		out = append(out, fixRepoState{
			Record:           rec,
			Meta:             metaByRepoKey[rec.RepoKey],
			Risk:             risk,
			SyncFeasibility:  syncFeasibility,
			IsDefaultCatalog: rec.Catalog == machine.DefaultCatalog,
		})
	}
	return out, nil
}

func (a *App) loadFixRepoByPathUnlocked(machine domain.MachineFile, repoPath string) (fixRepoState, error) {
	targetPath := filepath.Clean(repoPath)
	for _, rec := range machine.Repos {
		if filepath.Clean(rec.Path) != targetPath {
			continue
		}
		var meta *domain.RepoMetadataFile
		if repoKey := strings.TrimSpace(rec.RepoKey); repoKey != "" {
			loaded, err := state.LoadRepoMetadata(a.Paths, repoKey)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fixRepoState{}, err
			}
			if err == nil {
				loadedCopy := loaded
				meta = &loadedCopy
			}
		}
		risk, riskErr := collectFixRiskSnapshot(rec.Path, a.Git)
		if riskErr != nil && !errors.Is(riskErr, os.ErrNotExist) {
			a.logf("fix: risk scan failed for %s: %v", rec.Path, riskErr)
		}
		rec, syncFeasibility := a.enrichFixSyncFeasibility(rec)
		return fixRepoState{
			Record:           rec,
			Meta:             meta,
			Risk:             risk,
			SyncFeasibility:  syncFeasibility,
			IsDefaultCatalog: rec.Catalog == machine.DefaultCatalog,
		}, nil
	}
	return fixRepoState{}, fmt.Errorf("project %q not found", repoPath)
}

func (a *App) refreshFixRepoSnapshotLocked(cfg domain.ConfigFile, machine *domain.MachineFile, repoPath string) error {
	if machine == nil {
		return errors.New("machine snapshot is required")
	}
	targetPath := filepath.Clean(repoPath)

	index := -1
	var previous domain.MachineRepoRecord
	for i, rec := range machine.Repos {
		if filepath.Clean(rec.Path) != targetPath {
			continue
		}
		index = i
		previous = rec
		break
	}
	if index < 0 {
		return fmt.Errorf("project %q not found in machine snapshot", repoPath)
	}

	catalog := domain.Catalog{Name: previous.Catalog}
	for _, candidate := range machine.Catalogs {
		if candidate.Name == previous.Catalog {
			catalog = candidate
			break
		}
	}

	observed, err := a.observeRepoForScan(cfg, discoveredRepo{
		Catalog: catalog,
		Path:    previous.Path,
		Name:    previous.Name,
		RepoKey: previous.RepoKey,
	}, false)
	if err != nil {
		return err
	}

	observed = domain.UpdateObservedAt(previous, observed, a.Now())
	machine.Repos[index] = observed
	sort.Slice(machine.Repos, func(i, j int) bool {
		if repoRecordSortKey(machine.Repos[i]) == repoRecordSortKey(machine.Repos[j]) {
			return machine.Repos[i].Path < machine.Repos[j].Path
		}
		return repoRecordSortKey(machine.Repos[i]) < repoRecordSortKey(machine.Repos[j])
	})

	machine.UpdatedAt = a.Now()
	return state.SaveMachine(a.Paths, *machine)
}

func (a *App) executeFixAction(
	cfg domain.ConfigFile,
	target fixRepoState,
	action string,
	opts fixApplyOptions,
	observer fixApplyStepObserver,
) error {
	if err := validateFixApplyOptions(action, opts); err != nil {
		return err
	}

	path := target.Record.Path
	syncStrategy := normalizeFixSyncStrategy(opts.SyncStrategy)
	preferredRemote := ""
	if target.Meta != nil {
		preferredRemote = strings.TrimSpace(target.Meta.PreferredRemote)
	}
	planByID := fixActionPlanEntriesByID(
		fixActionExecutionPlanFor(action, buildFixActionPlanContext(cfg, target, opts)),
	)
	entryFor := func(id string, fallback fixActionPlanEntry) fixActionPlanEntry {
		return lookupFixActionPlanEntry(planByID, id, fallback)
	}
	runStep := func(id string, fallback fixActionPlanEntry, fn func() error) error {
		return runFixApplyStep(observer, entryFor(id, fallback), fn)
	}
	markSkipped := func(id string, fallback fixActionPlanEntry) {
		emitFixApplyStep(observer, entryFor(id, fallback), fixApplyStepSkipped, nil)
	}
	switch action {
	case FixActionAbortOperation:
		switch target.Record.OperationInProgress {
		case domain.OperationMerge:
			return runStep("abort-merge", fixActionPlanEntry{ID: "abort-merge", Command: true, Summary: "git merge --abort"}, func() error {
				return a.Git.MergeAbort(path)
			})
		case domain.OperationRebase:
			return runStep("abort-rebase", fixActionPlanEntry{ID: "abort-rebase", Command: true, Summary: "git rebase --abort"}, func() error {
				return a.Git.RebaseAbort(path)
			})
		case domain.OperationCherryPick:
			return runStep("abort-cherry-pick", fixActionPlanEntry{ID: "abort-cherry-pick", Command: true, Summary: "git cherry-pick --abort"}, func() error {
				return a.Git.CherryPickAbort(path)
			})
		case domain.OperationBisect:
			return runStep("abort-bisect", fixActionPlanEntry{ID: "abort-bisect", Command: true, Summary: "git bisect reset"}, func() error {
				return a.Git.BisectReset(path)
			})
		default:
			return fmt.Errorf("no operation in progress for %s", target.Record.Name)
		}
	case FixActionPush:
		if strings.TrimSpace(target.Record.Upstream) != "" && (target.Record.Diverged || target.Record.Behind > 0) {
			return &fixIneligibleError{
				Action: action,
				Reason: "push is blocked: branch is behind upstream; run sync-with-upstream first",
			}
		}
		return runStep("push-main", fixActionPlanEntry{ID: "push-main", Command: true, Summary: "git push"}, func() error {
			return a.Git.Push(path)
		})
	case FixActionSyncWithUpstream:
		if strings.TrimSpace(target.Record.Upstream) == "" {
			return errors.New("upstream is required for sync-with-upstream")
		}
		if target.SyncFeasibility.conflictFor(syncStrategy) {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf(
					"sync-with-upstream is blocked: %s (selected strategy: %s)",
					domain.ReasonSyncConflict,
					syncStrategy,
				),
			}
		}
		if target.SyncFeasibility.probeFailedFor(syncStrategy) {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf(
					"sync-with-upstream is blocked: %s (selected strategy: %s)",
					domain.ReasonSyncProbeFailed,
					syncStrategy,
				),
			}
		}
		if !target.SyncFeasibility.cleanFor(syncStrategy) {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf(
					"sync-with-upstream is blocked: sync strategy %s is not marked clean by feasibility validation",
					syncStrategy,
				),
			}
		}
		if cfg.Sync.FetchPrune {
			if err := runStep("sync-fetch-prune", fixActionPlanEntry{
				ID:      "sync-fetch-prune",
				Command: true,
				Summary: "git fetch --prune (if sync.fetch_prune is enabled)",
			}, func() error {
				return a.Git.FetchPrune(path)
			}); err != nil {
				return err
			}
		} else {
			markSkipped("sync-fetch-prune", fixActionPlanEntry{
				ID:      "sync-fetch-prune",
				Command: true,
				Summary: "git fetch --prune (if sync.fetch_prune is enabled)",
			})
		}
		if syncStrategy == FixSyncStrategyMerge {
			return runStep("sync-merge", fixActionPlanEntry{
				ID:      "sync-merge",
				Command: true,
				Summary: fmt.Sprintf("git merge --no-edit %s", target.Record.Upstream),
			}, func() error {
				return a.Git.MergeNoEdit(path, target.Record.Upstream)
			})
		}
		return runStep("sync-rebase", fixActionPlanEntry{
			ID:      "sync-rebase",
			Command: true,
			Summary: fmt.Sprintf("git rebase %s", target.Record.Upstream),
		}, func() error {
			return a.Git.Rebase(path, target.Record.Upstream)
		})
	case FixActionCreateProject:
		return a.createProjectFromFix(
			cfg,
			target,
			opts.CreateProjectName,
			opts.CreateProjectVisibility,
			observer,
			planByID,
		)
	case FixActionForkAndRetarget:
		return a.forkAndRetargetFromFix(cfg, target, observer, planByID)
	case FixActionStageCommitPush:
		if strings.TrimSpace(target.Record.OriginURL) != "" &&
			strings.TrimSpace(target.Record.Upstream) != "" &&
			(target.Record.Diverged || target.Record.Behind > 0) {
			return &fixIneligibleError{
				Action: action,
				Reason: "stage-commit-push is blocked: branch is behind upstream, so push would be rejected; run sync-with-upstream first",
			}
		}
		if opts.GenerateGitignore && len(opts.GitignorePatterns) > 0 {
			stepID := "stage-gitignore-append"
			summary := fmt.Sprintf("Append %d selected pattern(s) to root .gitignore.", len(opts.GitignorePatterns))
			if target.Risk.MissingRootGitignore {
				stepID = "stage-gitignore-generate"
				summary = fmt.Sprintf("Generate root .gitignore with %d selected pattern(s).", len(opts.GitignorePatterns))
			}
			if err := runStep(stepID, fixActionPlanEntry{
				ID:      stepID,
				Command: false,
				Summary: summary,
			}, func() error {
				return writeOrAppendGitignore(path, opts.GitignorePatterns)
			}); err != nil {
				return err
			}
		}
		msg := strings.TrimSpace(opts.CommitMessage)
		if msg == "" || msg == "auto" {
			msg = DefaultFixCommitMessage
		}
		if err := runStep("stage-git-add", fixActionPlanEntry{ID: "stage-git-add", Command: true, Summary: "git add -A"}, func() error {
			return a.Git.AddAll(path)
		}); err != nil {
			return err
		}
		if err := runStep("stage-git-commit", fixActionPlanEntry{
			ID:      "stage-git-commit",
			Command: true,
			Summary: fmt.Sprintf("git commit -m %q", msg),
		}, func() error {
			return a.Git.Commit(path, msg)
		}); err != nil {
			return err
		}
		if strings.TrimSpace(target.Record.OriginURL) == "" {
			markSkipped("stage-skip-push-no-origin", fixActionPlanEntry{
				ID:      "stage-skip-push-no-origin",
				Command: false,
				Summary: "Skip push because no origin remote is configured.",
			})
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
			return runStep("stage-push-set-upstream", fixActionPlanEntry{
				ID:      "stage-push-set-upstream",
				Command: true,
				Summary: fmt.Sprintf("git push -u %s %s", plannedRemote(preferredRemote, target.Record.Upstream), plannedBranch(branch)),
			}, func() error {
				return a.Git.PushUpstreamWithPreferredRemote(path, branch, preferredRemote)
			})
		}
		return runStep("stage-push", fixActionPlanEntry{ID: "stage-push", Command: true, Summary: "git push"}, func() error {
			return a.Git.Push(path)
		})
	case FixActionPullFFOnly:
		if cfg.Sync.FetchPrune {
			if err := runStep("pull-fetch-prune", fixActionPlanEntry{
				ID:      "pull-fetch-prune",
				Command: true,
				Summary: "git fetch --prune (if sync.fetch_prune is enabled)",
			}, func() error {
				return a.Git.FetchPrune(path)
			}); err != nil {
				return err
			}
		} else {
			markSkipped("pull-fetch-prune", fixActionPlanEntry{
				ID:      "pull-fetch-prune",
				Command: true,
				Summary: "git fetch --prune (if sync.fetch_prune is enabled)",
			})
		}
		return runStep("pull-ff-only", fixActionPlanEntry{ID: "pull-ff-only", Command: true, Summary: "git pull --ff-only"}, func() error {
			return a.Git.PullFFOnly(path)
		})
	case FixActionSetUpstreamPush:
		branch := target.Record.Branch
		if branch == "" {
			branch, _ = a.Git.CurrentBranch(path)
		}
		if strings.TrimSpace(branch) == "" {
			return errors.New("cannot determine branch for upstream push")
		}
		return runStep("upstream-push", fixActionPlanEntry{
			ID:      "upstream-push",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s", plannedRemote(preferredRemote, target.Record.Upstream), plannedBranch(branch)),
		}, func() error {
			return a.Git.PushUpstreamWithPreferredRemote(path, branch, preferredRemote)
		})
	case FixActionEnableAutoPush:
		if strings.TrimSpace(target.Record.RepoKey) == "" {
			return errors.New("repo_key is required for enable-auto-push")
		}
		branch := strings.TrimSpace(target.Record.Branch)
		defaultBranch := ""
		if strings.TrimSpace(target.Record.Path) != "" {
			defaultBranch, _ = a.Git.DefaultBranch(target.Record.Path, preferredRemote)
		}
		targetMode := domain.AutoPushModeEnabled
		if isDefaultBranch(branch, defaultBranch) {
			targetMode = domain.AutoPushModeIncludeDefaultBranch
		}
		return runStep("enable-auto-push", fixActionPlanEntry{
			ID:      "enable-auto-push",
			Command: false,
			Summary: fmt.Sprintf("Write repo metadata: set auto_push=%q.", targetMode),
		}, func() error {
			meta, err := state.LoadRepoMetadata(a.Paths, target.Record.RepoKey)
			if err != nil {
				return err
			}
			meta.AutoPush = targetMode
			return state.SaveRepoMetadata(a.Paths, meta)
		})
	default:
		return fmt.Errorf("unknown fix action %q", action)
	}
}

func buildFixActionPlanContext(cfg domain.ConfigFile, target fixRepoState, opts fixApplyOptions) fixActionPlanContext {
	preferredRemote := ""
	if target.Meta != nil {
		preferredRemote = strings.TrimSpace(target.Meta.PreferredRemote)
	}
	return fixActionPlanContext{
		Operation:               target.Record.OperationInProgress,
		Branch:                  strings.TrimSpace(target.Record.Branch),
		Upstream:                strings.TrimSpace(target.Record.Upstream),
		OriginURL:               strings.TrimSpace(target.Record.OriginURL),
		SyncStrategy:            normalizeFixSyncStrategy(opts.SyncStrategy),
		PreferredRemote:         preferredRemote,
		GitHubOwner:             strings.TrimSpace(cfg.GitHub.Owner),
		RemoteProtocol:          strings.TrimSpace(cfg.GitHub.RemoteProtocol),
		RepoName:                strings.TrimSpace(target.Record.Name),
		CommitMessage:           strings.TrimSpace(opts.CommitMessage),
		CreateProjectName:       strings.TrimSpace(opts.CreateProjectName),
		CreateProjectVisibility: opts.CreateProjectVisibility,
		GenerateGitignore:       opts.GenerateGitignore,
		GitignorePatterns:       append([]string(nil), opts.GitignorePatterns...),
		MissingRootGitignore:    target.Risk.MissingRootGitignore,
		FetchPrune:              cfg.Sync.FetchPrune,
	}
}

func fixActionPlanEntriesByID(entries []fixActionPlanEntry) map[string]fixActionPlanEntry {
	out := make(map[string]fixActionPlanEntry, len(entries))
	for _, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		out[id] = entry
	}
	return out
}

func lookupFixActionPlanEntry(planByID map[string]fixActionPlanEntry, id string, fallback fixActionPlanEntry) fixActionPlanEntry {
	if planByID != nil {
		if planned, ok := planByID[id]; ok {
			return planned
		}
	}
	if strings.TrimSpace(fallback.ID) == "" {
		fallback.ID = id
	}
	return fallback
}

func emitFixApplyStep(observer fixApplyStepObserver, entry fixActionPlanEntry, status fixApplyStepStatus, err error) {
	if observer == nil {
		return
	}
	observer(fixApplyStepEvent{
		Entry:  entry,
		Status: status,
		Err:    err,
	})
}

func runFixApplyStep(observer fixApplyStepObserver, entry fixActionPlanEntry, fn func() error) error {
	emitFixApplyStep(observer, entry, fixApplyStepRunning, nil)
	if err := fn(); err != nil {
		emitFixApplyStep(observer, entry, fixApplyStepFailed, err)
		return err
	}
	emitFixApplyStep(observer, entry, fixApplyStepDone, nil)
	return nil
}

func (a *App) forkAndRetargetFromFix(
	cfg domain.ConfigFile,
	target fixRepoState,
	observer fixApplyStepObserver,
	planByID map[string]fixActionPlanEntry,
) error {
	entryFor := func(id string, fallback fixActionPlanEntry) fixActionPlanEntry {
		return lookupFixActionPlanEntry(planByID, id, fallback)
	}
	runStep := func(id string, fallback fixActionPlanEntry, fn func() error) error {
		return runFixApplyStep(observer, entryFor(id, fallback), fn)
	}

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

	var forkURL string
	if err := runStep("fork-gh-fork", fixActionPlanEntry{
		ID:      "fork-gh-fork",
		Command: true,
		Summary: "gh repo fork <source-owner>/<repo> --remote=false --clone=false",
	}, func() error {
		var err error
		forkURL, err = a.ensureForkRemoteRepo(originURL, owner, cfg.GitHub.RemoteProtocol, repoPath)
		return err
	}); err != nil {
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
	if err := runStep("fork-set-remote", fixActionPlanEntry{
		ID:      "fork-set-remote",
		Command: true,
		Summary: fmt.Sprintf("git remote add %s <fork-url> (or git remote set-url when that remote already exists)", owner),
	}, func() error {
		if remoteExists {
			return a.Git.SetRemoteURL(repoPath, forkRemoteName, forkURL)
		}
		return a.Git.AddRemote(repoPath, forkRemoteName, forkURL)
	}); err != nil {
		return err
	}

	branch := strings.TrimSpace(target.Record.Branch)
	if branch == "" {
		branch, _ = a.Git.CurrentBranch(repoPath)
	}
	if strings.TrimSpace(branch) == "" {
		return errors.New("cannot determine branch for fork-and-retarget")
	}
	if err := runStep("fork-push-upstream", fixActionPlanEntry{
		ID:      "fork-push-upstream",
		Command: true,
		Summary: fmt.Sprintf("git push -u %s %s", owner, plannedBranch(branch)),
	}, func() error {
		return a.Git.PushUpstreamWithPreferredRemote(repoPath, branch, forkRemoteName)
	}); err != nil {
		return err
	}

	return runStep("fork-write-metadata", fixActionPlanEntry{
		ID:      "fork-write-metadata",
		Command: false,
		Summary: "Update repo metadata (preferred remote and push-access probe state).",
	}, func() error {
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
		return state.SaveRepoMetadata(a.Paths, updated)
	})
}

func (a *App) createProjectFromFix(
	cfg domain.ConfigFile,
	target fixRepoState,
	projectNameOverride string,
	visibilityOverride domain.Visibility,
	observer fixApplyStepObserver,
	planByID map[string]fixActionPlanEntry,
) error {
	entryFor := func(id string, fallback fixActionPlanEntry) fixActionPlanEntry {
		return lookupFixActionPlanEntry(planByID, id, fallback)
	}
	runStep := func(id string, fallback fixActionPlanEntry, fn func() error) error {
		return runFixApplyStep(observer, entryFor(id, fallback), fn)
	}
	markSkipped := func(id string, fallback fixActionPlanEntry) {
		emitFixApplyStep(observer, entryFor(id, fallback), fixApplyStepSkipped, nil)
	}

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
	projectName = sanitizeGitHubRepositoryNameInput(projectName)
	if strings.TrimSpace(projectName) == "" {
		return errors.New("project name is required for create-project")
	}
	if err := validateGitHubRepositoryName(projectName); err != nil {
		return fmt.Errorf("invalid repository name: %w", err)
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
		markSkipped("create-gh-repo", fixActionPlanEntry{
			ID:      "create-gh-repo",
			Command: true,
			Summary: fmt.Sprintf("gh repo create %s/%s %s", owner, projectName, plannedVisibilityFlag(visibility)),
		})
		if err := runStep("create-validate-origin", fixActionPlanEntry{
			ID:      "create-validate-origin",
			Command: false,
			Summary: "Validate existing origin URL matches the expected repository identity.",
		}, func() error {
			matches, err := originsMatchNormalized(origin, expectedOrigin)
			if err != nil {
				return fmt.Errorf("invalid existing origin: %w", err)
			}
			if !matches {
				return fmt.Errorf("conflicting origin: existing %q does not match expected %q", origin, expectedOrigin)
			}
			return nil
		}); err != nil {
			return err
		}
	} else {
		var createdOrigin string
		if err := runStep("create-gh-repo", fixActionPlanEntry{
			ID:      "create-gh-repo",
			Command: true,
			Summary: fmt.Sprintf("gh repo create %s/%s %s", owner, projectName, plannedVisibilityFlag(visibility)),
		}, func() error {
			var err error
			createdOrigin, err = a.createRemoteRepo(owner, projectName, visibility, cfg.GitHub.RemoteProtocol, target.Record.Path)
			return err
		}); err != nil {
			return err
		}
		if err := runStep("create-add-origin", fixActionPlanEntry{
			ID:      "create-add-origin",
			Command: true,
			Summary: fmt.Sprintf("git remote add origin %s", plannedOriginURL(owner, projectName, cfg.GitHub.RemoteProtocol)),
		}, func() error {
			if err := a.Git.AddOrigin(target.Record.Path, createdOrigin); err != nil {
				return fmt.Errorf("set origin failed: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
		origin = createdOrigin
	}

	if strings.TrimSpace(target.Record.RepoKey) == "" {
		return errors.New("repo_key is required for create-project")
	}
	var meta domain.RepoMetadataFile
	if err := runStep("create-write-metadata", fixActionPlanEntry{
		ID:      "create-write-metadata",
		Command: false,
		Summary: "Write/update repo metadata (origin URL, visibility, default auto-push policy).",
	}, func() error {
		var err error
		meta, _, err = a.ensureRepoMetadata(cfg, target.Record.RepoKey, projectName, origin, visibility, target.Record.Catalog)
		return err
	}); err != nil {
		return err
	}

	branch, _ := a.Git.CurrentBranch(target.Record.Path)
	headSHA, _ := a.Git.HeadSHA(target.Record.Path)
	upstream, _ := a.Git.Upstream(target.Record.Path)
	if headSHA != "" && upstream == "" {
		if branch == "" {
			branch = "main"
		}
		if err := runStep("create-initial-push", fixActionPlanEntry{
			ID:      "create-initial-push",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s (when HEAD has commits)", plannedRemote(meta.PreferredRemote, target.Record.Upstream), plannedBranch(branch)),
		}, func() error {
			if err := a.Git.PushUpstreamWithPreferredRemote(target.Record.Path, branch, meta.PreferredRemote); err != nil {
				return fmt.Errorf("initial push failed: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	} else {
		markSkipped("create-initial-push", fixActionPlanEntry{
			ID:      "create-initial-push",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s (when HEAD has commits)", plannedRemote(meta.PreferredRemote, target.Record.Upstream), plannedBranch(branch)),
		})
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
