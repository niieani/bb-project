package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"bb-project/internal/domain"
)

type GitHubCLIStatus struct {
	Checked       bool
	Installed     bool
	Authenticated bool
	AuthStatus    string
}

func (s GitHubCLIStatus) Ready() bool {
	return s.Checked && s.Installed && s.Authenticated
}

func (a *App) detectGitHubCLIStatus() GitHubCLIStatus {
	getenv := os.Getenv
	if a.Getenv != nil {
		getenv = a.Getenv
	}
	if strings.TrimSpace(getenv("BB_TEST_REMOTE_ROOT")) != "" {
		return GitHubCLIStatus{
			Checked:       true,
			Installed:     true,
			Authenticated: true,
		}
	}

	lookPath := a.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("gh"); err != nil {
		return GitHubCLIStatus{
			Checked:       true,
			Installed:     false,
			Authenticated: false,
		}
	}

	runCommand := a.RunCommand
	if runCommand == nil {
		runCommand = defaultRunCommand
	}
	out, err := runCommand("gh", "auth", "status")
	if err != nil {
		return GitHubCLIStatus{
			Checked:       true,
			Installed:     true,
			Authenticated: false,
			AuthStatus:    shortCommandOutput(out),
		}
	}

	return GitHubCLIStatus{
		Checked:       true,
		Installed:     true,
		Authenticated: true,
	}
}

func (a *App) ensureGitHubCLIReady() error {
	status := a.detectGitHubCLIStatus()
	if !status.Checked || status.Ready() {
		return nil
	}
	if !status.Installed {
		return errors.New("GitHub operations require gh in PATH; install it (for example `brew install gh`), run `gh auth login`, then verify with `gh auth status`")
	}

	msg := "GitHub operations require gh to be authenticated; run `gh auth login` and verify with `gh auth status`"
	if details := strings.TrimSpace(status.AuthStatus); details != "" {
		msg = fmt.Sprintf("%s (current status: %s)", msg, details)
	}
	return errors.New(msg)
}

func (a *App) reportGitHubCLIWarnings(cfg domain.ConfigFile, repos []domain.MachineRepoRecord, allowedCatalogs map[string]struct{}) int {
	if !requiresGitHubCLI(cfg, repos, allowedCatalogs) {
		return 0
	}

	status := a.detectGitHubCLIStatus()
	if !status.Checked || status.Ready() {
		return 0
	}

	if !status.Installed {
		fmt.Fprintln(a.Stdout, "warning: GitHub operations require gh, but it is not available on PATH")
		fmt.Fprintln(a.Stdout, "warning: install gh (for example `brew install gh`) and run `gh auth login`")
		return 1
	}

	fmt.Fprintln(a.Stdout, "warning: gh is installed but not authenticated for GitHub operations")
	fmt.Fprintln(a.Stdout, "warning: run `gh auth login` and confirm with `gh auth status`")
	if details := strings.TrimSpace(status.AuthStatus); details != "" {
		fmt.Fprintf(a.Stdout, "warning: gh auth status detail: %s\n", details)
	}
	return 1
}

func requiresGitHubCLI(cfg domain.ConfigFile, repos []domain.MachineRepoRecord, allowedCatalogs map[string]struct{}) bool {
	if strings.TrimSpace(cfg.GitHub.Owner) != "" {
		return true
	}
	for _, repo := range repos {
		if len(allowedCatalogs) > 0 {
			if _, ok := allowedCatalogs[repo.Catalog]; !ok {
				continue
			}
		}
		if _, _, ok := githubSourceRepoForOrigin(repo.OriginURL); ok {
			return true
		}
	}
	return false
}

func shortCommandOutput(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		const max = 180
		if len(line) <= max {
			return line
		}
		return line[:max] + "..."
	}
	return ""
}
