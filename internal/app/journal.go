package app

import (
	"bb-project/internal/domain"
	"bb-project/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type LogOptions struct {
	Repo    string
	Machine string
	Limit   int
	JSON    bool
}

func (a *App) appendJournal(kind, key, detail string) {
	id := strings.TrimSpace(a.Getenv("BB_MACHINE_ID"))
	if id == "" {
		if b, e := os.ReadFile(a.Paths.MachineIDPath()); e == nil {
			id = strings.TrimSpace(string(b))
		}
	}
	if id == "" {
		a.logf("journal: machine id is required")
		return
	}
	cfg, e := state.LoadConfig(a.Paths)
	if e != nil {
		a.logf("journal: %v", e)
		return
	}
	if e = state.AppendJournal(a.Paths, domain.JournalEvent{At: a.Now(), Machine: id, Event: kind, RepoKey: key, Detail: detail}, cfg.Journal.MaxEntries); e != nil {
		a.logf("journal: append failed: %v", e)
	}
}
func (a *App) RunLog(o LogOptions) (int, error) {
	events, e := state.LoadAllJournals(a.Paths)
	if e != nil {
		return 2, e
	}
	limit := o.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 {
		return 2, fmt.Errorf("limit must be >= 1")
	}
	out := []domain.JournalEvent{}
	resolvedRepo := o.Repo
	if o.Repo != "" {
		machines, err := state.LoadAllMachineFiles(a.Paths)
		if err != nil {
			return 2, err
		}
		records := []domain.MachineRepoRecord{}
		for _, m := range machines {
			records = append(records, m.Repos...)
		}
		if rec, found, err := resolveLocalProjectSelector(records, o.Repo); err != nil {
			return 2, err
		} else if found {
			resolvedRepo = rec.RepoKey
		} else {
			return 2, fmt.Errorf("selector %q could not be resolved", o.Repo)
		}
	}
	for _, v := range events {
		if o.Machine != "" && v.Machine != o.Machine {
			continue
		}
		if resolvedRepo != "" && v.RepoKey != resolvedRepo {
			continue
		}
		out = append(out, v)
		if len(out) == limit {
			break
		}
	}
	if o.JSON {
		enc := json.NewEncoder(a.Stdout)
		enc.SetIndent("", "  ")
		if e := enc.Encode(out); e != nil {
			return 2, e
		}
		return 0, nil
	}
	for _, v := range out {
		fmt.Fprintf(a.Stdout, "%s  %s  %s", v.At.Format("2006-01-02 15:04:05Z07:00"), v.Machine, v.Event)
		if v.RepoKey != "" {
			fmt.Fprintf(a.Stdout, "  %s", v.RepoKey)
		}
		if v.Detail != "" {
			fmt.Fprintf(a.Stdout, "  %s", v.Detail)
		}
		fmt.Fprintln(a.Stdout)
	}
	return 0, nil
}
