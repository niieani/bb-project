package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestConfigCommandRequiresInteractiveTTY(t *testing.T) {
	_, m, _ := setupSingleMachine(t)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	out, err := m.RunBB(now, "config")
	if err == nil {
		t.Fatalf("expected bb config to fail in non-interactive test harness, output=%s", out)
	}
	if !strings.Contains(out, "requires an interactive terminal") {
		t.Fatalf("expected non-interactive message, got: %s", out)
	}
}
