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

func validateFixApplyOptions(action string, opts fixApplyOptions) error {
	if action == FixActionCreateProject && strings.TrimSpace(opts.CreateProjectName) != "" {
		sanitized := sanitizeGitHubRepositoryNameInput(opts.CreateProjectName)
		if err := validateGitHubRepositoryName(sanitized); err != nil {
			return fmt.Errorf("invalid repository name: %w", err)
		}
	}
	return nil
}
