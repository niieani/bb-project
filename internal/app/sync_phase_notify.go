package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func (a *App) notifyUnsyncable(cfg domain.ConfigFile, repos []domain.MachineRepoRecord) error {
	if !cfg.Notify.Enabled {
		a.logf("notify: disabled in config")
		return nil
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
		a.logf("notify: emitting for %s (%s)", rec.Name, fingerprint)
		fmt.Fprintf(a.Stdout, "notify %s: %s\n", rec.Name, fingerprint)
		cache.LastSent[cacheKey] = domain.NotifyCacheEntry{Fingerprint: fingerprint, SentAt: now}
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
