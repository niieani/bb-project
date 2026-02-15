package app

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const githubRepositoryNameMaxLength = 100

var githubRepositoryNamePattern = regexp.MustCompile(`^[a-z0-9._-]+$`)

func validateGitHubRepositoryName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("repository name is required")
	}
	if len(name) > githubRepositoryNameMaxLength {
		return fmt.Errorf("repository name must be <= %d characters", githubRepositoryNameMaxLength)
	}
	if name == "." || name == ".." {
		return errors.New(`repository name cannot be "." or ".."`)
	}
	if !githubRepositoryNamePattern.MatchString(name) {
		return errors.New(`repository name may contain only lowercase letters, numbers, ".", "-", and "_"`)
	}
	return nil
}

func sanitizeGitHubRepositoryNameInput(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))
	pendingDash := false

	for _, r := range raw {
		switch {
		case r >= 'A' && r <= 'Z':
			r = unicode.ToLower(r)
		case isGitHubRepositoryNameRune(r):
		default:
			pendingDash = true
			continue
		}
		if pendingDash && b.Len() > 0 {
			b.WriteByte('-')
		}
		pendingDash = false
		b.WriteRune(r)
	}

	out := b.String()
	if len(out) > githubRepositoryNameMaxLength {
		out = out[:githubRepositoryNameMaxLength]
	}
	return out
}

func isGitHubRepositoryNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}

func validateGitBranchRenameTarget(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("branch name is required")
	}
	if name == "HEAD" {
		return errors.New(`branch name cannot be "HEAD"`)
	}
	if strings.HasPrefix(name, "-") {
		return errors.New("branch name cannot start with '-'")
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return errors.New("branch name cannot contain whitespace")
	}
	if strings.Contains(name, "..") {
		return errors.New("branch name cannot contain '..'")
	}
	if strings.Contains(name, "@{") {
		return errors.New(`branch name cannot contain "@{"`)
	}
	if strings.ContainsAny(name, "~^:?*[]\\") {
		return errors.New(`branch name contains invalid characters (~ ^ : ? * [ ] \)`)
	}
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
		return errors.New("branch name cannot contain empty path segments")
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return errors.New("branch name cannot start or end with '.'")
	}
	if strings.HasSuffix(name, ".lock") {
		return errors.New(`branch name cannot end with ".lock"`)
	}
	return nil
}

func validateFixApplyOptions(action string, opts fixApplyOptions) error {
	if _, err := ParseFixSyncStrategy(string(opts.SyncStrategy)); err != nil {
		return fmt.Errorf("invalid sync strategy: %w", err)
	}
	if opts.GenerateGitignore && action != FixActionStageCommitPush && action != FixActionCheckpointThenSync {
		return fmt.Errorf("invalid gitignore generation: action %q does not create a commit", action)
	}
	if action == FixActionCreateProject && strings.TrimSpace(opts.CreateProjectName) != "" {
		sanitized := sanitizeGitHubRepositoryNameInput(opts.CreateProjectName)
		if err := validateGitHubRepositoryName(sanitized); err != nil {
			return fmt.Errorf("invalid repository name: %w", err)
		}
	}
	if strings.TrimSpace(opts.ForkBranchRenameTo) != "" {
		if !actionSupportsPublishBranch(action) {
			return fmt.Errorf("invalid publish branch target: action %q does not support publish-to-new-branch", action)
		}
		if err := validateGitBranchRenameTarget(opts.ForkBranchRenameTo); err != nil {
			return fmt.Errorf("invalid publish branch target: %w", err)
		}
	}
	return nil
}

func actionSupportsPublishBranch(action string) bool {
	switch action {
	case FixActionForkAndRetarget,
		FixActionPush,
		FixActionStageCommitPush,
		FixActionCheckpointThenSync,
		FixActionSetUpstreamPush:
		return true
	default:
		return false
	}
}
