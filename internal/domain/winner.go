package domain

import "time"

func SelectWinner(records []MachineRepoRecordWithMachine) (MachineRepoRecordWithMachine, bool) {
	var winner MachineRepoRecordWithMachine
	var found bool

	for _, rec := range records {
		if !rec.Record.Syncable {
			continue
		}
		if !found {
			winner = rec
			found = true
			continue
		}

		if rec.Record.ObservedAt.After(winner.Record.ObservedAt) {
			winner = rec
			continue
		}
		if rec.Record.ObservedAt.Equal(winner.Record.ObservedAt) && rec.MachineID < winner.MachineID {
			winner = rec
		}
	}

	return winner, found
}

func newerObservedAt(a, b time.Time) bool {
	return a.After(b)
}
