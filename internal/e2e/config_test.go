package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigCases(t *testing.T) {
	now := time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC)

	t.Run("TC-CONFIG-001", func(t *testing.T) {
		_, m, _ := setupSingleMachine(t)
		bad := strings.TrimSpace(`
version: 1
state_transport:
  mode: unsupported
github:
  owner: you
  default_visibility: private
  remote_protocol: ssh
sync:
  auto_discover: true
  include_untracked_as_dirty: true
  default_auto_push_private: true
  default_auto_push_public: false
  fetch_prune: true
  pull_ff_only: true
notify:
  enabled: true
  dedupe: true
  throttle_minutes: 60
`) + "\n"
		m.MustWriteFile(m.ConfigPath(), bad)

		out, err := m.RunBB(now, "sync")
		if err == nil {
			t.Fatalf("expected unsupported mode failure, output=%s", out)
		}
	})

	t.Run("TC-CONFIG-002", func(t *testing.T) {
		_, m, _ := setupSingleMachine(t)
		machinePath := m.ConfigMachinePath()
		if err := os.Remove(machinePath); err != nil {
			t.Fatalf("remove machine file: %v", err)
		}

		if out, err := m.RunBB(now, "scan"); err != nil {
			t.Fatalf("scan failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(machinePath); err != nil {
			t.Fatalf("expected bootstrapped machine file: %v", err)
		}
		idPath := filepath.Join(m.LocalStateRoot(), "machine-id")
		if _, err := os.Stat(idPath); err != nil {
			t.Fatalf("expected persisted machine-id: %v", err)
		}
	})

	t.Run("TC-CONFIG-003", func(t *testing.T) {
		_, m, _ := setupSingleMachine(t)
		m.MustWriteFile(m.ConfigPath(), "version: [\n")
		out, err := m.RunBB(now, "sync")
		if err == nil {
			t.Fatalf("expected parse failure, output=%s", out)
		}
	})
}
