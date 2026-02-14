package domain

import (
	"fmt"
	"strings"
)

func ParsePushAccess(raw string) (PushAccess, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", string(PushAccessUnknown):
		return PushAccessUnknown, nil
	case string(PushAccessReadWrite):
		return PushAccessReadWrite, nil
	case string(PushAccessReadOnly):
		return PushAccessReadOnly, nil
	default:
		return PushAccessUnknown, fmt.Errorf("invalid push access %q", raw)
	}
}

func NormalizePushAccess(access PushAccess) PushAccess {
	switch access {
	case PushAccessReadWrite, PushAccessReadOnly:
		return access
	default:
		return PushAccessUnknown
	}
}
