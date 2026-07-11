package app

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

type fakeNotifySender struct {
	name    string
	sendErr error
	sent    []notifyMessage
}

func (f *fakeNotifySender) Send(msg notifyMessage) error {
	f.sent = append(f.sent, msg)
	return f.sendErr
}

func TestNotificationAttentionPolicyAndOverflow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	cfg := state.DefaultConfig().Notify
	repos := []domain.MachineRepoRecord{
		{Name: "active-blocked", RepoKey: "a", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}, LastActivityAt: now.Add(-time.Hour)},
		{Name: "old-blocked", RepoKey: "b", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}, LastActivityAt: now.Add(-3 * time.Hour)},
		{Name: "fresh-wip", RepoKey: "c", State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-23 * time.Hour)},
		{Name: "stale-wip", RepoKey: "d", State: domain.RepoStateWip, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked}, LastActivityAt: now.Add(-24 * time.Hour)},
		{Name: "pending", RepoKey: "e", State: domain.RepoStatePending, Reasons: []domain.UnsyncableReason{domain.ReasonCloneRequired}},
	}
	got := notificationAttentionSet(repos, now, cfg)
	if len(got) != 2 || got[0].Name != "old-blocked" || got[1].Name != "stale-wip" {
		t.Fatalf("attention = %+v", got)
	}
	many := make([]domain.MachineRepoRecord, 5)
	for i := range many {
		many[i] = domain.MachineRepoRecord{Name: fmt.Sprintf("repo-%d", i), RepoKey: fmt.Sprint(i), State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}}
	}
	body := attentionBody(many)
	if !strings.Contains(body, "5 repo(s) need attention:") || !strings.Contains(body, "+1 more") || strings.Contains(body, "repo-4:") {
		t.Fatalf("body = %q", body)
	}
	mixed := domain.MachineRepoRecord{Name: "mixed", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked, domain.ReasonDiverged}}
	if got := attentionBody([]domain.MachineRepoRecord{mixed}); !strings.Contains(got, "mixed: diverged") {
		t.Fatalf("dominant reason body = %q", got)
	}
}

func TestNotificationClockBoundaries(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cfg := state.DefaultConfig().Notify
	tests := []struct {
		name string
		rec  domain.MachineRepoRecord
		want bool
	}{
		{"unknown_blocked_is_attention", domain.MachineRepoRecord{State: domain.RepoStateBlocked}, true},
		{"future_blocked_is_quiet", domain.MachineRepoRecord{State: domain.RepoStateBlocked, LastActivityAt: now.Add(time.Hour)}, false},
		{"blocked_at_boundary", domain.MachineRepoRecord{State: domain.RepoStateBlocked, LastActivityAt: now.Add(-2 * time.Hour)}, true},
		{"wip_at_boundary", domain.MachineRepoRecord{State: domain.RepoStateWip, LastActivityAt: now.Add(-24 * time.Hour)}, true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := len(notificationAttentionSet([]domain.MachineRepoRecord{tt.rec}, now, cfg)) > 0
			if got != tt.want {
				t.Fatalf("attention=%t want %t", got, tt.want)
			}
		})
	}
}

func TestNotifyAttentionSetDedupe(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	sender := &fakeNotifySender{}
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.Getenv = func(key string) string {
		if key == "BB_MACHINE_ID" {
			return "machine-a"
		}
		return ""
	}
	a.NewNotifySender = func(string) (notifySender, error) { return sender, nil }
	now := time.Now()
	a.Now = func() time.Time { return now }
	cfg := state.DefaultConfig()
	rec := domain.MachineRepoRecord{Name: "api", RepoKey: "software/api", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}, LastActivityAt: now.Add(-3 * time.Hour)}
	if err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{rec}, notifyBackendStdout); err != nil {
		t.Fatal(err)
	}
	if err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{rec}, notifyBackendStdout); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d", len(sender.sent))
	}
	events, err := state.LoadJournalFile(paths.JournalPath("machine-a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "notified" || events[0].RepoKey != "software/api" {
		t.Fatalf("journal events = %+v", events)
	}
}

func TestAttentionFingerprintIgnoresReasonOrder(t *testing.T) {
	t.Parallel()
	a := domain.MachineRepoRecord{RepoKey: "x", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked, domain.ReasonDiverged}}
	b := a
	b.Reasons = []domain.UnsyncableReason{domain.ReasonDiverged, domain.ReasonDirtyTracked}
	if attentionFingerprint([]domain.MachineRepoRecord{a}) != attentionFingerprint([]domain.MachineRepoRecord{b}) {
		t.Fatal("fingerprint changed with reason order")
	}
}

