package app

import (
	"testing"

	"bb-project/internal/domain"
)

func TestEligibleFixActions(t *testing.T) {
	t.Parallel()

	base := domain.MachineRepoRecord{
		Name:                "api",
		Path:                "/tmp/api",
		RepoID:              "github.com/you/api",
		OriginURL:           "git@github.com:you/api.git",
		Branch:              "main",
		Upstream:            "origin/main",
		OperationInProgress: domain.OperationNone,
	}

	tests := []struct {
		name    string
		rec     domain.MachineRepoRecord
		meta    *domain.RepoMetadataFile
		actions []string
	}{
		{
			name:    "operation in progress only allows abort",
			rec:     func() domain.MachineRepoRecord { r := base; r.OperationInProgress = domain.OperationRebase; return r }(),
			actions: []string{FixActionAbortOperation},
		},
		{
			name:    "ahead allows push",
			rec:     func() domain.MachineRepoRecord { r := base; r.Ahead = 1; return r }(),
			actions: []string{FixActionPush},
		},
		{
			name:    "dirty allows stage commit push",
			rec:     func() domain.MachineRepoRecord { r := base; r.HasDirtyTracked = true; return r }(),
			actions: []string{FixActionStageCommitPush},
		},
		{
			name:    "behind allows pull ff only",
			rec:     func() domain.MachineRepoRecord { r := base; r.Behind = 2; return r }(),
			actions: []string{FixActionPullFFOnly},
		},
		{
			name:    "missing upstream allows set upstream push",
			rec:     func() domain.MachineRepoRecord { r := base; r.Upstream = ""; r.Ahead = 2; return r }(),
			actions: []string{FixActionSetUpstreamPush},
		},
		{
			name:    "auto push disabled allows enable action",
			rec:     base,
			meta:    &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
			actions: []string{FixActionEnableAutoPush},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := eligibleFixActions(tt.rec, tt.meta)
			for _, want := range tt.actions {
				if !containsAction(got, want) {
					t.Fatalf("expected action %q in %v", want, got)
				}
			}
		})
	}
}

func TestResolveFixTarget(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{Record: domain.MachineRepoRecord{Name: "api", Path: "/repos/api", RepoID: "github.com/you/api"}},
		{Record: domain.MachineRepoRecord{Name: "web", Path: "/repos/web", RepoID: "github.com/you/web"}},
	}

	got, err := resolveFixTarget("/repos/api", repos)
	if err != nil {
		t.Fatalf("resolve by path failed: %v", err)
	}
	if got.Record.Name != "api" {
		t.Fatalf("resolved name = %q, want api", got.Record.Name)
	}

	got, err = resolveFixTarget("github.com/you/web", repos)
	if err != nil {
		t.Fatalf("resolve by repo_id failed: %v", err)
	}
	if got.Record.Name != "web" {
		t.Fatalf("resolved name = %q, want web", got.Record.Name)
	}

	got, err = resolveFixTarget("api", repos)
	if err != nil {
		t.Fatalf("resolve by name failed: %v", err)
	}
	if got.Record.Path != "/repos/api" {
		t.Fatalf("resolved path = %q, want /repos/api", got.Record.Path)
	}
}
