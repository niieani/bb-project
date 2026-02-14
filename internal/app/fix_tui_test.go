package app

import (
	"fmt"
	"io"
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
				RepoKey:   "software/api",
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
		{
			Record: domain.MachineRepoRecord{
				RepoKey:   "software/web",
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{RepoKey: "software/web", OriginURL: "https://github.com/you/web.git", AutoPush: true},
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
				RepoKey:         "software/api",
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				Ahead:           1,
				HasDirtyTracked: true,
			},
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	m.cycleCurrentAction(1)
	before := actionForVisibleRepo(m, 0)
	if !strings.Contains(before, FixActionStageCommitPush) {
		t.Fatalf("expected second cycle to pick %q, got %q", FixActionStageCommitPush, before)
	}

	m.repos[0].Record = domain.MachineRepoRecord{
		RepoKey:   "software/api",
		Name:      "api",
		Path:      "/repos/api",
		OriginURL: "git@github.com:you/api.git",
		Upstream:  "origin/main",
		Ahead:     1,
	}
	m.repos[0].Meta = &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: true}
	m.rebuildList("/repos/api")

	if got := actionForVisibleRepo(m, 0); !strings.Contains(got, fixNoAction) {
		t.Fatalf("fallback action = %q, want default no-op", got)
	}
}

func TestFixTUIGroupedByCatalogDefaultFirstWithBreaks(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "ref-repo",
				Path:      "/references/ref-repo",
				Catalog:   "references",
				OriginURL: "git@github.com:you/ref-repo.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			IsDefaultCatalog: false,
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "soft-repo",
				Path:      "/software/soft-repo",
				Catalog:   "software",
				OriginURL: "git@github.com:you/soft-repo.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			IsDefaultCatalog: true,
		},
	}

	m := newFixTUIModelForTest(repos)
	items := m.repoList.Items()
	if len(items) < 5 {
		t.Fatalf("expected grouped list rows with headers and break, got %d item(s)", len(items))
	}

	firstHeader, ok := items[0].(fixListItem)
	if !ok || firstHeader.Kind != fixListItemCatalogHeader {
		t.Fatalf("first item should be catalog header, got %#v", items[0])
	}
	if !strings.Contains(firstHeader.Name, "software") || !strings.Contains(firstHeader.Name, "(default)") {
		t.Fatalf("first header = %q, want software default header", firstHeader.Name)
	}

	foundBreak := false
	foundSecondHeader := false
	for _, item := range items {
		row, ok := item.(fixListItem)
		if !ok {
			continue
		}
		if row.Kind == fixListItemCatalogBreak {
			foundBreak = true
		}
		if row.Kind == fixListItemCatalogHeader && strings.Contains(row.Name, "references") {
			foundSecondHeader = true
		}
	}
	if !foundBreak {
		t.Fatal("expected catalog break row between catalog groups")
	}
	if !foundSecondHeader {
		t.Fatal("expected references catalog header in grouped rows")
	}
}

func TestFixTUIRepoNameUsesOSC8ForGitHub(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				Catalog:   "software",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			IsDefaultCatalog: true,
		},
	}

	m := newFixTUIModelForTest(repos)
	items := m.repoList.Items()
	for _, item := range items {
		row, ok := item.(fixListItem)
		if !ok || row.Kind != fixListItemRepo {
			continue
		}
		if !strings.Contains(row.Name, "\x1b]8;;https://github.com/you/api\x1b\\") {
			t.Fatalf("repo name does not contain OSC8 github link: %q", row.Name)
		}
		return
	}
	t.Fatal("expected at least one repo row")
}

func TestFixTUIRepoNameUsesOSC8ForAliasedGitHubHost(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "condu",
				Path:      "/repos/condu",
				Catalog:   "software",
				OriginURL: "git@niieani.github.com:niieani/condu.git",
				Upstream:  "niieani/main",
				Ahead:     1,
			},
			IsDefaultCatalog: true,
		},
	}

	m := newFixTUIModelForTest(repos)
	items := m.repoList.Items()
	for _, item := range items {
		row, ok := item.(fixListItem)
		if !ok || row.Kind != fixListItemRepo {
			continue
		}
		if !strings.Contains(row.Name, "\x1b]8;;https://github.com/niieani/condu\x1b\\") {
			t.Fatalf("repo name does not contain canonical github OSC8 link: %q", row.Name)
		}
		return
	}
	t.Fatal("expected at least one repo row")
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
	return m.currentActionForRepo(repo.Record.Path, fixActionsForSelection(actions))
}

