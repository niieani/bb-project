package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func (a *App) notifyUnsyncable(cfg domain.ConfigFile, repos []domain.MachineRepoRecord, backendOverride string) error {
	if !cfg.Notify.Enabled {
		return nil
	}
	now := a.Now()
	attention := notificationAttentionSet(repos, now, cfg.Notify)
	cache, err := state.LoadNotifyCache(a.Paths)
	if err != nil {
		return err
	}
	if len(attention) == 0 {
		if cache.LastSent.Fingerprint != "" {
			cache.LastSent.Fingerprint = ""
			return state.SaveNotifyCache(a.Paths, cache)
		}
		return nil
	}
	fingerprint := attentionFingerprint(attention)
	if cache.LastSent.Fingerprint == fingerprint {
		return nil
	}
	throttle := time.Duration(cfg.Notify.ThrottleMinutes) * time.Minute
	if throttle > 0 && !cache.LastSent.SentAt.IsZero() && now.Sub(cache.LastSent.SentAt) >= 0 && now.Sub(cache.LastSent.SentAt) < throttle {
		return nil
	}
	backend, err := a.resolveNotifyBackend(backendOverride)
	if err != nil {
		return err
	}
	factory := a.NewNotifySender
	if factory == nil {
		factory = func(name string) (notifySender, error) { return newNotifySender(name, a.Stdout, a.RunCommand) }
	}
	sender, err := factory(backend)
	if err != nil {
		return err
	}
	msg := notifyMessage{Fingerprint: fingerprint, Body: attentionBody(attention)}
	if err := sender.Send(msg); err != nil {
		cache.DeliveryFailures[backend] = domain.NotifyDeliveryFailure{Backend: backend, Fingerprint: fingerprint, Error: err.Error(), FailedAt: now}
		return state.SaveNotifyCache(a.Paths, cache)
	}
	for _, repo := range attention {
		a.appendJournal("notified", repo.RepoKey, "backend="+backend)
	}
	cache.LastSent = domain.NotifyCacheEntry{Fingerprint: fingerprint, SentAt: now}
	delete(cache.DeliveryFailures, backend)
	return state.SaveNotifyCache(a.Paths, cache)
}

func notificationAttentionSet(repos []domain.MachineRepoRecord, now time.Time, cfg domain.NotifyConfig) []domain.MachineRepoRecord {
	out := make([]domain.MachineRepoRecord, 0)
	for _, r := range repos {
		if isNotificationEligible(r, now, cfg) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return notifyIdentity(out[i]) < notifyIdentity(out[j]) })
	return out
}

func isNotificationEligible(repo domain.MachineRepoRecord, now time.Time, cfg domain.NotifyConfig) bool {
	age := now.Sub(repo.LastActivityAt)
	unknown := repo.LastActivityAt.IsZero()
	quiet := time.Duration(cfg.QuietHours) * time.Hour
	stale := time.Duration(cfg.WIPStaleHours) * time.Hour
	return repo.State == domain.RepoStateBlocked && (unknown || age >= quiet) ||
		repo.State == domain.RepoStateWip && (unknown || age >= stale)
}

func notifyIdentity(r domain.MachineRepoRecord) string {
	if r.RepoKey != "" {
		return r.RepoKey
	}
	if r.Name != "" {
		return r.Name
	}
	return r.Path
}
func attentionFingerprint(repos []domain.MachineRepoRecord) string {
	lines := make([]string, 0, len(repos))
	for _, r := range repos {
		reasons := make([]string, len(r.Reasons))
		for i, v := range r.Reasons {
			reasons[i] = string(v)
		}
		sort.Strings(reasons)
		lines = append(lines, fmt.Sprintf("%s:%s:%s", notifyIdentity(r), r.State, strings.Join(reasons, "+")))
	}
	return strings.Join(lines, "\n")
}
func attentionBody(repos []domain.MachineRepoRecord) string {
	lines := []string{fmt.Sprintf("%d repo(s) need attention:", len(repos))}
	limit := min(4, len(repos))
	for _, r := range repos[:limit] {
		reason := "unknown"
		if dominant, ok := dominantAttentionReason(r.Reasons); ok {
			reason = string(dominant)
		}
		lines = append(lines, fmt.Sprintf("%s: %s", r.Name, reason))
	}
	if len(repos) > limit {
		lines = append(lines, fmt.Sprintf("+%d more", len(repos)-limit))
	}
	return strings.Join(lines, "\n")
}

func dominantAttentionReason(reasons []domain.UnsyncableReason) (domain.UnsyncableReason, bool) {
	if len(reasons) == 0 {
		return "", false
	}
	sorted := append([]domain.UnsyncableReason(nil), reasons...)
	rank := func(r domain.UnsyncableReason) int {
		switch domain.ReasonTier(r) {
		case domain.RepoStateBlocked:
			return 3
		case domain.RepoStatePending:
			return 2
		case domain.RepoStateWip:
			return 1
		}
		return 0
	}
	sort.Slice(sorted, func(i, j int) bool {
		ri, rj := rank(sorted[i]), rank(sorted[j])
		if ri != rj {
			return ri > rj
		}
		return sorted[i] < sorted[j]
	})
	return sorted[0], true
}
