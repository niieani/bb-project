package app

import (
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func testConfigWizardModel(t *testing.T) *configWizardModel {
	t.Helper()
	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "alice"
	machine := state.BootstrapMachine("machine-a", "host-a", time.Date(2026, 2, 13, 20, 31, 0, 0, time.UTC))
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: filepath.Join(t.TempDir(), "software")}}
	machine.DefaultCatalog = "software"
	return newConfigWizardModel(ConfigWizardInput{
		Config:      cfg,
		Machine:     machine,
		ConfigPath:  "/tmp/config.yaml",
		MachinePath: "/tmp/machine.yaml",
	})
}

func TestWizardAllowsTypingNInGitHubOwner(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepGitHub
	m.focusTabs = false
	m.githubFocus = 0
	m.updateGitHubFocus()
	m.githubInputs[0].SetValue("ali")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if got := m.githubInputs[0].Value(); got != "alin" {
		t.Fatalf("owner input = %q, want %q", got, "alin")
	}
	if m.step != stepGitHub {
		t.Fatalf("step changed unexpectedly: %v", m.step)
	}
}

func TestWizardLeftArrowDoesNotChangeStepWhenEditingField(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepGitHub
	m.focusTabs = false
	m.githubFocus = 0
	m.updateGitHubFocus()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})

	if m.step != stepGitHub {
		t.Fatalf("step changed unexpectedly: %v", m.step)
	}
}

func TestWizardRightArrowChangesStepWhenTabsFocused(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepGitHub
	m.focusTabs = true
	m.updateGitHubFocus()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	if m.step != stepSync {
		t.Fatalf("step = %v, want %v", m.step, stepSync)
	}
}

func TestWizardSyncSpaceTogglesAndEnterAdvances(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepSync
	m.focusTabs = false
	beforeToggle := m.config.Sync.AutoDiscover

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.config.Sync.AutoDiscover == beforeToggle {
		t.Fatal("expected space to toggle sync option")
	}
	if m.step != stepSync {
		t.Fatalf("step changed unexpectedly on space: %v", m.step)
	}

	current := m.config.Sync.AutoDiscover
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != stepNotify {
		t.Fatalf("step = %v, want %v", m.step, stepNotify)
	}
	if m.config.Sync.AutoDiscover != current {
		t.Fatal("enter should not toggle current option")
	}
}

func TestWizardUpFromFirstFieldFocusesTabsThenRightAdvances(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepGitHub
	m.focusTabs = false
	m.githubFocus = 0
	m.updateGitHubFocus()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if !m.focusTabs {
		t.Fatal("expected tabs to become focused")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.step != stepSync {
		t.Fatalf("step = %v, want %v", m.step, stepSync)
	}
}
