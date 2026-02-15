package app

import (
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func (a *App) observePhase(
	cfg domain.ConfigFile,
	selectedCatalogs []domain.Catalog,
	previous map[string]domain.MachineRepoRecord,
	opts SyncOptions,
) ([]domain.MachineRepoRecord, map[string]bool, error) {
	discovered, err := discoverRepos(selectedCatalogs)
	if err != nil {
		return nil, nil, err
	}
	a.logf("sync: discovered %d local repo(s)", len(discovered))

	localRecords := make([]domain.MachineRepoRecord, 0, len(discovered))
	transitionedToSyncable := map[string]bool{}
	for _, repo := range discovered {
		rec, err := a.observeAndApplyLocalSync(cfg, repo, opts)
		if err != nil {
			return nil, nil, err
		}
		key := repoRecordIdentityKey(rec)
		if old, ok := previous[key]; ok && !old.Syncable && rec.Syncable {
			transitionedToSyncable[key] = true
		}
		localRecords = append(localRecords, rec)
	}

	return localRecords, transitionedToSyncable, nil
}

func (a *App) observeAndApplyLocalSync(cfg domain.ConfigFile, repo discoveredRepo, opts SyncOptions) (domain.MachineRepoRecord, error) {
	a.logf("sync: observing local repo %s", repo.Path)
	rec, err := a.observeRepo(cfg, repo, opts.Push)
	if err != nil {
		return domain.MachineRepoRecord{}, err
	}

	if cfg.Sync.FetchPrune && !opts.DryRun {
		a.logf("sync: fetch --prune %s", repo.Path)
		if err := a.Git.FetchPrune(repo.Path); err != nil {
			rec.Syncable = false
			rec.UnsyncableReasons = appendUniqueReasons(rec.UnsyncableReasons, domain.ReasonPullFailed)
			rec.StateHash = domain.ComputeStateHash(rec)
			return rec, nil
		}
		rec, err = a.observeRepo(cfg, repo, opts.Push)
		if err != nil {
			return domain.MachineRepoRecord{}, err
		}
	}

	if !rec.Syncable || opts.DryRun {
		return rec, nil
	}

	if rec.Behind > 0 && rec.Ahead == 0 {
		a.logf("sync: pulling ff-only for %s", repo.Path)
		if err := a.Git.PullFFOnly(repo.Path); err != nil {
			rec.Syncable = false
			rec.UnsyncableReasons = appendUniqueReasons(rec.UnsyncableReasons, domain.ReasonPullFailed)
			rec.StateHash = domain.ComputeStateHash(rec)
			return rec, nil
		}
	}

	if rec.Ahead > 0 {
		autoPushMode := domain.AutoPushModeDisabled
		if strings.TrimSpace(rec.RepoKey) != "" {
			if meta, err := state.LoadRepoMetadata(a.Paths, rec.RepoKey); err == nil {
				autoPushMode = domain.NormalizeAutoPushMode(meta.AutoPush)
			}
		}
		if autoPushMode != domain.AutoPushModeDisabled || opts.Push {
			a.logf("sync: pushing ahead commits for %s", repo.Path)
			if err := a.Git.Push(repo.Path); err != nil {
				rec.Syncable = false
				rec.UnsyncableReasons = appendUniqueReasons(rec.UnsyncableReasons, domain.ReasonPushFailed)
				rec.StateHash = domain.ComputeStateHash(rec)
				return rec, nil
			}
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
