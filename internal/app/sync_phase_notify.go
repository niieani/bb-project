package app

import (
	"sort"
	"strings"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func (a *App) notifyUnsyncable(cfg domain.ConfigFile, repos []domain.MachineRepoRecord, backendOverride string) error {
	if !cfg.Notify.Enabled {
		a.logf("notify: disabled in config")
		return nil
	}
	backendName, err := a.resolveNotifyBackend(backendOverride)
	if err != nil {
		return err
	}
	factory := a.NewNotifySender
	if factory == nil {
		factory = func(name string) (notifySender, error) {
			return newNotifySender(name, a.Stdout, a.RunCommand)
		}
	}
	sender, err := factory(backendName)
	if err != nil {
		return err
	}

	cache, err := state.LoadNotifyCache(a.Paths)
	if err != nil {
		return err
	}
	now := a.Now()
	throttleWindow := time.Duration(cfg.Notify.ThrottleMinutes) * time.Minute
	for _, rec := range repos {
		if rec.Syncable {
			continue
		}
		if len(rec.UnsyncableReasons) > 0 && !domain.HasBlockingUnsyncableReason(rec.UnsyncableReasons) {
			continue
		}
		fingerprint := unsyncableFingerprint(rec.UnsyncableReasons)
		cacheKey := notifyCacheKey(rec)
		entry, ok := cache.LastSent[cacheKey]
		if ok && entry.Fingerprint == fingerprint && cfg.Notify.Dedupe {
			a.logf("notify: deduped %s (%s)", rec.Name, fingerprint)
			continue
		}
		if ok && throttleWindow > 0 {
			elapsed := now.Sub(entry.SentAt)
			if elapsed >= 0 && elapsed < throttleWindow {
				a.logf("notify: throttled %s (%s), remaining=%s", rec.Name, fingerprint, throttleWindow-elapsed)
				continue
			}
		}
		msg := notifyMessage{
			Repo:        rec,
			Fingerprint: fingerprint,
		}
		if err := sender.Send(msg); err != nil {
			failureKey := notifyFailureCacheKey(backendName, rec)
			a.logf("notify: backend %s failed for %s: %v", backendName, rec.Name, err)
			cache.DeliveryFailures[failureKey] = domain.NotifyDeliveryFailure{
				Backend:     backendName,
				RepoKey:     strings.TrimSpace(rec.RepoKey),
				RepoName:    strings.TrimSpace(rec.Name),
				RepoPath:    strings.TrimSpace(rec.Path),
				Fingerprint: fingerprint,
				Error:       err.Error(),
				FailedAt:    now,
			}
			continue
		}
		a.logf("notify: backend %s emitted for %s (%s)", backendName, rec.Name, fingerprint)
		cache.LastSent[cacheKey] = domain.NotifyCacheEntry{Fingerprint: fingerprint, SentAt: now}
		delete(cache.DeliveryFailures, notifyFailureCacheKey(backendName, rec))
	}
	return state.SaveNotifyCache(a.Paths, cache)
}

func notifyCacheKey(rec domain.MachineRepoRecord) string {
	if strings.TrimSpace(rec.RepoKey) != "" {
		return "repo_key:" + rec.RepoKey
	}
	if strings.TrimSpace(rec.Path) != "" {
		return "path:" + rec.Path
	}
	if strings.TrimSpace(rec.Name) != "" {
		return "name:" + rec.Name
	}
	return "unknown"
}

func unsyncableFingerprint(reasons []domain.UnsyncableReason) string {
	if len(reasons) == 0 {
		return ""
	}
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		parts = append(parts, string(r))
	}
	sort.Strings(parts)
	return strings.Join(parts, "+")
}
