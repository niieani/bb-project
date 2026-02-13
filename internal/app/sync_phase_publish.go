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
