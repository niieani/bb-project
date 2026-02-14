package app

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const githubRepositoryNameMaxLength = 100

var githubRepositoryNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

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
		return errors.New(`repository name may contain only letters, numbers, ".", "-", and "_"`)
	}
	return nil
}

func validateFixApplyOptions(action string, opts fixApplyOptions) error {
	if action == FixActionCreateProject && strings.TrimSpace(opts.CreateProjectName) != "" {
		if err := validateGitHubRepositoryName(opts.CreateProjectName); err != nil {
			return fmt.Errorf("invalid repository name: %w", err)
		}
	}
	return nil
}
