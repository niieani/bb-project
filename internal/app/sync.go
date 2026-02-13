package app

import (
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

func (a *App) runSync(opts SyncOptions) (int, error) {
	a.logf("sync: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths)
	if err != nil {
		return 2, err
	}
	defer func() {
		_ = lock.Release()
		a.logf("sync: released global lock")
	}()

	cfg, machine, err := a.loadContext()
	if err != nil {
		return 2, err
	}
	a.logf("sync: start push=%t notify=%t dry-run=%t", opts.Push, opts.Notify, opts.DryRun)

	selectedCatalogs, err := domain.SelectCatalogs(machine, opts.IncludeCatalogs)
	if err != nil {
		return 2, err
	}
	a.logf("sync: selected %d catalog(s)", len(selectedCatalogs))
	selectedCatalogMap := map[string]domain.Catalog{}
	for _, c := range selectedCatalogs {
		selectedCatalogMap[c.Name] = c
	}

	previous := map[string]domain.MachineRepoRecord{}
	for _, rec := range machine.Repos {
		previous[rec.RepoID+"|"+rec.Path] = rec
	}

	discovered, err := discoverRepos(selectedCatalogs)
	if err != nil {
		return 2, err
	}
	a.logf("sync: discovered %d local repo(s)", len(discovered))
	localRecords := make([]domain.MachineRepoRecord, 0, len(discovered))
	transitionedToSyncable := map[string]bool{}
	for _, repo := range discovered {
		rec, err := a.observeAndApplyLocalSync(cfg, repo, opts)
		if err != nil {
			return 2, err
		}
		key := rec.RepoID + "|" + rec.Path
		if old, ok := previous[key]; ok && !old.Syncable && rec.Syncable {
			transitionedToSyncable[key] = true
		}
		localRecords = append(localRecords, rec)
	}

	machine.Repos = localRecords
	machine.UpdatedAt = a.Now()
	if err := persistMachineRecords(a.Paths, &machine, previous, a.Now); err != nil {
		return 2, err
	}
	a.logf("sync: published local observations")

	machines, err := state.LoadAllMachineFiles(a.Paths)
	if err != nil {
		return 2, err
	}
	repoMetas, err := state.LoadAllRepoMetadata(a.Paths)
	if err != nil {
		return 2, err
	}

	if err := a.ensureFromWinners(cfg, &machine, machines, repoMetas, selectedCatalogMap, transitionedToSyncable, opts); err != nil {
		return 2, err
	}
	a.logf("sync: winner reconciliation completed")

	if err := persistMachineRecords(a.Paths, &machine, previous, a.Now); err != nil {
		return 2, err
	}
	a.logf("sync: published post-reconciliation observations")

	if opts.Notify {
		a.logf("sync: processing notifications")
		if err := a.notifyUnsyncable(cfg, machine.Repos); err != nil {
			return 2, err
		}
	}

	anyUnsyncable := false
	for _, rec := range machine.Repos {
		if _, ok := selectedCatalogMap[rec.Catalog]; !ok {
			continue
		}
		if !rec.Syncable {
			anyUnsyncable = true
			break
		}
	}
	if anyUnsyncable {
		a.logf("sync: completed with unsyncable repos")
		return 1, nil
	}
	a.logf("sync: completed successfully")
	return 0, nil
}

func persistMachineRecords(paths state.Paths, machine *domain.MachineFile, previous map[string]domain.MachineRepoRecord, now timeNowFn) error {
	for i := range machine.Repos {
		key := machine.Repos[i].RepoID + "|" + machine.Repos[i].Path
		old := previous[key]
		machine.Repos[i] = domain.UpdateObservedAt(old, machine.Repos[i], now())
	}
	sort.Slice(machine.Repos, func(i, j int) bool {
		if machine.Repos[i].RepoID == machine.Repos[j].RepoID {
			return machine.Repos[i].Path < machine.Repos[j].Path
		}
		return machine.Repos[i].RepoID < machine.Repos[j].RepoID
	})
	machine.UpdatedAt = now()
	return state.SaveMachine(paths, *machine)
}

type timeNowFn func() time.Time

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
		autoPush := false
		if meta, err := state.LoadRepoMetadata(a.Paths, rec.RepoID); err == nil {
			autoPush = meta.AutoPush
		}
		if autoPush || opts.Push {
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

func (a *App) ensureFromWinners(
	cfg domain.ConfigFile,
	machine *domain.MachineFile,
	allMachines []domain.MachineFile,
	repoMetas []domain.RepoMetadataFile,
	selectedCatalogMap map[string]domain.Catalog,
	transitionedToSyncable map[string]bool,
	opts SyncOptions,
) error {
	a.logf("sync: reconciling %d repo metadata entries", len(repoMetas))
	for _, meta := range repoMetas {
		if meta.RepoID == "" || meta.Name == "" {
			continue
		}
		if meta.PreferredCatalog != "" {
			if _, existsOnMachine := domain.FindCatalog(*machine, meta.PreferredCatalog); existsOnMachine {
				if _, ok := selectedCatalogMap[meta.PreferredCatalog]; !ok {
					continue
				}
			}
		}

		matches := findLocalMatches(machine.Repos, meta.RepoID, selectedCatalogMap)
		if len(matches) > 1 {
			a.logf("sync: duplicate local repo_id detected for %s", meta.RepoID)
			for _, idx := range matches {
				machine.Repos[idx].Syncable = false
				machine.Repos[idx].UnsyncableReasons = []domain.UnsyncableReason{domain.ReasonDuplicateLocalRepoID}
				machine.Repos[idx].StateHash = domain.ComputeStateHash(machine.Repos[idx])
			}
			continue
		}

		winner, ok := selectWinnerForRepo(allMachines, meta.RepoID)
		if !ok {
			a.logf("sync: no syncable winner for %s", meta.RepoID)
			continue
		}
		if winner.MachineID == machine.MachineID && len(matches) == 1 {
			if remoteWinner, ok := selectWinnerForRepoExcluding(allMachines, meta.RepoID, machine.MachineID); ok {
				key := machine.Repos[matches[0]].RepoID + "|" + machine.Repos[matches[0]].Path
				if transitionedToSyncable[key] && machine.Repos[matches[0]].Branch != remoteWinner.Record.Branch {
					winner = remoteWinner
				}
			}
		}

		targetCatalog, ok := chooseTargetCatalog(*machine, meta, selectedCatalogMap)
		if !ok {
			continue
		}
		targetPath := filepath.Join(targetCatalog.Root, meta.Name)
		a.logf("sync: repo %s winner=%s branch=%s target=%s", meta.RepoID, winner.MachineID, winner.Record.Branch, targetPath)

		pathConflictReason, err := validateTargetPath(a.Git, targetPath, meta.RepoID)
		if err != nil {
			return err
		}
		if pathConflictReason != "" {
			a.logf("sync: path conflict at %s: %s", targetPath, pathConflictReason)
			a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, pathConflictReason)
			continue
		}

		if len(matches) == 1 {
			idx := matches[0]
			local := machine.Repos[idx]
			if !local.Syncable {
				continue
			}
			if opts.DryRun {
				continue
			}

			if local.Branch != winner.Record.Branch {
				a.logf("sync: checking out branch %s in %s", winner.Record.Branch, local.Path)
				if err := a.Git.Checkout(local.Path, winner.Record.Branch); err != nil {
					machine.Repos[idx].Syncable = false
					machine.Repos[idx].UnsyncableReasons = appendUniqueReasons(machine.Repos[idx].UnsyncableReasons, domain.ReasonCheckoutFailed)
					machine.Repos[idx].StateHash = domain.ComputeStateHash(machine.Repos[idx])
					continue
				}
			}

			if cfg.Sync.FetchPrune {
				a.logf("sync: fetch --prune %s before pull", local.Path)
				_ = a.Git.FetchPrune(local.Path)
			}
			a.logf("sync: pull --ff-only %s", local.Path)
			if err := a.Git.PullFFOnly(local.Path); err != nil {
				machine.Repos[idx].Syncable = false
				machine.Repos[idx].UnsyncableReasons = appendUniqueReasons(machine.Repos[idx].UnsyncableReasons, domain.ReasonPullFailed)
				machine.Repos[idx].StateHash = domain.ComputeStateHash(machine.Repos[idx])
				continue
			}

			updated, err := a.observeRepo(cfg, discoveredRepo{CatalogName: local.Catalog, Path: local.Path, Name: local.Name}, opts.Push)
			if err != nil {
				return err
			}
			machine.Repos[idx] = updated
			continue
		}

		if opts.DryRun {
			continue
		}
		a.logf("sync: ensuring local copy at %s", targetPath)
		if err := a.ensureLocalCopy(cfg, machine, meta, winner, targetCatalog, targetPath, selectedCatalogMap, opts); err != nil {
			return err
		}
	}

	return nil
}

func selectWinnerForRepo(all []domain.MachineFile, repoID string) (domain.MachineRepoRecordWithMachine, bool) {
	records := make([]domain.MachineRepoRecordWithMachine, 0)
	for _, m := range all {
		for _, rec := range m.Repos {
			if rec.RepoID == repoID {
				records = append(records, domain.MachineRepoRecordWithMachine{MachineID: m.MachineID, Record: rec})
			}
		}
	}
	return domain.SelectWinner(records)
}

func selectWinnerForRepoExcluding(all []domain.MachineFile, repoID string, excludedMachineID string) (domain.MachineRepoRecordWithMachine, bool) {
	records := make([]domain.MachineRepoRecordWithMachine, 0)
	for _, m := range all {
		if m.MachineID == excludedMachineID {
			continue
		}
		for _, rec := range m.Repos {
			if rec.RepoID == repoID {
				records = append(records, domain.MachineRepoRecordWithMachine{MachineID: m.MachineID, Record: rec})
			}
		}
	}
	return domain.SelectWinner(records)
}

func findLocalMatches(records []domain.MachineRepoRecord, repoID string, selected map[string]domain.Catalog) []int {
	idx := []int{}
	for i, rec := range records {
		if rec.RepoID != repoID {
			continue
		}
		if _, ok := selected[rec.Catalog]; !ok {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

func chooseTargetCatalog(machine domain.MachineFile, meta domain.RepoMetadataFile, selected map[string]domain.Catalog) (domain.Catalog, bool) {
	if meta.PreferredCatalog != "" {
		if c, ok := selected[meta.PreferredCatalog]; ok {
			return c, true
		}
	}
	if machine.DefaultCatalog != "" {
		if c, ok := selected[machine.DefaultCatalog]; ok {
			return c, true
		}
	}
	for _, c := range selected {
		return c, true
	}
	return domain.Catalog{}, false
}

func validateTargetPath(g gitx.Runner, targetPath string, expectedRepoID string) (domain.UnsyncableReason, error) {
	info, err := os.Stat(targetPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return domain.ReasonTargetPathNonRepo, nil
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	if !g.IsGitRepo(targetPath) {
		return domain.ReasonTargetPathNonRepo, nil
	}
	origin, err := g.RepoOrigin(targetPath)
	if err != nil {
		return "", err
	}
	originID, err := domain.NormalizeOriginToRepoID(origin)
	if err != nil {
		return domain.ReasonTargetPathRepoMismatch, nil
	}
	if originID != expectedRepoID {
		return domain.ReasonTargetPathRepoMismatch, nil
	}
	return "", nil
}

func (a *App) ensureLocalCopy(
	cfg domain.ConfigFile,
	machine *domain.MachineFile,
	meta domain.RepoMetadataFile,
	winner domain.MachineRepoRecordWithMachine,
	targetCatalog domain.Catalog,
	targetPath string,
	selected map[string]domain.Catalog,
	opts SyncOptions,
) error {
	if info, err := os.Stat(targetPath); os.IsNotExist(err) {
		a.logf("sync: cloning %s into %s", winner.Record.OriginURL, targetPath)
		if err := a.Git.Clone(winner.Record.OriginURL, targetPath); err != nil {
			a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonCheckoutFailed)
			return nil
		}
		if err := a.Git.EnsureBranch(targetPath, winner.Record.Branch); err != nil {
			a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonCheckoutFailed)
			return nil
		}
	} else if err != nil {
		return err
	} else if info.IsDir() {
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			a.logf("sync: cloning into empty directory %s", targetPath)
			if err := a.Git.Clone(winner.Record.OriginURL, targetPath); err != nil {
				a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonCheckoutFailed)
				return nil
			}
			if err := a.Git.EnsureBranch(targetPath, winner.Record.Branch); err != nil {
				a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonCheckoutFailed)
				return nil
			}
		}
	}

	if !a.Git.IsGitRepo(targetPath) {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonTargetPathNonRepo)
		return nil
	}
	origin, _ := a.Git.RepoOrigin(targetPath)
	originID, _ := domain.NormalizeOriginToRepoID(origin)
	if originID != meta.RepoID {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonTargetPathRepoMismatch)
		return nil
	}

	if err := a.Git.EnsureBranch(targetPath, winner.Record.Branch); err != nil {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonCheckoutFailed)
		return nil
	}
	if cfg.Sync.FetchPrune {
		a.logf("sync: fetch --prune %s", targetPath)
		_ = a.Git.FetchPrune(targetPath)
	}
	a.logf("sync: pull --ff-only %s", targetPath)
	if err := a.Git.PullFFOnly(targetPath); err != nil {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, domain.ReasonPullFailed)
		return nil
	}

	rec, err := a.observeRepo(cfg, discoveredRepo{CatalogName: targetCatalog.Name, Path: targetPath, Name: meta.Name}, opts.Push)
	if err != nil {
		return err
	}
	machine.Repos = append(machine.Repos, rec)
	return nil
}

