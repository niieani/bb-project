package app

import (
	"testing"

	"bb-project/internal/state"
)

func TestGitHubRemoteURLUsesTemplate(t *testing.T) {
	t.Parallel()

	got, err := githubRemoteURL(
		"niieani",
		"bb-project",
		"https",
		"git@${org}.github.com:${org}/${repo}.git",
	)
	if err != nil {
		t.Fatalf("githubRemoteURL returned error: %v", err)
	}
	if got != "git@niieani.github.com:niieani/bb-project.git" {
		t.Fatalf("githubRemoteURL = %q, want %q", got, "git@niieani.github.com:niieani/bb-project.git")
	}
}

func TestParseCloneRepoSpecUsesTemplateForGitHubInputs(t *testing.T) {
	t.Parallel()

	cfg := state.DefaultConfig()
	cfg.GitHub.PreferredRemoteURLTemplate = "git@${org}.github.com:${org}/${repo}.git"

	spec, err := parseCloneRepoSpec(cfg, "openai/codex", nil)
	if err != nil {
		t.Fatalf("parseCloneRepoSpec returned error: %v", err)
	}
	if spec.CloneURL != "git@openai.github.com:openai/codex.git" {
		t.Fatalf("clone URL = %q, want %q", spec.CloneURL, "git@openai.github.com:openai/codex.git")
	}
}