func TestFixTUIViewUsesCanonicalChromeWithoutInlineKeyLegend(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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

func TestFixTUIBootViewShowsLoadingStatus(t *testing.T) {
	t.Parallel()

	m := newFixTUIBootModel(nil, nil, false)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	view := m.View()
	if !strings.Contains(view, "Preparing interactive fix startup") {
		t.Fatalf("expected boot heading in view, got %q", view)
	}
	if !strings.Contains(view, "Loading repositories and risk checks for interactive fix") {
		t.Fatalf("expected loading explanation in view, got %q", view)
	}
}

func TestFixTUIBootViewUsesLatestProgressLine(t *testing.T) {
	t.Parallel()

	m := newFixTUIBootModel(nil, nil, false)
	m.setProgress("scan: snapshot is stale, refreshing")

	view := m.View()
	if !strings.Contains(view, "scan: snapshot is stale, refreshing") {
		t.Fatalf("expected current progress line in boot view, got %q", view)
	}
}

func TestFixTUIBootLoadCmdCapturesAppLogProgress(t *testing.T) {
	t.Parallel()

	app := &App{Stderr: io.Discard, Verbose: false}
	boot := newFixTUIBootModel(app, nil, false)
	loaded := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
	})

	boot.loadFn = func() (*fixTUIModel, error) {
		app.logf("scan: snapshot is stale, refreshing")
		app.logf("fix: selected 1 repository")
		return loaded, nil
	}

	msg := boot.loadReposCmd()()
	loadedMsg, ok := msg.(fixTUILoadedMsg)
	if !ok {
		t.Fatalf("load command message type = %T, want fixTUILoadedMsg", msg)
	}
	if loadedMsg.err != nil {
		t.Fatalf("load command error = %v, want nil", loadedMsg.err)
	}
	if loadedMsg.model != loaded {
		t.Fatal("expected load command to return loaded model")
	}
	if got := boot.currentProgress(); got != "fix: selected 1 repository" {
		t.Fatalf("current progress = %q, want latest startup log line", got)
	}
}

func TestFixTUIBootTransfersWindowSizeToLoadedModel(t *testing.T) {
	t.Parallel()

	boot := newFixTUIBootModel(nil, nil, false)
	_, _ = boot.Update(tea.WindowSizeMsg{Width: 128, Height: 28})

	loaded := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
	})

	next, _ := boot.Update(fixTUILoadedMsg{model: loaded})
	got, ok := next.(*fixTUIModel)
	if !ok {
		t.Fatalf("expected transition to fixTUIModel, got %T", next)
	}
	if got.width != 128 || got.height != 28 {
		t.Fatalf("loaded model size = %dx%d, want 128x28", got.width, got.height)
	}
	if got.help.Width != 128 {
		t.Fatalf("loaded model help width = %d, want 128", got.help.Width)
	}
}

func TestFixTUIBootStoresLoadError(t *testing.T) {
	t.Parallel()

	boot := newFixTUIBootModel(nil, nil, false)
	next, _ := boot.Update(fixTUILoadedMsg{err: fmt.Errorf("load failed")})
	if next != boot {
		t.Fatalf("expected boot model to remain active on load error, got %T", next)
	}
	if boot.loadErr == nil || !strings.Contains(boot.loadErr.Error(), "load failed") {
		t.Fatalf("loadErr = %v, want load failure", boot.loadErr)
	}
}

func TestFixTUIDefaultSelectionIsNoAction(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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

func TestFixTUIActionCycleExcludesAutoPushSetting(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				RepoKey:   "software/api",
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("selected action = %q, want %q", got, FixActionPush)
	}

	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); got != fixNoAction {
		t.Fatalf("selected action after second cycle = %q, want %q", got, fixNoAction)
	}
}

func TestFixTUISettingToggleKeyUpdatesAutoPush(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				RepoKey:   "software/api",
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushPolicyBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	if !repoMetaAutoPush(m.visible[0].Meta) {
		t.Fatal("expected auto-push to be enabled after pressing s")
	}
	if !strings.Contains(m.status, "auto-push on") {
		t.Fatalf("status = %q, want auto-push on message", m.status)
	}

	rowItems := m.repoList.Items()
	for _, item := range rowItems {
		row, ok := item.(fixListItem)
		if !ok || row.Kind != fixListItemRepo {
			continue
		}
		if row.Path == "/repos/api" && !row.AutoPush {
			t.Fatal("expected list row auto-push column to be on")
		}
	}
}

