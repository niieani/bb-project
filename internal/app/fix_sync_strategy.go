package app

import (
	"fmt"
	"strings"
)

type FixSyncStrategy string

const (
	FixSyncStrategyRebase FixSyncStrategy = "rebase"
	FixSyncStrategyMerge  FixSyncStrategy = "merge"
)

func ParseFixSyncStrategy(raw string) (FixSyncStrategy, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return FixSyncStrategyRebase, nil
	}
	switch FixSyncStrategy(normalized) {
	case FixSyncStrategyRebase, FixSyncStrategyMerge:
		return FixSyncStrategy(normalized), nil
	default:
		return "", fmt.Errorf("unsupported sync strategy %q (expected rebase or merge)", raw)
	}
}

func normalizeFixSyncStrategy(strategy FixSyncStrategy) FixSyncStrategy {
	parsed, err := ParseFixSyncStrategy(string(strategy))
	if err != nil {
		return FixSyncStrategyRebase
	}
	return parsed
}
