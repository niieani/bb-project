package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

type StatusContract struct {
	MachineID      string          `json:"machine_id"`
	Repos          []StatusRepo    `json:"repos"`
	Summary        StatusSummary   `json:"summary"`
	LastSync       *StatusLastSync `json:"last_sync"`
	Attention      FleetAttention  `json:"attention"`
	SourceWarnings []string        `json:"source_warnings"`
}

type StatusLastSync struct {
	At      time.Time `json:"at"`
	Machine string    `json:"machine"`
	Event   string    `json:"event"`
	Detail  string    `json:"detail"`
}

type StatusRepo struct {
	RepoKey        string                    `json:"repo_key"`
	Name           string                    `json:"name"`
	Catalog        string                    `json:"catalog"`
	Path           string                    `json:"path"`
	State          domain.RepoSyncState      `json:"state"`
	Reasons        []domain.UnsyncableReason `json:"reasons"`
	Warnings       []domain.UnsyncableReason `json:"warnings"`
	LastActivityAt time.Time                 `json:"last_activity_at"`
	Actions        []ProjectAction           `json:"actions"`
}

type ProjectAction struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
}

type StatusSummary struct {
	Total    int `json:"total"`
	Synced   int `json:"synced"`
	Pending  int `json:"pending"`
	WIP      int `json:"wip"`
	Blocked  int `json:"blocked"`
	Warnings int `json:"warnings"`
}

type FleetAttention struct {
	ThrottleMinutes int             `json:"throttle_minutes"`
	Items           []AttentionItem `json:"items"`
	EligibleCount   int             `json:"eligible_count"`
	Fingerprint     string          `json:"fingerprint"`
}

type AttentionItem struct {
	MachineID      string                    `json:"machine_id"`
	RepoKey        string                    `json:"repo_key"`
	Name           string                    `json:"name"`
	State          domain.RepoSyncState      `json:"state"`
	DominantReason domain.UnsyncableReason   `json:"dominant_reason"`
	Reasons        []domain.UnsyncableReason `json:"reasons"`
	LastActivityAt time.Time                 `json:"last_activity_at"`
	Eligible       bool                      `json:"eligible"`
}

func buildStatusSummary(repos []domain.MachineRepoRecord) StatusSummary {
	var summary StatusSummary
	for _, repo := range repos {
		summary.Total++
		summary.Warnings += len(repo.Warnings)
		switch repo.State {
		case domain.RepoStateSynced:
			summary.Synced++
		case domain.RepoStatePending:
			summary.Pending++
		case domain.RepoStateWip:
			summary.WIP++
		case domain.RepoStateBlocked:
			summary.Blocked++
		}
	}
	return summary
}

func buildStatusRepos(repos []domain.MachineRepoRecord, catalogs []domain.Catalog) []StatusRepo {
	catalogByName := make(map[string]domain.Catalog, len(catalogs))
	for _, catalog := range catalogs {
		catalogByName[catalog.Name] = catalog
	}
	out := make([]StatusRepo, 0, len(repos))
	for _, repo := range repos {
		actions := []ProjectAction{}
		if catalog, ok := catalogByName[repo.Catalog]; ok && syncActionable(repo, catalog) {
			actions = append(actions, ProjectAction{Kind: "sync", ID: "sync", Label: "Sync"})
		}
		out = append(out, StatusRepo{
			RepoKey: repo.RepoKey, Name: repo.Name, Catalog: repo.Catalog, Path: repo.Path,
			State:          repo.State,
			Reasons:        append([]domain.UnsyncableReason{}, repo.Reasons...),
			Warnings:       append([]domain.UnsyncableReason{}, repo.Warnings...),
			LastActivityAt: repo.LastActivityAt,
			Actions:        actions,
		})
	}
	return out
}

func syncActionable(repo domain.MachineRepoRecord, catalog domain.Catalog) bool {
	if repo.State == domain.RepoStateSynced && repo.Behind > 0 && repo.Ahead == 0 &&
		!repo.HasDirtyTracked && !repo.HasUntracked && !repo.Diverged {
		return true
	}
	return repo.State == domain.RepoStatePending && len(repo.Reasons) == 1 &&
		repo.Reasons[0] == domain.ReasonCloneRequired && catalog.AllowsAutoCloneOnSync()
}

func latestSyncRun(paths state.Paths, machineID string) (*StatusLastSync, error) {
	events, err := state.LoadJournalFile(paths.JournalPath(machineID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Machine == machineID && event.Event == "sync_run" {
			return &StatusLastSync{At: event.At, Machine: event.Machine, Event: event.Event, Detail: event.Detail}, nil
		}
	}
	return nil, nil
}

func buildFleetAttention(records []domain.MachineRepoRecordWithMachine, now time.Time, cfg domain.AttentionConfig) FleetAttention {
	items := make([]AttentionItem, 0, len(records))
	for _, record := range records {
		repo := record.Record
		if repo.State == domain.RepoStateSynced {
			continue
		}
		eligible := isAttentionEligible(repo, now, cfg)
		reason, _ := dominantAttentionReason(repo.Reasons)
		items = append(items, AttentionItem{
			MachineID: record.MachineID, RepoKey: repo.RepoKey, Name: repo.Name,
			State: repo.State, DominantReason: reason, Reasons: append([]domain.UnsyncableReason{}, repo.Reasons...), LastActivityAt: repo.LastActivityAt, Eligible: eligible,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i].MachineID + "\x00" + items[i].RepoKey
		right := items[j].MachineID + "\x00" + items[j].RepoKey
		return left < right
	})
	lines := make([]string, 0, len(items))
	eligibleCount := 0
	for _, item := range items {
		if !item.Eligible {
			continue
		}
		eligibleCount++
		reasons := make([]string, len(item.Reasons))
		for i, reason := range item.Reasons {
			reasons[i] = string(reason)
		}
		sort.Strings(reasons)
		lines = append(lines, fmt.Sprintf("%s:%s:%s:%s", item.MachineID, item.RepoKey, item.State, strings.Join(reasons, "+")))
	}
	fingerprint := ""
	if len(lines) > 0 {
		digest := sha256.Sum256([]byte(strings.Join(lines, "\n")))
		fingerprint = hex.EncodeToString(digest[:])
	}
	return FleetAttention{ThrottleMinutes: cfg.ThrottleMinutes, Items: items, EligibleCount: eligibleCount, Fingerprint: fingerprint}
}