func TestFixTUISettingToggleKeyBlockedForReadOnlyRemote(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				RepoKey:   "software/api",
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushAccessBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:    "software/api",
				OriginURL:  "https://github.com/you/api.git",
				AutoPush:   false,
				PushAccess: domain.PushAccessReadOnly,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	if repoMetaAutoPush(m.visible[0].Meta) {
		t.Fatal("expected auto-push to remain disabled for read-only remote")
	}
	if !strings.Contains(m.errText, "n/a") {
		t.Fatalf("errText = %q, want n/a guidance", m.errText)
	}

	details := m.viewSelectedRepoDetails()
	if !strings.Contains(details, "n/a") {
		t.Fatalf("details should render auto-push as n/a, got %q", details)
	}

	rowItems := m.repoList.Items()
	for _, item := range rowItems {
		row, ok := item.(fixListItem)
		if !ok || row.Kind != fixListItemRepo {
			continue
		}
		if row.Path == "/repos/api" && row.AutoPushAvailable {
			t.Fatal("expected list row auto-push availability to be disabled")
		}
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
				OriginURL: "git@github.com:you/repo.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: fmt.Sprintf("https://github.com/you/repo-%02d.git", i), AutoPush: false},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
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

func TestFixTUIAbortSelectionOpensConfirmationWizard(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:                "api",
				Path:                "/repos/api",
				RepoKey:             "software/api",
				OriginURL:           "git@github.com:you/api.git",
				OperationInProgress: domain.OperationRebase,
			},
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); got != FixActionAbortOperation {
		t.Fatalf("selected action = %q, want %q", got, FixActionAbortOperation)
	}

	m.applyCurrentSelection()
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want wizard mode", m.viewMode)
	}
	if m.wizard.Action != FixActionAbortOperation {
		t.Fatalf("wizard action = %q, want %q", m.wizard.Action, FixActionAbortOperation)
	}
}

func TestFixTUIWizardSkipShowsSummary(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := m.viewWizardContent()
	if !strings.Contains(view, "Apply") || !strings.Contains(view, "Skip") || !strings.Contains(view, "Cancel") {
		t.Fatalf("wizard view should render action buttons, got %q", view)
	}
}

func TestFixTUIWizardDefaultsActionFocusToCancel(t *testing.T) {
	t.Parallel()

	newRepos := func() []fixRepoState {
		return []fixRepoState{
			{
				Record: domain.MachineRepoRecord{
					Name:      "api",
					Path:      "/repos/api",
					OriginURL: "git@github.com:you/api.git",
					Upstream:  "origin/main",
					Ahead:     1,
				},
				Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
			},
		}
	}

	actions := []string{
		FixActionPush,
		FixActionSetUpstreamPush,
		FixActionStageCommitPush,
		FixActionCreateProject,
		FixActionAbortOperation,
	}
	for _, action := range actions {
		action := action
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			m := newFixTUIModelForTest(newRepos())
			m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: action}})
			if got := m.wizard.ActionFocus; got != fixWizardActionCancel {
				t.Fatalf("default action focus = %d, want cancel(%d) for action %q", got, fixWizardActionCancel, action)
			}
		})
	}
}

func TestFixTUIWizardButtonsRenderCancelFirst(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := ansi.Strip(m.viewWizardContent())
	cancelIdx := strings.Index(view, "Cancel")
	skipIdx := strings.Index(view, "Skip")
	applyIdx := strings.Index(view, "Apply")
	if cancelIdx < 0 || skipIdx < 0 || applyIdx < 0 {
		t.Fatalf("wizard buttons missing in view: %q", view)
	}
	if !(cancelIdx < skipIdx && skipIdx < applyIdx) {
		t.Fatalf("expected button order Cancel -> Skip -> Apply, got %q", view)
	}
}

func TestFixTUIWizardEnterOnDefaultCancelReturnsToList(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.viewMode != fixViewList {
		t.Fatalf("view mode after enter on default cancel = %v, want list", m.viewMode)
	}
	if !strings.Contains(m.status, "cancelled remaining risky confirmations") {
		t.Fatalf("status = %q, want cancelled confirmation status", m.status)
	}
}

