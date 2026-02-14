package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	return r.RepoOriginWithPreferredRemote(path, "")
}

func (r Runner) RepoOriginWithPreferredRemote(path string, preferredRemote string) (string, error) {
	remote, err := r.pickRemote(path, preferredRemote)
	if err != nil {
		return "", err
	}
	if remote == "" {
		return "", nil
	}
	out, err := r.RunGit(path, "remote", "get-url", remote)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func (r Runner) RemoteNames(path string) ([]string, error) {
	out, err := r.RunGit(path, "remote")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (r Runner) PreferredRemote(path string) (string, error) {
	upstream, _ := r.Upstream(path)
	if remote := remoteFromUpstream(upstream); remote != "" {
		return remote, nil
	}
	names, err := r.RemoteNames(path)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	for _, name := range names {
		if name == "origin" {
			return name, nil
		}
	}
	return names[0], nil
}

func (r Runner) pickRemote(path string, preferredRemote string) (string, error) {
	preferredRemote = strings.TrimSpace(preferredRemote)
	if preferredRemote != "" {
		names, err := r.RemoteNames(path)
		if err != nil {
			return "", err
		}
		for _, name := range names {
			if name == preferredRemote {
				return preferredRemote, nil
			}
		}
		return "", fmt.Errorf("preferred remote %q not found", preferredRemote)
	}
	return r.PreferredRemote(path)
}

func remoteFromUpstream(upstream string) string {
	remote, _, ok := strings.Cut(strings.TrimSpace(upstream), "/")
	if !ok {
		return ""
	}
	return remote
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
	return r.PushUpstreamWithPreferredRemote(path, branch, "")
}

func (r Runner) PushUpstreamWithPreferredRemote(path, branch, preferredRemote string) error {
	remote, err := r.pickRemote(path, preferredRemote)
	if err != nil {
		return err
	}
	if remote == "" {
		remote = "origin"
	}
	_, err = r.RunGit(path, "push", "-u", remote, branch)
	return err
}

func (r Runner) AddAll(path string) error {
	_, err := r.RunGit(path, "add", "-A")
	return err
}

func (r Runner) Commit(path, message string) error {
	_, err := r.RunGit(path, "commit", "-m", message)
	return err
}

func (r Runner) Checkout(path, branch string) error {
	return r.CheckoutWithPreferredRemote(path, branch, "")
}

func (r Runner) CheckoutWithPreferredRemote(path, branch, preferredRemote string) error {
	remote, err := r.pickRemote(path, preferredRemote)
	if err != nil {
		return err
	}
	if remote == "" {
		remote = "origin"
	}

	_, err = r.RunGit(path, "checkout", branch)
	if err == nil {
		_, _ = r.RunGit(path, "branch", "--set-upstream-to", remote+"/"+branch, branch)
		return nil
	}
	_, err2 := r.RunGit(path, "checkout", "-B", branch, "--track", remote+"/"+branch)
	return err2
}

func (r Runner) Clone(origin, path string) error {
	_, err := r.RunGit("", "clone", origin, path)
	return err
}

func (r Runner) EnsureBranch(path, branch string) error {
	return r.CheckoutWithPreferredRemote(path, branch, "")
}

func (r Runner) EnsureBranchWithPreferredRemote(path, branch, preferredRemote string) error {
	return r.CheckoutWithPreferredRemote(path, branch, preferredRemote)
}

func (r Runner) MergeAbort(path string) error {
	_, err := r.RunGit(path, "merge", "--abort")
	return err
}

func (r Runner) RebaseAbort(path string) error {
	_, err := r.RunGit(path, "rebase", "--abort")
	return err
}

func (r Runner) CherryPickAbort(path string) error {
	_, err := r.RunGit(path, "cherry-pick", "--abort")
	return err
}

func (r Runner) BisectReset(path string) error {
	_, err := r.RunGit(path, "bisect", "reset")
	return err
}
