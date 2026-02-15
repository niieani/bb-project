package app

import (
	"fmt"
	"sort"
	"strings"

	"bb-project/internal/state"
)

func (a *App) reportNotifyDeliveryFailures() (int, error) {
	cache, err := state.LoadNotifyCache(a.Paths)
	if err != nil {
		return 0, err
	}
	if len(cache.DeliveryFailures) == 0 {
		return 0, nil
	}

	keys := make([]string, 0, len(cache.DeliveryFailures))
	for key := range cache.DeliveryFailures {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		failure := cache.DeliveryFailures[key]
		repoLabel := strings.TrimSpace(failure.RepoKey)
		if repoLabel == "" {
			repoLabel = strings.TrimSpace(failure.RepoName)
		}
		if repoLabel == "" {
			repoLabel = strings.TrimSpace(failure.RepoPath)
		}
		if repoLabel == "" {
			repoLabel = key
		}
		fmt.Fprintf(a.Stdout,
			"warning: notification delivery failed backend=%s repo=%s at=%s error=%s\n",
			strings.TrimSpace(failure.Backend),
			repoLabel,
			failure.FailedAt.UTC().Format(timeRFC3339),
			strings.TrimSpace(failure.Error),
		)
	}
	return len(keys), nil
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"