func TestFixTUIWizardViewUsesSingleTopLineWithoutExtraWizardHeaders(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 28})

	view := ansi.Strip(m.View())
	if strings.Contains(view, "Interactive remediation for unsyncable repositories") {
		t.Fatalf("wizard should use compact single top line, got %q", view)
	}
	if strings.Contains(view, "Review context before applying this fix.") {
		t.Fatalf("wizard should not render extra review header line, got %q", view)
	}
	if strings.Count(view, "Confirm Risky Fix") != 1 {
		t.Fatalf("expected exactly one wizard header label, got %q", view)
	}
}

func TestFixTUIWizardTopLineUsesSingleLineProgress(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})

	top := ansi.Strip(m.viewWizardTopLine())
	if strings.Contains(top, "\n") {
		t.Fatalf("wizard top line should be single-line, got %q", top)
	}
	if strings.Contains(top, "╭") || strings.Contains(top, "│") || strings.Contains(top, "╰") {
		t.Fatalf("wizard top line should not use multi-line boxed progress, got %q", top)
	}
}

func TestFixTUIWizardFooterHintIsStableSingleLine(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	m.wizard.FocusArea = fixWizardFocusProjectName
	hintTextInput := ansi.Strip(m.wizardFooterHint())
	m.wizard.FocusArea = fixWizardFocusActions
	hintActions := ansi.Strip(m.wizardFooterHint())

	if hintTextInput != hintActions {
		t.Fatalf("wizard footer hint should be stable across focus changes:\ninput=%q\nactions=%q", hintTextInput, hintActions)
	}
	if strings.Contains(hintTextInput, "\n") {
		t.Fatalf("wizard footer hint should stay single-line, got %q", hintTextInput)
	}
}

func TestFixTUIWizardInputAcceptsMappedLetters(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
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
				Name:      "repo-from-folder",
				Path:      "/repos/repo-from-folder",
				OriginURL: "",
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
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
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

func TestFixTUIWizardDownMovesFocusBeforeScrollingNonInputControls(t *testing.T) {
	t.Parallel()

	noisy := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		noisy = append(noisy, fmt.Sprintf("tmp/noisy-%02d.log", i))
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				NoisyChangedPaths: noisy,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	if m.wizard.FocusArea != fixWizardFocusProjectName {
		t.Fatalf("initial focus area = %v, want project-name", m.wizard.FocusArea)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project name -> visibility
	if m.wizard.FocusArea != fixWizardFocusVisibility {
		t.Fatalf("focus area after first down = %v, want visibility", m.wizard.FocusArea)
	}
	startOffset := m.wizard.BodyViewport.YOffset
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // should move focus to actions first

	if m.wizard.FocusArea != fixWizardFocusActions {
		t.Fatalf("focus area after second down = %v, want actions", m.wizard.FocusArea)
	}
	if m.wizard.BodyViewport.YOffset != startOffset {
		t.Fatalf("viewport should not scroll before focus reaches edge, yoffset %d -> %d", startOffset, m.wizard.BodyViewport.YOffset)
	}
}

func TestFixTUIWizardFocusMoveRevealsVisibilityFieldContent(t *testing.T) {
	t.Parallel()

	noisy := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		noisy = append(noisy, fmt.Sprintf("tmp/noisy-%02d.log", i))
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				NoisyChangedPaths: noisy,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project name -> visibility
	if m.wizard.FocusArea != fixWizardFocusVisibility {
		t.Fatalf("focus area after down = %v, want visibility", m.wizard.FocusArea)
	}

	visible := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(visible, "Project visibility") {
		t.Fatalf("focused visibility field should be visible in static controls, got %q", visible)
	}
	if !strings.Contains(visible, "private (default)") {
		t.Fatalf("visibility options should be visible in static controls, got %q", visible)
	}
}

func TestFixTUIWizardActionsDownScrollsContextAtFocusEdge(t *testing.T) {
	t.Parallel()

	noisy := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		noisy = append(noisy, fmt.Sprintf("tmp/noisy-%02d.log", i))
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				NoisyChangedPaths: noisy,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project -> visibility
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // visibility -> actions
	if m.wizard.FocusArea != fixWizardFocusActions {
		t.Fatalf("focus area before edge scroll = %v, want actions", m.wizard.FocusArea)
	}

	startOffset := m.wizard.BodyViewport.YOffset
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // at focus edge => scroll context

	if m.wizard.FocusArea != fixWizardFocusActions {
		t.Fatalf("focus area should stay on actions while scrolling context, got %v", m.wizard.FocusArea)
	}
	if m.wizard.BodyViewport.YOffset <= startOffset {
		t.Fatalf("expected context viewport to scroll down, yoffset %d -> %d", startOffset, m.wizard.BodyViewport.YOffset)
	}
}

func TestFixTUIWizardScrollIndicatorShowsBelowAtTop(t *testing.T) {
	t.Parallel()

	noisy := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		noisy = append(noisy, fmt.Sprintf("tmp/noisy-%02d.log", i))
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				NoisyChangedPaths: noisy,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "More context below") {
		t.Fatalf("expected bottom scroll indicator near top of context, got %q", view)
	}
	if strings.Contains(view, "More context above") {
		t.Fatalf("top indicator should not show while context at top, got %q", view)
	}
}

func TestFixTUIWizardScrollIndicatorShowsAboveAfterScroll(t *testing.T) {
	t.Parallel()

	noisy := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		noisy = append(noisy, fmt.Sprintf("tmp/noisy-%02d.log", i))
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				NoisyChangedPaths: noisy,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project -> visibility
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // visibility -> actions
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // actions edge -> scroll context down

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "More context above") {
		t.Fatalf("expected top scroll indicator after scrolling, got %q", view)
	}
}

