package app

import (
	"sort"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

type timeNowFn func() time.Time

func persistMachineRecords(paths state.Paths, machine *domain.MachineFile, previous map[string]domain.MachineRepoRecord, now timeNowFn) error {
	for i := range machine.Repos {
		key := repoRecordIdentityKey(machine.Repos[i])
		old := previous[key]
		machine.Repos[i] = domain.UpdateObservedAt(old, machine.Repos[i], now())
	}
	sort.Slice(machine.Repos, func(i, j int) bool {
		if repoRecordSortKey(machine.Repos[i]) == repoRecordSortKey(machine.Repos[j]) {
			return machine.Repos[i].Path < machine.Repos[j].Path
		}
		return repoRecordSortKey(machine.Repos[i]) < repoRecordSortKey(machine.Repos[j])
	})
	machine.UpdatedAt = now()
	return state.SaveMachine(paths, *machine)
}
