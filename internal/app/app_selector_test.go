package app

import (
	"strings"
	"testing"

	"bb-project/internal/domain"
)

func TestSelectRepoMetadataIndex(t *testing.T) {
	t.Parallel()

	repos := []domain.RepoMetadataFile{
		{RepoKey: "software/api-a", OriginURL: "https://github.com/you/api.git", Name: "api"},
		{RepoKey: "software/api-b", OriginURL: "https://github.com/you/api.git", Name: "api"},
		{RepoKey: "software/web", OriginURL: "https://github.com/you/web.git", Name: "web"},
	}

	idx, err := selectRepoMetadataIndex(repos, "software/web")
	if err != nil {
		t.Fatalf("select by repo_key returned error: %v", err)
	}
	if idx != 2 {
		t.Fatalf("index by repo_key = %d, want 2", idx)
	}

	idx, err = selectRepoMetadataIndex(repos, "web")
	if err != nil {
		t.Fatalf("select by name returned error: %v", err)
	}
	if idx != 2 {
		t.Fatalf("index by name = %d, want 2", idx)
	}

	_, err = selectRepoMetadataIndex(repos, "api")
	if err == nil {
		t.Fatal("expected ambiguous selector error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}