func TestFixTUIWizardUpDownCanReachProjectNameWithoutTab(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	if m.wizard.FocusArea != fixWizardFocusProjectName {
		t.Fatalf("initial focus area = %v, want project-name", m.wizard.FocusArea)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project -> visibility
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // visibility -> actions
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})   // actions -> visibility
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})   // visibility -> project

	if m.wizard.FocusArea != fixWizardFocusProjectName {
		t.Fatalf("expected up/down path to return to project-name focus, got %v", m.wizard.FocusArea)
	}
}

func TestFixTUIWizardDownMovesFocusAtViewportBottom(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				ChangedFiles: []fixChangedFile{
					{Path: "src/main.go", Added: 10, Deleted: 1},
				},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 20})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project name -> visibility
	if m.wizard.FocusArea != fixWizardFocusVisibility {
		t.Fatalf("focus area = %v, want visibility before bottom-edge check", m.wizard.FocusArea)
	}

	m.wizard.BodyViewport.GotoBottom()
	if !m.wizard.BodyViewport.AtBottom() {
		t.Fatal("expected wizard viewport to be at bottom")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // should move to actions at bottom edge
	if m.wizard.FocusArea != fixWizardFocusActions {
		t.Fatalf("focus area after down at bottom = %v, want actions", m.wizard.FocusArea)
	}
}

func TestFixTUIWizardUpFromFirstInputScrollsWhenNoPreviousField(t *testing.T) {
	t.Parallel()

	noisy := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		noisy = append(noisy, fmt.Sprintf("tmp/noisy-%02d.log", i))
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				NoisyChangedPaths: noisy,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	m.wizard.BodyViewport.GotoBottom()
	if m.wizard.BodyViewport.YOffset == 0 {
		t.Fatal("expected wizard viewport to have scrollable overflow")
	}
	startOffset := m.wizard.BodyViewport.YOffset

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp}) // no previous field from first input, should scroll
	if m.wizard.FocusArea != fixWizardFocusProjectName {
		t.Fatalf("focus area after up from first input = %v, want project-name", m.wizard.FocusArea)
	}
	if m.wizard.BodyViewport.YOffset >= startOffset {
		t.Fatalf("expected viewport to scroll up, yoffset %d -> %d", startOffset, m.wizard.BodyViewport.YOffset)
	}
}

func TestFixTUIWizardMissingGitignoreExplainsGenerationForStageCommit(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       true,
				NoisyChangedPaths:          []string{"node_modules/pkg/index.js"},
				SuggestedGitignorePatterns: []string{"node_modules/"},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	view := m.viewWizardContent()
	if !strings.Contains(view, "bb can generate a root .gitignore before commit") {
		t.Fatalf("expected explicit .gitignore generation note, got %q", view)
	}
	if !strings.Contains(view, "Generate .gitignore before commit") {
		t.Fatalf("expected .gitignore toggle in stage-commit wizard, got %q", view)
	}
}

