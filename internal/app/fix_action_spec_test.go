package app

import (
	"strings"
	"testing"

	"bb-project/internal/domain"
)

func TestFixActionSpecsHaveCoreMetadata(t *testing.T) {
	t.Parallel()

	actions := []string{
		FixActionIgnore,
		FixActionAbortOperation,
		FixActionCreateProject,
		FixActionForkAndRetarget,
		FixActionPush,
		FixActionStageCommitPush,
		FixActionPullFFOnly,
		FixActionSetUpstreamPush,
		FixActionEnableAutoPush,
	}

	for _, action := range actions {
		action := action
		t.Run(action, func(t *testing.T) {
			t.Parallel()

			spec, ok := fixActionSpecFor(action)
			if !ok {
				t.Fatalf("missing action spec for %q", action)
			}
			if strings.TrimSpace(spec.Label) == "" {
				t.Fatalf("spec label missing for %q", action)
			}
			if strings.TrimSpace(spec.Description) == "" {
				t.Fatalf("spec description missing for %q", action)
			}
			if spec.BuildPlan == nil {
				t.Fatalf("spec plan builder missing for %q", action)
			}
		})
	}
}

func TestFixActionRiskUsesSharedSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action string
		risky  bool
	}{
		{action: FixActionPush, risky: true},
		{action: FixActionStageCommitPush, risky: true},
		{action: FixActionPullFFOnly, risky: false},
		{action: FixActionEnableAutoPush, risky: false},
		{action: "unknown-action", risky: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.action, func(t *testing.T) {
			t.Parallel()
			if got := isRiskyFixAction(tt.action); got != tt.risky {
				t.Fatalf("isRiskyFixAction(%q) = %t, want %t", tt.action, got, tt.risky)
			}
		})
	}
}

func TestFixActionPlanStageCommitPushIncludesCommandsAndEffects(t *testing.T) {
	t.Parallel()

	plan := fixActionPlanFor(FixActionStageCommitPush, fixActionPlanContext{
		Branch:               "main",
		Upstream:             "",
		OriginURL:            "git@github.com:you/api.git",
		CommitMessage:        "auto",
		GenerateGitignore:    true,
		GitignorePatterns:    []string{"node_modules/"},
		MissingRootGitignore: true,
	})

	if len(plan) < 4 {
		t.Fatalf("stage-commit-push plan entries = %d, want >= 4", len(plan))
	}
	if !planContains(plan, true, "git add -A") {
		t.Fatalf("expected add command in plan, got %#v", plan)
	}
	if !planContains(plan, true, "git commit -m") {
		t.Fatalf("expected commit command in plan, got %#v", plan)
	}
	if !planContains(plan, true, "git push -u") {
		t.Fatalf("expected upstream push command in plan, got %#v", plan)
	}
	if !planContains(plan, false, "Generate root .gitignore") {
		t.Fatalf("expected gitignore generation effect in plan, got %#v", plan)
	}
}

func TestFixActionPlanCreateProjectIncludesGhAndMetadataWrite(t *testing.T) {
	t.Parallel()

	plan := fixActionPlanFor(FixActionCreateProject, fixActionPlanContext{
		Branch:                  "main",
		Upstream:                "",
		OriginURL:               "",
		PreferredRemote:         "origin",
		CreateProjectName:       "api",
		CreateProjectVisibility: domain.VisibilityPublic,
	})

	if !planContains(plan, true, "gh repo create") {
		t.Fatalf("expected gh create command in plan, got %#v", plan)
	}
	if !planContains(plan, true, "git remote add origin") {
		t.Fatalf("expected origin add command in plan, got %#v", plan)
	}
	if !planContains(plan, false, "Write/update repo metadata") {
		t.Fatalf("expected metadata write effect in plan, got %#v", plan)
	}
}

func planContains(plan []fixActionPlanEntry, command bool, fragment string) bool {
	for _, entry := range plan {
		if entry.Command != command {
			continue
		}
		if strings.Contains(entry.Summary, fragment) {
			return true
		}
	}
	return false
}
