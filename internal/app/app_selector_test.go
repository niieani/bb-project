package app

import (
	"strings"
	"testing"

	"bb-project/internal/domain"
)

func TestSelectRepoMetadataIndex(t *testing.T) {
	t.Parallel()

	repos := []domain.RepoMetadataFile{
		{RepoKey: "software/api-a", RepoID: "github.com/you/api", Name: "api-a"},
		{RepoKey: "software/api-b", RepoID: "github.com/you/api", Name: "api-b"},
		{RepoKey: "software/web", RepoID: "github.com/you/web", Name: "web"},
	}

	idx, err := selectRepoMetadataIndex(repos, "software/web")
	if err != nil {
		t.Fatalf("select by repo_key returned error: %v", err)
	}
	if idx != 2 {
		t.Fatalf("index by repo_key = %d, want 2", idx)
	}

	idx, err = selectRepoMetadataIndex(repos, "github.com/you/web")
	if err != nil {
		t.Fatalf("select by repo_id returned error: %v", err)
	}
	if idx != 2 {
		t.Fatalf("index by repo_id = %d, want 2", idx)
	}

	_, err = selectRepoMetadataIndex(repos, "github.com/you/api")
	if err == nil {
		t.Fatal("expected ambiguous selector error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}
