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

func TestValidateFixApplyOptionsCreateProject(t *testing.T) {
	t.Parallel()

	err := validateFixApplyOptions(FixActionCreateProject, fixApplyOptions{
		CreateProjectName: "invalid name",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid create-project repository name")
	}
}
