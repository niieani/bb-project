package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"bb-project/internal/domain"
)

func TestFixTUICycleSelectionPerRow(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				RepoID:    "github.com/you/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/web", AutoPush: true},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	firstRepoAction := actionForVisibleRepo(m, 0)

	m.setCursor(1)
	m.cycleCurrentAction(0)
	secondRepoAction := actionForVisibleRepo(m, 1)

	m.setCursor(0)
	m.cycleCurrentAction(0)
	if got := actionForVisibleRepo(m, 0); got != firstRepoAction {
		t.Fatalf("row 0 action changed unexpectedly: got %q want %q", got, firstRepoAction)
	}
	if got := actionForVisibleRepo(m, 1); got != secondRepoAction {
		t.Fatalf("row 1 action changed unexpectedly: got %q want %q", got, secondRepoAction)
	}
}

func TestFixTUISelectionFallbackAfterEligibilityChange(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	m.cycleCurrentAction(1)
	before := actionForVisibleRepo(m, 0)
	if !strings.Contains(before, FixActionEnableAutoPush) {
		t.Fatalf("expected second cycle to pick %q, got %q", FixActionEnableAutoPush, before)
	}

	m.repos[0].Record = domain.MachineRepoRecord{
		Name:      "api",
		Path:      "/repos/api",
		RepoID:    "github.com/you/api",
		OriginURL: "git@github.com:you/api.git",
		Upstream:  "origin/main",
		Ahead:     1,
	}
	m.repos[0].Meta = &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true}
	m.rebuildList("/repos/api")

	if got := actionForVisibleRepo(m, 0); !strings.Contains(got, fixNoAction) {
		t.Fatalf("fallback action = %q, want default no-op", got)
	}
}

func newFixTUIModelForTest(repos []fixRepoState) *fixTUIModel {
	m := &fixTUIModel{
		repos:          append([]fixRepoState(nil), repos...),
		ignored:        map[string]bool{},
		selectedAction: map[string]int{},
		repoList:       newFixRepoListModel(),
		keys:           defaultFixTUIKeyMap(),
		help:           help.New(),
	}
	m.rebuildList("")
	return m
}

func actionForVisibleRepo(m *fixTUIModel, idx int) string {
	repo := m.visible[idx]
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})
	return m.currentActionForRepo(repo.Record.Path, actions)
}

func TestFixTUIViewUsesCanonicalChromeWithoutInlineKeyLegend(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	view := m.View()

	if !strings.Contains(view, "bb") || !strings.Contains(view, "fix") {
		t.Fatalf("expected canonical header in view, got %q", view)
	}
	if strings.Contains(view, "Use ←/→") {
		t.Fatal("expected no inline key legend in body")
	}
}

func TestFixTUIDefaultSelectionIsNoAction(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)

	if got := actionForVisibleRepo(m, 0); !strings.Contains(got, fixNoAction) {
		t.Fatalf("selected fix = %q, want no-op '-'", got)
	}
}

func TestFixTUIActionCycleIncludesNoAction(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)

	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); strings.Contains(got, fixNoAction) {
		t.Fatalf("expected first cycle to move off no-op, got %q", got)
	}

	m.cycleCurrentAction(-1)
	if got := actionForVisibleRepo(m, 0); !strings.Contains(got, fixNoAction) {
		t.Fatalf("expected cycle back to no-op, got %q", got)
	}
}

func TestFixTUIViewportTracksCursor(t *testing.T) {
	t.Parallel()

	repos := make([]fixRepoState, 0, 40)
	for i := 0; i < 40; i++ {
		repos = append(repos, fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      fmt.Sprintf("repo-%02d", i),
				Path:      fmt.Sprintf("/repos/repo-%02d", i),
				RepoID:    fmt.Sprintf("github.com/you/repo-%02d", i),
				OriginURL: "git@github.com:you/repo.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: fmt.Sprintf("github.com/you/repo-%02d", i), AutoPush: false},
		})
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 26})
	m.setCursor(35)

	view := m.repoList.View()
	if !strings.Contains(view, "repo-35") {
		t.Fatalf("expected viewport to include selected cursor row, got %q", view)
	}
}

