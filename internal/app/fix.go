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
	FixActionIgnore             = "ignore"
	FixActionAbortOperation     = "abort-operation"
	FixActionClone              = "clone"
	FixActionCreateProject      = "create-project"
	FixActionForkAndRetarget    = "fork-and-retarget"
	FixActionSyncWithUpstream   = "sync-with-upstream"
	FixActionPush               = "push"
	FixActionStageCommitPush    = "stage-commit-push"
	FixActionPublishNewBranch   = "publish-new-branch"
	FixActionCheckpointThenSync = "checkpoint-then-sync"
	FixActionPullFFOnly         = "pull-ff-only"
	FixActionSetUpstreamPush    = "set-upstream-push"
	FixActionEnableAutoPush     = "enable-auto-push"

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
	Interactive                   bool
	CommitMessage                 string
	SyncStrategy                  FixSyncStrategy
	CreateProjectName             string
	CreateProjectVisibility       domain.Visibility
	ForkBranchRenameTo            string
	ReturnToOriginalBranchAndSync bool
	GenerateGitignore             bool
	GitignorePatterns             []string
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
		if opts.AIMessage {
			return 2, errors.New("--ai-message requires an explicit action")
		}
		if a.IsInteractiveTerminal == nil || !a.IsInteractiveTerminal() {
			return 2, errors.New("bb fix requires an interactive terminal")
		}
		return a.runFixInteractiveWithMutedLogs(opts.IncludeCatalogs, opts.NoRefresh)
	}
	if opts.AIMessage && strings.TrimSpace(opts.CommitMessage) != "" {
		return 2, errors.New("--message and --ai-message are mutually exclusive")
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
	if opts.AIMessage {
		if !isCommitProducingFixAction(action) {
			return 2, errors.New("--ai-message is only supported for stage-commit-push, publish-new-branch, and checkpoint-then-sync")
		}
	}
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

	if opts.AIMessage {
		message, err := a.generateLumenCommitMessage(target.Record.Path)
		if err != nil {
			return 2, err
		}
		opts.CommitMessage = message
	}

	updated, err := a.applyFixAction(opts.IncludeCatalogs, target.Record.Path, action, fixApplyOptions{
		Interactive:                   false,
		CommitMessage:                 opts.CommitMessage,
		ForkBranchRenameTo:            opts.PublishBranch,
		ReturnToOriginalBranchAndSync: opts.ReturnToOriginalBranchAndSync,
		SyncStrategy:                  strategy,
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

func isCommitProducingFixAction(action string) bool {
	switch strings.TrimSpace(action) {
	case FixActionStageCommitPush, FixActionPublishNewBranch, FixActionCheckpointThenSync:
		return true
	default:
		return false
	}
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
	pushAccess := domain.PushAccessReadWrite
	if meta != nil {
		pushAccess = domain.NormalizePushAccess(meta.PushAccess)
	}
	if meta != nil && strings.TrimSpace(rec.OriginURL) != "" {
		pushAllowed = pushAccess == domain.PushAccessReadWrite
	}
	if rec.OriginURL == "" {
		actions = append(actions, FixActionCreateProject)
	}
	if containsUnsyncableReason(rec.UnsyncableReasons, domain.ReasonCloneRequired) {
		actions = append(actions, FixActionClone)
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
	if (rec.HasDirtyTracked || rec.HasUntracked) &&
		strings.TrimSpace(rec.OriginURL) != "" &&
		strings.TrimSpace(rec.Branch) != "" &&
		strings.TrimSpace(rec.Branch) != "HEAD" &&
		pushAllowed &&
		!ctx.Risk.hasSecretLikeChanges() &&
		!(ctx.Risk.hasNoisyChangesWithoutGitignore() && !ctx.Interactive) {
		actions = append(actions, FixActionPublishNewBranch)
	}
	if (rec.HasDirtyTracked || rec.HasUntracked) &&
		!rec.Diverged &&
		strings.TrimSpace(rec.OriginURL) != "" &&
		strings.TrimSpace(rec.Upstream) != "" &&
		rec.Behind > 0 &&
		pushAllowed &&
		!ctx.Risk.hasSecretLikeChanges() &&
		!(ctx.Risk.hasNoisyChangesWithoutGitignore() && !ctx.Interactive) {
		actions = append(actions, FixActionCheckpointThenSync)
	}
	if rec.Upstream != "" && rec.Behind > 0 && rec.Ahead == 0 && !rec.Diverged && !rec.HasDirtyTracked && !rec.HasUntracked {
		actions = append(actions, FixActionPullFFOnly)
	}
	if rec.OriginURL != "" && rec.Upstream == "" && rec.Branch != "" && !rec.Diverged && pushAllowed {
		actions = append(actions, FixActionSetUpstreamPush)
	}
	if rec.OriginURL != "" && pushAccess == domain.PushAccessReadOnly && strings.TrimSpace(rec.RepoKey) != "" {
		actions = append(actions, FixActionForkAndRetarget)
	}
	if meta != nil && strings.TrimSpace(rec.RepoKey) != "" && pushAccess == domain.PushAccessReadWrite {
		mode := domain.NormalizeAutoPushMode(meta.AutoPush)
		if mode == domain.AutoPushModeDisabled || (mode == domain.AutoPushModeEnabled && containsUnsyncableReason(rec.UnsyncableReasons, domain.ReasonPushPolicyBlocked)) {
			actions = append(actions, FixActionEnableAutoPush)
		}
	}
	return actions
}

func ineligibleFixReason(action string, rec domain.MachineRepoRecord, ctx fixEligibilityContext) string {
	if action == FixActionPublishNewBranch {
		if rec.OperationInProgress != domain.OperationNone && rec.OperationInProgress != "" {
			return "publish-new-branch is blocked: a git operation is in progress; run abort-operation first"
		}
		if strings.TrimSpace(rec.OriginURL) == "" {
			return "publish-new-branch is blocked: origin remote is required"
		}
		if containsUnsyncableReason(rec.UnsyncableReasons, domain.ReasonPushAccessBlocked) {
			return "publish-new-branch is blocked: push access is read-only; run fork-and-retarget first"
		}
		if strings.TrimSpace(rec.Branch) == "" {
			return "publish-new-branch is blocked: cannot determine current branch"
		}
		if strings.TrimSpace(rec.Branch) == "HEAD" {
			return "publish-new-branch is blocked: detached HEAD is not supported; checkout a branch first"
		}
		if !rec.HasDirtyTracked && !rec.HasUntracked {
			return "publish-new-branch is blocked: no local uncommitted changes to commit"
		}
		if ctx.Risk.hasSecretLikeChanges() {
			return fmt.Sprintf(
				"publish-new-branch is blocked: secret-like uncommitted files detected (%s)",
				strings.Join(ctx.Risk.SecretLikeChangedPaths, ", "),
			)
		}
		if ctx.Risk.hasNoisyChangesWithoutGitignore() && !ctx.Interactive {
			return "publish-new-branch is blocked: root .gitignore is missing and noisy uncommitted paths were detected; run interactive `bb fix` to review/generate .gitignore or add it manually"
		}
		return ""
	}
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
	if action == FixActionCheckpointThenSync {
		if strings.TrimSpace(rec.OriginURL) == "" {
			return "checkpoint-then-sync is blocked: origin remote is required"
		}
		if strings.TrimSpace(rec.Upstream) == "" {
			return "checkpoint-then-sync is blocked: upstream is required"
		}
		if rec.Diverged {
			return "checkpoint-then-sync is blocked: branch diverged from upstream; run sync-with-upstream first"
		}
		if rec.Behind == 0 {
			return "checkpoint-then-sync is blocked: branch is not behind upstream"
		}
		if !rec.HasDirtyTracked && !rec.HasUntracked {
			return "checkpoint-then-sync is blocked: no local uncommitted changes to checkpoint"
		}
		if ctx.Risk.hasSecretLikeChanges() {
			return fmt.Sprintf(
				"checkpoint-then-sync is blocked: secret-like uncommitted files detected (%s)",
				strings.Join(ctx.Risk.SecretLikeChangedPaths, ", "),
			)
		}
		if ctx.Risk.hasNoisyChangesWithoutGitignore() && !ctx.Interactive {
			return "checkpoint-then-sync is blocked: root .gitignore is missing and noisy uncommitted paths were detected; run interactive `bb fix` to review/generate .gitignore or add it manually"
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
	metaByRepoKey := repoMetadataByKey(metas)
	if err := a.augmentFixMachineWithKnownRepos(&machine, includeCatalogs, metaByRepoKey); err != nil {
		return nil, err
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

	pushAccessUpdated, err := a.refreshUnknownPushAccessForFixReposLocked(out)
	if err != nil {
		return nil, err
	}
	if pushAccessUpdated {
		metas, err = state.LoadAllRepoMetadata(a.Paths)
		if err != nil {
			return nil, err
		}
		metaByRepoKey = repoMetadataByKey(metas)
		for i := range out {
			out[i].Meta = metaByRepoKey[out[i].Record.RepoKey]
		}
	}

	return out, nil
}

func (a *App) loadFixReposForInteractive(includeCatalogs []string, refreshMode scanRefreshMode) ([]fixRepoState, error) {
	return a.loadFixRepos(includeCatalogs, refreshMode)
}

func (a *App) refreshUnknownPushAccessForFixRepos(repos []fixRepoState) (bool, error) {
	a.logf("fix: acquiring global lock for unknown push-access refresh")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("fix: released global lock for unknown push-access refresh")
	}()
	return a.refreshUnknownPushAccessForFixReposLocked(repos)
}

func (a *App) refreshUnknownPushAccessForFixReposLocked(repos []fixRepoState) (bool, error) {
	type probeTarget struct {
		repoKey   string
		repoPath  string
		originURL string
	}

	byRepoKey := make(map[string]probeTarget, len(repos))
	for _, repo := range repos {
		if repo.Meta == nil {
			continue
		}
		if domain.NormalizePushAccess(repo.Meta.PushAccess) != domain.PushAccessUnknown {
			continue
		}
		repoKey := strings.TrimSpace(repo.Record.RepoKey)
		repoPath := strings.TrimSpace(repo.Record.Path)
		originURL := strings.TrimSpace(repo.Record.OriginURL)
		if repoKey == "" || repoPath == "" || originURL == "" {
			continue
		}
		byRepoKey[repoKey] = probeTarget{
			repoKey:   repoKey,
			repoPath:  repoPath,
			originURL: originURL,
		}
	}
	if len(byRepoKey) == 0 {
		return false, nil
	}

	keys := make([]string, 0, len(byRepoKey))
	for repoKey := range byRepoKey {
		keys = append(keys, repoKey)
	}
	sort.Strings(keys)

	changed := false
	for _, repoKey := range keys {
		target := byRepoKey[repoKey]

		meta, err := state.LoadRepoMetadata(a.Paths, target.repoKey)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, err
		}
		if domain.NormalizePushAccess(meta.PushAccess) != domain.PushAccessUnknown {
			continue
		}

		updated, probeChanged, err := a.probeAndUpdateRepoPushAccess(target.repoPath, target.originURL, meta, true)
		if err != nil {
			return false, err
		}
		if !probeChanged {
			continue
		}
		if err := state.SaveRepoMetadata(a.Paths, updated); err != nil {
			return false, err
		}
		changed = true
	}

	return changed, nil
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

type fixStepRunner func(id string, fallback fixActionPlanEntry, fn func() error) error
type fixStepSkipper func(id string, fallback fixActionPlanEntry)

func (a *App) runFixStageCommitSteps(cfg domain.ConfigFile, path string, target fixRepoState, opts fixApplyOptions, runStep fixStepRunner) error {
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

	if err := runStep("stage-git-add", fixActionPlanEntry{ID: "stage-git-add", Command: true, Summary: "git add -A"}, func() error {
		return a.Git.AddAll(path)
	}); err != nil {
		return err
	}

	msg := strings.TrimSpace(opts.CommitMessage)
	if shouldAutoGenerateCommitMessageWithLumen(msg, cfg.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty) {
		if err := runStep("stage-lumen-draft", fixActionPlanEntry{
			ID:      "stage-lumen-draft",
			Command: true,
			Summary: "lumen draft",
		}, func() error {
			generated, err := a.generateLumenCommitMessageFromStagedDiff(path)
			if err != nil {
				return err
			}
			msg = strings.TrimSpace(generated)
			return nil
		}); err != nil {
			return err
		}
	}
	if msg == "" || msg == "auto" {
		msg = DefaultFixCommitMessage
	}
	return runStep("stage-git-commit", fixActionPlanEntry{
		ID:      "stage-git-commit",
		Command: true,
		Summary: fmt.Sprintf("git commit -m %q", msg),
	}, func() error {
		return a.Git.Commit(path, msg)
	})
}

func validateSyncWithUpstreamEligibility(action string, target fixRepoState, syncStrategy FixSyncStrategy) error {
	blocked := func(reason string) *fixIneligibleError {
		return &fixIneligibleError{
			Action: action,
			Reason: reason,
		}
	}
	actionLabel := strings.TrimSpace(action)
	if actionLabel == "" {
		actionLabel = FixActionSyncWithUpstream
	}
	if strings.TrimSpace(target.Record.Upstream) == "" {
		return blocked(fmt.Sprintf("%s is blocked: upstream is required", actionLabel))
	}
	if target.SyncFeasibility.conflictFor(syncStrategy) {
		return blocked(fmt.Sprintf(
			"%s is blocked: %s (selected strategy: %s)",
			actionLabel,
			domain.ReasonSyncConflict,
			syncStrategy,
		))
	}
	if target.SyncFeasibility.probeFailedFor(syncStrategy) {
		return blocked(fmt.Sprintf(
			"%s is blocked: %s (selected strategy: %s)",
			actionLabel,
			domain.ReasonSyncProbeFailed,
			syncStrategy,
		))
	}
	if !target.SyncFeasibility.cleanFor(syncStrategy) {
		return blocked(fmt.Sprintf(
			"%s is blocked: sync strategy %s is not marked clean by feasibility validation",
			actionLabel,
			syncStrategy,
		))
	}
	return nil
}

func (a *App) runFixSyncWithUpstreamSteps(
	cfg domain.ConfigFile,
	path string,
	target fixRepoState,
	syncStrategy FixSyncStrategy,
	runStep fixStepRunner,
	markSkipped fixStepSkipper,
) error {
	if cfg.Sync.FetchPrune {
		if err := runStep("sync-fetch-prune", fixActionPlanEntry{
			ID:      "sync-fetch-prune",
			Command: true,
			Summary: "git fetch --prune",
		}, func() error {
			return a.Git.FetchPrune(path)
		}); err != nil {
			return err
		}
	} else {
		markSkipped("sync-fetch-prune", fixActionPlanEntry{
			ID:      "sync-fetch-prune",
			Command: true,
			Summary: "git fetch --prune",
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
	if err := a.refreshFixRepoSnapshotLocked(cfg, &machine, repoPath); err != nil && !isFixProjectNotFoundInSnapshot(err) {
		return fixRepoState{}, err
	}
	target, err := a.loadFixRepoByPathUnlocked(machine, repoPath)
	if err != nil && action != FixActionClone {
		return fixRepoState{}, err
	}
	if err != nil {
		metas, loadErr := state.LoadAllRepoMetadata(a.Paths)
		if loadErr != nil {
			return fixRepoState{}, loadErr
		}
		metaByRepoKey := repoMetadataByKey(metas)
		if augmentErr := a.augmentFixMachineWithKnownRepos(&machine, includeCatalogs, metaByRepoKey); augmentErr != nil {
			return fixRepoState{}, augmentErr
		}
		target, err = a.loadFixRepoByPathUnlocked(machine, repoPath)
		if err != nil {
			return fixRepoState{}, err
		}
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
		fixActionExecutionPlanFor(action, a.buildFixActionPlanContext(cfg, target, opts)),
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
		return a.refreshFixRepoSnapshotLocked(cfg, &machine, target.Record.Path)
	}); err != nil {
		return fixRepoState{}, err
	}

	return a.loadFixRepoByPathUnlocked(machine, target.Record.Path)
}

func (a *App) loadFixReposUnlocked(machine domain.MachineFile) ([]fixRepoState, error) {
	metas, err := state.LoadAllRepoMetadata(a.Paths)
	if err != nil {
		return nil, err
	}
	metaByRepoKey := repoMetadataByKey(metas)

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

func repoMetadataByKey(metas []domain.RepoMetadataFile) map[string]*domain.RepoMetadataFile {
	metaByRepoKey := make(map[string]*domain.RepoMetadataFile, len(metas))
	for i := range metas {
		meta := metas[i]
		metaByRepoKey[meta.RepoKey] = &meta
	}
	return metaByRepoKey
}

func (a *App) augmentFixMachineWithKnownRepos(
	machine *domain.MachineFile,
	includeCatalogs []string,
	metaByRepoKey map[string]*domain.RepoMetadataFile,
) error {
	if machine == nil {
		return errors.New("machine snapshot is required")
	}
	selectedCatalogs, err := domain.SelectCatalogs(*machine, includeCatalogs)
	if err != nil {
		return err
	}
	selectedCatalogMap := map[string]domain.Catalog{}
	for _, catalog := range selectedCatalogs {
		selectedCatalogMap[catalog.Name] = catalog
	}
	keys := make([]string, 0, len(metaByRepoKey))
	for key := range metaByRepoKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, repoKey := range keys {
		meta := metaByRepoKey[repoKey]
		if meta == nil {
			continue
		}
		keyCatalog, keyRelativePath, keyRepoName, err := domain.ParseRepoKey(meta.RepoKey)
		if err != nil {
			continue
		}
		targetCatalog, ok := selectedCatalogMap[keyCatalog]
		if !ok {
			continue
		}
		if len(findLocalMatches(machine.Repos, meta.RepoKey, selectedCatalogMap)) > 0 {
			continue
		}
		targetPath := filepath.Join(targetCatalog.Root, filepath.FromSlash(keyRelativePath))
		pathConflictReason, err := validateTargetPath(a.Git, targetPath, meta.OriginURL, meta.PreferredRemote)
		if err != nil {
			return err
		}
		if pathConflictReason != "" {
			a.addOrUpdateSyntheticUnsyncable(machine, *meta, targetCatalog.Name, targetPath, keyRepoName, pathConflictReason)
			continue
		}
		if !targetCatalog.AllowsAutoCloneOnSync() {
			a.addOrUpdateSyntheticUnsyncable(machine, *meta, targetCatalog.Name, targetPath, keyRepoName, domain.ReasonCloneRequired)
		}
	}
	return nil
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
	if strings.TrimSpace(previous.Path) == "" {
		return nil
	}
	if containsUnsyncableReason(previous.UnsyncableReasons, domain.ReasonCloneRequired) && !a.Git.IsGitRepo(previous.Path) {
		return nil
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

func isFixProjectNotFoundInSnapshot(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found in machine snapshot")
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
		fixActionExecutionPlanFor(action, a.buildFixActionPlanContext(cfg, target, opts)),
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
	resolveCurrentBranch := func() (string, error) {
		branch := strings.TrimSpace(target.Record.Branch)
		if branch == "" {
			branch, _ = a.Git.CurrentBranch(path)
		}
		branch = strings.TrimSpace(branch)
		if branch == "" {
			return "", errors.New("cannot determine current branch")
		}
		return branch, nil
	}
	maybeRenameForPublish := func(stepID string, branch string) (string, bool, error) {
		renameTo := strings.TrimSpace(opts.ForkBranchRenameTo)
		if renameTo == "" || renameTo == branch {
			return branch, false, nil
		}
		fallback := fixActionPlanEntry{
			ID:      stepID,
			Command: true,
			Summary: fmt.Sprintf("git branch -m %s", renameTo),
		}
		if err := runStep(stepID, fallback, func() error {
			return a.Git.RenameCurrentBranch(path, renameTo)
		}); err != nil {
			return "", false, err
		}
		return renameTo, true, nil
	}
	localBranchExists := func(branch string) bool {
		if strings.TrimSpace(branch) == "" {
			return false
		}
		_, err := a.Git.RunGit(path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
		return err == nil
	}
	remoteBranchExists := func(remote string, branch string) bool {
		remote = strings.TrimSpace(remote)
		branch = strings.TrimSpace(branch)
		if remote == "" || branch == "" {
			return false
		}
		out, err := a.Git.RunGit(path, "ls-remote", "--heads", remote, "refs/heads/"+branch)
		if err != nil {
			return false
		}
		return strings.TrimSpace(out) != ""
	}
	resolvePushRemote := func(upstream string) string {
		remote := plannedRemote(preferredRemote, upstream)
		if _, err := a.Git.EffectiveRemote(path, remote); err == nil {
			return remote
		}
		fallback, err := a.Git.EffectiveRemote(path, "")
		if err != nil || strings.TrimSpace(fallback) == "" {
			return remote
		}
		return fallback
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
		if strings.TrimSpace(opts.ForkBranchRenameTo) != "" {
			branch, err := resolveCurrentBranch()
			if err != nil {
				return errors.New("cannot determine branch for publish")
			}
			branch, _, err = maybeRenameForPublish("push-rename-branch", branch)
			if err != nil {
				return err
			}
			pushRemote := resolvePushRemote(target.Record.Upstream)
			return runStep("push-main", fixActionPlanEntry{
				ID:      "push-main",
				Command: true,
				Summary: fmt.Sprintf("git push -u %s %s", pushRemote, plannedBranch(branch)),
			}, func() error {
				return a.Git.PushUpstreamWithPreferredRemote(path, branch, pushRemote)
			})
		}
		return runStep("push-main", fixActionPlanEntry{ID: "push-main", Command: true, Summary: "git push"}, func() error {
			return a.Git.Push(path)
		})
	case FixActionSyncWithUpstream:
		if err := validateSyncWithUpstreamEligibility(action, target, syncStrategy); err != nil {
			return err
		}
		return a.runFixSyncWithUpstreamSteps(cfg, path, target, syncStrategy, runStep, markSkipped)
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
		return a.forkAndRetargetFromFix(cfg, target, opts, observer, planByID)
	case FixActionStageCommitPush:
		if strings.TrimSpace(target.Record.OriginURL) != "" &&
			strings.TrimSpace(target.Record.Upstream) != "" &&
			(target.Record.Diverged || target.Record.Behind > 0) {
			return &fixIneligibleError{
				Action: action,
				Reason: "stage-commit-push is blocked: branch is behind upstream, so push would be rejected; run sync-with-upstream first",
			}
		}
		if err := a.runFixStageCommitSteps(cfg, path, target, opts, runStep); err != nil {
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
		branch, err := resolveCurrentBranch()
		if err != nil {
			return errors.New("cannot determine branch for upstream push")
		}
		branch, renamed, err := maybeRenameForPublish("stage-rename-branch", branch)
		if err != nil {
			return err
		}
		if target.Record.Upstream == "" || renamed {
			pushRemote := resolvePushRemote(target.Record.Upstream)
			return runStep("stage-push-set-upstream", fixActionPlanEntry{
				ID:      "stage-push-set-upstream",
				Command: true,
				Summary: fmt.Sprintf("git push -u %s %s", pushRemote, plannedBranch(branch)),
			}, func() error {
				return a.Git.PushUpstreamWithPreferredRemote(path, branch, pushRemote)
			})
		}
		return runStep("stage-push", fixActionPlanEntry{ID: "stage-push", Command: true, Summary: "git push"}, func() error {
			return a.Git.Push(path)
		})
	case FixActionPublishNewBranch:
		originalBranch, err := resolveCurrentBranch()
		if err != nil {
			return &fixIneligibleError{
				Action: action,
				Reason: "publish-new-branch is blocked: cannot determine current branch",
			}
		}
		targetBranch := strings.TrimSpace(opts.ForkBranchRenameTo)
		if targetBranch == "" {
			return &fixIneligibleError{
				Action: action,
				Reason: "publish-new-branch is blocked: publish branch target is required",
			}
		}
		if targetBranch == originalBranch {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf("publish-new-branch is blocked: publish branch target %q matches current branch; choose a different branch name", targetBranch),
			}
		}
		if localBranchExists(targetBranch) {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf("publish-new-branch is blocked: publish branch target %q already exists locally; choose a new branch name", targetBranch),
			}
		}
		pushRemote := resolvePushRemote(target.Record.Upstream)
		if remoteBranchExists(pushRemote, targetBranch) {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf("publish-new-branch is blocked: publish branch target %q already exists on %s; choose a new branch name", targetBranch, pushRemote),
			}
		}
		if opts.ReturnToOriginalBranchAndSync && strings.TrimSpace(target.Record.Upstream) == "" {
			return &fixIneligibleError{
				Action: action,
				Reason: "publish-new-branch is blocked: return-and-sync requires original branch upstream",
			}
		}

		if err := runStep("publish-checkout-new-branch", fixActionPlanEntry{
			ID:      "publish-checkout-new-branch",
			Command: true,
			Summary: fmt.Sprintf("git checkout -b %s", targetBranch),
		}, func() error {
			_, err := a.Git.RunGit(path, "checkout", "-b", targetBranch)
			return err
		}); err != nil {
			return err
		}
		if err := a.runFixStageCommitSteps(cfg, path, target, opts, runStep); err != nil {
			return err
		}
		if err := runStep("publish-push-set-upstream", fixActionPlanEntry{
			ID:      "publish-push-set-upstream",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s", pushRemote, plannedBranch(targetBranch)),
		}, func() error {
			return a.Git.PushUpstreamWithPreferredRemote(path, targetBranch, pushRemote)
		}); err != nil {
			return err
		}
		if !opts.ReturnToOriginalBranchAndSync {
			return nil
		}
		if err := runStep("publish-return-original-branch", fixActionPlanEntry{
			ID:      "publish-return-original-branch",
			Command: true,
			Summary: fmt.Sprintf("git checkout %s", originalBranch),
		}, func() error {
			_, err := a.Git.RunGit(path, "checkout", originalBranch)
			return err
		}); err != nil {
			return err
		}
		if cfg.Sync.FetchPrune {
			if err := runStep("publish-return-fetch-prune", fixActionPlanEntry{
				ID:      "publish-return-fetch-prune",
				Command: true,
				Summary: "git fetch --prune",
			}, func() error {
				return a.Git.FetchPrune(path)
			}); err != nil {
				return err
			}
		} else {
			markSkipped("publish-return-fetch-prune", fixActionPlanEntry{
				ID:      "publish-return-fetch-prune",
				Command: false,
				Summary: "Skip fetch prune because sync.fetch_prune is disabled.",
			})
		}
		return runStep("publish-return-pull-ff-only", fixActionPlanEntry{
			ID:      "publish-return-pull-ff-only",
			Command: true,
			Summary: "git pull --ff-only",
		}, func() error {
			return a.Git.PullFFOnly(path)
		})
	case FixActionCheckpointThenSync:
		if strings.TrimSpace(target.Record.OriginURL) == "" {
			return &fixIneligibleError{
				Action: action,
				Reason: "checkpoint-then-sync is blocked: origin remote is required",
			}
		}
		if strings.TrimSpace(target.Record.Upstream) == "" {
			return &fixIneligibleError{
				Action: action,
				Reason: "checkpoint-then-sync is blocked: upstream is required",
			}
		}
		if target.Record.Diverged {
			return &fixIneligibleError{
				Action: action,
				Reason: "checkpoint-then-sync is blocked: branch diverged from upstream; run sync-with-upstream first",
			}
		}
		if target.Record.Behind == 0 {
			return &fixIneligibleError{
				Action: action,
				Reason: "checkpoint-then-sync is blocked: branch is not behind upstream",
			}
		}
		if !target.Record.HasDirtyTracked && !target.Record.HasUntracked {
			return &fixIneligibleError{
				Action: action,
				Reason: "checkpoint-then-sync is blocked: no local uncommitted changes to checkpoint",
			}
		}
		if err := a.runFixStageCommitSteps(cfg, path, target, opts, runStep); err != nil {
			return err
		}
		probeOutcome, probeErr := a.Git.ProbeSyncWithUpstream(path, target.Record.Upstream, string(syncStrategy))
		if probeErr != nil {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf(
					"checkpoint-then-sync is blocked: %s (selected strategy: %s)",
					domain.ReasonSyncProbeFailed,
					syncStrategy,
				),
			}
		}
		switch mapSyncProbeOutcome(probeOutcome) {
		case fixSyncProbeConflict:
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf(
					"checkpoint-then-sync is blocked: %s (selected strategy: %s)",
					domain.ReasonSyncConflict,
					syncStrategy,
				),
			}
		case fixSyncProbeFailed, fixSyncProbeUnknown:
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf(
					"checkpoint-then-sync is blocked: %s (selected strategy: %s)",
					domain.ReasonSyncProbeFailed,
					syncStrategy,
				),
			}
		}
		if err := a.runFixSyncWithUpstreamSteps(cfg, path, target, syncStrategy, runStep, markSkipped); err != nil {
			return err
		}
		branch, err := resolveCurrentBranch()
		if err != nil {
			return errors.New("cannot determine branch for publish")
		}
		branch, renamed, err := maybeRenameForPublish("checkpoint-rename-branch", branch)
		if err != nil {
			return err
		}
		if renamed {
			pushRemote := resolvePushRemote(target.Record.Upstream)
			return runStep("checkpoint-push", fixActionPlanEntry{
				ID:      "checkpoint-push",
				Command: true,
				Summary: fmt.Sprintf("git push -u %s %s", pushRemote, plannedBranch(branch)),
			}, func() error {
				return a.Git.PushUpstreamWithPreferredRemote(path, branch, pushRemote)
			})
		}
		return runStep("checkpoint-push", fixActionPlanEntry{
			ID:      "checkpoint-push",
			Command: true,
			Summary: "git push",
		}, func() error {
			return a.Git.Push(path)
		})
	case FixActionPullFFOnly:
		if cfg.Sync.FetchPrune {
			if err := runStep("pull-fetch-prune", fixActionPlanEntry{
				ID:      "pull-fetch-prune",
				Command: true,
				Summary: "git fetch --prune",
			}, func() error {
				return a.Git.FetchPrune(path)
			}); err != nil {
				return err
			}
		} else {
			markSkipped("pull-fetch-prune", fixActionPlanEntry{
				ID:      "pull-fetch-prune",
				Command: true,
				Summary: "git fetch --prune",
			})
		}
		return runStep("pull-ff-only", fixActionPlanEntry{ID: "pull-ff-only", Command: true, Summary: "git pull --ff-only"}, func() error {
			return a.Git.PullFFOnly(path)
		})
	case FixActionSetUpstreamPush:
		branch, err := resolveCurrentBranch()
		if err != nil {
			return errors.New("cannot determine branch for upstream push")
		}
		branch, _, err = maybeRenameForPublish("upstream-rename-branch", branch)
		if err != nil {
			return err
		}
		pushRemote := resolvePushRemote(target.Record.Upstream)
		return runStep("upstream-push", fixActionPlanEntry{
			ID:      "upstream-push",
			Command: true,
			Summary: fmt.Sprintf("git push -u %s %s", pushRemote, plannedBranch(branch)),
		}, func() error {
			return a.Git.PushUpstreamWithPreferredRemote(path, branch, pushRemote)
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
	case FixActionClone:
		if strings.TrimSpace(target.Record.RepoKey) == "" {
			return errors.New("repo_key is required for clone")
		}
		if target.Meta == nil {
			return errors.New("repo metadata is required for clone")
		}
		if strings.TrimSpace(path) == "" {
			return errors.New("target path is required for clone")
		}

		allMachines, _, err := loadSyncReconcileInputs(a.Paths)
		if err != nil {
			return err
		}
		winner, ok := selectWinnerForRepo(allMachines, target.Record.RepoKey)
		if !ok {
			return &fixIneligibleError{
				Action: action,
				Reason: "clone is blocked: no syncable winner is available for this repository yet",
			}
		}

		pathConflictReason, err := validateTargetPath(a.Git, path, target.Meta.OriginURL, preferredRemote)
		if err != nil {
			return err
		}
		if pathConflictReason != "" {
			return &fixIneligibleError{
				Action: action,
				Reason: fmt.Sprintf("clone is blocked: %s", pathConflictReason),
			}
		}

		shouldClone := false
		if info, err := os.Stat(path); os.IsNotExist(err) {
			shouldClone = true
		} else if err != nil {
			return err
		} else if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			shouldClone = len(entries) == 0
		}
		if shouldClone {
			if err := runStep("clone-ensure-parent-dir", fixActionPlanEntry{
				ID:      "clone-ensure-parent-dir",
				Command: false,
				Summary: "Create parent directory for clone target if missing.",
			}, func() error {
				return os.MkdirAll(filepath.Dir(path), 0o755)
			}); err != nil {
				return err
			}
			if err := runStep("clone-repo", fixActionPlanEntry{
				ID:      "clone-repo",
				Command: true,
				Summary: fmt.Sprintf("git clone %s %s", target.Meta.OriginURL, path),
			}, func() error {
				return a.Git.Clone(target.Meta.OriginURL, path)
			}); err != nil {
				return err
			}
		} else {
			markSkipped("clone-repo", fixActionPlanEntry{
				ID:      "clone-repo",
				Command: true,
				Summary: fmt.Sprintf("git clone %s %s", target.Meta.OriginURL, path),
			})
		}

		if !a.Git.IsGitRepo(path) {
			return fmt.Errorf("clone failed: %s is not a git repository", path)
		}
		origin, _ := a.Git.RepoOriginWithPreferredRemote(path, preferredRemote)
		matches, _ := originsMatchNormalized(origin, target.Meta.OriginURL)
		if !matches {
			return fmt.Errorf("clone failed: target path origin does not match repo metadata")
		}

		if err := runStep("clone-checkout-branch", fixActionPlanEntry{
			ID:      "clone-checkout-branch",
			Command: true,
			Summary: fmt.Sprintf("git checkout %s", winner.Record.Branch),
		}, func() error {
			return a.Git.EnsureBranchWithPreferredRemote(path, winner.Record.Branch, preferredRemote)
		}); err != nil {
			return err
		}
		if cfg.Sync.FetchPrune {
			if err := runStep("clone-fetch-prune", fixActionPlanEntry{
				ID:      "clone-fetch-prune",
				Command: true,
				Summary: "git fetch --prune",
			}, func() error {
				return a.Git.FetchPrune(path)
			}); err != nil {
				return err
			}
		} else {
			markSkipped("clone-fetch-prune", fixActionPlanEntry{
				ID:      "clone-fetch-prune",
				Command: true,
				Summary: "git fetch --prune",
			})
		}
		return runStep("clone-pull-ff-only", fixActionPlanEntry{
			ID:      "clone-pull-ff-only",
			Command: true,
			Summary: "git pull --ff-only",
		}, func() error {
			return a.Git.PullFFOnly(path)
		})
	default:
		return fmt.Errorf("unknown fix action %q", action)
	}
}

func (a *App) buildFixActionPlanContext(cfg domain.ConfigFile, target fixRepoState, opts fixApplyOptions) fixActionPlanContext {
	preferredRemote := ""
	if target.Meta != nil {
		preferredRemote = strings.TrimSpace(target.Meta.PreferredRemote)
	}
	owner := strings.TrimSpace(cfg.GitHub.Owner)
	forkRemoteExists := false
	if owner != "" && strings.TrimSpace(target.Record.Path) != "" {
		remoteNames, err := a.Git.RemoteNames(target.Record.Path)
		if err == nil {
			for _, name := range remoteNames {
				if name == owner {
					forkRemoteExists = true
					break
				}
			}
		}
	}
	return fixActionPlanContext{
		Operation:                          target.Record.OperationInProgress,
		Branch:                             strings.TrimSpace(target.Record.Branch),
		Upstream:                           strings.TrimSpace(target.Record.Upstream),
		HeadSHA:                            strings.TrimSpace(target.Record.HeadSHA),
		OriginURL:                          strings.TrimSpace(target.Record.OriginURL),
		SyncStrategy:                       normalizeFixSyncStrategy(opts.SyncStrategy),
		PreferredRemote:                    preferredRemote,
		GitHubOwner:                        owner,
		RemoteProtocol:                     strings.TrimSpace(cfg.GitHub.RemoteProtocol),
		ForkRemoteExists:                   forkRemoteExists,
		RepoName:                           strings.TrimSpace(target.Record.Name),
		CommitMessage:                      strings.TrimSpace(opts.CommitMessage),
		CreateProjectName:                  strings.TrimSpace(opts.CreateProjectName),
		CreateProjectVisibility:            opts.CreateProjectVisibility,
		ForkBranchRenameTo:                 strings.TrimSpace(opts.ForkBranchRenameTo),
		ReturnToOriginalBranchAndSync:      opts.ReturnToOriginalBranchAndSync,
		GenerateGitignore:                  opts.GenerateGitignore,
		GitignorePatterns:                  append([]string(nil), opts.GitignorePatterns...),
		MissingRootGitignore:               target.Risk.MissingRootGitignore,
		FetchPrune:                         cfg.Sync.FetchPrune,
		AutoGenerateCommitMessageWhenEmpty: cfg.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty,
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
	opts fixApplyOptions,
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
	forkSource := plannedForkSource(originURL)
	if forkSource == "" {
		forkSource = strings.TrimSpace(originURL)
	}

	var forkURL string
	if err := runStep("fork-gh-fork", fixActionPlanEntry{
		ID:      "fork-gh-fork",
		Command: true,
		Summary: fmt.Sprintf("gh repo fork %s --remote=false --clone=false", forkSource),
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
	setRemoteSummary := fmt.Sprintf("git remote add %s %s", owner, forkURL)
	if remoteExists {
		setRemoteSummary = fmt.Sprintf("git remote set-url %s %s", owner, forkURL)
	}
	if err := runStep("fork-set-remote", fixActionPlanEntry{
		ID:      "fork-set-remote",
		Command: true,
		Summary: setRemoteSummary,
	}, func() error {
		if remoteExists {
			return a.Git.SetRemoteURL(repoPath, forkRemoteName, forkURL)
		}
		return a.Git.AddRemote(repoPath, forkRemoteName, forkURL)
	}); err != nil {
		return err
	}

	if err := runStep("fork-write-metadata", fixActionPlanEntry{
		ID:      "fork-write-metadata",
		Command: false,
		Summary: "Update repo metadata immediately after retargeting remote (preferred remote and push-access probe state reset).",
	}, func() error {
		meta := *target.Meta
		meta.PreferredRemote = forkRemoteName
		meta.PushAccess = domain.PushAccessUnknown
		meta.PushAccessCheckedAt = time.Time{}
		meta.PushAccessCheckedRemote = ""
		meta.PushAccessManualOverride = false
		return state.SaveRepoMetadata(a.Paths, meta)
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

	renamedBranch := false
	renameTo := strings.TrimSpace(opts.ForkBranchRenameTo)
	if renameTo != "" && renameTo != branch {
		if err := runStep("fork-rename-branch", fixActionPlanEntry{
			ID:      "fork-rename-branch",
			Command: true,
			Summary: fmt.Sprintf("git branch -m %s", renameTo),
		}, func() error {
			return a.Git.RenameCurrentBranch(repoPath, renameTo)
		}); err != nil {
			return err
		}
		branch = renameTo
		renamedBranch = true
	}

	pushSummary := fmt.Sprintf("git push -u --force %s %s", owner, plannedBranch(branch))
	push := func() error {
		return a.Git.PushUpstreamForceWithPreferredRemote(repoPath, branch, forkRemoteName)
	}
	if renamedBranch {
		pushSummary = fmt.Sprintf("git push -u %s %s", owner, plannedBranch(branch))
		push = func() error {
			return a.Git.PushUpstreamWithPreferredRemote(repoPath, branch, forkRemoteName)
		}
	}

	if err := runStep("fork-push-upstream", fixActionPlanEntry{
		ID:      "fork-push-upstream",
		Command: true,
		Summary: pushSummary,
	}, func() error {
		return push()
	}); err != nil {
		return err
	}

	return runStep("fork-refresh-metadata", fixActionPlanEntry{
		ID:      "fork-refresh-metadata",
		Command: false,
		Summary: "Refresh repo metadata push-access probe state after retarget push.",
	}, func() error {
		meta, err := state.LoadRepoMetadata(a.Paths, target.Meta.RepoKey)
		if err != nil {
			return err
		}

		updated, _, err := a.probeAndUpdateRepoPushAccess(repoPath, forkURL, meta, true)
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
			Summary: fmt.Sprintf("git push -u %s %s", plannedRemote(meta.PreferredRemote, target.Record.Upstream), plannedBranch(branch)),
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
			Summary: fmt.Sprintf("git push -u %s %s", plannedRemote(meta.PreferredRemote, target.Record.Upstream), plannedBranch(branch)),
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
