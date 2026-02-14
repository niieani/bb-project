package app

import (
	"os"
	"path/filepath"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/gitx"
)

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
		if strings.TrimSpace(meta.RepoKey) == "" || strings.TrimSpace(meta.RepoID) == "" {
			continue
		}
		keyCatalog, keyRelativePath, keyRepoName, err := domain.ParseRepoKey(meta.RepoKey)
		if err != nil {
			continue
		}
		if meta.PreferredCatalog != "" {
			if _, existsOnMachine := domain.FindCatalog(*machine, meta.PreferredCatalog); existsOnMachine {
				if _, ok := selectedCatalogMap[meta.PreferredCatalog]; !ok {
					continue
				}
			}
		}

		matches := findLocalMatches(machine.Repos, meta.RepoKey, selectedCatalogMap)

		winner, ok := selectWinnerForRepo(allMachines, meta.RepoKey)
		if !ok {
			a.logf("sync: no syncable winner for %s", meta.RepoKey)
			continue
		}
		if winner.MachineID == machine.MachineID && len(matches) == 1 {
			if remoteWinner, ok := selectWinnerForRepoExcluding(allMachines, meta.RepoKey, machine.MachineID); ok {
				key := repoRecordIdentityKey(machine.Repos[matches[0]])
				if transitionedToSyncable[key] && machine.Repos[matches[0]].Branch != remoteWinner.Record.Branch {
					winner = remoteWinner
				}
			}
		}

		targetCatalog, ok := chooseTargetCatalog(*machine, keyCatalog, meta, selectedCatalogMap)
		if !ok {
			continue
		}
		targetPath := filepath.Join(targetCatalog.Root, filepath.FromSlash(keyRelativePath))
		a.logf("sync: repo %s winner=%s branch=%s target=%s", meta.RepoKey, winner.MachineID, winner.Record.Branch, targetPath)

		pathConflictReason, err := validateTargetPath(a.Git, targetPath, meta.RepoID, meta.PreferredRemote)
		if err != nil {
			return err
		}
		if pathConflictReason != "" {
			a.logf("sync: path conflict at %s: %s", targetPath, pathConflictReason)
			a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, keyRepoName, pathConflictReason)
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
				if err := a.Git.CheckoutWithPreferredRemote(local.Path, winner.Record.Branch, meta.PreferredRemote); err != nil {
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

			catalog := domain.Catalog{Name: local.Catalog}
			if selected, ok := selectedCatalogMap[local.Catalog]; ok {
				catalog = selected
			}
			updated, err := a.observeRepo(cfg, discoveredRepo{
				Catalog: catalog,
				Path:    local.Path,
				Name:    local.Name,
				RepoKey: meta.RepoKey,
			}, opts.Push)
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
		if err := a.ensureLocalCopy(cfg, machine, meta, winner, targetCatalog, targetPath, keyRepoName, opts); err != nil {
			return err
		}
	}

	return nil
}

func selectWinnerForRepo(all []domain.MachineFile, repoKey string) (domain.MachineRepoRecordWithMachine, bool) {
	records := make([]domain.MachineRepoRecordWithMachine, 0)
	for _, m := range all {
		for _, rec := range m.Repos {
			if rec.RepoKey == repoKey {
				records = append(records, domain.MachineRepoRecordWithMachine{MachineID: m.MachineID, Record: rec})
			}
		}
	}
	return domain.SelectWinner(records)
}

func selectWinnerForRepoExcluding(all []domain.MachineFile, repoKey string, excludedMachineID string) (domain.MachineRepoRecordWithMachine, bool) {
	records := make([]domain.MachineRepoRecordWithMachine, 0)
	for _, m := range all {
		if m.MachineID == excludedMachineID {
			continue
		}
		for _, rec := range m.Repos {
			if rec.RepoKey == repoKey {
				records = append(records, domain.MachineRepoRecordWithMachine{MachineID: m.MachineID, Record: rec})
			}
		}
	}
	return domain.SelectWinner(records)
}

