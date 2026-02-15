package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunDoctorReportsNotifyDeliveryWarning(t *testing.T) {
	home := t.TempDir()
	paths := state.NewPaths(home)
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	t.Setenv("BB_MACHINE_ID", "machine-a")

	cfg := state.DefaultConfig()
	cfg.Sync.ScanFreshnessSeconds = 300
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	machine := state.BootstrapMachine("machine-a", "host-a", now)
	machine.LastScanAt = now
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	if err := state.SaveNotifyCache(paths, domain.NotifyCacheFile{
		Version:  1,
		LastSent: map[string]domain.NotifyCacheEntry{},
		DeliveryFailures: map[string]domain.NotifyDeliveryFailure{
			"stdout|repo_key:software/api": {
				Backend:  notifyBackendStdout,
				RepoKey:  "software/api",
				Error:    "mock failure",
				FailedAt: now,
			},
		},
	}); err != nil {
		t.Fatalf("save notify cache: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	a := New(paths, &stdout, &stderr)
	a.Now = func() time.Time { return now }
	a.Hostname = func() (string, error) { return "host-a", nil }

	code, err := a.RunDoctor(nil)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	output := stdout.String()
	if !strings.Contains(output, "warning: notification delivery failed") {
		t.Fatalf("expected warning output, got:\n%s", output)
	}
	if !strings.Contains(output, "backend=stdout") {
		t.Fatalf("expected backend in warning output, got:\n%s", output)
	}
}
