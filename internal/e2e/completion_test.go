package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestCompletionCommandBash(t *testing.T) {
	t.Parallel()

	_, m, _ := setupSingleMachine(t)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	out, err := m.RunBB(now, "completion", "bash")
	if err != nil {
		t.Fatalf("completion command failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "__start_bb") {
		t.Fatalf("expected bash completion function in output, got:\n%s", out)
	}
}