func TestFixTUIWizardCreateProjectMissingGitignoreOffersGenerationToggle(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       true,
				NoisyChangedPaths:          []string{"dist/app.js"},
				SuggestedGitignorePatterns: []string{"dist/"},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{
		{RepoPath: "/repos/api", Action: FixActionCreateProject},
		{RepoPath: "/repos/api", Action: FixActionStageCommitPush},
	})

	view := m.viewWizardContent()
	if !strings.Contains(view, "bb can generate a root .gitignore before commit") {
		t.Fatalf("expected explicit .gitignore generation note for create-project, got %q", view)
	}
	if !strings.Contains(view, "Generate .gitignore before commit") {
		t.Fatalf("expected .gitignore toggle in create-project wizard, got %q", view)
	}
}

func TestFixTUIWizardGitignoreToggleIsFocusableAndSelectable(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       true,
				NoisyChangedPaths:          []string{"node_modules/pkg/index.js"},
				SuggestedGitignorePatterns: []string{"node_modules/"},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	if !m.wizard.GenerateGitignore {
		t.Fatal("expected gitignore generation toggle to default on")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // commit -> gitignore toggle
	if m.wizard.FocusArea != fixWizardFocusGitignore {
		t.Fatalf("focus area after down = %v, want gitignore toggle", m.wizard.FocusArea)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.wizard.GenerateGitignore {
		t.Fatal("expected space to toggle gitignore generation off when focused")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // toggle -> actions
	if m.wizard.FocusArea != fixWizardFocusActions {
		t.Fatalf("focus area after enter on toggle = %v, want actions", m.wizard.FocusArea)
	}
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode after enter on toggle = %v, want wizard", m.viewMode)
	}
}

func TestFixTUIRowsRenderWithoutReplacementRuneAndWithoutDoubleSpacing(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:              "api",
				Path:              "/repos/api",
				OriginURL:         "git@github.com:you/api.git",
				Upstream:          "origin/main",
				Syncable:          false,
				UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked, domain.ReasonMissingOrigin},
				Ahead:             1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/web.git", AutoPush: true},
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
				OriginURL: "git@github.com:you/zzz-sync.git",
				Upstream:  "origin/main",
				Syncable:  true,
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/zzz-sync.git", AutoPush: false},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "aaa-blocked",
				Path:      "/repos/aaa-blocked",
				OriginURL: "git@github.com:you/aaa-blocked.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Diverged:  true,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonDiverged,
				},
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/aaa-blocked.git", AutoPush: true},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "mmm-auto",
				Path:      "/repos/mmm-auto",
				OriginURL: "git@github.com:you/mmm-auto.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushPolicyBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/mmm-auto.git", AutoPush: false},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	view := m.View()

	trailing := trailingEmptyLineCount(view)
	if trailing != 0 {
		t.Fatalf("expected zero trailing empty lines after footer, got %d", trailing)
	}
}

func TestFixTUIWizardStatusToFooterGapIsCompact(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	})
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 28})

	view := ansi.Strip(m.View())
	lines := strings.Split(view, "\n")

	statusIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "reviewing 1 risky fix(es)") {
			statusIdx = i
			break
		}
	}
	if statusIdx < 0 {
		t.Fatalf("status line not found in wizard view: %q", view)
	}

	helpTopIdx := -1
	for i := statusIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "╭") && strings.HasSuffix(trimmed, "╮") {
			helpTopIdx = i
			break
		}
	}
	if helpTopIdx < 0 {
		t.Fatalf("footer help panel top border not found after status line: %q", view)
	}

	emptyBetween := 0
	for i := statusIdx + 1; i < helpTopIdx; i++ {
		if strings.TrimSpace(lines[i]) == "" {
			emptyBetween++
		}
	}
	if emptyBetween > 1 {
		t.Fatalf("expected at most one separator line between status and footer, got %d in view: %q", emptyBetween, view)
	}
}

func TestFixTUIWizardFooterDoesNotLeaveExtraTrailingBlankRows(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: true},
		},
	})
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 28})

	view := ansi.Strip(m.View())
	trailing := trailingEmptyLineCount(view)
	if trailing != 0 {
		t.Fatalf("expected zero trailing empty lines after wizard footer, got %d", trailing)
	}
}

