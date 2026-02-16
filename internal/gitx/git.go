package gitx

import (
	"bytes"
	"fmt"
	"io"
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

type CloneOptions struct {
	Origin  string
	Path    string
	Shallow bool
	Filter  string
	Only    []string
	Stdout  io.Writer
	Stderr  io.Writer
}

type SyncProbeOutcome string

const (
	SyncProbeOutcomeClean    SyncProbeOutcome = "clean"
	SyncProbeOutcomeConflict SyncProbeOutcome = "conflict"
	SyncProbeOutcomeFailed   SyncProbeOutcome = "probe_failed"
	SyncProbeOutcomeUnknown  SyncProbeOutcome = "unknown"
)

var gitProcessEnv = []string{
	"GIT_CONFIG_GLOBAL=" + os.DevNull,
	"GIT_CONFIG_NOSYSTEM=1",
	"GIT_AUTHOR_NAME=bb",
	"GIT_AUTHOR_EMAIL=bb@example.com",
	"GIT_COMMITTER_NAME=bb",
	"GIT_COMMITTER_EMAIL=bb@example.com",
	"GIT_TERMINAL_PROMPT=0",
	"GIT_ASKPASS=",
	"SSH_ASKPASS=",
	"SSH_ASKPASS_REQUIRE=never",
	"GCM_INTERACTIVE=never",
}

func gitCommandEnv(base []string) []string {
	out := make([]string, 0, len(base)+len(gitProcessEnv))
	out = append(out, base...)
	out = append(out, gitProcessEnv...)
	return out
}

func (r Runner) run(dir string, name string, args ...string) (Result, error) {
	return r.runWithEnv(dir, nil, name, args...)
}

func (r Runner) runWithEnv(dir string, extraEnv []string, name string, args ...string) (Result, error) {
	return r.runWithEnvStreaming(dir, extraEnv, nil, nil, name, args...)
}

func (r Runner) runWithEnvStreaming(dir string, extraEnv []string, stdoutWriter io.Writer, stderrWriter io.Writer, name string, args ...string) (Result, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	base := os.Environ()
	if len(extraEnv) > 0 {
		base = append(base, extraEnv...)
	}
	cmd.Env = gitCommandEnv(base)
	var stdout, stderr bytes.Buffer
	if stdoutWriter != nil {
		cmd.Stdout = io.MultiWriter(stdoutWriter, &stdout)
	} else {
		cmd.Stdout = &stdout
	}
	if stderrWriter != nil {
		cmd.Stderr = io.MultiWriter(stderrWriter, &stderr)
	} else {
		cmd.Stderr = &stderr
	}
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

func (r Runner) EffectiveRemote(path string, preferredRemote string) (string, error) {
	return r.pickRemote(path, preferredRemote)
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

func (r Runner) AddRemote(path, name, url string) error {
	_, err := r.RunGit(path, "remote", "add", name, url)
	return err
}

func (r Runner) SetRemoteURL(path, name, url string) error {
	_, err := r.RunGit(path, "remote", "set-url", name, url)
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

func (r Runner) DefaultBranch(path string, preferredRemote string) (string, error) {
	remote, err := r.pickRemote(path, preferredRemote)
	if err != nil {
		return "", err
	}
	if remote == "" {
		return "", nil
	}
	ref := "refs/remotes/" + remote + "/HEAD"
	out, err := r.RunGit(path, "symbolic-ref", "--quiet", "--short", ref)
	if err != nil {
		return "", nil
	}
	out = strings.TrimSpace(out)
	prefix := remote + "/"
	if strings.HasPrefix(out, prefix) {
		return strings.TrimPrefix(out, prefix), nil
	}
	_, branch, ok := strings.Cut(out, "/")
	if !ok {
		return out, nil
	}
	return branch, nil
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

func (r Runner) RunGitNoHooks(dir string, args ...string) (string, error) {
	out, err := r.runGitNoHooks(dir, args...)
	return strings.TrimSpace(out.Stdout), err
}

func (r Runner) runGitNoHooks(dir string, args ...string) (Result, error) {
	return r.runWithEnv(dir, []string{"HUSKY=0"}, "git", append([]string{"-c", "core.hooksPath=" + os.DevNull}, args...)...)
}

func (r Runner) Rebase(path, upstream string) error {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		upstream = "@{u}"
	}
	_, err := r.RunGit(path, "rebase", upstream)
	return err
}

func (r Runner) MergeNoEdit(path, upstream string) error {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		upstream = "@{u}"
	}
	_, err := r.RunGit(path, "merge", "--no-edit", upstream)
	return err
}

func (r Runner) CanSyncWithUpstream(path string, upstream string, strategy string) (bool, error) {
	outcome, err := r.ProbeSyncWithUpstream(path, upstream, strategy)
	if err != nil {
		return false, err
	}
	return outcome == SyncProbeOutcomeClean, nil
}

func (r Runner) ProbeSyncWithUpstream(path string, upstream string, strategy string) (SyncProbeOutcome, error) {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return SyncProbeOutcomeUnknown, nil
	}
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy != "rebase" && strategy != "merge" {
		return SyncProbeOutcomeUnknown, fmt.Errorf("unsupported sync strategy %q", strategy)
	}

	tmpRoot, err := os.MkdirTemp("", "bb-sync-feasibility-*")
	if err != nil {
		return SyncProbeOutcomeFailed, err
	}
	defer os.RemoveAll(tmpRoot)
	worktreePath := filepath.Join(tmpRoot, "worktree")

	if _, err := r.RunGitNoHooks(path, "worktree", "add", "--detach", worktreePath, "HEAD"); err != nil {
		return SyncProbeOutcomeFailed, err
	}
	defer func() {
		_, _ = r.RunGitNoHooks(path, "worktree", "remove", "--force", worktreePath)
	}()

	var syncErr error
	var syncResult Result
	switch strategy {
	case "merge":
		syncResult, syncErr = r.runGitNoHooks(worktreePath, "merge", "--no-commit", "--no-ff", upstream)
		if syncErr != nil {
			_, _ = r.RunGitNoHooks(worktreePath, "merge", "--abort")
		} else {
			_, _ = r.RunGitNoHooks(worktreePath, "merge", "--abort")
		}
	default:
		syncResult, syncErr = r.runGitNoHooks(worktreePath, "rebase", upstream)
		if syncErr != nil {
			_, _ = r.RunGitNoHooks(worktreePath, "rebase", "--abort")
		}
	}

	if syncErr == nil {
		return SyncProbeOutcomeClean, nil
	}
	if looksLikeSyncConflict(errorAndResultOutput(syncErr, syncResult)) {
		return SyncProbeOutcomeConflict, nil
	}
	return SyncProbeOutcomeFailed, nil
}

func (r Runner) Push(path string) error {
	_, err := r.RunGit(path, "push")
	return err
}

func (r Runner) ProbePushAccess(path string, preferredRemote string) (domain.PushAccess, string, error) {
	remote, err := r.pickRemote(path, preferredRemote)
	if err != nil {
		return domain.PushAccessUnknown, "", err
	}
	if remote == "" {
		return domain.PushAccessUnknown, "", nil
	}

	branch, err := r.CurrentBranch(path)
	if err != nil {
		return domain.PushAccessUnknown, remote, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "HEAD" {
		return domain.PushAccessUnknown, remote, nil
	}

	pushResult, err := r.runGitNoHooks(path, "push", "--dry-run", "--porcelain", remote, "HEAD:refs/heads/"+branch)
	if err == nil {
		return domain.PushAccessReadWrite, remote, nil
	}

	if looksLikePushAccessDenied(errorAndResultOutput(err, pushResult)) {
		return domain.PushAccessReadOnly, remote, nil
	}
	return domain.PushAccessUnknown, remote, err
}

func errorAndResultOutput(err error, result Result) string {
	parts := make([]string, 0, 3)
	if err != nil {
		parts = append(parts, strings.TrimSpace(err.Error()))
	}
	if strings.TrimSpace(result.Stderr) != "" {
		parts = append(parts, strings.TrimSpace(result.Stderr))
	}
	if strings.TrimSpace(result.Stdout) != "" {
		parts = append(parts, strings.TrimSpace(result.Stdout))
	}
	return strings.Join(parts, "\n")
}

func looksLikePushAccessDenied(msg string) bool {
	if strings.TrimSpace(msg) == "" {
		return false
	}
	lower := strings.ToLower(msg)
	indicators := []string{
		"permission denied",
		"access denied",
		"not permitted",
		"write access to repository not granted",
		"insufficient permission",
		"forbidden",
		"authentication failed",
		"could not read from remote repository",
	}
	for _, indicator := range indicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

func looksLikeSyncConflict(msg string) bool {
	if strings.TrimSpace(msg) == "" {
		return false
	}
	lower := strings.ToLower(msg)
	indicators := []string{
		"conflict",
		"automatic merge failed",
		"could not apply",
		"resolve all conflicts manually",
		"merge conflict",
	}
	for _, indicator := range indicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

func (r Runner) PushUpstream(path, branch string) error {
	return r.PushUpstreamWithPreferredRemote(path, branch, "")
}

func (r Runner) PushUpstreamWithPreferredRemote(path, branch, preferredRemote string) error {
	return r.pushUpstream(path, branch, preferredRemote, false)
}

func (r Runner) PushUpstreamForce(path, branch string) error {
	return r.PushUpstreamForceWithPreferredRemote(path, branch, "")
}

func (r Runner) PushUpstreamForceWithPreferredRemote(path, branch, preferredRemote string) error {
	return r.pushUpstream(path, branch, preferredRemote, true)
}

func (r Runner) pushUpstream(path, branch, preferredRemote string, force bool) error {
	remote, err := r.pickRemote(path, preferredRemote)
	if err != nil {
		return err
	}
	if remote == "" {
		remote = "origin"
	}
	args := []string{"push", "-u"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, remote, branch)
	_, err = r.RunGit(path, args...)
	return err
}

func (r Runner) AddAll(path string) error {
	_, err := r.RunGit(path, "add", "-A")
	return err
}

func (r Runner) RenameCurrentBranch(path, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	_, err := r.RunGit(path, "branch", "-m", name)
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
	return r.CloneWithOptions(CloneOptions{Origin: origin, Path: path})
}

func (r Runner) CloneWithOptions(opts CloneOptions) error {
	origin := strings.TrimSpace(opts.Origin)
	path := strings.TrimSpace(opts.Path)
	if origin == "" {
		return fmt.Errorf("clone origin is required")
	}
	if path == "" {
		return fmt.Errorf("clone path is required")
	}

	args := []string{"clone"}
	if opts.Shallow {
		args = append(args, "--depth", "1")
	}
	if filter := strings.TrimSpace(opts.Filter); filter != "" {
		args = append(args, "--filter="+filter)
	}
	only := make([]string, 0, len(opts.Only))
	seen := map[string]struct{}{}
	for _, raw := range opts.Only {
		pathSpec := strings.TrimSpace(raw)
		if pathSpec == "" {
			continue
		}
		if _, ok := seen[pathSpec]; ok {
			continue
		}
		seen[pathSpec] = struct{}{}
		only = append(only, pathSpec)
	}
	if len(only) > 0 {
		args = append(args, "--sparse")
	}
	args = append(args, origin, path)
	if _, err := r.runWithEnvStreaming("", nil, opts.Stdout, opts.Stderr, "git", args...); err != nil {
		return err
	}
	if len(only) == 0 {
		return nil
	}
	sparseArgs := append([]string{"sparse-checkout", "set", "--no-cone"}, only...)
	_, err := r.runWithEnvStreaming(path, nil, opts.Stdout, opts.Stderr, "git", sparseArgs...)
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
