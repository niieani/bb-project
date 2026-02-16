package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"bb-project/internal/domain"
)

func TestFixTUIWizardCreateProjectShowsStageCommitToggleEnabledByDefault(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				RepoKey:         "software/demo",
				Name:            "demo",
				Path:            "/repos/demo",
				Catalog:         "software",
				OriginURL:       "",
				Branch:          "main",
				HasDirtyTracked: true,
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				ChangedFiles: []fixChangedFile{
					{Path: "README.md", Status: "modified", Added: 1},
				},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/demo", Action: FixActionCreateProject}})
	m.wizard.GitHubOwner = "you"

	if !m.wizard.EnableCreateProjectStageCommit {
		t.Fatal("expected create-project stage/commit toggle to be enabled")
	}
	if !m.wizard.CreateProjectStageCommit {
		t.Fatal("expected create-project stage/commit toggle to default to enabled")
	}
	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "Stage & commit before initial push") {
		t.Fatalf("expected create-project stage/commit toggle in wizard view, got %q", view)
	}

	plan := m.wizardApplyPlanEntries()
	if !planContains(plan, true, "git add -A") {
		t.Fatalf("expected create-project plan to include stage-all when toggle is enabled, got %#v", plan)
	}
	if !planContains(plan, true, "git commit -m") {
		t.Fatalf("expected create-project plan to include commit when toggle is enabled, got %#v", plan)
	}
}

func TestFixTUIWizardCreateProjectToggleCanDisableStageCommitSteps(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				RepoKey:         "software/demo",
				Name:            "demo",
				Path:            "/repos/demo",
				Catalog:         "software",
				OriginURL:       "",
				Branch:          "main",
				HasDirtyTracked: true,
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				ChangedFiles: []fixChangedFile{
					{Path: "README.md", Status: "modified", Added: 1},
				},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/demo", Action: FixActionCreateProject}})
	m.wizard.GitHubOwner = "you"
	m.wizard.FocusArea = fixWizardFocusVisibility

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.wizard.CreateProjectStageCommit {
		t.Fatal("expected create-project stage/commit toggle to switch off on space")
	}

	plan := m.wizardApplyPlanEntries()
	if planContains(plan, true, "git add -A") {
		t.Fatalf("did not expect create-project plan to include stage-all when toggle is disabled, got %#v", plan)
	}
	if planContains(plan, true, "git commit -m") {
		t.Fatalf("did not expect create-project plan to include commit when toggle is disabled, got %#v", plan)
	}
}

func TestFixTUIWizardStashSupportsStagedOnlyAndStagedPlusUnstagedModes(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				RepoKey:         "software/demo",
				Name:            "demo",
				Path:            "/repos/demo",
				Catalog:         "software",
				OriginURL:       "git@github.com:you/demo.git",
				Branch:          "main",
				HasDirtyTracked: true,
				HasUntracked:    true,
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:  "software/demo",
				OriginURL: "https://github.com/you/demo.git",
				AutoPush: domain.AutoPushModeEnabled,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/demo", Action: FixActionStash}})

	if !m.wizard.EnableStashMode {
		t.Fatal("expected stash mode control to be enabled")
	}
	if !m.wizard.StashIncludeUnstaged {
		t.Fatal("expected stash mode to default to staged + unstaged")
	}
	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "Stash mode") {
		t.Fatalf("expected stash mode field in wizard view, got %q", view)
	}
	if !strings.Contains(view, "Staged + unstaged") {
		t.Fatalf("expected staged+unstaged option in stash mode field, got %q", view)
	}

	stageAllPlan := m.wizardApplyPlanEntries()
	if !planContains(stageAllPlan, true, "git add -A") {
		t.Fatalf("expected staged+unstaged stash plan to include stage-all, got %#v", stageAllPlan)
	}
	if !planContains(stageAllPlan, true, "git stash push --staged -m") {
		t.Fatalf("expected stash command in staged+unstaged plan, got %#v", stageAllPlan)
	}

	m.wizard.FocusArea = fixWizardFocusStashMode
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.wizard.StashIncludeUnstaged {
		t.Fatal("expected stash mode to switch to staged-only on right key")
	}

	stagedOnlyPlan := m.wizardApplyPlanEntries()
	if planContains(stagedOnlyPlan, true, "git add -A") {
		t.Fatalf("did not expect staged-only stash plan to include stage-all, got %#v", stagedOnlyPlan)
	}
	if !planContains(stagedOnlyPlan, true, "git stash push --staged -m") {
		t.Fatalf("expected stash command in staged-only plan, got %#v", stagedOnlyPlan)
	}
}

func TestFixTUISummaryShowsCreatedCommitsWithMessages(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:     "demo",
				Path:     "/repos/demo",
				Syncable: true,
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:  "software/demo",
				OriginURL: "https://github.com/you/demo.git",
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.summaryResults = []fixSummaryResult{
		{
			RepoName: "demo",
			RepoPath: "/repos/demo",
			Action:   fixActionLabel(FixActionStageCommitPush),
			Status:   "applied",
			Commits: []fixCreatedCommit{
				{SHA: "abcdef1234567890", Message: "feat: generated commit subject"},
			},
		},
	}
	m.viewMode = fixViewSummary

	view := ansi.Strip(m.viewSummaryContent())
	if !strings.Contains(view, "Commits created") {
		t.Fatalf("expected commits-created section in summary, got %q", view)
	}
	if !strings.Contains(view, "abcdef1 feat: generated commit subject") {
		t.Fatalf("expected created commit SHA+subject in summary, got %q", view)
	}
}