func TestFixActionLabelAndDescriptionIncludeCreateProject(t *testing.T) {
	t.Parallel()

	if got := fixActionLabel(FixActionCreateProject); got == FixActionCreateProject {
		t.Fatalf("create-project label should be user-friendly, got raw action %q", got)
	}
	if got := fixActionDescription(FixActionCreateProject); strings.Contains(got, "no help text") {
		t.Fatalf("create-project should have description, got %q", got)
	}
}

func TestFixTUIViewShowsMainPanelTopBorderBeforeContent(t *testing.T) {
	t.Parallel()

	m := newFixTUIModelForTest([]fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: false},
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

func TestClassifyFixRepoMarksUnsyncableRepoAsFixableWhenReasonsAreCoverable(t *testing.T) {
	t.Parallel()

	repo := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:      "api",
			Path:      "/repos/api",
			OriginURL: "",
			Upstream:  "",
			Syncable:  false,
			UnsyncableReasons: []domain.UnsyncableReason{
				domain.ReasonMissingOrigin,
			},
		},
		Meta: nil,
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})

	if got := classifyFixRepo(repo, actions); got != fixRepoTierAutofixable {
		t.Fatalf("tier = %v, want fixable when there are eligible bb actions", got)
	}
}

func TestClassifyFixRepoMarksUnsyncableWhenReasonsAreNotCoverable(t *testing.T) {
	t.Parallel()

	repo := fixRepoState{
		Record: domain.MachineRepoRecord{
			RepoKey:      "software/api",
			Name:         "api",
			Path:         "/repos/api",
			OriginURL:    "git@github.com:you/api.git",
			Upstream:     "origin/main",
			Syncable:     false,
			Diverged:     true,
			Ahead:        1,
			HasUntracked: true,
			UnsyncableReasons: []domain.UnsyncableReason{
				domain.ReasonDirtyUntracked,
				domain.ReasonDiverged,
				domain.ReasonPushPolicyBlocked,
			},
		},
		Meta: &domain.RepoMetadataFile{
			RepoKey:  "software/api",
			AutoPush: false,
		},
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})

	if got := classifyFixRepo(repo, actions); got != fixRepoTierUnsyncableBlocked {
		t.Fatalf("tier = %v, want unsyncable when no bb fix covers all reasons", got)
	}
}

func TestClassifyFixRepoMarksCreateProjectDirtyMissingOriginAsFixable(t *testing.T) {
	t.Parallel()

	repo := fixRepoState{
		Record: domain.MachineRepoRecord{
			RepoKey:      "software/api",
			Name:         "api",
			Path:         "/repos/api",
			OriginURL:    "",
			Upstream:     "",
			Syncable:     false,
			HasUntracked: true,
			UnsyncableReasons: []domain.UnsyncableReason{
				domain.ReasonDirtyUntracked,
				domain.ReasonMissingOrigin,
				domain.ReasonMissingUpstream,
			},
		},
		Meta: nil,
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})

	if !containsAction(actions, FixActionCreateProject) {
		t.Fatalf("expected %q action, got %v", FixActionCreateProject, actions)
	}
	if got := classifyFixRepo(repo, actions); got != fixRepoTierAutofixable {
		t.Fatalf("tier = %v, want fixable when create-project can resolve all reasons", got)
	}
}

func TestClassifyFixRepoMarksReadOnlyPushAccessAsFixableWithForkAction(t *testing.T) {
	t.Parallel()

	repo := fixRepoState{
		Record: domain.MachineRepoRecord{
			RepoKey:   "software/api",
			Name:      "api",
			Path:      "/repos/api",
			OriginURL: "git@github.com:you/api.git",
			Upstream:  "origin/main",
			Syncable:  false,
			Ahead:     1,
			UnsyncableReasons: []domain.UnsyncableReason{
				domain.ReasonPushAccessBlocked,
			},
		},
		Meta: &domain.RepoMetadataFile{
			RepoKey:    "software/api",
			PushAccess: domain.PushAccessReadOnly,
			AutoPush:   false,
		},
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})

	if !containsAction(actions, FixActionForkAndRetarget) {
		t.Fatalf("expected fork-and-retarget action, got %v", actions)
	}
	if got := classifyFixRepo(repo, actions); got != fixRepoTierAutofixable {
		t.Fatalf("tier = %v, want fixable for read-only remote", got)
	}
}

func TestSelectableFixActionsAddsAllFixesOptionForMultiple(t *testing.T) {
	t.Parallel()

	options := selectableFixActions([]string{FixActionPush, FixActionStageCommitPush})
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
