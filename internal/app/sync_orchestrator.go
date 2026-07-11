package app

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func (a *App) runSync(opts SyncOptions) (code int, runErr error) {
	started := a.Now()
	completed, total := 0, 0
	a.emitOperationEvent(opts.EventsJSON, OperationEvent{Event: "operation_started", Operation: "sync", Repository: opts.Repository, Message: "Starting sync"})
	defer func() {
		if opts.progress != nil {
			completed, total = opts.progress.completed, opts.progress.total
		}
		event := OperationEvent{Event: "operation_finished", Operation: "sync", Repository: opts.Repository, Message: "Sync completed", Result: "success"}
		if runErr != nil || code != 0 {
			event.Message = "Sync failed"
			event.Result = "failure"
			if runErr != nil {
				event.Error = runErr.Error()
			}
		}
		a.emitOperationEvent(opts.EventsJSON, withOperationCounts(event, completed, total))
	}()
	a.logf("sync: acquiring global lock")
	lock, err := state.AcquireLock(a.Paths, "sync")
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
	defer func() {
		counts := map[domain.RepoSyncState]int{}
		for _, r := range machine.Repos {
			counts[r.State]++
		}
		a.appendJournal("sync_run", "", fmt.Sprintf("synced=%d pending=%d wip=%d blocked=%d duration=%s", counts[domain.RepoStateSynced], counts[domain.RepoStatePending], counts[domain.RepoStateWip], counts[domain.RepoStateBlocked], a.Now().Sub(started)))
	}()
	a.logf("sync: start push=%t dry-run=%t", opts.Push, opts.DryRun)

	if strings.TrimSpace(opts.Repository) != "" {
		target, found, resolveErr := resolveLocalProjectSelector(machine.Repos, opts.Repository)
		if resolveErr != nil {
			return 2, resolveErr
		}
		if !found {
			return 2, fmt.Errorf("repository %q not found locally", opts.Repository)
		}
		opts.Repository = target.RepoKey
		opts.IncludeCatalogs = []string{target.Catalog}
	}
	selectedCatalogs, selectedCatalogMap, err := selectSyncCatalogs(a.Paths, machine, opts.IncludeCatalogs)
	if err != nil {
		return 2, err
	}
	a.logf("sync: selected %d catalog(s)", len(selectedCatalogs))
	if opts.Repository == "" {
		if err := a.alignRemoteFormatsBeforeObservation(cfg, selectedCatalogs, opts.DryRun, opts.EventsJSON); err != nil {
			return 2, err
		}
	}
	discovered, err := discoverRepos(selectedCatalogs)
	if err != nil {
		return 2, err
	}
	if opts.Repository != "" {
		discovered = filterDiscoveredRepositories(discovered, opts.Repository)
	}
	plannedMachines, plannedMetas, err := loadSyncReconcileInputs(a.Paths)
	if err != nil {
		return 2, err
	}
	planned := plannedSyncRepositories(discovered, plannedMachines, plannedMetas, selectedCatalogMap, opts.Repository)
	total = len(planned)
	opts.progress = newSyncOperationProgress(a, opts.EventsJSON, total)
	a.emitOperationEvent(opts.EventsJSON, withOperationCounts(OperationEvent{Event: "progress", Operation: "sync", Message: "Discovered repositories", Phase: "discover"}, completed, total))

	previous := previousRepoRecords(machine.Repos)
	localRecords, transitionedToSynced, err := a.observePhase(cfg, discovered, previous, opts)
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
	if err := a.ensureFromWinners(cfg, &machine, machines, repoMetas, selectedCatalogMap, transitionedToSynced, opts); err != nil {
		return 2, err
	}
	a.logf("sync: winner reconciliation completed")
	for _, repository := range planned {
		opts.progress.finish(repository, findRepositoryRecord(machine.Repos, repository))
	}
	completed = opts.progress.completed

	if err := persistMachineRecords(a.Paths, &machine, previous, a.Now); err != nil {
		return 2, err
	}
	a.logf("sync: published post-reconciliation observations")
	blocked := anyUnsyncableInScope(machine.Repos, selectedCatalogMap, opts.Repository)

	if blocked {
		a.logf("sync: completed with blocked repos")
		return 1, nil
	}
	a.logf("sync: completed successfully")
	return 0, nil
}

func plannedSyncRepositories(discovered []discoveredRepo, machines []domain.MachineFile, metas []domain.RepoMetadataFile, selected map[string]domain.Catalog, repository string) []string {
	keys := map[string]bool{}
	for _, repo := range discovered {
		keys[repo.RepoKey] = true
	}
	moveIndex, _ := buildRepoMoveIndex(metas)
	for _, meta := range metas {
		if repository != "" && meta.RepoKey != repository {
			continue
		}
		if strings.TrimSpace(meta.RepoKey) == "" || strings.TrimSpace(meta.OriginURL) == "" {
			continue
		}
		if _, historical := moveIndex[strings.TrimSpace(meta.RepoKey)]; historical {
			continue
		}
		if _, ok := selectWinnerForRepo(machines, meta.RepoKey); !ok {
			continue
		}
		catalog, _, _, err := domain.ParseRepoKey(meta.RepoKey)
		if err == nil {
			if _, ok := selected[catalog]; ok {
				keys[meta.RepoKey] = true
			}
		}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func findRepositoryRecord(repos []domain.MachineRepoRecord, repository string) *domain.MachineRepoRecord {
	for i := range repos {
		if repos[i].RepoKey == repository {
			return &repos[i]
		}
	}
	return nil
}

func filterDiscoveredRepositories(repos []discoveredRepo, repository string) []discoveredRepo {
	filtered := make([]discoveredRepo, 0, 1)
	for _, repo := range repos {
		if repo.RepoKey == repository {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}

func repositoryFailureDetail(repos []domain.MachineRepoRecord, repository string) string {
	for _, repo := range repos {
		if repo.RepoKey != repository {
			continue
		}
		if len(repo.Reasons) == 0 {
			return "repository remains blocked"
		}
		parts := make([]string, len(repo.Reasons))
		for i, reason := range repo.Reasons {
			parts[i] = string(reason)
		}
		return "repository remains blocked: " + strings.Join(parts, ", ")
	}
	return "repository result unavailable"
}

func anyUnsyncableInScope(repos []domain.MachineRepoRecord, catalogs map[string]domain.Catalog, repository string) bool {
	for _, repo := range repos {
		if repository != "" && repo.RepoKey != repository {
			continue
		}
		if _, ok := catalogs[repo.Catalog]; ok && repo.State == domain.RepoStateBlocked {
			return true
		}
	}
	return false
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
	machines, warnings, err := state.LoadAllMachineFilesWithWarnings(paths)
	if err != nil {
		return nil, nil, err
	}
	for _, warning := range warnings {
		log.Printf("bb: warning: %s", warning)
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
		if rec.State == domain.RepoStateBlocked {
			return true
		}
	}
	return false
}