func findLocalMatches(records []domain.MachineRepoRecord, repoKey string, selected map[string]domain.Catalog) []int {
	idx := []int{}
	for i, rec := range records {
		if rec.RepoKey != repoKey {
			continue
		}
		if _, ok := selected[rec.Catalog]; !ok {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

func chooseTargetCatalog(machine domain.MachineFile, keyCatalog string, meta domain.RepoMetadataFile, selected map[string]domain.Catalog) (domain.Catalog, bool) {
	if keyCatalog != "" {
		if _, existsOnMachine := domain.FindCatalog(machine, keyCatalog); existsOnMachine {
			if c, ok := selected[keyCatalog]; ok {
				return c, true
			}
			return domain.Catalog{}, false
		}
	}
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

func validateTargetPath(g gitx.Runner, targetPath string, expectedRepoID string, preferredRemote string) (domain.UnsyncableReason, error) {
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
	origin, err := g.RepoOriginWithPreferredRemote(targetPath, preferredRemote)
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
	repoName string,
	opts SyncOptions,
) error {
	if info, err := os.Stat(targetPath); os.IsNotExist(err) {
		a.logf("sync: cloning %s into %s", winner.Record.OriginURL, targetPath)
		if err := a.Git.Clone(winner.Record.OriginURL, targetPath); err != nil {
			a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonCheckoutFailed)
			return nil
		}
		if err := a.Git.EnsureBranchWithPreferredRemote(targetPath, winner.Record.Branch, meta.PreferredRemote); err != nil {
			a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonCheckoutFailed)
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
				a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonCheckoutFailed)
				return nil
			}
			if err := a.Git.EnsureBranchWithPreferredRemote(targetPath, winner.Record.Branch, meta.PreferredRemote); err != nil {
				a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonCheckoutFailed)
				return nil
			}
		}
	}

	if !a.Git.IsGitRepo(targetPath) {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonTargetPathNonRepo)
		return nil
	}
	origin, _ := a.Git.RepoOriginWithPreferredRemote(targetPath, meta.PreferredRemote)
	originID, _ := domain.NormalizeOriginToRepoID(origin)
	if originID != meta.RepoID {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonTargetPathRepoMismatch)
		return nil
	}

	if err := a.Git.EnsureBranchWithPreferredRemote(targetPath, winner.Record.Branch, meta.PreferredRemote); err != nil {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonCheckoutFailed)
		return nil
	}
	if cfg.Sync.FetchPrune {
		a.logf("sync: fetch --prune %s", targetPath)
		_ = a.Git.FetchPrune(targetPath)
	}
	a.logf("sync: pull --ff-only %s", targetPath)
	if err := a.Git.PullFFOnly(targetPath); err != nil {
		a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonPullFailed)
		return nil
	}

	rec, err := a.observeRepo(cfg, discoveredRepo{
		Catalog: targetCatalog,
		Path:    targetPath,
		Name:    repoName,
		RepoKey: meta.RepoKey,
	}, opts.Push)
	if err != nil {
		return err
	}
	machine.Repos = append(machine.Repos, rec)
	return nil
}

func (a *App) addOrUpdateSyntheticUnsyncable(machine *domain.MachineFile, meta domain.RepoMetadataFile, catalog, targetPath string, repoName string, reason domain.UnsyncableReason) {
	for i := range machine.Repos {
		if machine.Repos[i].RepoKey == meta.RepoKey && machine.Repos[i].Path == targetPath {
			machine.Repos[i].Syncable = false
			machine.Repos[i].UnsyncableReasons = []domain.UnsyncableReason{reason}
			machine.Repos[i].StateHash = domain.ComputeStateHash(machine.Repos[i])
			return
		}
	}
	name := strings.TrimSpace(repoName)
	if name == "" {
		name = strings.TrimSpace(meta.Name)
	}
	rec := domain.MachineRepoRecord{
		RepoKey:           meta.RepoKey,
		RepoID:            meta.RepoID,
		Name:              name,
		Catalog:           catalog,
		Path:              targetPath,
		OriginURL:         meta.OriginURL,
		Syncable:          false,
		UnsyncableReasons: []domain.UnsyncableReason{reason},
	}
	rec.StateHash = domain.ComputeStateHash(rec)
	machine.Repos = append(machine.Repos, rec)
}