func TestFixTUIApplyAllSkipsNoOpSelections(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.applyAllSelections()

	if !strings.Contains(m.status, "applied 0") {
		t.Fatalf("status = %q, expected skipped apply-all summary", m.status)
	}
}

func TestFixTUIRiskySelectionOpensConfirmationWizard(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1) // push
	m.applyCurrentSelection()

	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want wizard mode", m.viewMode)
	}
	if m.wizard.Action != FixActionPush {
		t.Fatalf("wizard action = %q, want %q", m.wizard.Action, FixActionPush)
	}
}

func TestFixTUIWizardSkipShowsSummary(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	m.skipWizardCurrent()

	if m.viewMode != fixViewSummary {
		t.Fatalf("view mode = %v, want summary mode", m.viewMode)
	}
	if len(m.summaryResults) != 1 {
		t.Fatalf("summary result count = %d, want 1", len(m.summaryResults))
	}
	if got := m.summaryResults[0].Status; got != "skipped" {
		t.Fatalf("summary status = %q, want skipped", got)
	}
}

func TestFixTUIWizardCommitInputStartsEmptyWithPlaceholder(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	if got := m.wizard.CommitMessage.Value(); got != "" {
		t.Fatalf("wizard commit message initial value = %q, want empty", got)
	}
	if got := m.wizard.CommitMessage.Placeholder; got != DefaultFixCommitMessage {
		t.Fatalf("wizard commit message placeholder = %q, want %q", got, DefaultFixCommitMessage)
	}
}

func TestFixTUIWizardViewShowsActionButtons(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := m.viewWizardContent()
	if !strings.Contains(view, "Apply") || !strings.Contains(view, "Skip") || !strings.Contains(view, "Cancel") {
		t.Fatalf("wizard view should render action buttons, got %q", view)
	}
}

func TestFixTUIWizardInputAcceptsMappedLetters(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if got := m.wizard.CommitMessage.Value(); got != "hq" {
		t.Fatalf("commit input value = %q, want %q", got, "hq")
	}
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want wizard (q should type, not quit)", m.viewMode)
	}
}

func TestFixTUIWizardChangedFilesRenderAsListWithStats(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true},
			Risk: fixRiskSnapshot{
				ChangedFiles: []fixChangedFile{
					{Path: "src/main.go", Added: 12, Deleted: 5},
				},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := m.viewWizardContent()
	if !strings.Contains(view, "•") || !strings.Contains(view, "src/main.go") {
		t.Fatalf("expected bullet list row for changed file, got %q", view)
	}
	if !strings.Contains(view, "+12") || !strings.Contains(view, "-5") {
		t.Fatalf("expected +/- stats in changed file row, got %q", view)
	}
}

func TestFixTUIWizardChangedFilesTrimIndicator(t *testing.T) {
	t.Parallel()

	files := make([]fixChangedFile, 0, 12)
	for i := 0; i < 12; i++ {
		files = append(files, fixChangedFile{Path: fmt.Sprintf("src/file-%02d.go", i), Added: i + 1, Deleted: i})
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: true},
			Risk: fixRiskSnapshot{
				ChangedFiles: files,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := m.viewWizardContent()
	if !strings.Contains(view, "showing first 10 of 12") {
		t.Fatalf("expected trim indicator in changed file list, got %q", view)
	}
}

func TestFixTUIWizardCreateProjectNameStartsEmptyWithPlaceholder(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:   "repo-from-folder",
				Path:   "/repos/repo-from-folder",
				RepoID: "",
			},
			Meta: nil,
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/repo-from-folder", Action: FixActionCreateProject}})

	if !m.wizard.EnableProjectName {
		t.Fatal("expected project name input to be enabled for create-project")
	}
	if got := m.wizard.ProjectName.Value(); got != "" {
		t.Fatalf("project name initial value = %q, want empty", got)
	}
	if got := m.wizard.ProjectName.Placeholder; got != "repo-from-folder" {
		t.Fatalf("project name placeholder = %q, want %q", got, "repo-from-folder")
	}
}

