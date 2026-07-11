package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

func ComputeStateHash(record MachineRepoRecord) string {
	payload := struct {
		ExpectedRepoKey     string             `json:"expected_repo_key,omitempty"`
		ExpectedCatalog     string             `json:"expected_catalog,omitempty"`
		ExpectedPath        string             `json:"expected_path,omitempty"`
		Branch              string             `json:"branch"`
		HeadSHA             string             `json:"head_sha"`
		Upstream            string             `json:"upstream"`
		RemoteHeadSHA       string             `json:"remote_head_sha"`
		Ahead               int                `json:"ahead"`
		Behind              int                `json:"behind"`
		Diverged            bool               `json:"diverged"`
		HasDirtyTracked     bool               `json:"has_dirty_tracked"`
		HasUntracked        bool               `json:"has_untracked"`
		OperationInProgress Operation          `json:"operation_in_progress"`
		State               RepoSyncState      `json:"state"`
		Reasons             []UnsyncableReason `json:"reasons"`
		Warnings            []UnsyncableReason `json:"warnings"`
	}{
		ExpectedRepoKey:     record.ExpectedRepoKey,
		ExpectedCatalog:     record.ExpectedCatalog,
		ExpectedPath:        record.ExpectedPath,
		Branch:              record.Branch,
		HeadSHA:             record.HeadSHA,
		Upstream:            record.Upstream,
		RemoteHeadSHA:       record.RemoteHeadSHA,
		Ahead:               record.Ahead,
		Behind:              record.Behind,
		Diverged:            record.Diverged,
		HasDirtyTracked:     record.HasDirtyTracked,
		HasUntracked:        record.HasUntracked,
		OperationInProgress: record.OperationInProgress,
		State:               record.State,
		Reasons:             record.Reasons,
		Warnings:            record.Warnings,
	}

	buf, _ := json.Marshal(payload)
	sum := sha256.Sum256(buf)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func UpdateObservedAt(previous MachineRepoRecord, current MachineRepoRecord, now time.Time) MachineRepoRecord {
	if previous.StateHash == current.StateHash && !previous.ObservedAt.IsZero() {
		current.ObservedAt = previous.ObservedAt
		return current
	}
	current.ObservedAt = now
	return current
}
