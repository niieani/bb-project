package app

import (
	"fmt"
	"sort"
	"strings"

	"bb-project/internal/domain"
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

	selectedCatalogs, selectedCatalogMap, err := selectSyncCatalogs(a.Paths, machine, opts.IncludeCatalogs)
	if err != nil {
		return 2, err
	}
	a.logf("sync: selected %d catalog(s)", len(selectedCatalogs))

	previous := previousRepoRecords(machine.Repos)
	localRecords, transitionedToSyncable, err := a.observePhase(cfg, selectedCatalogs, previous, opts)
	if err != nil {
		return 2, err
	}
	machine.Repos = localRecords
	machine.UpdatedAt = a.Now()
	if err := persistMachineRecords(a.Paths, &machine, previous, a.Now); err != nil {
		return 2, err
	}
	a.logf("sync: published local observations")

	machines, repoMetas, err := loadSyncReconcileInputs(a.Paths)
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
		if err := a.notifyUnsyncable(cfg, machine.Repos, opts.NotifyBackend); err != nil {
			return 2, err
		}
	}

	if anyUnsyncableInSelectedCatalogs(machine.Repos, selectedCatalogMap) {
		a.logf("sync: completed with unsyncable repos")
		return 1, nil
	}
	a.logf("sync: completed successfully")
	return 0, nil
}

func selectSyncCatalogs(paths state.Paths, machine domain.MachineFile, include []string) ([]domain.Catalog, map[string]domain.Catalog, error) {
	selectedCatalogs, err := domain.SelectCatalogs(machine, include)
	if err != nil {
		return nil, nil, annotateRemoteCatalogSelectionError(paths, machine, include, err)
	}

	selectedCatalogMap := map[string]domain.Catalog{}
	for _, c := range selectedCatalogs {
		selectedCatalogMap[c.Name] = c
	}

	return selectedCatalogs, selectedCatalogMap, nil
}

func annotateRemoteCatalogSelectionError(paths state.Paths, machine domain.MachineFile, include []string, selectErr error) error {
	if selectErr == nil || len(include) == 0 {
		return selectErr
	}
	knownCatalogRoots, err := loadKnownCatalogRoots(paths, machine.MachineID)
	if err != nil {
		return selectErr
	}
	localCatalogs := map[string]struct{}{}
	for _, catalog := range machine.Catalogs {
		name := strings.TrimSpace(catalog.Name)
		if name == "" {
			continue
		}
		localCatalogs[name] = struct{}{}
	}

	remoteOnlySelections := []string{}
	seen := map[string]struct{}{}
	for _, item := range include {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		if _, ok := localCatalogs[name]; ok {
			continue
		}
		if _, ok := knownCatalogRoots[name]; !ok {
			continue
		}
		remoteOnlySelections = append(remoteOnlySelections, name)
	}
	if len(remoteOnlySelections) == 0 {
		return selectErr
	}
	sort.Strings(remoteOnlySelections)
	return fmt.Errorf(
		"%w; catalog(s) %s are known on other machines but not mapped locally; run bb config to add local catalog mappings",
		selectErr,
		strings.Join(remoteOnlySelections, ", "),
	)
}

func previousRepoRecords(repos []domain.MachineRepoRecord) map[string]domain.MachineRepoRecord {
	previous := map[string]domain.MachineRepoRecord{}
	for _, rec := range repos {
		previous[repoRecordIdentityKey(rec)] = rec
	}
	return previous
}

func loadSyncReconcileInputs(paths state.Paths) ([]domain.MachineFile, []domain.RepoMetadataFile, error) {
	machines, err := state.LoadAllMachineFiles(paths)
	if err != nil {
		return nil, nil, err
	}

	repoMetas, err := state.LoadAllRepoMetadata(paths)
	if err != nil {
		return nil, nil, err
	}

	return machines, repoMetas, nil
}

func anyUnsyncableInSelectedCatalogs(repos []domain.MachineRepoRecord, selectedCatalogMap map[string]domain.Catalog) bool {
	for _, rec := range repos {
		if _, ok := selectedCatalogMap[rec.Catalog]; !ok {
			continue
		}
		if !rec.Syncable {
			if len(rec.UnsyncableReasons) == 0 || domain.HasBlockingUnsyncableReason(rec.UnsyncableReasons) {
				return true
			}
		}
	}
	return false
}
