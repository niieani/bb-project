package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"bb-project/internal/domain"
)

const minUsableInputWidth = 24

func TestConfigWizardInputsHaveMinimumWidth(t *testing.T) {
	t.Parallel()

	m := testConfigWizardModel(t)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 38})

	if got := m.githubOwnerInput.Width(); got < minUsableInputWidth {
		t.Fatalf("github owner input width = %d, want >= %d", got, minUsableInputWidth)
	}
	if got := m.schedulerInterval.Width(); got < minUsableInputWidth {
		t.Fatalf("scheduler interval input width = %d, want >= %d", got, minUsableInputWidth)
	}
	if got := m.notifyThrottle.Width(); got < minUsableInputWidth {
		t.Fatalf("notify throttle input width = %d, want >= %d", got, minUsableInputWidth)
	}

	m.startCatalogAddEditor()
	if got := m.catalogEdit.inputs[0].Width(); got < minUsableInputWidth {
		t.Fatalf("catalog add name input width = %d, want >= %d", got, minUsableInputWidth)
	}
	if got := m.catalogEdit.inputs[1].Width(); got < minUsableInputWidth {
		t.Fatalf("catalog add root input width = %d, want >= %d", got, minUsableInputWidth)
	}

	m.catalogEdit = nil
	m.startCatalogEditRootEditor()
	if m.catalogEdit == nil {
		t.Fatal("expected catalog edit editor")
	}
	if got := m.catalogEdit.inputs[0].Width(); got < minUsableInputWidth {
		t.Fatalf("catalog edit root input width = %d, want >= %d", got, minUsableInputWidth)
	}
}

func TestFixWizardInputsHaveMinimumWidth(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Branch:    "main",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL:       "https://github.com/you/api.git",
				PreferredRemote: "origin",
				AutoPush:        domain.AutoPushModeEnabled,
			},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "project-bootstrap",
				Path:      "/repos/project-bootstrap",
				OriginURL: "",
			},
			Meta: nil,
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "publish-branch",
				Path:      "/repos/publish-branch",
				OriginURL: "git@github.com:you/publish-branch.git",
				Branch:    "main",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL:       "https://github.com/you/publish-branch.git",
				PreferredRemote: "origin",
				AutoPush:        domain.AutoPushModeEnabled,
			},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 38})

	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})
	if got := m.wizard.CommitMessage.Width(); got < minUsableInputWidth {
		t.Fatalf("wizard commit input width = %d, want >= %d", got, minUsableInputWidth)
	}

	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/project-bootstrap", Action: FixActionCreateProject}})
	if got := m.wizard.ProjectName.Width(); got < minUsableInputWidth {
		t.Fatalf("wizard project-name input width = %d, want >= %d", got, minUsableInputWidth)
	}

	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/publish-branch", Action: FixActionPublishNewBranch}})
	if !m.wizard.EnableForkBranchRename {
		t.Fatal("expected publish-branch input to be enabled")
	}
	if got := m.wizard.ForkBranchName.Width(); got < minUsableInputWidth {
		t.Fatalf("wizard publish-branch input width = %d, want >= %d", got, minUsableInputWidth)
	}
}
