package app

import "testing"

func TestValidateGitHubRepositoryName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		repoName  string
		shouldErr bool
	}{
		{name: "simple", repoName: "repo", shouldErr: false},
		{name: "with dash underscore dot", repoName: "repo-name_v2.1", shouldErr: false},
		{name: "uppercase is invalid", repoName: "RepoName", shouldErr: true},
		{name: "contains space", repoName: "repo name", shouldErr: true},
		{name: "contains slash", repoName: "owner/repo", shouldErr: true},
		{name: "empty", repoName: "", shouldErr: true},
		{name: "dot", repoName: ".", shouldErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateGitHubRepositoryName(tt.repoName)
			if tt.shouldErr && err == nil {
				t.Fatalf("expected error for %q", tt.repoName)
			}
			if !tt.shouldErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.repoName, err)
			}
		})
	}
}

func TestSanitizeGitHubRepositoryNameInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "spaces become dashes", raw: "my repo name", want: "my-repo-name"},
		{name: "invalid run collapses", raw: "repo///***name", want: "repo-name"},
		{name: "trim separators", raw: "  ///repo//  ", want: "repo"},
		{name: "keep allowed punctuation", raw: "repo.name_v2", want: "repo.name_v2"},
		{name: "uppercase becomes lowercase", raw: "RepoName", want: "reponame"},
		{name: "keep trailing dash", raw: "repo-", want: "repo-"},
		{name: "keep leading dash", raw: "-repo", want: "-repo"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeGitHubRepositoryNameInput(tt.raw); got != tt.want {
				t.Fatalf("sanitizeGitHubRepositoryNameInput(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestValidateFixApplyOptionsCreateProject(t *testing.T) {
	t.Parallel()

	err := validateFixApplyOptions(FixActionCreateProject, fixApplyOptions{
		CreateProjectName: "invalid name",
	})
	if err != nil {
		t.Fatalf("expected sanitizable create-project name to be accepted, got %v", err)
	}

	err = validateFixApplyOptions(FixActionCreateProject, fixApplyOptions{
		CreateProjectName: "###",
	})
	if err == nil {
		t.Fatal("expected validation error for unsanitizable create-project repository name")
	}

	err = validateFixApplyOptions(FixActionCreateProject, fixApplyOptions{
		GenerateGitignore: true,
		GitignorePatterns: []string{"node_modules/"},
	})
	if err == nil {
		t.Fatal("expected validation error when gitignore generation is requested for non-commit action")
	}

	err = validateFixApplyOptions(FixActionStageCommitPush, fixApplyOptions{
		GenerateGitignore: true,
		GitignorePatterns: []string{"node_modules/"},
	})
	if err != nil {
		t.Fatalf("expected gitignore generation to be allowed for stage-commit-push, got %v", err)
	}

	err = validateFixApplyOptions(FixActionCheckpointThenSync, fixApplyOptions{
		GenerateGitignore: true,
		GitignorePatterns: []string{"node_modules/"},
	})
	if err != nil {
		t.Fatalf("expected gitignore generation to be allowed for checkpoint-then-sync, got %v", err)
	}

	err = validateFixApplyOptions(FixActionPush, fixApplyOptions{
		SyncStrategy: FixSyncStrategy("squash"),
	})
	if err == nil {
		t.Fatal("expected validation error for invalid sync strategy")
	}

	err = validateFixApplyOptions(FixActionPush, fixApplyOptions{
		SyncStrategy: FixSyncStrategyMerge,
	})
	if err != nil {
		t.Fatalf("expected merge sync strategy to be valid, got %v", err)
	}

	err = validateFixApplyOptions(FixActionForkAndRetarget, fixApplyOptions{
		ForkBranchRenameTo: "feature/fork-target",
	})
	if err != nil {
		t.Fatalf("expected branch rename target to be valid for fork-and-retarget, got %v", err)
	}

	err = validateFixApplyOptions(FixActionPush, fixApplyOptions{
		ForkBranchRenameTo: "feature/fork-target",
	})
	if err == nil {
		t.Fatal("expected validation error when branch rename target is set for non-fork action")
	}

	err = validateFixApplyOptions(FixActionForkAndRetarget, fixApplyOptions{
		ForkBranchRenameTo: "feature fork target",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid branch rename target")
	}
}
