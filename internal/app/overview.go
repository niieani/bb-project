package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

type OverviewOptions struct {
	IncludeCatalogs []string
	All             bool
	JSON            bool
}
type OverviewMatrix struct {
	Machines         []OverviewMachine `json:"machines"`
	Repos            []OverviewRepo    `json:"repos"`
	SyncedEverywhere int               `json:"synced_everywhere"`
	Warnings         []string          `json:"warnings"`
}
type OverviewMachine struct {
	ID        string     `json:"id"`
	Here      bool       `json:"here"`
	Published bool       `json:"published"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Stale     bool       `json:"stale"`
}
type OverviewRepo struct {
	RepoKey          string         `json:"repo_key"`
	Cells            []OverviewCell `json:"cells"`
	SyncedEverywhere bool           `json:"synced_everywhere"`
}
type OverviewCell struct {
	MachineID      string                    `json:"machine_id"`
	Present        bool                      `json:"present"`
	State          domain.RepoSyncState      `json:"state,omitempty"`
	Reasons        []domain.UnsyncableReason `json:"reasons"`
	Warnings       []domain.UnsyncableReason `json:"warnings"`
	LastActivityAt time.Time                 `json:"last_activity_at"`
}

func (a *App) RunOverview(opts OverviewOptions) (int, error) {
	cfg := state.DefaultConfig()
	if _, statErr := os.Stat(a.Paths.ConfigPath()); statErr == nil {
		var err error
		cfg, err = state.LoadConfig(a.Paths)
		if err != nil {
			return 2, err
		}
	} else if !os.IsNotExist(statErr) {
		return 2, statErr
	}
	if cfg.Overview.MachineStaleDays < 1 {
		return 2, fmt.Errorf("overview.machine_stale_days must be >= 1")
	}
	machines, loadWarnings, err := state.LoadAllMachineFilesWithWarnings(a.Paths)
	if err != nil {
		return 2, err
	}
	local := strings.TrimSpace(a.Getenv("BB_MACHINE_ID"))
	if local == "" {
		if b, e := os.ReadFile(a.Paths.MachineIDPath()); e == nil {
			local = strings.TrimSpace(string(b))
		}
	}
	foundLocal := false
	for _, m := range machines {
		if m.MachineID == local {
			foundLocal = true
			break
		}
	}
	if local != "" && !foundLocal {
		machines = append(machines, domain.MachineFile{Version: domain.Version, MachineID: local})
	}
	sort.Slice(machines, func(i, j int) bool {
		iLocal, jLocal := machines[i].MachineID == local, machines[j].MachineID == local
		if iLocal != jLocal {
			return iLocal
		}
		return machines[i].MachineID < machines[j].MachineID
	})
	allowed := map[string]bool{}
	for _, v := range opts.IncludeCatalogs {
		allowed[v] = true
	}
	keys := map[string]bool{}
	for _, m := range machines {
		for _, r := range m.Repos {
			cat, _, _, _ := domain.ParseRepoKey(r.RepoKey)
			if len(allowed) == 0 || allowed[cat] {
				keys[r.RepoKey] = true
			}
		}
	}
	metas, metaErr := state.LoadAllRepoMetadata(a.Paths)
	if metaErr != nil {
		return 2, metaErr
	}
	for _, meta := range metas {
		cat, _, _, _ := domain.ParseRepoKey(meta.RepoKey)
		if len(allowed) == 0 || allowed[cat] {
			keys[meta.RepoKey] = true
		}
	}
	matrix := OverviewMatrix{
		Machines: []OverviewMachine{},
		Repos:    []OverviewRepo{},
		Warnings: append([]string{}, loadWarnings...),
	}
	now := a.Now()
	staleAfter := time.Duration(cfg.Overview.MachineStaleDays) * 24 * time.Hour
	for _, m := range machines {
		published := !m.UpdatedAt.IsZero()
		var updated *time.Time
		if published {
			value := m.UpdatedAt
			updated = &value
		}
		matrix.Machines = append(matrix.Machines, OverviewMachine{ID: m.MachineID, Here: m.MachineID == local, Published: published, UpdatedAt: updated, Stale: published && now.Sub(m.UpdatedAt) >= staleAfter})
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		row := OverviewRepo{RepoKey: key, Cells: []OverviewCell{}, SyncedEverywhere: true}
		for _, m := range machines {
			cell := OverviewCell{MachineID: m.MachineID, Reasons: []domain.UnsyncableReason{}, Warnings: []domain.UnsyncableReason{}}
			for _, r := range m.Repos {
				if r.RepoKey == key {
					cell.Present = true
					cell.State = r.State
					cell.Reasons = append([]domain.UnsyncableReason{}, r.Reasons...)
					cell.Warnings = append([]domain.UnsyncableReason{}, r.Warnings...)
					cell.LastActivityAt = r.LastActivityAt
					break
				}
			}
			if !cell.Present || cell.State != domain.RepoStateSynced {
				row.SyncedEverywhere = false
			}
			row.Cells = append(row.Cells, cell)
		}
		if row.SyncedEverywhere {
			matrix.SyncedEverywhere++
		}
		matrix.Repos = append(matrix.Repos, row)
	}
	if opts.JSON {
		enc := json.NewEncoder(a.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(matrix); err != nil {
			return 2, err
		}
		return 0, nil
	}
	for _, warning := range matrix.Warnings {
		fmt.Fprintf(a.Stdout, "warning: %s\n", warning)
	}
	for _, m := range matrix.Machines {
		if m.Stale {
			fmt.Fprintf(a.Stdout, "%s last published %s ago — its data may be stale.\n", m.ID, humanAge(now.Sub(*m.UpdatedAt)))
		}
	}
	for _, r := range matrix.Repos {
		if r.SyncedEverywhere && !opts.All {
			continue
		}
		fmt.Fprint(a.Stdout, r.RepoKey)
		for i, c := range r.Cells {
			label := c.MachineID
			if i == 0 && matrix.Machines[i].Here {
				label = "here"
			}
			fmt.Fprintf(a.Stdout, "   %s: %s", label, overviewCellText(c, now))
		}
		fmt.Fprintln(a.Stdout)
	}
	if !opts.All {
		fmt.Fprintf(a.Stdout, "synced everywhere: %d repos (--all to list)\n", matrix.SyncedEverywhere)
	}
	return 0, nil
}
func overviewCellText(c OverviewCell, now time.Time) string {
	if !c.Present {
		return "— (not cloned)"
	}
	s := string(c.State)
	if c.State == domain.RepoStateWip || c.State == domain.RepoStateBlocked {
		reason := "unknown reason"
		if dominant, ok := dominantAttentionReason(c.Reasons); ok {
			reason = string(dominant)
		}
		activity := "activity unknown"
		if !c.LastActivityAt.IsZero() {
			activity = humanAge(now.Sub(c.LastActivityAt)) + " ago"
		}
		s += " (" + reason + " · " + activity + ")"
	} else if len(c.Reasons) > 0 {
		reason := string(c.Reasons[0])
		if dominant, ok := dominantAttentionReason(c.Reasons); ok {
			reason = string(dominant)
		}
		activity := "activity unknown"
		if !c.LastActivityAt.IsZero() {
			activity = humanAge(now.Sub(c.LastActivityAt)) + " ago"
		}
		s += " (" + reason + " · " + activity + ")"
	}
	return s
}
func humanAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
