package domain

import (
	"fmt"
	"strings"
)

func ParseAutoPushMode(raw string) (AutoPushMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(AutoPushModeDisabled):
		return AutoPushModeDisabled, nil
	case string(AutoPushModeEnabled):
		return AutoPushModeEnabled, nil
	case string(AutoPushModeIncludeDefaultBranch):
		return AutoPushModeIncludeDefaultBranch, nil
	default:
		return AutoPushModeDisabled, fmt.Errorf("invalid auto-push mode %q", raw)
	}
}

func NormalizeAutoPushMode(mode AutoPushMode) AutoPushMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case string(AutoPushModeDisabled):
		return AutoPushModeDisabled
	case string(AutoPushModeEnabled):
		return AutoPushModeEnabled
	case string(AutoPushModeIncludeDefaultBranch):
		return AutoPushModeIncludeDefaultBranch
	default:
		return AutoPushModeDisabled
	}
}

func AutoPushModeFromEnabled(enabled bool) AutoPushMode {
	if enabled {
		return AutoPushModeEnabled
	}
	return AutoPushModeDisabled
}