func TestFixTUIWizardCreateProjectVisibilityUsesLeftRightWhenFocused(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:   "api",
				Path:   "/repos/api",
				RepoID: "",
			},
			Meta: nil,
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})

	if m.wizard.Visibility != domain.VisibilityPrivate {
		t.Fatalf("initial visibility = %q, want private default", m.wizard.Visibility)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})  // project name -> visibility
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // visibility private -> public

	if m.wizard.Visibility != domain.VisibilityPublic {
		t.Fatalf("visibility = %q, want public after right arrow on focused visibility", m.wizard.Visibility)
	}

	view := m.viewWizardContent()
	if !strings.Contains(view, "private (default)") || !strings.Contains(view, "public") {
		t.Fatalf("expected two-option visibility picker with default label, got %q", view)
	}
	if strings.Contains(view, "default") && !strings.Contains(view, "private (default)") {
		t.Fatalf("expected no third default option, got %q", view)
	}
}

func TestFixTUIRowsRenderWithoutReplacementRuneAndWithoutDoubleSpacing(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:              "api",
				Path:              "/repos/api",
				RepoID:            "github.com/you/api",
				OriginURL:         "git@github.com:you/api.git",
				Upstream:          "origin/main",
				Syncable:          false,
				UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked, domain.ReasonMissingOrigin},
				Ahead:             1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				RepoID:    "github.com/you/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/web", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 26})

	view := m.repoList.View()
	if strings.Contains(view, "�") {
		t.Fatalf("list view contains replacement rune: %q", view)
	}

	apiLine := lineIndexContaining(view, "api")
	webLine := lineIndexContaining(view, "web")
	if apiLine < 0 || webLine < 0 {
		t.Fatalf("expected both rows in view, got %q", view)
	}
	if webLine != apiLine+1 {
		t.Fatalf("rows should be adjacent lines, got api at %d and web at %d", apiLine, webLine)
	}
}

func lineIndexContaining(view, needle string) int {
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func TestFixTUIResizeExpandsWideColumns(t *testing.T) {
	t.Parallel()

	layout := fixListColumnsForWidth(180)
	if layout.Reasons <= 32 {
		t.Fatalf("reasons width = %d, want > 32 in wide viewport", layout.Reasons)
	}
	if layout.Action <= 22 {
		t.Fatalf("selected-fix width = %d, want > 22 in wide viewport", layout.Action)
	}
}

func TestFixTUIOrdersReposByTier(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "zzz-sync",
				Path:      "/repos/zzz-sync",
				RepoID:    "github.com/you/zzz-sync",
				OriginURL: "git@github.com:you/zzz-sync.git",
				Upstream:  "origin/main",
				Syncable:  true,
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/zzz-sync", AutoPush: false},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "aaa-blocked",
				Path:      "/repos/aaa-blocked",
				RepoID:    "github.com/you/aaa-blocked",
				OriginURL: "git@github.com:you/aaa-blocked.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Diverged:  true,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonDiverged,
				},
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/aaa-blocked", AutoPush: true},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "mmm-auto",
				Path:      "/repos/mmm-auto",
				RepoID:    "github.com/you/mmm-auto",
				OriginURL: "git@github.com:you/mmm-auto.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushPolicyBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/mmm-auto", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)

	if got := m.visible[0].Record.Name; got != "mmm-auto" {
		t.Fatalf("first row = %q, want fixable repo first", got)
	}
	if got := m.visible[1].Record.Name; got != "aaa-blocked" {
		t.Fatalf("second row = %q, want unsyncable blocked repo second", got)
	}
	if got := m.visible[2].Record.Name; got != "zzz-sync" {
		t.Fatalf("third row = %q, want syncable repo last", got)
	}
}

func TestFixTUIViewDoesNotRenderNestedNormalBorderAroundList(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	view := m.View()

	if strings.Contains(view, "┘") {
		t.Fatalf("unexpected nested normal-border corner glyph in view: %q", view)
	}
}

