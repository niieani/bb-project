package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bb-project/internal/state"
)

func TestRunSchedulerInstallWritesLaunchAgent(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	paths := state.NewPaths(home)
	cfg := state.DefaultConfig()
	cfg.Scheduler.IntervalMinutes = 60
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	a := New(paths, &stdout, &stderr)
	a.GOOS = func() string { return "darwin" }
	a.ExecutablePath = func() (string, error) { return "/tmp/bb", nil }

	var calls [][]string
	a.RunCommand = func(name string, args ...string) (string, error) {
		call := append([]string{name}, args...)
		calls = append(calls, call)
		if len(args) > 0 && args[0] == "unload" {
			return "", errors.New("not loaded")
		}
		return "", nil
	}

	code, err := a.RunSchedulerInstall(SchedulerInstallOptions{})
	if err != nil {
		t.Fatalf("RunSchedulerInstall failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	plistPath := schedulerPlistPath(home)
	content, err := os.ReadFile(filepath.Clean(plistPath))
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "<string>/tmp/bb</string>") {
		t.Fatalf("expected executable in plist, got:\n%s", text)
	}
	if !strings.Contains(text, "<integer>3600</integer>") {
		t.Fatalf("expected StartInterval seconds in plist, got:\n%s", text)
	}
	if !strings.Contains(text, "<string>--notify-backend</string>\n    <string>osascript</string>") {
		t.Fatalf("expected osascript notify backend in plist, got:\n%s", text)
	}
	if len(calls) < 2 {
		t.Fatalf("launchctl calls = %d, want at least 2", len(calls))
	}
}

func TestRunSchedulerInstallRejectsInvalidBackend(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	cfg := state.DefaultConfig()
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.GOOS = func() string { return "darwin" }

	code, err := a.RunSchedulerInstall(SchedulerInstallOptions{NotifyBackend: "invalid"})
	if err == nil {
		t.Fatal("expected error")
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRunSchedulerStatusReportsInstalled(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	paths := state.NewPaths(home)
	if err := os.MkdirAll(filepath.Dir(schedulerPlistPath(home)), 0o755); err != nil {
		t.Fatalf("mkdir launch agents: %v", err)
	}
	if err := os.WriteFile(schedulerPlistPath(home), []byte(sampleSchedulerPlist("/tmp/bb", 3600, notifyBackendOSAScript, "/tmp/bb.log", "/tmp/bb.err.log")), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	var stdout bytes.Buffer
	a := New(paths, &stdout, &bytes.Buffer{})
	a.GOOS = func() string { return "darwin" }
	a.RunCommand = func(name string, args ...string) (string, error) {
		return schedulerLaunchdLabel + "\n", nil
	}

	code, err := a.RunSchedulerStatus()
	if err != nil {
		t.Fatalf("RunSchedulerStatus failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "installed=true") {
		t.Fatalf("expected installed status, got:\n%s", out)
	}
	if !strings.Contains(out, "interval_minutes=60") {
		t.Fatalf("expected interval in output, got:\n%s", out)
	}
	if !strings.Contains(out, "notify_backend=osascript") {
		t.Fatalf("expected backend in output, got:\n%s", out)
	}
}

func TestRunSchedulerRemoveRemovesPlist(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	paths := state.NewPaths(home)
	if err := os.MkdirAll(filepath.Dir(schedulerPlistPath(home)), 0o755); err != nil {
		t.Fatalf("mkdir launch agents: %v", err)
	}
	if err := os.WriteFile(schedulerPlistPath(home), []byte("x"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.GOOS = func() string { return "darwin" }
	calls := 0
	a.RunCommand = func(name string, args ...string) (string, error) {
		calls++
		return "", nil
	}

	code, err := a.RunSchedulerRemove()
	if err != nil {
		t.Fatalf("RunSchedulerRemove failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if _, err := os.Stat(schedulerPlistPath(home)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected plist removal, stat err=%v", err)
	}
	if calls == 0 {
		t.Fatalf("expected launchctl unload call")
	}
}

func TestRunSchedulerUnsupportedOS(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, &bytes.Buffer{}, &bytes.Buffer{})
	a.GOOS = func() string { return "linux" }

	code, err := a.RunSchedulerStatus()
	if err == nil {
		t.Fatal("expected unsupported OS error")
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