func (a *App) addOrUpdateSyntheticUnsyncable(machine *domain.MachineFile, meta domain.RepoMetadataFile, catalog, targetPath string, reason domain.UnsyncableReason) {
	for i := range machine.Repos {
		if machine.Repos[i].RepoID == meta.RepoID && machine.Repos[i].Path == targetPath {
			machine.Repos[i].Syncable = false
			machine.Repos[i].UnsyncableReasons = []domain.UnsyncableReason{reason}
			machine.Repos[i].StateHash = domain.ComputeStateHash(machine.Repos[i])
			return
		}
	}
	rec := domain.MachineRepoRecord{
		RepoID:            meta.RepoID,
		Name:              meta.Name,
		Catalog:           catalog,
		Path:              targetPath,
		OriginURL:         meta.OriginURL,
		Syncable:          false,
		UnsyncableReasons: []domain.UnsyncableReason{reason},
	}
	rec.StateHash = domain.ComputeStateHash(rec)
	machine.Repos = append(machine.Repos, rec)
}

func appendUniqueReasons(in []domain.UnsyncableReason, reason domain.UnsyncableReason) []domain.UnsyncableReason {
	for _, r := range in {
		if r == reason {
			return in
		}
	}
	return append(in, reason)
}

func (a *App) notifyUnsyncable(cfg domain.ConfigFile, repos []domain.MachineRepoRecord) error {
	if !cfg.Notify.Enabled {
		a.logf("notify: disabled in config")
		return nil
	}
	cache, err := state.LoadNotifyCache(a.Paths)
	if err != nil {
		return err
	}
	for _, rec := range repos {
		if rec.Syncable {
			continue
		}
		fingerprint := unsyncableFingerprint(rec.UnsyncableReasons)
		entry, ok := cache.LastSent[rec.RepoID]
		if ok && entry.Fingerprint == fingerprint && cfg.Notify.Dedupe {
			a.logf("notify: deduped %s (%s)", rec.Name, fingerprint)
			continue
		}
		a.logf("notify: emitting for %s (%s)", rec.Name, fingerprint)
		fmt.Fprintf(a.Stdout, "notify %s: %s\n", rec.Name, fingerprint)
		cache.LastSent[rec.RepoID] = domain.NotifyCacheEntry{Fingerprint: fingerprint, SentAt: a.Now()}
	}
	return state.SaveNotifyCache(a.Paths, cache)
}

func unsyncableFingerprint(reasons []domain.UnsyncableReason) string {
	if len(reasons) == 0 {
		return ""
	}
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		parts = append(parts, string(r))
	}
	sort.Strings(parts)
	return strings.Join(parts, "+")
}
