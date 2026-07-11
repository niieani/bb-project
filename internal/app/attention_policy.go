package app

import (
	"sort"
	"time"

	"bb-project/internal/domain"
)

func isAttentionEligible(repo domain.MachineRepoRecord, now time.Time, cfg domain.AttentionConfig) bool {
	age := now.Sub(repo.LastActivityAt)
	unknown := repo.LastActivityAt.IsZero()
	quiet := time.Duration(cfg.QuietHours) * time.Hour
	stale := time.Duration(cfg.WIPStaleHours) * time.Hour
	return repo.State == domain.RepoStateBlocked && (unknown || age >= quiet) ||
		repo.State == domain.RepoStateWip && (unknown || age >= stale)
}

func dominantAttentionReason(reasons []domain.UnsyncableReason) (domain.UnsyncableReason, bool) {
	if len(reasons) == 0 {
		return "", false
	}
	sorted := append([]domain.UnsyncableReason(nil), reasons...)
	rank := func(reason domain.UnsyncableReason) int {
		switch domain.ReasonTier(reason) {
		case domain.RepoStateBlocked:
			return 3
		case domain.RepoStatePending:
			return 2
		case domain.RepoStateWip:
			return 1
		default:
			return 0
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		left, right := rank(sorted[i]), rank(sorted[j])
		if left != right {
			return left > right
		}
		return sorted[i] < sorted[j]
	})
	return sorted[0], true
}
