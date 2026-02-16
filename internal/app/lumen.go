package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

const (
	lumenInstallTipText = "tip: install lumen with 'brew install jnsahaj/lumen/lumen'; for AI features run 'lumen configure'."
	lumenTipDisableText = "set integrations.lumen.show_install_tip=false to hide this tip."
)

type gitIndexSnapshot struct {
	path    string
	exists  bool
	data    []byte
	mode    os.FileMode
	enabled bool
}

func defaultRunCommandInDir(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func defaultRunCommandAttached(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (a *App) runDiff(project string, args []string) (int, error) {
	return a.runLumenCommandForProject(project, "diff", args)
}

func (a *App) runOperate(project string, args []string) (int, error) {
	return a.runLumenCommandForProject(project, "operate", args)
}

func (a *App) runLumenCommandForProject(project string, subcommand string, args []string) (int, error) {
	repoPath, err := a.resolveLocalRepoPath(project)
	if err != nil {
		return 2, err
	}

	if err := a.runLumenAttached(repoPath, subcommand, args); err != nil {
		return 2, err
	}
	return 0, nil
}

func (a *App) resolveLocalRepoPath(project string) (string, error) {
	cfg, machine, err := a.loadContext()
	if err != nil {
		return "", err
	}
	target, found, err := a.resolveProjectOrRepoSelector(cfg, &machine, project, resolveProjectOrRepoSelectorOptions{
		AllowClone: false,
	})
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("project %q not found", project)
	}
	repoPath := strings.TrimSpace(target.Path)
	if repoPath == "" {
		return "", fmt.Errorf("project %q has no local repository path", project)
	}
	return repoPath, nil
}

func (a *App) runLumenAttached(repoPath string, subcommand string, args []string) error {
	runtime, err := a.resolveLumenRuntime()
	if err != nil {
		return err
	}

	run := a.RunCommandAttached
	if run == nil {
		run = defaultRunCommandAttached
	}
	commandArgs := append([]string{subcommand}, args...)
	if err := run(repoPath, runtime.binary, commandArgs...); err != nil {
		return runtime.wrapError(fmt.Sprintf("lumen %s failed", subcommand), err)
	}
	return nil
}

func (a *App) generateLumenCommitMessage(repoPath string) (string, error) {
	runtime, err := a.resolveLumenRuntime()
	if err != nil {
		return "", err
	}

	snapshot, err := captureGitIndexSnapshot(a, repoPath)
	if err != nil {
		return "", err
	}
	if snapshot.enabled {
		defer func() {
			_ = snapshot.restore()
		}()
	}

	if err := a.Git.AddAll(repoPath); err != nil {
		return "", fmt.Errorf("git add -A failed before lumen draft: %w", err)
	}

	run := a.RunCommandInDir
	if run == nil {
		run = defaultRunCommandInDir
	}
	out, err := run(repoPath, runtime.binary, "draft")
	if err != nil {
		detail := strings.TrimSpace(out)
		if detail != "" {
			return "", runtime.wrapError("lumen draft failed", fmt.Errorf("%w: %s", err, detail))
		}
		return "", runtime.wrapError("lumen draft failed", err)
	}

	message := strings.TrimSpace(out)
	if message == "" {
		return "", runtime.wrapError("lumen draft returned an empty commit message", nil)
	}
	return message, nil
}

func (a *App) prepareLumenDiffExecCommand(repoPath string, args []string) (*exec.Cmd, func(error) error, error) {
	runtime, err := a.resolveLumenRuntime()
	if err != nil {
		return nil, nil, err
	}
	commandArgs := append([]string{"diff"}, args...)
	cmd := exec.Command(runtime.binary, commandArgs...)
	cmd.Dir = repoPath
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, func(runErr error) error {
		if runErr == nil {
			return nil
		}
		return runtime.wrapError("lumen diff failed", runErr)
	}, nil
}

type lumenRuntime struct {
	settings domain.LumenIntegrationConfig
	binary   string
}

func (r lumenRuntime) wrapError(summary string, err error) error {
	msg := strings.TrimSpace(summary)
	if msg == "" {
		msg = "lumen integration failed"
	}
	if err != nil {
		msg = fmt.Sprintf("%s: %v", msg, err)
	}
	if r.settings.ShowInstallTip {
		return errors.New(msg + "\n" + lumenInstallTipText + "\n" + lumenTipDisableText)
	}
	return errors.New(msg)
}

func (a *App) resolveLumenRuntime() (lumenRuntime, error) {
	settings, err := a.loadLumenIntegrationConfig()
	if err != nil {
		return lumenRuntime{}, err
	}
	runtime := lumenRuntime{settings: settings}
	if !settings.Enabled {
		return lumenRuntime{}, runtime.wrapError("lumen integration is disabled by config (integrations.lumen.enabled=false)", nil)
	}

	lookPath := a.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("lumen")
	if err != nil {
		return lumenRuntime{}, runtime.wrapError("lumen is not installed or not available on PATH", err)
	}
	runtime.binary = binary
	return runtime, nil
}

func (a *App) loadLumenIntegrationConfig() (domain.LumenIntegrationConfig, error) {
	cfg, err := state.LoadConfig(a.Paths)
	if err != nil {
		return domain.LumenIntegrationConfig{}, err
	}
	return cfg.Integrations.Lumen, nil
}

func captureGitIndexSnapshot(a *App, repoPath string) (gitIndexSnapshot, error) {
	if a == nil {
		return gitIndexSnapshot{}, nil
	}
	indexPath, err := resolveGitIndexPath(a, repoPath)
	if err != nil {
		return gitIndexSnapshot{}, err
	}
	info, err := os.Stat(indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return gitIndexSnapshot{
			path:    indexPath,
			exists:  false,
			enabled: true,
		}, nil
	}
	if err != nil {
		return gitIndexSnapshot{}, err
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return gitIndexSnapshot{}, err
	}
	return gitIndexSnapshot{
		path:    indexPath,
		exists:  true,
		data:    data,
		mode:    info.Mode().Perm(),
		enabled: true,
	}, nil
}

func resolveGitIndexPath(a *App, repoPath string) (string, error) {
	indexPath := filepath.Join(repoPath, ".git", "index")
	if a == nil {
		return indexPath, nil
	}
	out, err := a.Git.RunGit(repoPath, "rev-parse", "--git-path", "index")
	if err != nil {
		return indexPath, nil
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return indexPath, nil
	}
	if filepath.IsAbs(trimmed) {
		return trimmed, nil
	}
	return filepath.Join(repoPath, trimmed), nil
}

func (s gitIndexSnapshot) restore() error {
	if !s.enabled || strings.TrimSpace(s.path) == "" {
		return nil
	}
	if !s.exists {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	mode := s.mode
	if mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, s.data, mode)
}
