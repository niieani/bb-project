package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStatusJSONLastSyncComesFromLatestRealSyncRun(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	_, machine, root := setupSingleMachine(t)
	createRepoWithOrigin(t, machine, root, "api", now)
	if out, err := machine.RunBB(now.Add(time.Minute), "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	out, err := machine.RunBB(now.Add(2*time.Minute), "status", "--json")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	var payload struct {
		LastSync *struct {
			At      time.Time `json:"at"`
			Machine string    `json:"machine"`
			Event   string    `json:"event"`
		} `json:"last_sync"`
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&payload); err != nil {
		t.Fatalf("decode status: %v\n%s", err, out)
	}
	if payload.LastSync == nil || payload.LastSync.Event != "sync_run" || payload.LastSync.Machine != machine.ID || !payload.LastSync.At.Equal(now.Add(time.Minute)) {
		t.Fatalf("last_sync = %#v", payload.LastSync)
	}
}
