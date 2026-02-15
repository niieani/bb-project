package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"bb-project/internal/state"
)

const schedulerLaunchdLabel = "com.bb-project.sync"

var (
	reStartInterval = regexp.MustCompile(`(?s)<key>\s*StartInterval\s*</key>\s*<integer>\s*([0-9]+)\s*</integer>`)
	reNotifyBackend = regexp.MustCompile(`(?s)<string>\s*--notify-backend\s*</string>\s*<string>\s*([^<]+)\s*</string>`)
)

func (a *App) RunSchedulerInstall(opts SchedulerInstallOptions) (int, error) {
	if !a.supportsScheduler() {
		return 2, fmt.Errorf("scheduler is only supported on macOS")
	}

	cfg, err := state.LoadConfig(a.Paths)
	if err != nil {
		return 2, err
	}

	backend, err := a.resolveNotifyBackendWithDefault(opts.NotifyBackend, notifyBackendOSAScript)
	if err != nil {
		return 2, err
	}

	intervalMinutes := cfg.Scheduler.IntervalMinutes
	if intervalMinutes < 1 {
		intervalMinutes = 60
	}

	executablePathFn := a.ExecutablePath
	if executablePathFn == nil {
		executablePathFn = os.Executable
	}
	executable, err := executablePathFn()
	if err != nil {
		return 2, fmt.Errorf("resolve executable path: %w", err)
	}
	if !filepath.IsAbs(executable) {
		executable, err = filepath.Abs(executable)
		if err != nil {
			return 2, fmt.Errorf("resolve executable path: %w", err)
		}
	}

	plistPath := schedulerPlistPath(a.Paths.Home)
	stdoutPath := filepath.Join(a.Paths.LocalStateRoot(), "scheduler-sync.log")
	stderrPath := filepath.Join(a.Paths.LocalStateRoot(), "scheduler-sync.err.log")
	plist := sampleSchedulerPlist(executable, intervalMinutes*60, backend, stdoutPath, stderrPath)

	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return 2, fmt.Errorf("create launch agents directory: %w", err)
	}
	if err := os.MkdirAll(a.Paths.LocalStateRoot(), 0o755); err != nil {
		return 2, fmt.Errorf("create local state directory: %w", err)
	}

	runCommand := a.RunCommand
	if runCommand == nil {
		runCommand = defaultRunCommand
	}
	_, _ = runCommand("launchctl", "unload", plistPath)

	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return 2, fmt.Errorf("write launch agent: %w", err)
	}

	if _, err := runCommand("launchctl", "load", plistPath); err != nil {
		return 2, fmt.Errorf("load launch agent: %w", err)
	}

	fmt.Fprintf(a.Stdout, "installed scheduler label=%s interval_minutes=%d notify_backend=%s\n", schedulerLaunchdLabel, intervalMinutes, backend)
	return 0, nil
}

func (a *App) RunSchedulerStatus() (int, error) {
	if !a.supportsScheduler() {
		return 2, fmt.Errorf("scheduler is only supported on macOS")
	}

	plistPath := schedulerPlistPath(a.Paths.Home)
	raw, err := os.ReadFile(plistPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(a.Stdout, "installed=false label=%s path=%s\n", schedulerLaunchdLabel, plistPath)
			return 0, nil
		}
		return 2, err
	}

	intervalMinutes := extractSchedulerIntervalMinutes(string(raw))
	backend := extractSchedulerNotifyBackend(string(raw))
	if backend == "" {
		backend = notifyBackendOSAScript
	}

	loaded := false
	runCommand := a.RunCommand
	if runCommand == nil {
		runCommand = defaultRunCommand
	}
	if out, err := runCommand("launchctl", "list", schedulerLaunchdLabel); err == nil && strings.Contains(out, schedulerLaunchdLabel) {
		loaded = true
	}

	fmt.Fprintf(a.Stdout, "installed=true loaded=%t label=%s interval_minutes=%d notify_backend=%s path=%s\n", loaded, schedulerLaunchdLabel, intervalMinutes, backend, plistPath)
	return 0, nil
}

func (a *App) RunSchedulerRemove() (int, error) {
	if !a.supportsScheduler() {
		return 2, fmt.Errorf("scheduler is only supported on macOS")
	}

	plistPath := schedulerPlistPath(a.Paths.Home)
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Fprintf(a.Stdout, "removed=false label=%s path=%s\n", schedulerLaunchdLabel, plistPath)
		return 0, nil
	}

	runCommand := a.RunCommand
	if runCommand == nil {
		runCommand = defaultRunCommand
	}
	_, _ = runCommand("launchctl", "unload", plistPath)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return 2, fmt.Errorf("remove launch agent: %w", err)
	}
	fmt.Fprintf(a.Stdout, "removed=true label=%s path=%s\n", schedulerLaunchdLabel, plistPath)
	return 0, nil
}

func (a *App) supportsScheduler() bool {
	goos := "darwin"
	if a.GOOS != nil {
		goos = strings.TrimSpace(a.GOOS())
	}
	return goos == "darwin"
}

func schedulerPlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", schedulerLaunchdLabel+".plist")
}

func sampleSchedulerPlist(executable string, intervalSeconds int, backend string, stdoutPath string, stderrPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
    <string>%s</string>
    <string>sync</string>
    <string>--notify</string>
    <string>--quiet</string>
    <string>--notify-backend</string>
    <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>StartInterval</key>
    <integer>%d</integer>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, schedulerLaunchdLabel, executable, backend, intervalSeconds, stdoutPath, stderrPath)
}

func extractSchedulerIntervalMinutes(plist string) int {
	matches := reStartInterval.FindStringSubmatch(plist)
	if len(matches) != 2 {
		return 0
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil || seconds < 1 {
		return 0
	}
	return seconds / 60
}

func extractSchedulerNotifyBackend(plist string) string {
	matches := reNotifyBackend.FindStringSubmatch(plist)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}
