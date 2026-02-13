package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestLoggingVerboseAndQuiet(t *testing.T) {
	_, m, _ := setupSingleMachine(t)
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	outVerbose, err := m.RunBB(now, "scan")
	if err != nil {
		t.Fatalf("scan failed: %v\n%s", err, outVerbose)
	}
	if !strings.Contains(outVerbose, "bb: scan:") {
		t.Fatalf("expected verbose log output by default, got: %s", outVerbose)
	}

	outQuiet, err := m.RunBB(now.Add(time.Minute), "scan", "--quiet")
	if err != nil {
		t.Fatalf("scan --quiet failed: %v\n%s", err, outQuiet)
	}
	if strings.Contains(outQuiet, "bb:") {
		t.Fatalf("expected quiet mode to suppress logs, got: %s", outQuiet)
	}
}