func TestNotifyEmptySetResetsFingerprint(t *testing.T) {
	t.Parallel()
	paths := state.NewPaths(t.TempDir())
	sender := &fakeNotifySender{}
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.NewNotifySender = func(string) (notifySender, error) { return sender, nil }
	now := time.Now()
	a.Now = func() time.Time { return now }
	cfg := state.DefaultConfig()
	cfg.Notify.ThrottleMinutes = 0
	rec := domain.MachineRepoRecord{Name: "api", RepoKey: "x", State: domain.RepoStateBlocked, Reasons: []domain.UnsyncableReason{domain.ReasonDiverged}}
	if err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{rec}, notifyBackendStdout); err != nil {
		t.Fatal(err)
	}
	if err := a.notifyUnsyncable(cfg, nil, notifyBackendStdout); err != nil {
		t.Fatal(err)
	}
	if err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{rec}, notifyBackendStdout); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 2 {
		t.Fatalf("sent=%d", len(sender.sent))
	}
}

func TestNotifyUnsyncableUsesExplicitBackend(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	sender := &fakeNotifySender{name: notifyBackendOSAScript}
	a.NewNotifySender = func(name string) (notifySender, error) {
		if name != notifyBackendOSAScript {
			t.Fatalf("backend name = %q, want %q", name, notifyBackendOSAScript)
		}
		return sender, nil
	}

	cfg := state.DefaultConfig()
	err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{{
		RepoKey: "software/api",
		Name:    "api",
		State:   domain.RepoStateBlocked,
		Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
	}}, notifyBackendOSAScript)
	if err != nil {
		t.Fatalf("notifyUnsyncable failed: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent count = %d, want 1", len(sender.sent))
	}
}

func TestNotifyUnsyncableInvalidBackend(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	cfg := state.DefaultConfig()
	err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{{
		RepoKey: "software/api",
		Name:    "api",
		State:   domain.RepoStateBlocked,
		Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
	}}, "invalid-backend")
	if err == nil {
		t.Fatal("expected error for invalid backend")
	}
	if !strings.Contains(err.Error(), "invalid notify backend") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNotifyUnsyncablePersistsAndClearsDeliveryFailures(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	a := New(paths, stdout, stderr)
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }

	currentSender := &fakeNotifySender{name: notifyBackendStdout, sendErr: errors.New("notify failed")}
	a.NewNotifySender = func(name string) (notifySender, error) {
		if name != notifyBackendStdout {
			t.Fatalf("backend name = %q, want %q", name, notifyBackendStdout)
		}
		return currentSender, nil
	}

	record := domain.MachineRepoRecord{
		RepoKey: "software/api",
		Name:    "api",
		Path:    "/tmp/software/api",
		State:   domain.RepoStateBlocked,
		Reasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
	}
	cfg := state.DefaultConfig()
	if err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{record}, notifyBackendStdout); err != nil {
		t.Fatalf("notifyUnsyncable failed: %v", err)
	}

	cache, err := state.LoadNotifyCache(paths)
	if err != nil {
		t.Fatalf("load notify cache: %v", err)
	}
	failureKey := notifyBackendStdout
	if _, ok := cache.DeliveryFailures[failureKey]; !ok {
		t.Fatalf("expected delivery failure for key %q", failureKey)
	}
	if cache.LastSent.Fingerprint != "" {
		t.Fatalf("did not expect last_sent entry for failed delivery")
	}

	currentSender = &fakeNotifySender{name: notifyBackendStdout}
	a.Now = func() time.Time { return now.Add(time.Minute) }
	if err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{record}, notifyBackendStdout); err != nil {
		t.Fatalf("notifyUnsyncable failed on retry: %v", err)
	}

	cache, err = state.LoadNotifyCache(paths)
	if err != nil {
		t.Fatalf("load notify cache: %v", err)
	}
	if _, ok := cache.DeliveryFailures[failureKey]; ok {
		t.Fatalf("expected delivery failure to clear for key %q", failureKey)
	}
	if cache.LastSent.Fingerprint == "" {
		t.Fatalf("expected last_sent entry after successful delivery")
	}
}

func TestResolveNotifyBackendUsesEnv(t *testing.T) {
	t.Setenv(notifyBackendEnvVar, notifyBackendOSAScript)

	paths := state.NewPaths(t.TempDir())
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	got, err := a.resolveNotifyBackend("")
	if err != nil {
		t.Fatalf("resolve backend: %v", err)
	}
	if got != notifyBackendOSAScript {
		t.Fatalf("backend = %q, want %q", got, notifyBackendOSAScript)
	}
}

func TestResolveNotifyBackendExplicitWinsEnv(t *testing.T) {
	t.Setenv(notifyBackendEnvVar, notifyBackendOSAScript)

	paths := state.NewPaths(t.TempDir())
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	got, err := a.resolveNotifyBackend(notifyBackendStdout)
	if err != nil {
		t.Fatalf("resolve backend: %v", err)
	}
	if got != notifyBackendStdout {
		t.Fatalf("backend = %q, want %q", got, notifyBackendStdout)
	}
}
