package app

import (
	"path/filepath"
	"strings"
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

func TestWizardCatalogEmptyStateShowsButtonsInsteadOfAutoEditor(t *testing.T) {
	m := testConfigWizardModel(t)
	m.machine.Catalogs = nil
	m.machine.DefaultCatalog = ""
	m.step = stepCatalogs
	m.catalogEdit = nil
	m.focusTabs = false
	m.onStepChanged()

	if m.catalogEdit != nil {
		t.Fatal("expected no auto-open editor in empty catalog state")
	}
	if m.catalogFocus != catalogFocusButtons {
		t.Fatalf("catalog focus = %v, want buttons", m.catalogFocus)
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

func TestWizardCatalogEmptyStateDownFromTabsFocusesButtons(t *testing.T) {
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
	if m.catalogEdit != nil {
		t.Fatal("expected no editor to open when moving down into empty catalog content")
	}
	if m.catalogFocus != catalogFocusButtons {
		t.Fatalf("catalog focus = %v, want buttons", m.catalogFocus)
	}
}

func TestWizardCatalogEditorReplacesEmptyState(t *testing.T) {
	m := testConfigWizardModel(t)
	m.machine.Catalogs = nil
	m.machine.DefaultCatalog = ""
	m.step = stepCatalogs
	m.startCatalogAddEditor()

	view := m.viewCatalogs()
	if strings.Contains(view, "No catalogs configured yet") {
		t.Fatal("expected empty-state placeholder to be hidden while add editor is open")
	}
	if !strings.Contains(view, "Add catalog") {
		t.Fatal("expected add catalog editor content to be visible")
	}
}

func TestWizardCatalogButtonsCanOpenAddEditorAndContinue(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 1 // add
	m.onStepChanged()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.catalogEdit == nil || m.catalogEdit.mode != catalogEditorAdd {
		t.Fatal("expected Add button enter to open add editor")
	}

	m.catalogEdit = nil
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 6 // continue
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.step != stepReview {
		t.Fatalf("step = %v, want %v", m.step, stepReview)
	}
}

func TestWizardCatalogButtonsSetDefault(t *testing.T) {
	m := testConfigWizardModel(t)
	m.machine.Catalogs = append(m.machine.Catalogs, domain.Catalog{Name: "alt", Root: filepath.Join(t.TempDir(), "alt")})
	m.machine.DefaultCatalog = "software"
	m.rebuildCatalogRows()
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.onStepChanged()
	m.catalogTable.SetCursor(1)
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 2 // set default

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.machine.DefaultCatalog != "alt" {
		t.Fatalf("default catalog = %q, want %q", m.machine.DefaultCatalog, "alt")
	}
}

func TestWizardCatalogButtonsToggleDefaultBranchAutoPushPolicies(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 4 // toggle private
	m.onStepChanged()
	m.catalogTable.SetCursor(0)

	if !m.machine.Catalogs[0].AllowsDefaultBranchAutoPush(domain.VisibilityPrivate) {
		t.Fatal("expected private default policy to start as on")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.machine.Catalogs[0].AllowsDefaultBranchAutoPush(domain.VisibilityPrivate) {
		t.Fatal("expected private default policy to toggle off")
	}

	m.catalogBtn = 5 // toggle public
	if m.machine.Catalogs[0].AllowsDefaultBranchAutoPush(domain.VisibilityPublic) {
		t.Fatal("expected public default policy to start as off")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.machine.Catalogs[0].AllowsDefaultBranchAutoPush(domain.VisibilityPublic) {
		t.Fatal("expected public default policy to toggle on")
	}
}

func TestWizardCatalogButtonsToggleLayoutDepth(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 3 // toggle layout
	m.onStepChanged()
	m.catalogTable.SetCursor(0)

	if got := domain.EffectiveRepoPathDepth(m.machine.Catalogs[0]); got != 1 {
		t.Fatalf("initial depth = %d, want 1", got)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := domain.EffectiveRepoPathDepth(m.machine.Catalogs[0]); got != 2 {
		t.Fatalf("depth after toggle = %d, want 2", got)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := domain.EffectiveRepoPathDepth(m.machine.Catalogs[0]); got != 1 {
		t.Fatalf("depth after second toggle = %d, want 1", got)
	}
}

func TestWizardCatalogButtonsLeftRightWorkImmediately(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 0
	m.onStepChanged()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.catalogBtn != 1 {
		t.Fatalf("catalog button = %d, want 1 after right", m.catalogBtn)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.catalogBtn != 0 {
		t.Fatalf("catalog button = %d, want 0 after left", m.catalogBtn)
	}
}

func TestWizardCatalogButtonsDefaultToEditWhenCatalogsExist(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 99
	m.onStepChanged()

	if m.catalogBtn != 0 {
		t.Fatalf("catalog button = %d, want 0 (Edit)", m.catalogBtn)
	}
}

func TestWizardCatalogButtonsAllowChangingSelectedRowWithUpDown(t *testing.T) {
	m := testConfigWizardModel(t)
	m.machine.Catalogs = append(m.machine.Catalogs, domain.Catalog{Name: "alt", Root: filepath.Join(t.TempDir(), "alt")})
	m.machine.DefaultCatalog = "software"
	m.rebuildCatalogRows()
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 2 // set default
	m.catalogTable.SetCursor(1)

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.catalogTable.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0 after up", m.catalogTable.Cursor())
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.machine.DefaultCatalog != "software" {
		t.Fatalf("default catalog = %q, want %q", m.machine.DefaultCatalog, "software")
	}
}

func TestWizardCatalogEnterOnTableRowOpensEditor(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusButtons
	m.catalogBtn = 0 // edit
	m.onStepChanged()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.catalogEdit == nil || m.catalogEdit.mode != catalogEditorEditRoot {
		t.Fatal("expected Enter on selected catalog to open edit editor")
	}
}

func TestWizardCatalogEditorDeleteRequiresConfirmation(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusTable
	m.startCatalogEditRootEditor()
	if m.catalogEdit == nil {
		t.Fatal("expected edit editor")
	}
	if len(m.machine.Catalogs) != 1 {
		t.Fatalf("unexpected initial catalog count %d", len(m.machine.Catalogs))
	}

	// Move focus to Delete button (root input -> Save -> Delete).
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.catalogEdit == nil {
		t.Fatal("expected editor to stay open on first delete confirmation")
	}
	if !strings.Contains(m.catalogEdit.err, "confirm delete") {
		t.Fatalf("expected confirmation message, got %q", m.catalogEdit.err)
	}
	if len(m.machine.Catalogs) != 1 {
		t.Fatal("catalog should not be deleted on first confirmation enter")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.machine.Catalogs) != 0 {
		t.Fatal("expected catalog to be deleted on second confirmation enter")
	}
}

func TestWizardCatalogEditorActionButtonsNavigateWithLeftRight(t *testing.T) {
	m := testConfigWizardModel(t)
	m.step = stepCatalogs
	m.focusTabs = false
	m.catalogFocus = catalogFocusTable
	m.startCatalogEditRootEditor()
	if m.catalogEdit == nil {
		t.Fatal("expected edit editor")
	}

	// Move from input to Save action.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.catalogEdit.focus != 1 {
		t.Fatalf("focus = %d, want 1 (Save)", m.catalogEdit.focus)
	}
	// Left from Save should stay.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.catalogEdit.focus != 1 {
		t.Fatalf("focus = %d, want 1 after left at first action", m.catalogEdit.focus)
	}
	// Right to Delete then Cancel.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.catalogEdit.focus != 2 {
		t.Fatalf("focus = %d, want 2 (Delete)", m.catalogEdit.focus)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.catalogEdit.focus != 3 {
		t.Fatalf("focus = %d, want 3 (Cancel)", m.catalogEdit.focus)
	}
	// Right at end should stay.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.catalogEdit.focus != 3 {
		t.Fatalf("focus = %d, want 3 after right at last action", m.catalogEdit.focus)
	}
	// Left back to Delete.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.catalogEdit.focus != 2 {
		t.Fatalf("focus = %d, want 2 after left from cancel", m.catalogEdit.focus)
	}
}

func TestRenderEnumLineIsSingleLine(t *testing.T) {
	line := renderEnumLine("private", []string{"private", "public"})
	if strings.Contains(line, "\n") {
		t.Fatalf("enum line should render in one line, got %q", line)
	}
}

func TestWizardCatalogListShowsAddedCatalogValues(t *testing.T) {
	m := testConfigWizardModel(t)
	m.machine.Catalogs = nil
	m.machine.DefaultCatalog = ""
	m.step = stepCatalogs
	m.focusTabs = false
	m.onStepChanged()
	if m.catalogEdit != nil {
		t.Fatal("expected no auto-open editor")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.catalogEdit == nil {
		t.Fatal("expected add editor to open from Add button")
	}
	m.catalogEdit.inputs[0].SetValue("software")
	m.catalogEdit.inputs[1].SetValue("/tmp/software")
	m.catalogEdit.focus = 2
	m.updateCatalogEditorFocus()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	view := m.viewCatalogs()
	if !strings.Contains(view, "software") {
		t.Fatalf("expected catalog name in view, got %q", view)
	}
	if !strings.Contains(view, "/tmp/software") {
		t.Fatalf("expected catalog root in view, got %q", view)
	}
	if !strings.Contains(view, "1-level") {
		t.Fatalf("expected catalog layout depth in view, got %q", view)
	}
}
