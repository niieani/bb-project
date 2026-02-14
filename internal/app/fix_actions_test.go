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
		ctx     fixEligibilityContext
		actions []string
	}{
		{
			name:    "operation in progress only allows abort",
			rec:     func() domain.MachineRepoRecord { r := base; r.OperationInProgress = domain.OperationRebase; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionAbortOperation},
		},
		{
			name:    "ahead allows push",
			rec:     func() domain.MachineRepoRecord { r := base; r.Ahead = 1; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionPush},
		},
		{
			name:    "dirty allows stage commit push",
			rec:     func() domain.MachineRepoRecord { r := base; r.HasDirtyTracked = true; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionStageCommitPush},
		},
		{
			name:    "behind allows pull ff only",
			rec:     func() domain.MachineRepoRecord { r := base; r.Behind = 2; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionPullFFOnly},
		},
		{
			name:    "missing upstream allows set upstream push",
			rec:     func() domain.MachineRepoRecord { r := base; r.Upstream = ""; r.Ahead = 2; return r }(),
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionCreateProject, FixActionSetUpstreamPush},
		},
		{
			name:    "auto push disabled allows enable action",
			rec:     base,
			meta:    &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
			ctx:     fixEligibilityContext{},
			actions: []string{FixActionEnableAutoPush},
		},
		{
			name: "secret-like uncommitted files block stage commit push",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				return r
			}(),
			ctx: fixEligibilityContext{
				Interactive: false,
				Risk: fixRiskSnapshot{
					SecretLikeChangedPaths: []string{".env"},
				},
			},
			actions: []string{},
		},
		{
			name: "noisy paths with missing root gitignore block stage commit push in non-interactive mode",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				return r
			}(),
			ctx: fixEligibilityContext{
				Interactive: false,
				Risk: fixRiskSnapshot{
					MissingRootGitignore: true,
					NoisyChangedPaths:    []string{"node_modules/pkg/index.js"},
				},
			},
			actions: []string{},
		},
		{
			name: "noisy paths do not block stage commit push in interactive mode",
			rec: func() domain.MachineRepoRecord {
				r := base
				r.HasDirtyTracked = true
				return r
			}(),
			ctx: fixEligibilityContext{
				Interactive: true,
				Risk: fixRiskSnapshot{
					MissingRootGitignore: true,
					NoisyChangedPaths:    []string{"node_modules/pkg/index.js"},
				},
			},
			actions: []string{FixActionStageCommitPush},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := eligibleFixActions(tt.rec, tt.meta, tt.ctx)
			for _, want := range tt.actions {
				if !containsAction(got, want) {
					t.Fatalf("expected action %q in %v", want, got)
				}
			}
			if len(tt.actions) == 0 && containsAction(got, FixActionStageCommitPush) {
				t.Fatalf("did not expect %q in %v", FixActionStageCommitPush, got)
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