func TestFixRepoDelegateLeavesWrapGuardColumn(t *testing.T) {
	t.Parallel()

	repoList := newFixRepoListModel()
	repoList.SetSize(80, 10)
	repoList.Select(0)

	item := fixListItem{
		Name:      "phomemo_m02s",
		Branch:    "main",
		State:     "unsyncable",
		Reasons:   "dirty_tracked, dirty_untracked, missing_origin",
		Action:    fixActionLabel(FixActionEnableAutoPush),
		ActionKey: FixActionEnableAutoPush,
		Tier:      fixRepoTierAutofixable,
	}

	var b strings.Builder
	fixRepoDelegate{}.Render(&b, repoList, 0, item)
	row := b.String()

	if strings.Contains(row, "\n") {
		t.Fatalf("delegate row should be single-line, got %q", row)
	}
	if width := ansi.StringWidth(row); width >= repoList.Width() {
		t.Fatalf("delegate row width %d must stay below list width %d to avoid hard wrap", width, repoList.Width())
	}
}

func TestFixTUIFooterDoesNotLeaveExtraTrailingBlankRows(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	view := m.View()

	trailing := trailingEmptyLineCount(view)
	if trailing != 0 {
		t.Fatalf("expected zero trailing empty lines after footer, got %d", trailing)
	}
}

func TestFixTUIViewShowsMainPanelTopBorderBeforeContent(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 26})
	view := m.View()

	lines := strings.Split(view, "\n")
	idx := lineIndexContaining(view, "Repository Fixes")
	if idx <= 0 || idx >= len(lines) {
		t.Fatalf("could not locate repository fixes title in view: %q", view)
	}
	previous := previousNonEmptyLine(lines, idx-1)
	if !strings.Contains(previous, "╭") {
		t.Fatalf("expected panel top border before content, got %q", previous)
	}
}

func TestFixTUISelectedDetailsRenderActionHelp(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1) // push

	details := m.viewSelectedRepoDetails()
	if !strings.Contains(details, "Action help:") {
		t.Fatalf("expected action help in selected details, got %q", details)
	}
	if !strings.Contains(details, "Push local commits") {
		t.Fatalf("expected push action description, got %q", details)
	}
}

func TestFixTUISelectedDetailsHeaderHasNoFieldBorderAndUsesSelectedLabel(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				RepoID:    "github.com/you/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)
	details := m.viewSelectedRepoDetails()

	firstLine := strings.Split(details, "\n")[0]
	if strings.Contains(firstLine, "│") {
		t.Fatalf("selected details header should not use field border glyph, got %q", firstLine)
	}
	if !strings.Contains(firstLine, "Selected:") || !strings.Contains(firstLine, "api") {
		t.Fatalf("selected details header missing expected label/value, got %q", firstLine)
	}
}

func TestClassifyFixRepoRequiresFullReasonCoverage(t *testing.T) {
	t.Parallel()

	repo := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:      "api",
			Path:      "/repos/api",
			RepoID:    "github.com/you/api",
			OriginURL: "git@github.com:you/api.git",
			Upstream:  "origin/main",
			Syncable:  false,
			Diverged:  true,
			UnsyncableReasons: []domain.UnsyncableReason{
				domain.ReasonDiverged,
				domain.ReasonPushPolicyBlocked,
			},
		},
		Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/api", AutoPush: false},
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})

	if got := classifyFixRepo(repo, actions); got != fixRepoTierUnsyncableBlocked {
		t.Fatalf("tier = %v, want unsyncable blocked when not all reasons are coverable", got)
	}
}

func TestSelectableFixActionsAddsAllFixesOptionForMultiple(t *testing.T) {
	t.Parallel()

	options := selectableFixActions([]string{FixActionPush, FixActionEnableAutoPush})
	if len(options) != 3 {
		t.Fatalf("options len = %d, want 3", len(options))
	}
	if got := options[2]; got != fixAllActions {
		t.Fatalf("last option = %q, want %q", got, fixAllActions)
	}
}

func trailingEmptyLineCount(s string) int {
	lines := strings.Split(s, "\n")
	n := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			n++
			continue
		}
		break
	}
	return n
}

func previousNonEmptyLine(lines []string, start int) string {
	for i := start; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}
