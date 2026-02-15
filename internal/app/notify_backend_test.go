package app

import (
	"bytes"
	"errors"
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
		RepoKey:           "software/api",
		Name:              "api",
		Syncable:          false,
		UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
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
		RepoKey:           "software/api",
		Name:              "api",
		Syncable:          false,
		UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
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
		RepoKey:           "software/api",
		Name:              "api",
		Path:              "/tmp/software/api",
		Syncable:          false,
		UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
	}
	cfg := state.DefaultConfig()
	if err := a.notifyUnsyncable(cfg, []domain.MachineRepoRecord{record}, notifyBackendStdout); err != nil {
		t.Fatalf("notifyUnsyncable failed: %v", err)
	}

	cache, err := state.LoadNotifyCache(paths)
	if err != nil {
		t.Fatalf("load notify cache: %v", err)
	}
	failureKey := notifyFailureCacheKey(notifyBackendStdout, record)
	if _, ok := cache.DeliveryFailures[failureKey]; !ok {
		t.Fatalf("expected delivery failure for key %q", failureKey)
	}
	if _, ok := cache.LastSent[notifyCacheKey(record)]; ok {
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
	if _, ok := cache.LastSent[notifyCacheKey(record)]; !ok {
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
