package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bb-project/internal/domain"
)

type Runner struct {
	Now func() time.Time
}

type Result struct {
	Stdout string
	Stderr string
}

func (r Runner) run(dir string, name string, args ...string) (Result, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=bb",
		"GIT_AUTHOR_EMAIL=bb@example.com",
		"GIT_COMMITTER_NAME=bb",
		"GIT_COMMITTER_EMAIL=bb@example.com",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return Result{Stdout: stdout.String(), Stderr: stderr.String()}, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderr.String())
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func (r Runner) RunGit(dir string, args ...string) (string, error) {
	res, err := r.run(dir, "git", args...)
	return strings.TrimSpace(res.Stdout), err
}

func (r Runner) IsGitRepo(path string) bool {
	_, err := r.RunGit(path, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

func (r Runner) RepoOrigin(path string) (string, error) {
	out, err := r.RunGit(path, "remote", "get-url", "origin")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func (r Runner) InitRepo(path string) error {
	_, err := r.RunGit(path, "init", "-b", "main")
	return err
}

func (r Runner) AddOrigin(path, url string) error {
	_, err := r.RunGit(path, "remote", "add", "origin", url)
	return err
}

func (r Runner) SetOrigin(path, url string) error {
	_, err := r.RunGit(path, "remote", "set-url", "origin", url)
	return err
}

func (r Runner) CurrentBranch(path string) (string, error) {
	out, err := r.RunGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func (r Runner) HeadSHA(path string) (string, error) {
	out, err := r.RunGit(path, "rev-parse", "HEAD")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func (r Runner) Upstream(path string) (string, error) {
	out, err := r.RunGit(path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func (r Runner) RemoteHeadSHA(path string) (string, error) {
	out, err := r.RunGit(path, "rev-parse", "@{u}")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func (r Runner) AheadBehind(path string) (ahead, behind int, diverged bool, err error) {
	out, err := r.RunGit(path, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if err != nil {
		return 0, 0, false, nil
	}
	parts := strings.Fields(out)
	if len(parts) != 2 {
		return 0, 0, false, fmt.Errorf("unexpected rev-list output %q", out)
	}
	behind, _ = strconv.Atoi(parts[0])
	ahead, _ = strconv.Atoi(parts[1])
	diverged = ahead > 0 && behind > 0
	return ahead, behind, diverged, nil
}

func (r Runner) Dirty(path string) (tracked bool, untracked bool, err error) {
	out, err := r.RunGit(path, "status", "--porcelain")
	if err != nil {
		return false, false, err
	}
	if strings.TrimSpace(out) == "" {
		return false, false, nil
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "??") {
			untracked = true
			continue
		}
		tracked = true
	}
	return tracked, untracked, nil
}

func (r Runner) Operation(path string) domain.Operation {
	gitDir := filepath.Join(path, ".git")
	if hasFile(filepath.Join(gitDir, "MERGE_HEAD")) {
		return domain.OperationMerge
	}
	if hasDir(filepath.Join(gitDir, "rebase-apply")) || hasDir(filepath.Join(gitDir, "rebase-merge")) {
		return domain.OperationRebase
	}
	if hasFile(filepath.Join(gitDir, "CHERRY_PICK_HEAD")) {
		return domain.OperationCherryPick
	}
	if hasDir(filepath.Join(gitDir, "BISECT_LOG")) || hasFile(filepath.Join(gitDir, "BISECT_LOG")) {
		return domain.OperationBisect
	}
	return domain.OperationNone
}

func hasFile(path string) bool {
	i, err := os.Stat(path)
	return err == nil && !i.IsDir()
}

func hasDir(path string) bool {
	i, err := os.Stat(path)
	return err == nil && i.IsDir()
}

func (r Runner) FetchPrune(path string) error {
	_, err := r.RunGit(path, "fetch", "--prune")
	return err
}

func (r Runner) PullFFOnly(path string) error {
	_, err := r.RunGit(path, "pull", "--ff-only")
	return err
}

func (r Runner) Push(path string) error {
	_, err := r.RunGit(path, "push")
	return err
}

func (r Runner) PushUpstream(path, branch string) error {
	_, err := r.RunGit(path, "push", "-u", "origin", branch)
	return err
}

func (r Runner) Checkout(path, branch string) error {
	_, err := r.RunGit(path, "checkout", branch)
	if err == nil {
		_, _ = r.RunGit(path, "branch", "--set-upstream-to", "origin/"+branch, branch)
		return nil
	}
	_, err2 := r.RunGit(path, "checkout", "-B", branch, "--track", "origin/"+branch)
	return err2
}

func (r Runner) Clone(origin, path string) error {
	_, err := r.RunGit("", "clone", origin, path)
	return err
}

func (r Runner) EnsureBranch(path, branch string) error {
	return r.Checkout(path, branch)
}
