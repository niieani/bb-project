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
	moveIndex, err := buildRepoMoveIndex(repoMetas)
	if err != nil {
		return err
	}
	warnedUnmappedCatalogs := map[string]bool{}
	for _, meta := range repoMetas {
		if _, historical := moveIndex[strings.TrimSpace(meta.RepoKey)]; historical {
			continue
		}
		if strings.TrimSpace(meta.RepoKey) == "" || strings.TrimSpace(meta.OriginURL) == "" {
			continue
		}
		keyCatalog, keyRelativePath, keyRepoName, err := domain.ParseRepoKey(meta.RepoKey)
		if err != nil {
			continue
		}
		targetCatalog, ok := selectedCatalogMap[keyCatalog]
		if !ok {
			if _, existsOnMachine := domain.FindCatalog(*machine, keyCatalog); !existsOnMachine {
				if !warnedUnmappedCatalogs[keyCatalog] {
					a.logf(
						"warning: repo %s references catalog %q that is not configured locally; run bb config to map catalogs",
						meta.RepoKey,
						keyCatalog,
					)
					warnedUnmappedCatalogs[keyCatalog] = true
				}
			}
			staleMatches := findLocalMatchesByRepoKeys(machine.Repos, meta.PreviousRepoKeys, selectedCatalogMap)
			for _, idx := range staleMatches {
				a.markCatalogMismatch(
					&machine.Repos[idx],
					meta.RepoKey,
					keyCatalog,
					"",
					true,
				)
			}
			continue
		}

		matches := findLocalMatches(machine.Repos, meta.RepoKey, selectedCatalogMap)
		targetPath := filepath.Join(targetCatalog.Root, filepath.FromSlash(keyRelativePath))
		staleMatches := findLocalMatchesByRepoKeys(machine.Repos, meta.PreviousRepoKeys, selectedCatalogMap)
		if len(matches) == 0 && len(staleMatches) > 0 {
			for _, idx := range staleMatches {
				a.markCatalogMismatch(
					&machine.Repos[idx],
					meta.RepoKey,
					targetCatalog.Name,
					targetPath,
					false,
				)
			}
			continue
		}
		if len(matches) == 0 && len(staleMatches) == 0 && len(meta.PreviousRepoKeys) > 0 {
			// Repository was moved on another machine. If this machine never had the old path,
			// treat it as a no-op and avoid synthesizing clone_required.
			continue
		}

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

		a.logf("sync: repo %s winner=%s branch=%s target=%s", meta.RepoKey, winner.MachineID, winner.Record.Branch, targetPath)

		pathConflictReason, err := validateTargetPath(a.Git, targetPath, meta.OriginURL, meta.PreferredRemote)
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
		if err := a.ensureLocalCopy(
			cfg,
			machine,
			meta,
			winner,
			targetCatalog,
			targetPath,
			keyRepoName,
			opts,
			targetCatalog.AllowsAutoCloneOnSync(),
		); err != nil {
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

func findLocalMatchesByRepoKeys(records []domain.MachineRepoRecord, repoKeys []string, selected map[string]domain.Catalog) []int {
	if len(repoKeys) == 0 {
		return nil
	}
	allowedKeys := make(map[string]struct{}, len(repoKeys))
	for _, repoKey := range repoKeys {
		repoKey = strings.TrimSpace(repoKey)
		if repoKey == "" {
			continue
		}
		allowedKeys[repoKey] = struct{}{}
	}
	if len(allowedKeys) == 0 {
		return nil
	}
	idx := make([]int, 0, 2)
	for i, rec := range records {
		if _, ok := selected[rec.Catalog]; !ok {
			continue
		}
		if _, ok := allowedKeys[strings.TrimSpace(rec.RepoKey)]; !ok {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

func validateTargetPath(g gitx.Runner, targetPath string, expectedOriginURL string, preferredRemote string) (domain.UnsyncableReason, error) {
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
	matches, err := originsMatchNormalized(origin, expectedOriginURL)
	if err != nil {
		return domain.ReasonTargetPathRepoMismatch, nil
	}
	if !matches {
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
	allowClone bool,
) error {
	if info, err := os.Stat(targetPath); os.IsNotExist(err) {
		if !allowClone {
			a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonCloneRequired)
			return nil
		}
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
			if !allowClone {
				a.addOrUpdateSyntheticUnsyncable(machine, meta, targetCatalog.Name, targetPath, repoName, domain.ReasonCloneRequired)
				return nil
			}
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
	matches, _ := originsMatchNormalized(origin, meta.OriginURL)
	if !matches {
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

func (a *App) markCatalogMismatch(
	rec *domain.MachineRepoRecord,
	expectedRepoKey string,
	expectedCatalog string,
	expectedPath string,
	catalogNotMapped bool,
) {
	if rec == nil {
		return
	}
	rec.Syncable = false
	reasons := []domain.UnsyncableReason{domain.ReasonCatalogMismatch}
	if catalogNotMapped {
		reasons = append(reasons, domain.ReasonCatalogNotMapped)
	}
	rec.UnsyncableReasons = reasons
	rec.ExpectedRepoKey = strings.TrimSpace(expectedRepoKey)
	rec.ExpectedCatalog = strings.TrimSpace(expectedCatalog)
	rec.ExpectedPath = strings.TrimSpace(expectedPath)
	rec.StateHash = domain.ComputeStateHash(*rec)
}
