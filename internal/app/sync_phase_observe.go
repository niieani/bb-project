package app

import (
	"sort"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func (a *App) observePhase(
	cfg domain.ConfigFile,
	discovered []discoveredRepo,
	previous map[string]domain.MachineRepoRecord,
	opts SyncOptions,
) ([]domain.MachineRepoRecord, map[string]bool, error) {
	a.logf("sync: discovered %d local repo(s)", len(discovered))

	localRecords := make([]domain.MachineRepoRecord, 0, len(previous))
	if opts.Repository != "" {
		for _, record := range previous {
			if record.RepoKey != opts.Repository {
				localRecords = append(localRecords, record)
			}
		}
	}
	transitionedToSynced := map[string]bool{}
	for _, repo := range discovered {
		opts.progress.start(repo.RepoKey, "observe", "Checking "+repo.Name)
		opts.progress.progress(repo.RepoKey, "observe", "Observing "+repo.Name)
		rec, err := a.observeAndApplyLocalSync(cfg, repo, opts)
		if err != nil {
			return nil, nil, opts.progress.fail(repo.RepoKey, err)
		}
		key := repoRecordIdentityKey(rec)
		if old, ok := previous[key]; ok && old.State != domain.RepoStateSynced && rec.State == domain.RepoStateSynced {
			transitionedToSynced[key] = true
		}
		localRecords = append(localRecords, rec)
	}
	if opts.Repository != "" {
		sort.Slice(localRecords, func(i, j int) bool {
			return repoRecordIdentityKey(localRecords[i]) < repoRecordIdentityKey(localRecords[j])
		})
	}

	return localRecords, transitionedToSynced, nil
}

func (a *App) observeAndApplyLocalSync(cfg domain.ConfigFile, repo discoveredRepo, opts SyncOptions) (domain.MachineRepoRecord, error) {
	a.logf("sync: observing local repo %s", repo.Path)
	rec, err := a.observeRepo(cfg, repo, opts.Push)
	if err != nil {
		return domain.MachineRepoRecord{}, err
	}

	if cfg.Sync.FetchPrune && !opts.DryRun {
		opts.progress.progress(repo.RepoKey, "fetch", "Fetching origin")
		a.logf("sync: fetch --prune %s", repo.Path)
		if err := a.Git.FetchPrune(repo.Path); err != nil {
			rec.State = domain.RepoStateBlocked
			rec.Reasons = appendUniqueReasons(rec.Reasons, domain.ReasonPullFailed)
			rec.StateHash = domain.ComputeStateHash(rec)
			return rec, nil
		}
		rec, err = a.observeRepo(cfg, repo, opts.Push)
		if err != nil {
			return domain.MachineRepoRecord{}, err
		}
	}

	if rec.State != domain.RepoStateSynced || opts.DryRun {
		return rec, nil
	}

	if rec.Behind > 0 && rec.Ahead == 0 {
		opts.progress.progress(repo.RepoKey, "pull", "Pulling fast-forward")
		a.logf("sync: pulling ff-only for %s", repo.Path)
		if err := a.Git.PullFFOnly(repo.Path); err != nil {
			rec.State = domain.RepoStateBlocked
			rec.Reasons = appendUniqueReasons(rec.Reasons, domain.ReasonPullFailed)
			rec.StateHash = domain.ComputeStateHash(rec)
			return rec, nil
		}
		a.appendJournal("converged", rec.RepoKey, "pulled fast-forward")
	}

	if rec.Ahead > 0 {
		autoPushMode := domain.AutoPushModeDisabled
		if strings.TrimSpace(rec.RepoKey) != "" {
			if meta, err := state.LoadRepoMetadata(a.Paths, rec.RepoKey); err == nil {
				autoPushMode = domain.NormalizeAutoPushMode(meta.AutoPush)
			}
		}
		if autoPushMode != domain.AutoPushModeDisabled || opts.Push {
			opts.progress.progress(repo.RepoKey, "push", "Pushing commits")
			a.logf("sync: pushing ahead commits for %s", repo.Path)
			if err := a.Git.Push(repo.Path); err != nil {
				rec.State = domain.RepoStateBlocked
				rec.Reasons = appendUniqueReasons(rec.Reasons, domain.ReasonPushFailed)
				rec.StateHash = domain.ComputeStateHash(rec)
				return rec, nil
			}
			a.appendJournal("pushed", rec.RepoKey, "pushed ahead commits")
		}
	}

	return a.observeRepo(cfg, repo, opts.Push)
}

func appendUniqueReasons(in []domain.UnsyncableReason, reason domain.UnsyncableReason) []domain.UnsyncableReason {
	for _, r := range in {
		if r == reason {
			return in
		}
	}
	return append(in, reason)
}
