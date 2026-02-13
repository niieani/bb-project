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
	m.githubOwnerInput.SetValue("ali")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if got := m.githubOwnerInput.Value(); got != "alin" {
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
	if !m.focusTabs {
		t.Fatal("expected tabs to remain focused after switching step with right arrow")
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

func TestWizardGitHubEnumsAreSelectionFields(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepGitHub
	m.focusTabs = false
	m.githubFocus = 1
	m.updateGitHubFocus()

	m.config.GitHub.DefaultVisibility = "private"
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.config.GitHub.DefaultVisibility != "public" {
		t.Fatalf("default visibility = %q, want public", m.config.GitHub.DefaultVisibility)
	}
	if m.step != stepGitHub {
		t.Fatalf("step changed unexpectedly: %v", m.step)
	}

	m.githubFocus = 2
	m.updateGitHubFocus()
	m.config.GitHub.RemoteProtocol = "ssh"
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.config.GitHub.RemoteProtocol != "https" {
		t.Fatalf("remote protocol = %q, want https", m.config.GitHub.RemoteProtocol)
	}
}

func TestWizardGitHubEnumsCycleWithLeftRight(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepGitHub
	m.focusTabs = false
	m.githubFocus = 1
	m.updateGitHubFocus()
	m.config.GitHub.DefaultVisibility = "private"

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.config.GitHub.DefaultVisibility != "public" {
		t.Fatalf("default visibility = %q, want public", m.config.GitHub.DefaultVisibility)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.config.GitHub.DefaultVisibility != "private" {
		t.Fatalf("default visibility = %q, want private", m.config.GitHub.DefaultVisibility)
	}

	m.githubFocus = 2
	m.config.GitHub.RemoteProtocol = "ssh"
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.config.GitHub.RemoteProtocol != "https" {
		t.Fatalf("remote protocol = %q, want https", m.config.GitHub.RemoteProtocol)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.config.GitHub.RemoteProtocol != "ssh" {
		t.Fatalf("remote protocol = %q, want ssh", m.config.GitHub.RemoteProtocol)
	}
}

func TestWizardTabsFocusedCanSwitchAcrossMultipleSteps(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepGitHub
	m.focusTabs = true
	m.updateGitHubFocus()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.step != stepSync || !m.focusTabs {
		t.Fatalf("after first right: step=%v focusTabs=%v", m.step, m.focusTabs)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.step != stepNotify || !m.focusTabs {
		t.Fatalf("after second right: step=%v focusTabs=%v", m.step, m.focusTabs)
	}
}

func TestWizardCatalogEmptyStateStartsAddEditor(t *testing.T) {
	m := testConfigWizardModel(t)
	m.machine.Catalogs = nil
	m.machine.DefaultCatalog = ""
	m.step = stepCatalogs
	m.catalogEdit = nil
	m.focusTabs = false
	m.onStepChanged()

	if m.catalogEdit == nil {
		t.Fatal("expected add-catalog editor to auto-open for empty catalog state")
	}
	if m.catalogEdit.mode != catalogEditorAdd {
		t.Fatalf("catalog editor mode = %v, want %v", m.catalogEdit.mode, catalogEditorAdd)
	}
}

func TestWizardCatalogEditorUsesUpDownAndCanReturnToTabs(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.startCatalogAddEditor()

	if m.catalogEdit == nil {
		t.Fatal("expected editor to be present")
	}
	if m.catalogEdit.focus != 0 {
		t.Fatalf("initial focus = %d, want 0", m.catalogEdit.focus)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.catalogEdit == nil || m.catalogEdit.focus != 1 {
		t.Fatalf("focus after down = %v, want 1", m.catalogEdit.focus)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.catalogEdit == nil || m.catalogEdit.focus != 0 {
		t.Fatalf("focus after up = %v, want 0", m.catalogEdit.focus)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.catalogEdit != nil {
		t.Fatal("expected editor to close when pressing up on first field")
	}
	if !m.focusTabs {
		t.Fatal("expected tabs to become focused after leaving editor from first field")
	}
}

func TestWizardReviewSpaceTogglesCreateMissingRoots(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepReview
	m.focusTabs = false
	m.createMissingRoots = true
	m.machine.Catalogs = append(m.machine.Catalogs, domain.Catalog{Name: "missing", Root: filepath.Join(t.TempDir(), "does-not-exist")})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.createMissingRoots {
		t.Fatal("expected space to toggle createMissingRoots off")
	}
}

func TestWizardCatalogUpOnFirstRowFocusesTabs(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.onStepChanged()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if !m.focusTabs {
		t.Fatal("expected up on first catalog row to focus tabs")
	}
}

func TestWizardCatalogEmptyStateDownFromTabsOpensEditor(t *testing.T) {
	m := testConfigWizardModel(t)
	m.machine.Catalogs = nil
	m.machine.DefaultCatalog = ""
	m.step = stepCatalogs
	m.focusTabs = true
	m.catalogEdit = nil
	m.onStepChanged()

	if m.catalogEdit != nil {
		t.Fatal("expected no editor while tabs are focused")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.catalogEdit == nil {
		t.Fatal("expected add editor to open when moving down into empty catalog content")
	}
}
