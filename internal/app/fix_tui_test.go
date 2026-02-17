package app

import (
	"fmt"
	"io"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"bb-project/internal/domain"
	"bb-project/internal/state"
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
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
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
			Meta: &domain.RepoMetadataFile{RepoKey: "software/web", OriginURL: "https://github.com/you/web.git", AutoPush: domain.AutoPushModeEnabled},
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

func TestFixTUICursorFallbackAfterEligibilityChange(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	initialActions := eligibleFixActions(m.visible[0].Record, m.visible[0].Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        m.visible[0].Risk,
	})
	initialOptions := selectableFixActions(fixActionsForSelection(initialActions))
	if len(initialOptions) < 2 {
		t.Fatalf("expected at least two options before fallback test, got %v", initialOptions)
	}
	m.actionCursor["/repos/api"] = 1
	before := actionForVisibleRepo(m, 0)
	if before == fixNoAction {
		t.Fatalf("expected preselected action to be non-default before fallback, got %q", before)
	}

	m.repos[0].Record = domain.MachineRepoRecord{
		RepoKey:   "software/api",
		Name:      "api",
		Path:      "/repos/api",
		OriginURL: "git@github.com:you/api.git",
		Upstream:  "origin/main",
		Ahead:     1,
	}
	m.repos[0].Meta = &domain.RepoMetadataFile{
		RepoKey:    "software/api",
		OriginURL:  "https://github.com/you/api.git",
		AutoPush:   domain.AutoPushModeEnabled,
		PushAccess: domain.PushAccessReadWrite,
	}
	m.rebuildList("/repos/api")

	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("fallback action = %q, want %q", got, FixActionPush)
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
	cloned := append([]fixRepoState(nil), repos...)
	for i := range cloned {
		if cloned[i].Meta == nil {
			continue
		}
		if strings.TrimSpace(cloned[i].Record.OriginURL) == "" {
			continue
		}
		if domain.NormalizePushAccess(cloned[i].Meta.PushAccess) != domain.PushAccessUnknown {
			continue
		}
		metaCopy := *cloned[i].Meta
		metaCopy.PushAccess = domain.PushAccessReadWrite
		cloned[i].Meta = &metaCopy
	}

	m := &fixTUIModel{
		repos:                 cloned,
		ignored:               map[string]bool{},
		actionCursor:          map[string]int{},
		scheduled:             map[string][]string{},
		execProcessFn:         tea.ExecProcess,
		repoList:              newFixRepoListModel(),
		keys:                  defaultFixTUIKeyMap(),
		help:                  help.New(),
		revalidateSpinner:     newFixProgressSpinner(),
		immediateApplySpinner: newFixProgressSpinner(),
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
	return m.currentActionForRepo(repo.Record.Path, selectableFixActions(fixActionsForSelection(actions)))
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
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

func TestFixTUIViewUsesCompactMainPanelChrome(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})

	view := ansi.Strip(m.View())
	if strings.Contains(view, "Repository Fixes") {
		t.Fatalf("expected compact border title instead of repository-fixes heading, got %q", view)
	}
	if strings.Contains(view, "Enter runs selected fixes for the selected repo; ctrl+a runs selected fixes across repos.") {
		t.Fatalf("expected long enter/ctrl+a sentence removed, got %q", view)
	}
	if strings.Contains(view, "Grouped by catalog (default catalog first), then fixable, unsyncable, syncable, and ignored.") {
		t.Fatalf("expected grouping sentence removed, got %q", view)
	}

	lines := strings.Split(view, "\n")
	top := firstNonEmptyLine(lines)
	if !strings.Contains(top, "╭") || !strings.Contains(top, "bb") || !strings.Contains(top, "fix") || !strings.Contains(top, "Interactive remediation for unsyncable repositories") {
		t.Fatalf("expected combined border title/subtitle on first content line, got %q", top)
	}

	statsIdx := lineIndexContaining(view, "REPOS")
	if statsIdx <= 0 || statsIdx >= len(lines)-1 {
		t.Fatalf("could not locate stats row in compact view: %q", view)
	}
	if strings.TrimSpace(lines[statsIdx-1]) == "" || strings.TrimSpace(lines[statsIdx+1]) == "" {
		t.Fatalf("expected no blank row before or after stats row, got prev=%q next=%q", lines[statsIdx-1], lines[statsIdx+1])
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
	if !strings.Contains(view, "Refreshing repository snapshot...") {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	})

	boot.loadFn = func() (*fixTUIModel, error) {
		app.logf("scan: snapshot is stale, refreshing")
		app.logf("fix: collecting risk checks (1/2)")
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
	if got := boot.currentProgress(); got != "fix: collecting risk checks (1/2)" {
		t.Fatalf("current progress = %q, want latest startup log line", got)
	}
}

func TestFixTUIBootProgressNormalizesProbeFailures(t *testing.T) {
	t.Parallel()

	m := newFixTUIBootModel(nil, nil, false)
	m.setProgress(`scan: push-access probe failed for /repo/path: fatal: could not read Password for 'https://user@github.com': terminal prompts disabled`)

	if got := m.currentProgress(); got != "Verifying repository push access (manual authentication needed for some remotes)..." {
		t.Fatalf("current progress = %q", got)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
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

func TestFixTUIDefaultBrowseStartsAtFirstEligibleFix(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)

	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("browsed fix = %q, want %q", got, FixActionPush)
	}
}

func TestFixTUIActionCycleWrapsWithinEligibleFixes(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				Ahead:           1,
				HasDirtyTracked: true,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)

	if got := actionForVisibleRepo(m, 0); got != FixActionStageCommitPush {
		t.Fatalf("default browsed action = %q, want %q", got, FixActionStageCommitPush)
	}

	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("action after forward cycle = %q, want %q", got, FixActionPush)
	}

	m.cycleCurrentAction(-1)
	if got := actionForVisibleRepo(m, 0); got != FixActionStageCommitPush {
		t.Fatalf("action after reverse cycle = %q, want %q", got, FixActionStageCommitPush)
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
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("selected action = %q, want %q", got, FixActionPush)
	}

	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("selected action after second cycle = %q, want %q", got, FixActionPush)
	}
}

func TestFixTUIIgnoreKeepsRepoVisibleAndMarksIgnoredState(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.ignoreCurrentRepo()

	if len(m.visible) != 1 {
		t.Fatalf("visible repo count = %d, want 1 ignored repo still visible", len(m.visible))
	}
	if !m.ignored["/repos/api"] {
		t.Fatal("expected repo path to remain in ignored set")
	}

	items := m.repoList.Items()
	found := false
	for _, item := range items {
		row, ok := item.(fixListItem)
		if !ok || row.Kind != fixListItemRepo || row.Path != "/repos/api" {
			continue
		}
		found = true
		if !row.Ignored {
			t.Fatal("expected ignored row marker to be true")
		}
		if row.State != "ignored" {
			t.Fatalf("row state = %q, want ignored", row.State)
		}
	}
	if !found {
		t.Fatal("expected ignored repo row to remain in list")
	}

	details := ansi.Strip(m.viewSelectedRepoDetails())
	if !strings.Contains(details, "State: ignored") {
		t.Fatalf("selected details should show ignored state, got %q", details)
	}
}

func TestFixTUIIgnoredRepoIsExcludedFromApplyAllSelection(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1) // select push
	m.ignoreCurrentRepo()

	if m.hasAnySelectedFixes() {
		t.Fatal("expected ignored repo action selection to be excluded from apply-all eligibility")
	}

	m.applyAllSelections()
	if !strings.Contains(m.status, "applied 0, skipped 1, failed 0") {
		t.Fatalf("status = %q, want ignored repo counted as skipped in apply-all", m.status)
	}
}

func TestFixTUIUnignoreDoesNotClearOtherIgnoredRepos(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/web.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.ignoreCurrentRepo()
	m.setCursor(1)
	m.unignoreCurrentRepo()

	if !m.ignored["/repos/api"] {
		t.Fatal("expected other ignored repo to remain ignored")
	}
	if got := m.status; !strings.Contains(got, "is not ignored") {
		t.Fatalf("status = %q, want non-destructive unignore message", got)
	}
}

func TestFixTUIIgnoreKeyTogglesIgnoredState(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !m.ignored["/repos/api"] {
		t.Fatal("expected i to ignore selected repository")
	}
	if got := m.status; got != "ignored api for this session" {
		t.Fatalf("status after first i = %q, want ignored message", got)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if m.ignored["/repos/api"] {
		t.Fatal("expected second i to unignore selected repository")
	}
	if got := m.status; got != "unignored api" {
		t.Fatalf("status after second i = %q, want unignored message", got)
	}
}

func TestFixTUIHelpUsesIToIgnoreAndUnignore(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)

	initial := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if !helpContains(initial, "i ignore repo") {
		t.Fatalf("expected ignore help shortcut, got %v", initial)
	}
	if helpContains(initial, "u unignore repo") {
		t.Fatalf("expected no u unignore shortcut, got %v", initial)
	}

	m.ignoreCurrentRepo()
	ignored := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if !helpContains(ignored, "i unignore repo") {
		t.Fatalf("expected ignored state to advertise i unignore shortcut, got %v", ignored)
	}
	if helpContains(ignored, "u unignore repo") {
		t.Fatalf("expected no u unignore shortcut while ignored, got %v", ignored)
	}
}

func TestFixTUIHelpDoesNotExposeClearIgnoredShortcut(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/web.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.ignoreCurrentRepo()
	m.setCursor(1)

	entries := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if helpContains(entries, "u clear ignored") {
		t.Fatalf("help should not expose clear ignored shortcut, got %v", entries)
	}
}

func TestFixTUIRevalidateShortcutEntersBusyState(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.loadReposFn = func(_ []string, _ scanRefreshMode) ([]fixRepoState, error) {
		return repos, nil
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	if !m.revalidating {
		t.Fatal("expected revalidate key to set in-progress state")
	}
	rawTitleLine := firstNonEmptyLine(strings.Split(m.View(), "\n"))
	titleLine := firstNonEmptyLine(strings.Split(ansi.Strip(m.View()), "\n"))
	if !strings.Contains(titleLine, "bb") || !strings.Contains(titleLine, "fix") {
		t.Fatalf("expected compact titled border line, got %q", titleLine)
	}
	if !strings.Contains(titleLine, m.revalidateSpinner.View()) {
		t.Fatalf("expected spinner in bordered title while revalidating, got %q", titleLine)
	}
	accentTopCorner := lipgloss.NewStyle().Foreground(accentColor).Render("╭")
	if !strings.Contains(rawTitleLine, accentTopCorner) {
		t.Fatalf("expected top border corner to use busy accent color, got %q", rawTitleLine)
	}
	if got := m.mainContentPanelStyle().GetBorderTopForeground(); !reflect.DeepEqual(got, accentColor) {
		t.Fatalf("busy border color = %#v, want accent %#v", got, accentColor)
	}
}

func TestRenderPanelWithTopTitleUsesPanelTopBorderColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	panel := panelStyle.Copy().Width(56).BorderForeground(accentColor)
	view := renderPanelWithTopTitle(panel, "bb fix", "content")
	topLine := firstNonEmptyLine(strings.Split(view, "\n"))

	accentTopCorner := lipgloss.NewStyle().Foreground(accentColor).Render("╭")
	if !strings.Contains(topLine, accentTopCorner) {
		t.Fatalf("expected top border corner to use panel top border color, got %q", topLine)
	}
}

func TestFixTUIRevalidateCommandUsesFullRefreshAndCompletionClearsBusyState(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	updated := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Syncable:  true,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	refreshMode := scanRefreshNever
	m.loadReposFn = func(_ []string, mode scanRefreshMode) ([]fixRepoState, error) {
		refreshMode = mode
		return updated, nil
	}

	msg := m.revalidateReposCmd("/repos/api")()
	revalidated, ok := msg.(fixTUIRevalidatedMsg)
	if !ok {
		t.Fatalf("revalidate command msg type = %T, want fixTUIRevalidatedMsg", msg)
	}
	if refreshMode != scanRefreshAlways {
		t.Fatalf("revalidate refresh mode = %v, want %v", refreshMode, scanRefreshAlways)
	}
	m.revalidating = true
	_, _ = m.Update(revalidated)
	if m.revalidating {
		t.Fatal("expected busy state to clear after revalidate completion")
	}
	if got := m.mainContentPanelStyle().GetBorderTopForeground(); reflect.DeepEqual(got, accentColor) {
		t.Fatalf("idle border color should not use accent, got %#v", got)
	}
	if !m.visible[0].Record.Syncable {
		t.Fatal("expected revalidated repo data to replace list state")
	}
}

func TestFixTUIImmediateApplyEntersBusyStateWithSpinnerAndLockedStatus(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // schedule pull-ff-only

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected immediate apply command to be scheduled")
	}
	if !m.immediateApplying {
		t.Fatal("expected immediate apply to set in-progress state")
	}
	titleLine := firstNonEmptyLine(strings.Split(ansi.Strip(m.View()), "\n"))
	if !strings.Contains(titleLine, "bb") || !strings.Contains(titleLine, "fix") {
		t.Fatalf("expected compact titled border line, got %q", titleLine)
	}
	if !strings.Contains(titleLine, m.immediateApplySpinner.View()) {
		t.Fatalf("expected spinner in bordered title while immediate apply is running, got %q", titleLine)
	}
	if !strings.Contains(ansi.Strip(m.viewMainContent()), "controls are locked until execution completes") {
		t.Fatalf("expected locked-controls status line during immediate apply, got %q", ansi.Strip(m.viewMainContent()))
	}
	if got := m.mainContentPanelStyle().GetBorderTopForeground(); !reflect.DeepEqual(got, accentColor) {
		t.Fatalf("busy border color = %#v, want accent %#v", got, accentColor)
	}
}

func TestFixTUIImmediateApplyLocksNavigationKeys(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/web.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // schedule pull-ff-only

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected immediate apply command to be scheduled")
	}
	if !m.immediateApplying {
		t.Fatal("expected immediate apply to set in-progress state")
	}

	before := m.repoList.Index()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.repoList.Index(); got != before {
		t.Fatalf("cursor moved while immediate apply was running: before=%d after=%d", before, got)
	}
}

func TestFixTUIImmediateApplyAllEntersBusyState(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // schedule pull-ff-only

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if cmd == nil {
		t.Fatal("expected immediate apply-all command to be scheduled")
	}
	if !m.immediateApplying {
		t.Fatal("expected immediate apply-all to set in-progress state")
	}
}

func TestFixTUIImmediateApplyCompletedSurfacesFirstFailureDetail(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "markdown-llm",
				Path:      "/repos/markdown-llm",
				OriginURL: "https://github.com/niieani/markdown-llm.git",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/niieani/markdown-llm.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.immediateApplying = true

	_, _ = m.Update(fixTUIImmediateApplyCompletedMsg{
		Results: []fixSummaryResult{
			{
				RepoName: "markdown-llm",
				RepoPath: "/repos/markdown-llm",
				Action:   fixActionLabel(FixActionClone),
				Status:   "failed",
				Detail:   "fatal: could not read Username for 'https://github.com': terminal prompts disabled",
			},
		},
		Failed: 1,
	})

	if m.errText == "" {
		t.Fatal("expected concrete failure detail in errText")
	}
	if strings.Contains(m.errText, "one or more fixes failed") {
		t.Fatalf("errText should not be generic, got %q", m.errText)
	}
	if !strings.Contains(m.errText, "terminal prompts disabled") {
		t.Fatalf("errText = %q, want concrete command failure detail", m.errText)
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
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	if repoMetaAutoPushMode(m.visible[0].Meta) != domain.AutoPushModeEnabled {
		t.Fatal("expected auto-push to be enabled after pressing s")
	}
	if !strings.Contains(m.status, "auto-push true") {
		t.Fatalf("status = %q, want auto-push true message", m.status)
	}

	rowItems := m.repoList.Items()
	for _, item := range rowItems {
		row, ok := item.(fixListItem)
		if !ok || row.Kind != fixListItemRepo {
			continue
		}
		if row.Path == "/repos/api" && row.AutoPushMode != domain.AutoPushModeEnabled {
			t.Fatal("expected list row auto-push column to be on")
		}
	}
}

func TestFixTUISettingToggleKeyCyclesAutoPushModes(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{
				RepoKey:   "software/api",
				OriginURL: "https://github.com/you/api.git",
				AutoPush:  domain.AutoPushModeDisabled,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if got := repoMetaAutoPushMode(m.visible[0].Meta); got != domain.AutoPushModeEnabled {
		t.Fatalf("mode after first toggle = %q, want %q", got, domain.AutoPushModeEnabled)
	}
	if !strings.Contains(m.status, "auto-push true") {
		t.Fatalf("status after first toggle = %q, want true mode status", m.status)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if got := repoMetaAutoPushMode(m.visible[0].Meta); got != domain.AutoPushModeIncludeDefaultBranch {
		t.Fatalf("mode after second toggle = %q, want %q", got, domain.AutoPushModeIncludeDefaultBranch)
	}
	if !strings.Contains(m.status, "auto-push include-default-branch") {
		t.Fatalf("status after second toggle = %q, want include-default-branch mode status", m.status)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if got := repoMetaAutoPushMode(m.visible[0].Meta); got != domain.AutoPushModeDisabled {
		t.Fatalf("mode after third toggle = %q, want %q", got, domain.AutoPushModeDisabled)
	}
	if !strings.Contains(m.status, "auto-push false") {
		t.Fatalf("status after third toggle = %q, want false mode status", m.status)
	}
}

func TestFixTUISettingToggleKeyPersistsAutoPushModeViaRepoPolicy(t *testing.T) {
	t.Parallel()

	paths := state.NewPaths(t.TempDir())
	a := New(paths, io.Discard, io.Discard)
	seed := domain.RepoMetadataFile{
		RepoKey:    "software/api",
		Name:       "api",
		OriginURL:  "https://github.com/you/api.git",
		AutoPush:   domain.AutoPushModeDisabled,
		PushAccess: domain.PushAccessReadWrite,
	}
	if err := state.SaveRepoMetadata(paths, seed); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				RepoKey:   "software/api",
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Branch:    "main",
				Upstream:  "origin/main",
				Syncable:  false,
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushPolicyBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:    "software/api",
				OriginURL:  "https://github.com/you/api.git",
				AutoPush:   domain.AutoPushModeDisabled,
				PushAccess: domain.PushAccessReadWrite,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.app = a
	m.setCursor(0)

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	updated, err := state.LoadRepoMetadata(paths, "software/api")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if got := domain.NormalizeAutoPushMode(updated.AutoPush); got != domain.AutoPushModeIncludeDefaultBranch {
		t.Fatalf("persisted auto_push mode = %q, want %q", got, domain.AutoPushModeIncludeDefaultBranch)
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
				AutoPush:   domain.AutoPushModeDisabled,
				PushAccess: domain.PushAccessReadOnly,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	if repoMetaAutoPushMode(m.visible[0].Meta) != domain.AutoPushModeDisabled {
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
			Meta: &domain.RepoMetadataFile{OriginURL: fmt.Sprintf("https://github.com/you/repo-%02d.git", i), AutoPush: domain.AutoPushModeDisabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.applyAllSelections()

	if !strings.Contains(m.status, "applied 0") {
		t.Fatalf("status = %q, expected skipped apply-all summary", m.status)
	}
}

func TestFixTUIEnterRunsCurrentFixWhenNothingSelected(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to run current fix even with no explicit selection")
	}
	if !m.immediateApplying {
		t.Fatal("expected immediate apply state when running current non-risky fix")
	}
}

func TestFixTUIEnterRunsCurrentRiskyFixWhenNothingSelected(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1) // push

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want wizard mode", m.viewMode)
	}
	if m.wizard.Action != FixActionPush {
		t.Fatalf("wizard action = %q, want %q", m.wizard.Action, FixActionPush)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1) // push
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
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
			Meta: &domain.RepoMetadataFile{RepoKey: "software/api", OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	if got := actionForVisibleRepo(m, 0); got != FixActionAbortOperation {
		t.Fatalf("selected action = %q, want %q", got, FixActionAbortOperation)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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

func TestFixTUIWizardSummaryViewShowsSinglePreciseHeadingAndTotals(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Syncable:  true,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.summaryResults = []fixSummaryResult{
		{
			RepoName: "api",
			RepoPath: "/repos/api",
			Action:   fixActionLabel(FixActionPush),
			Status:   "applied",
		},
	}
	m.viewMode = fixViewSummary

	view := ansi.Strip(m.viewSummaryContent())
	if !strings.Contains(view, "Fix outcomes and current syncability after revalidation.") {
		t.Fatalf("expected precise summary heading, got %q", view)
	}
	if strings.Contains(view, "Fix Summary") || strings.Contains(view, "Results from this apply session.") {
		t.Fatalf("expected redundant summary copy to be removed, got %q", view)
	}
	if !strings.Contains(view, "Session totals") {
		t.Fatalf("expected totals block inside summary view, got %q", view)
	}
	if !strings.Contains(view, "Action outcomes") {
		t.Fatalf("expected action outcomes totals section, got %q", view)
	}
	if !strings.Contains(view, "Applied: 1") || !strings.Contains(view, "Skipped: 0") {
		t.Fatalf("expected applied/skipped counts, got %q", view)
	}
	if strings.Contains(view, "Failed: 0") {
		t.Fatalf("expected zero-failure case to avoid failure marker, got %q", view)
	}
	if !strings.Contains(view, "Errors: none") {
		t.Fatalf("expected explicit zero-error text, got %q", view)
	}
	if !strings.Contains(view, "Revalidation") || !strings.Contains(view, "Syncable now: 1") || !strings.Contains(view, "Still unsyncable: 0") {
		t.Fatalf("expected revalidation totals, got %q", view)
	}
	if !strings.Contains(view, "Revalidation: syncable now.") {
		t.Fatalf("expected post-revalidation syncable outcome, got %q", view)
	}
}

func TestFixTUIWizardSummaryGroupsMultipleActionsPerRepoIntoSingleRepoBlock(t *testing.T) {
	t.Parallel()

	repoPath := "/Volumes/Projects/Software/ultrasound-extractor"
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "ultrasound-extractor",
				Path:      repoPath,
				OriginURL: "git@github.com:you/ultrasound-extractor.git",
				Upstream:  "origin/main",
				Syncable:  true,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/ultrasound-extractor.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.summaryResults = []fixSummaryResult{
		{
			RepoName: "ultrasound-extractor",
			RepoPath: repoPath,
			Action:   fixActionLabel(FixActionStageCommitPush),
			Status:   "applied",
		},
		{
			RepoName: "ultrasound-extractor",
			RepoPath: repoPath,
			Action:   fixActionLabel(FixActionCreateProject),
			Status:   "applied",
		},
	}
	m.viewMode = fixViewSummary

	view := ansi.Strip(m.viewSummaryContent())
	if got := strings.Count(view, repoPath); got != 1 {
		t.Fatalf("expected single repo block for %q, path occurrence count=%d, view=%q", repoPath, got, view)
	}
	if !strings.Contains(view, "✓ Stage, commit & push: applied") {
		t.Fatalf("expected stage-commit-push result in grouped repo block, got %q", view)
	}
	if !strings.Contains(view, "✓ Create project & push: applied") {
		t.Fatalf("expected create-project result in grouped repo block, got %q", view)
	}
	if got := strings.Count(view, "Revalidation: syncable now."); got != 1 {
		t.Fatalf("expected single revalidation outcome line in grouped repo block, got count=%d, view=%q", got, view)
	}
}

func TestFixTUIWizardSummaryViewReportsWhenMoreFixesAreStillNeeded(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Diverged:  true,
				Syncable:  false,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonDiverged,
				},
			},
			SyncFeasibility: fixSyncFeasibility{
				Checked:       true,
				RebaseOutcome: fixSyncProbeClean,
				MergeOutcome:  fixSyncProbeClean,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.summaryResults = []fixSummaryResult{
		{
			RepoName: "api",
			RepoPath: "/repos/api",
			Action:   fixActionLabel(FixActionPush),
			Status:   "applied",
		},
	}
	m.viewMode = fixViewSummary

	view := ansi.Strip(m.viewSummaryContent())
	if !strings.Contains(view, "Revalidation: unsyncable (1 blocker).") {
		t.Fatalf("expected explicit unsyncable blocker count, got %q", view)
	}
	if !strings.Contains(view, "Remaining blockers") || !strings.Contains(view, "Branch diverged from upstream (diverged)") {
		t.Fatalf("expected blocker list with reason labels, got %q", view)
	}
	if !strings.Contains(view, "Automated next fixes") || !strings.Contains(view, "[ ] Sync with upstream") {
		t.Fatalf("expected actionable automated follow-up fixes, got %q", view)
	}
}

func TestFixTUIWizardSummaryViewFlagsManualInterventionWhenNoAutomatedFixesRemain(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Diverged:  true,
				Syncable:  false,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonSyncConflict,
				},
			},
			SyncFeasibility: fixSyncFeasibility{
				Checked:       true,
				RebaseOutcome: fixSyncProbeConflict,
				MergeOutcome:  fixSyncProbeConflict,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.summaryResults = []fixSummaryResult{
		{
			RepoName: "api",
			RepoPath: "/repos/api",
			Action:   fixActionLabel(FixActionPush),
			Status:   "applied",
		},
	}
	m.viewMode = fixViewSummary

	view := ansi.Strip(m.viewSummaryContent())
	if !strings.Contains(view, "Manual intervention required - bb has no additional safe automated fixes for this repo.") {
		t.Fatalf("expected explicit manual intervention message, got %q", view)
	}
	if !strings.Contains(view, "Resolve merge/rebase conflicts manually, then revalidate.") {
		t.Fatalf("expected concrete manual resolution guidance, got %q", view)
	}
	if strings.Contains(view, "Automated next fixes") {
		t.Fatalf("expected no automated fix list when none are available, got %q", view)
	}
}

func TestFixTUISummaryFollowUpSelectionCanQueueAndRunFixes(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:              "api",
				Path:              "/repos/api",
				OriginURL:         "git@github.com:you/api.git",
				Upstream:          "origin/main",
				HasDirtyTracked:   true,
				Syncable:          false,
				UnsyncableReasons: []domain.UnsyncableReason{domain.ReasonDirtyTracked},
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.summaryResults = []fixSummaryResult{
		{
			RepoName: "api",
			RepoPath: "/repos/api",
			Action:   fixActionLabel(FixActionPush),
			Status:   "applied",
		},
	}
	m.viewMode = fixViewSummary

	view := ansi.Strip(m.viewSummaryContent())
	if !strings.Contains(view, "[ ] Stage, commit & push") {
		t.Fatalf("expected selectable follow-up fixes, got %q", view)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	view = ansi.Strip(m.viewSummaryContent())
	if !strings.Contains(view, "[x] Stage, commit & push") {
		t.Fatalf("expected selected follow-up fix checkbox, got %q", view)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode after running selected follow-up fix = %v, want wizard", m.viewMode)
	}
	if m.wizard.Action != FixActionStageCommitPush {
		t.Fatalf("wizard action after summary follow-up run = %q, want %q", m.wizard.Action, FixActionStageCommitPush)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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

func TestFixTUIWizardCommitGenerateButtonIsSymbolic(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "✨") {
		t.Fatalf("expected symbolic commit generation button, got %q", view)
	}
}

func TestFixTUIWizardCommitGenerateButtonMatchesInputHeight(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	input := renderInputContainer(m.wizard.CommitMessage.View(), true)
	button := m.renderWizardCommitGenerateButton("✨", lipgloss.Height(input), false)
	if lipgloss.Height(button) != lipgloss.Height(input) {
		t.Fatalf("commit generate button height = %d, want %d", lipgloss.Height(button), lipgloss.Height(input))
	}
}

func TestFixTUIWizardCommitGenerateButtonRunsImmediatelyAndReplacesMessage(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	m.generateCommitMessageFn = func(repoPath string) (string, error) {
		if repoPath != "/repos/api" {
			t.Fatalf("generate repo path = %q, want %q", repoPath, "/repos/api")
		}
		return "feat: generated message", nil
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if !m.wizard.CommitButtonFocused {
		t.Fatal("expected commit button focus after right key")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.wizard.CommitGenerating {
		t.Fatal("expected commit generation to start immediately on enter")
	}
	if cmd == nil {
		t.Fatal("expected async commit generation command")
	}
	_, _ = m.Update(fixWizardCommitGeneratedMsg{Message: "feat: generated message"})

	if m.wizard.CommitGenerating {
		t.Fatal("expected commit generation spinner to stop after completion")
	}
	if got := m.wizard.CommitMessage.Value(); got != "feat: generated message" {
		t.Fatalf("commit input value = %q, want %q", got, "feat: generated message")
	}
	if m.wizard.CommitButtonFocused {
		t.Fatal("expected focus to return to commit input after generation")
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := m.viewWizardContent()
	if !strings.Contains(view, "Apply") || !strings.Contains(view, "Skip") || !strings.Contains(view, "Cancel") {
		t.Fatalf("wizard view should render action buttons, got %q", view)
	}
}

func TestFixTUIWizardViewIncludesApplyingPlanBlock(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "Review before applying") {
		t.Fatalf("expected applying-plan block heading, got %q", view)
	}
	if !strings.Contains(view, "Applying this fix will execute these steps in order:") {
		t.Fatalf("expected ordered apply steps heading in applying-plan block, got %q", view)
	}
	if !strings.Contains(view, "git push") {
		t.Fatalf("expected push command in applying-plan block, got %q", view)
	}
	if !strings.Contains(view, "Revalidate repository status and syncability state.") {
		t.Fatalf("expected post-apply revalidation step in applying-plan block, got %q", view)
	}
}

func TestFixTUIWizardApplyingPlanShowsCommandsAndNonCommandActions(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "",
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL:        "https://github.com/you/api.git",
				AutoPush:         domain.AutoPushModeEnabled,
				PreferredRemote:  "origin",
				PushAccess:       domain.PushAccessReadWrite,
				PreferredCatalog: "software",
			},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       true,
				NoisyChangedPaths:          []string{"node_modules/pkg/index.js"},
				SuggestedGitignorePatterns: []string{"node_modules/"},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "git add -A") {
		t.Fatalf("expected git add command in applying-plan block, got %q", view)
	}
	if !strings.Contains(view, "git commit -m") {
		t.Fatalf("expected git commit command in applying-plan block, got %q", view)
	}
	if !strings.Contains(view, "Generate root .gitignore") {
		t.Fatalf("expected non-command gitignore action in applying-plan block, got %q", view)
	}
	if strings.Contains(view, "Applying this fix will run these commands:") || strings.Contains(view, "It will also perform these side effects:") {
		t.Fatalf("expected a single ordered steps list without split sections, got %q", view)
	}
	if strings.Index(view, "Generate root .gitignore") > strings.Index(view, "git add -A") {
		t.Fatalf("expected side effect to be listed before later commands in execution order, got %q", view)
	}
}

func TestFixTUIWizardApplyingPlanShowsRuntimeStepMarkers(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL:       "https://github.com/you/api.git",
				AutoPush:        domain.AutoPushModeEnabled,
				PreferredRemote: "origin",
			},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       true,
				NoisyChangedPaths:          []string{"node_modules/pkg/index.js"},
				SuggestedGitignorePatterns: []string{"node_modules/"},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	entries := m.wizardApplyPlanEntries()
	if len(entries) < 3 {
		t.Fatalf("expected multi-step plan, got %d entries", len(entries))
	}
	m.wizard.ApplyPlan = append([]fixActionPlanEntry(nil), entries...)
	m.wizard.ApplyStepStatus = map[string]fixWizardApplyStepStatus{}
	m.setWizardStepStatus(entries[0], fixWizardApplyStepDone)
	m.setWizardStepStatus(entries[1], fixWizardApplyStepRunning)
	m.setWizardStepStatus(entries[2], fixWizardApplyStepFailed)

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "✓") {
		t.Fatalf("expected success marker in applying-plan block, got %q", view)
	}
	if !strings.Contains(view, "✗") {
		t.Fatalf("expected failure marker in applying-plan block, got %q", view)
	}
}

func TestFixTUIWizardApplyingStatusLineShowsGlobalPhaseAndLockedState(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	m.wizard.Applying = true
	m.wizard.ApplyPhase = fixWizardApplyPhasePreparing

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "Preparing operation... controls are locked until execution completes.") {
		t.Fatalf("expected locked apply status line with preparation phase, got %q", view)
	}
}

func TestFixTUIWizardApplyProgressUpdatesGlobalPhase(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	m.wizard.Applying = true

	m.handleWizardApplyProgress(fixWizardApplyProgressMsg{
		Event: fixApplyStepEvent{
			Entry:  fixActionPlanEntry{ID: "push-main", Summary: "git push"},
			Status: fixApplyStepRunning,
		},
	})
	if got := m.wizard.ApplyPhase; got != fixWizardApplyPhaseExecuting {
		t.Fatalf("apply phase after non-revalidation running step = %q, want %q", got, fixWizardApplyPhaseExecuting)
	}

	m.handleWizardApplyProgress(fixWizardApplyProgressMsg{
		Event: fixApplyStepEvent{
			Entry:  fixActionPlanEntry{ID: fixActionPlanRevalidateStateID, Summary: "Revalidate repository status and syncability state."},
			Status: fixApplyStepRunning,
		},
	})
	if got := m.wizard.ApplyPhase; got != fixWizardApplyPhaseRechecking {
		t.Fatalf("apply phase after revalidation running step = %q, want %q", got, fixWizardApplyPhaseRechecking)
	}
}

func TestFixTUIWizardApplyFailureStopsQueueOnCurrentItem(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/web.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{
		{RepoPath: "/repos/api", Action: FixActionPush},
		{RepoPath: "/repos/web", Action: FixActionPush},
	})
	m.wizard.Applying = true
	m.wizard.ApplyPhase = fixWizardApplyPhaseExecuting

	m.handleWizardApplyCompleted(fixWizardApplyCompletedMsg{Err: fmt.Errorf("push failed")})

	if m.wizard.Applying {
		t.Fatal("expected applying=false after completion error")
	}
	if got := m.wizard.ApplyPhase; got != "" {
		t.Fatalf("apply phase after completion error = %q, want empty", got)
	}
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want wizard mode after failed apply", m.viewMode)
	}
	if got := m.wizard.Index; got != 0 {
		t.Fatalf("wizard index = %d, want 0 to stop on current item", got)
	}
	if len(m.summaryResults) != 0 {
		t.Fatalf("summary results = %d, want no auto-advance result on failure", len(m.summaryResults))
	}
	if !strings.Contains(m.status, "apply failed for api") {
		t.Fatalf("status = %q, want failure guidance for current repo", m.status)
	}
}

func TestFixTUIWizardApplyCompletionUpdatesCurrentRepoState(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Syncable:  false,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})
	m.wizard.Applying = true

	updated := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:      "api",
			Path:      "/repos/api",
			OriginURL: "git@github.com:you/api.git",
			Upstream:  "origin/main",
			Syncable:  true,
		},
		Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
	}

	m.handleWizardApplyCompleted(fixWizardApplyCompletedMsg{Updated: updated})

	if !m.repos[0].Record.Syncable {
		t.Fatal("expected in-memory repo state to be updated after successful apply completion")
	}
	if m.viewMode != fixViewSummary {
		t.Fatalf("view mode = %v, want summary after single-item queue completion", m.viewMode)
	}
}

func TestFixTUIWizardApplyPlanBlockRendersAfterRiskContext(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "",
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL: "https://github.com/you/api.git",
				AutoPush:  domain.AutoPushModeEnabled,
			},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       true,
				NoisyChangedPaths:          []string{"node_modules/pkg/index.js"},
				SuggestedGitignorePatterns: []string{"node_modules/"},
				ChangedFiles: []fixChangedFile{
					{Path: "node_modules/pkg/index.js", Added: 1, Deleted: 0},
				},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	view := ansi.Strip(m.viewWizardContent())
	changedIdx := strings.Index(view, "Uncommitted changed files")
	noisyIdx := strings.Index(view, "Missing root .gitignore")
	planIdx := strings.Index(view, "Review before applying")
	if changedIdx < 0 || noisyIdx < 0 || planIdx < 0 {
		t.Fatalf("expected changed/noisy/plan sections, got %q", view)
	}
	if !(changedIdx < noisyIdx && noisyIdx < planIdx) {
		t.Fatalf("expected apply-plan block after risk context, got %q", view)
	}
}

func TestFixTUIWizardChangedFilesDescriptionDependsOnAction(t *testing.T) {
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
					{Path: "src/main.go", Added: 1, Deleted: 1},
				},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	createProjectView := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(createProjectView, "These uncommitted files will be staged and committed before the initial push.") {
		t.Fatalf("expected stage/commit changed-files description for create-project, got %q", createProjectView)
	}

	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})
	stageCommitView := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(stageCommitView, "These uncommitted files will be staged and committed by this fix.") {
		t.Fatalf("expected stage/commit changed-files description for stage-commit-push, got %q", stageCommitView)
	}
}

func TestFixTUIWizardHidesNoOpChangedFilesSection(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
			Risk: fixRiskSnapshot{
				ChangedFiles: nil,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := ansi.Strip(m.viewWizardContent())
	if strings.Contains(view, "Uncommitted changed files") {
		t.Fatalf("did not expect changed-files section with no changed files, got %q", view)
	}
	if strings.Contains(view, "No uncommitted changes detected.") {
		t.Fatalf("did not expect no-op changed-files placeholder, got %q", view)
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
				Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	view := ansi.Strip(m.viewWizardContent())
	buttonsLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Cancel") && strings.Contains(line, "Skip") && strings.Contains(line, "Apply") {
			buttonsLine = line
			break
		}
	}
	cancelIdx := strings.Index(buttonsLine, "Cancel")
	skipIdx := strings.Index(buttonsLine, "Skip")
	applyIdx := strings.Index(buttonsLine, "Apply")
	if cancelIdx < 0 || skipIdx < 0 || applyIdx < 0 {
		t.Fatalf("wizard buttons missing in view: %q", view)
	}
	if !(cancelIdx < skipIdx && skipIdx < applyIdx) {
		t.Fatalf("expected button order Cancel -> Skip -> Apply, got line %q", buttonsLine)
	}
}

func TestRenderWizardActionButtonsUsesVisibleFocusMarker(t *testing.T) {
	t.Parallel()

	view := ansi.Strip(renderWizardActionButtons(fixWizardActionSkip, "Apply"))
	if !strings.Contains(view, "[Skip]") {
		t.Fatalf("expected focused marker on Skip, got %q", view)
	}
	if strings.Contains(view, "[Cancel]") || strings.Contains(view, "[Apply]") {
		t.Fatalf("expected only Skip to be focused, got %q", view)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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

func TestFixTUIWizardUsesOnlyGlobalFooterHelpLegend(t *testing.T) {
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

	body := ansi.Strip(m.viewWizardContent())
	if strings.Contains(body, "tab/shift+tab") || strings.Contains(body, "↑/↓") || strings.Contains(body, "←/→") {
		t.Fatalf("wizard body should not render inline key legend, got %q", body)
	}

	full := ansi.Strip(m.View())
	if count := strings.Count(full, "tab/shift+tab"); count != 1 {
		t.Fatalf("expected key legend to appear exactly once in global footer, count=%d view=%q", count, full)
	}
}

func TestFixTUIWizardGlobalFooterHelpReflectsCurrentFocus(t *testing.T) {
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
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	projectHelp := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if !helpContains(projectHelp, "enter next field") {
		t.Fatalf("expected project-name focus help to include enter-next, got %v", projectHelp)
	}
	if helpContains(projectHelp, "←/→ change visibility") || helpContains(projectHelp, "←/→ select button") || helpContains(projectHelp, "space toggle") {
		t.Fatalf("project-name focus should not include unrelated shortcuts, got %v", projectHelp)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project name -> visibility
	visibilityHelp := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if !helpContains(visibilityHelp, "←/→ change visibility") {
		t.Fatalf("expected visibility focus help to include left/right visibility shortcut, got %v", visibilityHelp)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // visibility -> actions
	actionsHelp := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if !helpContains(actionsHelp, "enter activate") || !helpContains(actionsHelp, "←/→ select button") {
		t.Fatalf("expected actions focus help to include action shortcuts, got %v", actionsHelp)
	}
}

func TestFixTUIListGlobalFooterHelpShowsOnlyAvailableActions(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     2,
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:    "software/api",
				OriginURL:  "https://github.com/you/api.git",
				AutoPush:   domain.AutoPushModeDisabled,
				PushAccess: domain.PushAccessReadWrite,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.setCursor(0)

	initial := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if !helpContains(initial, "enter run current") {
		t.Fatalf("expected enter to advertise running current fix when nothing is selected, got %v", initial)
	}
	if helpContains(initial, "ctrl+a run all selected") {
		t.Fatalf("expected apply-all hidden when nothing is selected, got %v", initial)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // schedule push
	selected := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if len(selected) == 0 || selected[0] != "enter run selected/current" {
		t.Fatalf("expected most important shortcut first (enter run selected/current), got %v", selected)
	}
	if !helpContains(selected, "ctrl+a run all selected") || !helpContains(selected, "s cycle auto-push") {
		t.Fatalf("expected selected-action footer help to include apply-all and auto-push, got %v", selected)
	}
	if strings.Contains(strings.Join(selected, " • "), "Left") || strings.Contains(strings.Join(selected, " • "), "Right") {
		t.Fatalf("expected compact lowercase/symbol key labels, got %v", selected)
	}

	m.repos[0].Meta.PushAccess = domain.PushAccessReadOnly
	m.rebuildList("/repos/api")
	withoutAutoPush := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if helpContains(withoutAutoPush, "s cycle auto-push") {
		t.Fatalf("expected auto-push shortcut hidden when n/a, got %v", withoutAutoPush)
	}

	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, " • ") {
		t.Fatalf("expected global footer to use bullet separators, got %q", view)
	}
}

func TestFixTUIWizardShortFooterHelpIsSingleLineWithEllipsis(t *testing.T) {
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
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 86, Height: 24})

	lines := footerHelpContentLines(m.View())
	if len(lines) != 1 {
		t.Fatalf("expected collapsed footer help to stay single-line, got %d lines: %v", len(lines), lines)
	}
	if !strings.HasSuffix(lines[0], "…") {
		t.Fatalf("expected collapsed footer help to end with ellipsis when truncated, got %q", lines[0])
	}
}

func TestFixTUIWizardQuestionMarkTogglesExpandedFooterHelp(t *testing.T) {
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
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	collapsed := footerHelpContentLines(m.View())
	if len(collapsed) != 1 {
		t.Fatalf("expected collapsed footer help to start as single-line, got %d lines: %v", len(collapsed), collapsed)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.help.ShowAll {
		t.Fatal("expected ? to enable expanded help in wizard")
	}

	expanded := footerHelpContentLines(m.View())
	if len(expanded) <= 1 {
		t.Fatalf("expected expanded footer help to include multiple lines, got %d lines: %v", len(expanded), expanded)
	}
	expandedText := strings.Join(expanded, " ")
	if !strings.Contains(expandedText, "q quit") {
		t.Fatalf("expected expanded footer help to include comprehensive keys, got %q", expandedText)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.help.ShowAll {
		t.Fatal("expected ? to collapse help in wizard")
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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

func TestFixTUIWizardVisualDiffButtonLaunchesAndReturnsToSameWizardState(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
			Risk: fixRiskSnapshot{
				ChangedFiles: []fixChangedFile{
					{Path: "src/main.go", Added: 2, Deleted: 1},
				},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	shortcutLabel := visualDiffShortcutDisplayLabel(runtime.GOOS)
	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, shortcutLabel) || !strings.Contains(view, "open visual diff viewer (lumen).") {
		t.Fatalf("expected shortcut hint copy in changed-files block, got %q", view)
	}

	execCalled := false
	m.prepareVisualDiffCmdFn = func(repoPath string, args []string) (*exec.Cmd, func(error) error, error) {
		if repoPath != "/repos/api" {
			t.Fatalf("visual diff repo path = %q, want %q", repoPath, "/repos/api")
		}
		if len(args) != 0 {
			t.Fatalf("visual diff args = %v, want empty", args)
		}
		return &exec.Cmd{}, func(err error) error { return err }, nil
	}
	m.execProcessFn = func(_ *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		execCalled = true
		return func() tea.Msg {
			return fn(nil)
		}
	}

	focusBefore := m.wizard.FocusArea
	indexBefore := m.wizard.Index

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v"), Alt: true})
	if !execCalled {
		t.Fatal("expected visual diff exec process to be launched")
	}
	if cmd == nil {
		t.Fatal("expected exec callback command after launching visual diff")
	}
	_, _ = m.Update(cmd())

	if got := m.wizard.FocusArea; got != focusBefore {
		t.Fatalf("focus area after returning from visual diff = %v, want %v", got, focusBefore)
	}
	if got := m.wizard.Index; got != indexBefore {
		t.Fatalf("wizard index after returning from visual diff = %d, want %d", got, indexBefore)
	}
}

func TestFixTUIWizardVisualDiffShortcutUsesAltV(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
			Risk: fixRiskSnapshot{
				ChangedFiles: []fixChangedFile{
					{Path: "src/main.go", Added: 2, Deleted: 1},
				},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	execCalled := false
	m.prepareVisualDiffCmdFn = func(_ string, _ []string) (*exec.Cmd, func(error) error, error) {
		return &exec.Cmd{}, func(err error) error { return err }, nil
	}
	m.execProcessFn = func(_ *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		execCalled = true
		return func() tea.Msg { return fn(nil) }
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v"), Alt: true})
	if !execCalled {
		t.Fatal("expected alt+v shortcut to launch visual diff")
	}
	if cmd == nil {
		t.Fatal("expected exec callback command")
	}
}

func TestFixTUIWizardHelpShowsAltVForVisualDiff(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
			Risk: fixRiskSnapshot{
				ChangedFiles: []fixChangedFile{
					{Path: "src/main.go", Added: 2, Deleted: 1},
				},
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionPush}})

	shortcutLabel := visualDiffShortcutDisplayLabel(runtime.GOOS)
	entries := shortHelpEntries(m.contextualHelpMap().ShortHelp())
	if !helpContains(entries, shortcutLabel+" visual diff") {
		t.Fatalf("expected wizard help to include visual diff shortcut %q, got %v", shortcutLabel, entries)
	}
}

func TestFixTUIWizardChangedFilesShortcutNotInFocusFlow(t *testing.T) {
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

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // project -> visibility
	if m.wizard.FocusArea != fixWizardFocusVisibility {
		t.Fatalf("focus area = %v, want visibility before bottom-edge check", m.wizard.FocusArea)
	}

	m.wizard.BodyViewport.GotoBottom()
	if !m.wizard.BodyViewport.AtBottom() {
		t.Fatal("expected wizard viewport to be at bottom")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // visibility -> actions (no intermediate visual-diff focus)
	if m.wizard.FocusArea != fixWizardFocusActions {
		t.Fatalf("focus area after down at bottom = %v, want actions", m.wizard.FocusArea)
	}
}

func TestFixTUIWizardApplyActionRequiresReviewBeforeBottom(t *testing.T) {
	t.Parallel()

	changed := make([]fixChangedFile, 0, 24)
	for i := 0; i < 24; i++ {
		changed = append(changed, fixChangedFile{Path: fmt.Sprintf("src/file-%02d.go", i), Added: i + 1, Deleted: i})
	}
	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "",
			},
			Meta: nil,
			Risk: fixRiskSnapshot{ChangedFiles: changed},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionCreateProject}})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 16})

	for m.wizard.FocusArea != fixWizardFocusActions {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m.wizard.ActionFocus = fixWizardActionApply
	m.wizard.BodyViewport.GotoTop()
	if m.wizard.BodyViewport.AtBottom() {
		t.Fatal("expected viewport to require review before apply")
	}

	viewBefore := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(viewBefore, "Review") {
		t.Fatalf("expected Review action before reaching bottom, got %q", viewBefore)
	}
	if strings.Contains(viewBefore, "[Apply]") {
		t.Fatalf("did not expect Apply action before reaching bottom, got %q", viewBefore)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Review
	if !m.wizard.BodyViewport.AtBottom() {
		t.Fatal("expected Review action to scroll viewport to bottom")
	}
	if m.wizard.FocusArea != fixWizardFocusActions || m.wizard.ActionFocus != fixWizardActionApply {
		t.Fatalf("expected focus to remain on apply action, got focus=%v actionFocus=%d", m.wizard.FocusArea, m.wizard.ActionFocus)
	}
	if m.wizard.Applying {
		t.Fatal("did not expect apply execution to start from Review action")
	}

	viewAfter := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(viewAfter, "Apply") {
		t.Fatalf("expected Apply action after review scroll, got %q", viewAfter)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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

func TestFixTUIWizardChangedFilesShowsAutoIgnoreBadgeForSuggestedPaths(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       true,
				SuggestedGitignorePatterns: []string{"node_modules/"},
				NoisyChangedPaths:          []string{"node_modules/pkg/index.js"},
				ChangedFiles: []fixChangedFile{
					{Path: "node_modules/pkg/index.js", Added: 12, Deleted: 3},
					{Path: "src/main.go", Added: 1, Deleted: 1},
				},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	view := ansi.Strip(m.viewWizardContent())
	noisyLine := lineContaining(view, "node_modules/pkg/index.js")
	if !strings.Contains(noisyLine, "AUTO-IGNORE") {
		t.Fatalf("expected AUTO-IGNORE badge for suggested noisy file, got line %q", noisyLine)
	}
	normalLine := lineContaining(view, "src/main.go")
	if strings.Contains(normalLine, "AUTO-IGNORE") {
		t.Fatalf("did not expect AUTO-IGNORE badge for normal file, got line %q", normalLine)
	}
}

func TestFixTUIWizardDoesNotShowGitignoreToggleWhenNoisyPatternsAlreadyCovered(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:       false,
				NoisyChangedPaths:          []string{"node_modules/pkg/index.js"},
				SuggestedGitignorePatterns: []string{"node_modules/"},
				MissingGitignorePatterns:   nil,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	view := ansi.Strip(m.viewWizardContent())
	if strings.Contains(view, "Generate .gitignore before commit") || strings.Contains(view, "Append to .gitignore before commit") {
		t.Fatalf("expected no gitignore toggle when noisy paths are already covered, got %q", view)
	}
}

func TestFixTUIWizardShowsAppendGitignoreToggleWhenEntriesMissing(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
			Risk: fixRiskSnapshot{
				MissingRootGitignore:     false,
				NoisyChangedPaths:        []string{"node_modules/pkg/index.js"},
				MissingGitignorePatterns: []string{"node_modules/"},
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "Append to .gitignore before commit") {
		t.Fatalf("expected append-to-gitignore toggle, got %q", view)
	}
	if strings.Contains(view, "Only when root .gitignore is missing.") {
		t.Fatalf("expected gitignore toggle subtext to be removed, got %q", view)
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

func TestFixTUIWizardCreateProjectNameValidationBlocksApply(t *testing.T) {
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
	m.wizard.ProjectName.SetValue(".")
	m.wizard.ActionFocus = fixWizardActionApply

	m.applyWizardCurrent()

	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want wizard after validation error", m.viewMode)
	}
	if m.errText == "" {
		t.Fatal("expected validation error text for invalid repository name")
	}
	if !strings.Contains(m.errText, "invalid repository name") {
		t.Fatalf("unexpected error text: %q", m.errText)
	}
	if len(m.summaryResults) != 0 {
		t.Fatalf("summary results = %d, want 0 when apply is blocked by validation", len(m.summaryResults))
	}
}

func TestFixTUIWizardCreateProjectNameSanitizesTypingAndPaste(t *testing.T) {
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

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("repo name")})
	if got := m.wizard.ProjectName.Value(); got != "repo-name" {
		t.Fatalf("project name after typing = %q, want %q", got, "repo-name")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/with*chars")})
	if got := m.wizard.ProjectName.Value(); got != "repo-name-with-chars" {
		t.Fatalf("project name after paste-like input = %q, want %q", got, "repo-name-with-chars")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-._ABC")})
	if got := m.wizard.ProjectName.Value(); got != "repo-name-with-chars-._abc" {
		t.Fatalf("project name after allowed punctuation and uppercase = %q, want %q", got, "repo-name-with-chars-._abc")
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

func TestFixTUIWizardForkAndRetargetDefaultBranchShowsOptionalBranchRename(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "bun",
				Path:      "/repos/bun",
				OriginURL: "git@github.com:oven-sh/bun.git",
				Branch:    "main",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:         "software/bun",
				Name:            "bun",
				OriginURL:       "https://github.com/oven-sh/bun.git",
				PreferredRemote: "origin",
				PushAccess:      domain.PushAccessReadOnly,
				AutoPush:        domain.AutoPushModeDisabled,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/bun", Action: FixActionForkAndRetarget}})

	if !m.wizard.EnableForkBranchRename {
		t.Fatal("expected fork branch rename input to be enabled on default branch")
	}
	if got := m.wizard.ForkBranchName.Value(); got != "" {
		t.Fatalf("fork branch rename initial value = %q, want empty", got)
	}
	if got := m.wizard.FocusArea; got != fixWizardFocusForkBranch {
		t.Fatalf("initial focus area = %v, want fork-branch input", got)
	}

	m.wizard.GitHubOwner = "acme"
	m.wizard.ForkBranchName.SetValue("feature/fork-bun")
	plan := m.wizardApplyPlanEntries()
	if !planContains(plan, true, "git checkout -b feature/fork-bun") {
		t.Fatalf("expected branch checkout step in wizard plan, got %#v", plan)
	}
	if !planContains(plan, true, "git push -u") {
		t.Fatalf("expected non-force push step in wizard plan after branch rename, got %#v", plan)
	}
	if planContains(plan, true, "--force") {
		t.Fatalf("did not expect force push in wizard plan when rename target is set, got %#v", plan)
	}

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "Publish as new branch (optional)") {
		t.Fatalf("expected optional branch rename control in wizard view, got %q", view)
	}
}

func TestFixTUIWizardForkAndRetargetNonDefaultBranchHidesBranchRename(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "bun",
				Path:      "/repos/bun",
				OriginURL: "git@github.com:oven-sh/bun.git",
				Branch:    "feature/local-change",
				Upstream:  "origin/feature/local-change",
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:         "software/bun",
				Name:            "bun",
				OriginURL:       "https://github.com/oven-sh/bun.git",
				PreferredRemote: "origin",
				PushAccess:      domain.PushAccessReadOnly,
				AutoPush:        domain.AutoPushModeDisabled,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/bun", Action: FixActionForkAndRetarget}})

	if m.wizard.EnableForkBranchRename {
		t.Fatal("did not expect fork branch rename input on non-default branch")
	}
	view := ansi.Strip(m.viewWizardContent())
	if strings.Contains(view, "Publish as new branch (optional)") {
		t.Fatalf("did not expect optional branch rename control in wizard view, got %q", view)
	}
}

func TestFixTUIWizardStageCommitPushDefaultBranchShowsOptionalPublishBranch(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Branch:    "main",
				Upstream:  "origin/main",
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:         "software/api",
				Name:            "api",
				OriginURL:       "https://github.com/you/api.git",
				PreferredRemote: "origin",
				AutoPush:        domain.AutoPushModeDisabled,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	if !m.wizard.EnableForkBranchRename {
		t.Fatal("expected optional publish-branch input to be enabled on default branch")
	}
	m.wizard.ForkBranchName.SetValue("feature/safe-publish")

	plan := m.wizardApplyPlanEntries()
	if !planContains(plan, true, "git checkout -b feature/safe-publish") {
		t.Fatalf("expected branch checkout step in wizard plan, got %#v", plan)
	}
	if !planContains(plan, true, "git push -u origin feature/safe-publish") {
		t.Fatalf("expected publish-to-new-branch push step in wizard plan, got %#v", plan)
	}

	view := ansi.Strip(m.viewWizardContent())
	if !strings.Contains(view, "Publish as new branch (optional)") {
		t.Fatalf("expected publish-branch control in wizard view, got %q", view)
	}
}

func TestFixTUIWizardStageCommitPushNonDefaultBranchHidesOptionalPublishBranch(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Branch:    "feature/existing-work",
				Upstream:  "origin/feature/existing-work",
			},
			Meta: &domain.RepoMetadataFile{
				RepoKey:         "software/api",
				Name:            "api",
				OriginURL:       "https://github.com/you/api.git",
				PreferredRemote: "origin",
				AutoPush:        domain.AutoPushModeDisabled,
			},
		},
	}

	m := newFixTUIModelForTest(repos)
	m.startWizardQueue([]fixWizardDecision{{RepoPath: "/repos/api", Action: FixActionStageCommitPush}})

	if m.wizard.EnableForkBranchRename {
		t.Fatal("did not expect optional publish-branch input on non-default branch")
	}
	view := ansi.Strip(m.viewWizardContent())
	if strings.Contains(view, "Publish as new branch (optional)") {
		t.Fatalf("did not expect publish-branch control in wizard view, got %q", view)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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

func TestFixTUIWizardCreateProjectMissingGitignoreDoesNotOfferGenerationToggle(t *testing.T) {
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
	if strings.Contains(view, "Generate .gitignore before commit") || strings.Contains(view, "Append to .gitignore before commit") {
		t.Fatalf("did not expect gitignore generation toggle in create-project wizard, got %q", view)
	}
	if !strings.Contains(view, "This step will not generate .gitignore.") {
		t.Fatalf("expected explicit no-generation note for create-project, got %q", view)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/web.git", AutoPush: domain.AutoPushModeEnabled},
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

func lineContaining(view, needle string) string {
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func TestFixTUIResizeExpandsWideColumns(t *testing.T) {
	t.Parallel()

	layout := fixListColumnsForWidth(180)
	if layout.Reasons <= 32 {
		t.Fatalf("reasons width = %d, want > 32 in wide viewport", layout.Reasons)
	}
	if layout.SelectFixes <= 30 {
		t.Fatalf("select-fixes width = %d, want > 30 in wide viewport", layout.SelectFixes)
	}
}

func TestFixTUIResizeUsesFullPanelInnerWidthForRepoList(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})

	want := m.viewContentWidth() - panelStyle.GetHorizontalFrameSize()
	if got := m.repoList.Width(); got != want {
		t.Fatalf("repo list width = %d, want full panel inner width %d", got, want)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/zzz-sync.git", AutoPush: domain.AutoPushModeDisabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/aaa-blocked.git", AutoPush: domain.AutoPushModeEnabled},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "nnn-clone",
				Path:      "/repos/nnn-clone",
				OriginURL: "git@github.com:you/nnn-clone.git",
				Upstream:  "origin/main",
				Syncable:  false,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonCloneRequired,
				},
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/nnn-clone.git", AutoPush: domain.AutoPushModeDisabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/mmm-auto.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)

	if got := m.visible[0].Record.Name; got != "mmm-auto" {
		t.Fatalf("first row = %q, want fixable repo first", got)
	}
	if got := m.visible[1].Record.Name; got != "aaa-blocked" {
		t.Fatalf("second row = %q, want unsyncable blocked repo second", got)
	}
	if got := m.visible[2].Record.Name; got != "nnn-clone" {
		t.Fatalf("third row = %q, want not-cloned repo third", got)
	}
	if got := m.visible[3].Record.Name; got != "zzz-sync" {
		t.Fatalf("fourth row = %q, want syncable repo last", got)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
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
	repoList.SetSize(120, 10)
	repoList.Select(0)

	item := fixListItem{
		Name:             "phomemo_m02s",
		Branch:           "main",
		State:            "unsyncable",
		Reasons:          "dirty_tracked, dirty_untracked, missing_origin",
		ScheduledActions: []string{FixActionEnableAutoPush},
		Tier:             fixRepoTierAutofixable,
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

func TestFixTUIRepoHeaderUsesSelectFixesColumn(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 170, Height: 24})

	header := ansi.Strip(m.viewRepoHeader())
	if strings.Contains(header, "Scheduled") {
		t.Fatalf("repo header should not include old Scheduled label, got %q", header)
	}
	if strings.Contains(header, "Current") || strings.Contains(header, "Selected ") {
		t.Fatalf("repo header should not split selection/current into separate columns, got %q", header)
	}
	if !strings.Contains(header, "Select Fixes") {
		t.Fatalf("repo header should include Select Fixes column, got %q", header)
	}
}

func TestFixTUISelectFixesInteractiveLabelAppearsOnlyOnSelectedRow(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				Ahead:           1,
				HasDirtyTracked: true,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "web",
				Path:      "/repos/web",
				OriginURL: "git@github.com:you/web.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/web.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 190, Height: 24})

	m.setCursor(0)
	viewSelectedFirst := ansi.Strip(m.repoList.View())
	lineFirst := lineContaining(viewSelectedFirst, "api")
	if !strings.Contains(viewSelectedFirst, fixActionLabel(FixActionStageCommitPush)) {
		t.Fatalf("expected selected row to show current stage-commit action, got %q", viewSelectedFirst)
	}
	if !strings.Contains(lineFirst, "←") || !strings.Contains(lineFirst, "→") {
		t.Fatalf("expected selected row to include left/right affordance arrows, got %q", lineFirst)
	}
	if strings.Contains(viewSelectedFirst, fixActionLabel(FixActionPullFFOnly)) {
		t.Fatalf("expected non-selected row to hide current action label, got %q", viewSelectedFirst)
	}

	m.setCursor(1)
	viewSelectedSecond := ansi.Strip(m.repoList.View())
	lineSecond := lineContaining(viewSelectedSecond, "web")
	if !strings.Contains(viewSelectedSecond, fixActionLabel(FixActionPullFFOnly)) {
		t.Fatalf("expected selected row to show current pull action, got %q", viewSelectedSecond)
	}
	if strings.Contains(lineSecond, "←") || strings.Contains(lineSecond, "→") {
		t.Fatalf("expected no arrows for row with a single fix option, got %q", lineSecond)
	}
	if strings.Contains(viewSelectedSecond, fixActionLabel(FixActionPush)) {
		t.Fatalf("expected non-selected row to hide current action label, got %q", viewSelectedSecond)
	}
}

func TestFixTUISelectFixesLabelUpdatesWhenCyclingChoices(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				Ahead:           1,
				HasDirtyTracked: true,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 190, Height: 24})
	m.setCursor(0)

	before := ansi.Strip(m.repoList.View())
	if !strings.Contains(before, fixActionLabel(FixActionStageCommitPush)) {
		t.Fatalf("expected initial current action label in row, got %q", before)
	}
	m.cycleCurrentAction(1)
	after := ansi.Strip(m.repoList.View())
	if !strings.Contains(after, fixActionLabel(FixActionPush)) {
		t.Fatalf("expected current action label to update after cycle, got %q", after)
	}
}

func TestFixTUISelectFixesColumnPrependsSelectedSquares(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				Ahead:           1,
				HasDirtyTracked: true,
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 190, Height: 24})
	m.setCursor(0)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // select stage-commit-push
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}) // move to push

	line := lineContaining(ansi.Strip(m.repoList.View()), "api")
	ixSquare := strings.Index(line, "■")
	ixLabel := strings.Index(line, fixActionLabel(FixActionPush))
	if ixSquare < 0 || ixLabel < 0 || ixSquare > ixLabel {
		t.Fatalf("expected selected square prefix before current label in row, got %q", line)
	}
}

func TestFixTUIRepoListStickyCatalogDoesNotDuplicateVisibleHeader(t *testing.T) {
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
			Meta:             &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
			IsDefaultCatalog: true,
		},
		{
			Record: domain.MachineRepoRecord{
				Name:      "ref",
				Path:      "/repos/ref",
				Catalog:   "references",
				OriginURL: "git@github.com:you/ref.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta:             &domain.RepoMetadataFile{OriginURL: "https://github.com/you/ref.git", AutoPush: domain.AutoPushModeDisabled},
			IsDefaultCatalog: false,
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 170, Height: 24})

	view := ansi.Strip(m.viewRepoList())
	if strings.Count(view, "Catalog: software (default)") != 1 {
		t.Fatalf("expected catalog header to appear once without sticky duplication at top, got %q", view)
	}
}

func TestFixTUIRepoListStickyCatalogTracksTopVisibleCatalogNotCursor(t *testing.T) {
	t.Parallel()

	repos := make([]fixRepoState, 0, 8)
	for i := 0; i < 6; i++ {
		repos = append(repos, fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      fmt.Sprintf("projects-%d", i),
				Path:      fmt.Sprintf("/repos/projects-%d", i),
				Catalog:   "projects",
				OriginURL: "git@github.com:you/projects.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta:             &domain.RepoMetadataFile{OriginURL: "https://github.com/you/projects.git", AutoPush: domain.AutoPushModeDisabled},
			IsDefaultCatalog: true,
		})
	}
	repos = append(repos,
		fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      "references-a",
				Path:      "/repos/references-a",
				Catalog:   "references",
				OriginURL: "git@github.com:you/references-a.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta:             &domain.RepoMetadataFile{OriginURL: "https://github.com/you/references-a.git", AutoPush: domain.AutoPushModeDisabled},
			IsDefaultCatalog: false,
		},
		fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      "references-b",
				Path:      "/repos/references-b",
				Catalog:   "references",
				OriginURL: "git@github.com:you/references-b.git",
				Upstream:  "origin/main",
				Behind:    1,
			},
			Meta:             &domain.RepoMetadataFile{OriginURL: "https://github.com/you/references-b.git", AutoPush: domain.AutoPushModeDisabled},
			IsDefaultCatalog: false,
		},
	)

	m := newFixTUIModelForTest(repos)
	m.repoList.SetSize(170, 6)
	m.setCursor(6) // first references repo on page where top visible row is still projects catalog

	view := ansi.Strip(m.viewRepoList())
	lines := strings.Split(view, "\n")
	if len(lines) < 2 || !strings.Contains(lines[1], "Catalog: projects (default)") {
		t.Fatalf("expected sticky catalog to follow top visible rows, got %q", view)
	}
	if strings.Contains(lines[1], "Catalog: references") {
		t.Fatalf("sticky catalog should not follow cursor catalog when top rows are projects, got %q", view)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	view := m.View()

	trailing := trailingEmptyLineCount(view)
	if trailing != 0 {
		t.Fatalf("expected zero trailing empty lines after footer, got %d", trailing)
	}
}

func TestFixTUIListViewFillsWindowHeightWithoutRowsAfterFooter(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	})
	const (
		width  = 140
		height = 24
	)
	_, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})

	view := ansi.Strip(m.View())
	if got := lipgloss.Height(view); got != height {
		t.Fatalf("view should fill terminal height so footer remains sticky: got=%d want=%d view=%q", got, height, view)
	}
}

func TestFixTUIListViewHasNoBlankRowsBetweenMainPanelAndFooter(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 28})

	view := ansi.Strip(m.View())
	lines := strings.Split(view, "\n")
	firstNonEmpty := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstNonEmpty = i
			break
		}
	}
	if firstNonEmpty != 0 {
		t.Fatalf("expected list view to stay top-anchored with no leading blank rows, first non-empty line=%d view=%q", firstNonEmpty, view)
	}
	helpTopIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "╭") && strings.HasSuffix(trimmed, "╮") {
			helpTopIdx = i
			break
		}
	}
	if helpTopIdx <= 0 {
		t.Fatalf("footer top border not found: %q", view)
	}
	if strings.TrimSpace(lines[helpTopIdx-1]) == "" {
		t.Fatalf("expected no blank line between main panel and footer, got view %q", view)
	}
	if !strings.Contains(lines[helpTopIdx-1], "╯") {
		t.Fatalf("expected main panel bottom border immediately above footer, got %q", lines[helpTopIdx-1])
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeEnabled},
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 26})
	view := m.View()

	lines := strings.Split(ansi.Strip(view), "\n")
	first := firstNonEmptyLine(lines)
	if !strings.Contains(first, "╭") || !strings.Contains(first, "bb") || !strings.Contains(first, "fix") {
		t.Fatalf("expected titled main border at top of view, got %q", first)
	}
}

func TestFixTUIResizeShrinksListHeightWhenSelectedDetailsWrap(t *testing.T) {
	t.Parallel()

	shortPath := "/repos/api"
	longPath := "/Volumes/Projects/Software/" + strings.Repeat("codegen-typescript-graphql-module-declarations-plugin-", 2) + "api"

	newModel := func(path string, rows int) *fixTUIModel {
		repos := make([]fixRepoState, 0, rows)
		repos = append(repos, fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      path,
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushAccessBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL:  "https://github.com/you/api.git",
				AutoPush:   domain.AutoPushModeDisabled,
				PushAccess: domain.PushAccessReadOnly,
			},
		})
		for i := 0; i < rows-1; i++ {
			repos = append(repos, fixRepoState{
				Record: domain.MachineRepoRecord{
					Name:      fmt.Sprintf("repo-%02d", i),
					Path:      fmt.Sprintf("/repos/repo-%02d", i),
					OriginURL: fmt.Sprintf("git@github.com:you/repo-%02d.git", i),
					Upstream:  "origin/main",
					Ahead:     1,
				},
				Meta: &domain.RepoMetadataFile{
					OriginURL: fmt.Sprintf("https://github.com/you/repo-%02d.git", i),
					AutoPush:  domain.AutoPushModeDisabled,
				},
			})
		}
		return newFixTUIModelForTest(repos)
	}

	shortModel := newModel(shortPath, 20)
	longModel := newModel(longPath, 20)
	_, _ = shortModel.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	_, _ = longModel.Update(tea.WindowSizeMsg{Width: 120, Height: 36})

	shortHeight := shortModel.repoList.Height()
	longHeight := longModel.repoList.Height()
	if longHeight >= shortHeight {
		t.Fatalf("expected wrapped selected details to shrink list height: short=%d long=%d", shortHeight, longHeight)
	}
}

func TestFixTUIViewStaysWithinWindowHeightWhenSelectedDetailsWrap(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "codegen-typescript-graphql-module-declarations-plugin",
				Path:      "/Volumes/Projects/Software/" + strings.Repeat("codegen-typescript-graphql-module-declarations-plugin-", 2) + "repo",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Syncable:  false,
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushAccessBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL:  "https://github.com/you/api.git",
				AutoPush:   domain.AutoPushModeDisabled,
				PushAccess: domain.PushAccessReadOnly,
			},
		},
	}
	for i := 0; i < 24; i++ {
		repos = append(repos, fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      fmt.Sprintf("repo-%02d", i),
				Path:      fmt.Sprintf("/repos/repo-%02d", i),
				OriginURL: fmt.Sprintf("git@github.com:you/repo-%02d.git", i),
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL: fmt.Sprintf("https://github.com/you/repo-%02d.git", i),
				AutoPush:  domain.AutoPushModeDisabled,
			},
		})
	}
	m := newFixTUIModelForTest(repos)

	const width = 118
	const height = 26
	_, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})

	view := m.View()
	if got := lipgloss.Height(view); got > height {
		t.Fatalf("view height overflowed terminal height: got=%d want<=%d", got, height)
	}
	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "bb") || !strings.Contains(stripped, "fix") {
		t.Fatalf("expected compact border title to stay visible, got %q", view)
	}
}

func TestFixTUIViewportTopDoesNotJumpUpWhenMovingDownOneRow(t *testing.T) {
	t.Parallel()

	repos := make([]fixRepoState, 0, 80)
	for i := 0; i < 80; i++ {
		name := fmt.Sprintf("repo-%02d", i)
		path := fmt.Sprintf("/repos/%s", name)
		// Alternate short and very long details so moving by one row can change
		// selected-details wrapping and trigger list-height reflow.
		if i%2 == 1 {
			path = "/Volumes/Projects/Software/" + strings.Repeat("codegen-typescript-graphql-module-declarations-plugin-", 3) + name
		}
		repos = append(repos, fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      name,
				Path:      path,
				OriginURL: fmt.Sprintf("git@github.com:you/%s.git", name),
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL: fmt.Sprintf("https://github.com/you/%s.git", name),
				AutoPush:  domain.AutoPushModeDisabled,
			},
		})
	}

	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 118, Height: 22})
	m.setCursor(3)
	_ = m.View()
	beforeItems := m.repoList.Items()
	beforeStart, beforeEnd := m.repoList.Paginator.GetSliceBounds(len(beforeItems))
	beforeSelected := m.repoList.Index()
	beforeHeight := m.repoList.Height()
	if beforeEnd <= beforeStart {
		t.Fatalf("invalid initial list bounds: start=%d end=%d", beforeStart, beforeEnd)
	}
	beforePos := beforeSelected - beforeStart
	if beforePos < 0 || beforePos >= (beforeEnd-beforeStart)-1 {
		t.Fatalf("fixture should place selection away from viewport end: selected=%d start=%d end=%d", beforeSelected, beforeStart, beforeEnd)
	}

	m.moveRepoCursor(1)
	_ = m.View()
	afterItems := m.repoList.Items()
	afterStart, afterEnd := m.repoList.Paginator.GetSliceBounds(len(afterItems))
	afterHeight := m.repoList.Height()
	if afterEnd <= afterStart {
		t.Fatalf("invalid post-move list bounds: start=%d end=%d", afterStart, afterEnd)
	}
	if afterHeight != beforeHeight {
		t.Fatalf("list height changed after one-row down move: beforeHeight=%d afterHeight=%d", beforeHeight, afterHeight)
	}
	if afterStart != beforeStart {
		t.Fatalf("viewport page start jumped after one-row down move: beforeStart=%d afterStart=%d beforeSelected=%d afterSelected=%d", beforeStart, afterStart, beforeSelected, m.repoList.Index())
	}
}

func TestFixTUIListViewStaysFooterAttachedAcrossWrapChangingSelectionMove(t *testing.T) {
	t.Parallel()

	repos := make([]fixRepoState, 0, 40)
	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("repo-%02d", i)
		path := fmt.Sprintf("/repos/%s", name)
		if i%2 == 1 {
			path = "/Volumes/Projects/Software/" + strings.Repeat("codegen-typescript-graphql-module-declarations-plugin-", 3) + name
		}
		repos = append(repos, fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      name,
				Path:      path,
				OriginURL: fmt.Sprintf("git@github.com:you/%s.git", name),
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL: fmt.Sprintf("https://github.com/you/%s.git", name),
				AutoPush:  domain.AutoPushModeDisabled,
			},
		})
	}

	const (
		width  = 118
		height = 24
	)
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m.setCursor(3)

	before := ansi.Strip(m.View())
	assertListViewFillsHeightAndFooterHasNoGap(t, before, height)

	m.moveRepoCursor(1)
	after := ansi.Strip(m.View())
	assertListViewFillsHeightAndFooterHasNoGap(t, after, height)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1) // push

	details := m.viewSelectedRepoDetails()
	if !strings.Contains(details, "Action:") {
		t.Fatalf("expected action label in selected details, got %q", details)
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
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

func TestFixTUISelectedDetailsUsesCompactMetaLineWithDotSeparators(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonPushPolicyBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	details := ansi.Strip(m.viewSelectedRepoDetails())
	lines := strings.Split(details, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected compact selected details lines, got %q", details)
	}
	if !strings.Contains(lines[1], "State:") || !strings.Contains(lines[1], " · Auto-push:") || !strings.Contains(lines[1], " · Branch:") || !strings.Contains(lines[1], " · Reasons:") || !strings.Contains(lines[1], " · Selected fixes:") {
		t.Fatalf("expected dot-separated metadata line, got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "Action: ") {
		t.Fatalf("expected standalone action line with compact label, got %q", lines[2])
	}
}

func TestFixTUISelectedDetailsWrapAvoidsOrphanSelectedFixValueLine(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "api",
				Path:      "/repos/api",
				OriginURL: "git@github.com:you/api.git",
				Upstream:  "origin/main",
				Ahead:     1,
				UnsyncableReasons: []domain.UnsyncableReason{
					domain.ReasonDirtyTracked,
					domain.ReasonDirtyUntracked,
					domain.ReasonMissingUpstream,
					domain.ReasonPushFailed,
					domain.ReasonPushPolicyBlocked,
				},
			},
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 92, Height: 24})

	details := ansi.Strip(m.viewSelectedRepoDetails())
	for _, line := range strings.Split(details, "\n") {
		if strings.TrimSpace(line) == "-" {
			t.Fatalf("selected-fixes value should not orphan on its own wrapped line, got details %q", details)
		}
	}
}

func TestFixTUIFixSummaryFallsBackToWrappedTextWhenPillsDoNotFit(t *testing.T) {
	t.Parallel()

	repos := make([]fixRepoState, 0, 12)
	for i := 0; i < 12; i++ {
		repos = append(repos, fixRepoState{
			Record: domain.MachineRepoRecord{
				Name:      fmt.Sprintf("repo-%02d", i),
				Path:      fmt.Sprintf("/repos/repo-%02d", i),
				OriginURL: fmt.Sprintf("git@github.com:you/repo-%02d.git", i),
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{
				OriginURL: fmt.Sprintf("https://github.com/you/repo-%02d.git", i),
				AutoPush:  domain.AutoPushModeDisabled,
			},
		})
	}

	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 92, Height: 24})

	summary := ansi.Strip(m.viewFixSummary())
	if strings.Contains(summary, "╭") || strings.Contains(summary, "╮") || strings.Contains(summary, "╰") || strings.Contains(summary, "╯") {
		t.Fatalf("expected narrow summary fallback without pill boxes, got %q", summary)
	}
	if strings.Contains(summary, ":") {
		t.Fatalf("fallback should keep pill content format (NUMBER TEXT), got %q", summary)
	}
	wantOrder := []string{"12 REPOS", "0 SELECTED", "0 FIXABLE", "12 UNSYNCABLE", "0 NOT CLONED", "0 SYNCABLE", "0 IGNORED"}
	last := -1
	for _, token := range wantOrder {
		idx := strings.Index(summary, token)
		if idx < 0 {
			t.Fatalf("fallback summary missing token %q in %q", token, summary)
		}
		if idx < last {
			t.Fatalf("fallback summary changed metric order, token=%q summary=%q", token, summary)
		}
		last = idx
	}
	summaryWidth := m.repoDetailsLineWidth()
	for _, line := range strings.Split(summary, "\n") {
		if w := ansi.StringWidth(line); summaryWidth > 0 && w > summaryWidth {
			t.Fatalf("summary line exceeds available width: got=%d want<=%d line=%q summary=%q", w, summaryWidth, line, summary)
		}
	}
}

func TestFixTUIFixSummaryUsesPillsWhenThereIsEnoughWidth(t *testing.T) {
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
			Meta: &domain.RepoMetadataFile{OriginURL: "https://github.com/you/api.git", AutoPush: domain.AutoPushModeDisabled},
		},
	}

	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 170, Height: 24})

	summary := ansi.Strip(m.viewFixSummary())
	if !strings.Contains(summary, "╭") || !strings.Contains(summary, "╮") {
		t.Fatalf("expected wide summary to keep pill boxes, got %q", summary)
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
			AutoPush: domain.AutoPushModeDisabled,
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
			AutoPush:   domain.AutoPushModeDisabled,
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

func TestClassifyFixRepoMarksSyncProbeFailedAsBlocked(t *testing.T) {
	t.Parallel()

	repo := fixRepoState{
		Record: domain.MachineRepoRecord{
			RepoKey:   "software/api",
			Name:      "api",
			Path:      "/repos/api",
			OriginURL: "git@github.com:you/api.git",
			Upstream:  "origin/main",
			Syncable:  false,
			Diverged:  true,
			Ahead:     1,
			Behind:    1,
			UnsyncableReasons: []domain.UnsyncableReason{
				domain.ReasonDiverged,
				domain.ReasonSyncProbeFailed,
			},
		},
	}
	actions := []string{FixActionSyncWithUpstream}

	if got := classifyFixRepo(repo, actions); got != fixRepoTierUnsyncableBlocked {
		t.Fatalf("tier = %v, want unsyncable when sync feasibility probe was inconclusive", got)
	}
}

func TestClassifyFixRepoMarksCloneRequiredAsNotCloned(t *testing.T) {
	t.Parallel()

	repo := fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:      "api",
			Path:      "/repos/api",
			OriginURL: "git@github.com:you/api.git",
			Upstream:  "origin/main",
			Syncable:  false,
			UnsyncableReasons: []domain.UnsyncableReason{
				domain.ReasonCloneRequired,
			},
		},
	}
	actions := []string{FixActionClone}

	if got := classifyFixRepo(repo, actions); got != fixRepoTierNotCloned {
		t.Fatalf("tier = %v, want not-cloned tier for clone-required repos", got)
	}
}

func TestSelectableFixActionsKeepsPriorityOrderWithoutSyntheticAllOption(t *testing.T) {
	t.Parallel()

	options := selectableFixActions(fixActionsForSelection([]string{
		FixActionStageCommitPush,
		FixActionSyncWithUpstream,
		FixActionPush,
	}))
	if len(options) != 3 {
		t.Fatalf("options len = %d, want 3", len(options))
	}
	syncIndex := -1
	stageIndex := -1
	pushIndex := -1
	for i, option := range options {
		if option == FixActionSyncWithUpstream {
			syncIndex = i
		}
		if option == FixActionStageCommitPush {
			stageIndex = i
		}
		if option == FixActionPush {
			pushIndex = i
		}
	}
	if syncIndex < 0 || stageIndex < 0 || pushIndex < 0 {
		t.Fatalf("expected sync, stage, and push options in %v", options)
	}
	if syncIndex > stageIndex {
		t.Fatalf("expected sync-with-upstream to be ordered before stage-commit-push, got %v", options)
	}
	if stageIndex > pushIndex {
		t.Fatalf("expected stage-commit-push to be ordered before push, got %v", options)
	}
}

func TestSelectableFixActionsOrdersStageCommitBeforeCreateProject(t *testing.T) {
	t.Parallel()

	options := selectableFixActions(fixActionsForSelection([]string{
		FixActionCreateProject,
		FixActionStageCommitPush,
	}))
	if len(options) != 2 {
		t.Fatalf("options len = %d, want 2", len(options))
	}
	stageIndex := -1
	createIndex := -1
	for i, option := range options {
		if option == FixActionStageCommitPush {
			stageIndex = i
		}
		if option == FixActionCreateProject {
			createIndex = i
		}
	}
	if stageIndex < 0 || createIndex < 0 {
		t.Fatalf("expected stage and create options in %v", options)
	}
	if stageIndex > createIndex {
		t.Fatalf("expected stage-commit-push before create-project, got %v", options)
	}
}

func TestFixActionsForAllExecutionSkipsRedundantPushAfterStageCommitPush(t *testing.T) {
	t.Parallel()

	got := fixActionsForAllExecution([]string{
		FixActionStageCommitPush,
		FixActionPush,
		FixActionSetUpstreamPush,
	})
	want := []string{FixActionStageCommitPush}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("all-fixes execution actions = %v, want %v", got, want)
	}
}

func TestFixTUISpaceTogglesScheduledFixForCurrentRepo(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				HasDirtyTracked: true,
				Ahead:           1,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	m.cycleCurrentAction(1)
	if m.hasAnySelectedFixes() {
		t.Fatal("expected browsing with left/right alone not to schedule fixes")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !m.hasAnySelectedFixes() {
		t.Fatal("expected space to schedule the currently browsed fix")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if m.hasAnySelectedFixes() {
		t.Fatal("expected pressing space again to unschedule the currently browsed fix")
	}
}

func TestFixTUIScheduledQueueUsesExecutionOrderAndDeduplicatesRiskyActions(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				HasDirtyTracked: true,
				Ahead:           1,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	for i := 0; i < 8 && actionForVisibleRepo(m, 0) != FixActionStageCommitPush; i++ {
		m.cycleCurrentAction(1)
	}
	if got := actionForVisibleRepo(m, 0); got != FixActionStageCommitPush {
		t.Fatalf("current action = %q, want %q before scheduling", got, FixActionStageCommitPush)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	for i := 0; i < 8 && actionForVisibleRepo(m, 0) != FixActionPush; i++ {
		m.cycleCurrentAction(1)
	}
	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("current action = %q, want %q before scheduling", got, FixActionPush)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m.applyCurrentSelection()
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want %v", m.viewMode, fixViewWizard)
	}
	if len(m.wizard.Queue) != 1 {
		t.Fatalf("wizard queue len = %d, want 1 after dedupe", len(m.wizard.Queue))
	}
	if got := m.wizard.Queue[0].Action; got != FixActionStageCommitPush {
		t.Fatalf("wizard queue[0] action = %q, want %q", got, FixActionStageCommitPush)
	}
}

func TestFixTUIApplyAllSelectionsUsesScheduledExecutionOrderPerRepo(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:            "api",
				Path:            "/repos/api",
				OriginURL:       "git@github.com:you/api.git",
				Upstream:        "origin/main",
				HasDirtyTracked: true,
				Ahead:           1,
			},
		},
	}
	m := newFixTUIModelForTest(repos)
	m.setCursor(0)
	for i := 0; i < 8 && actionForVisibleRepo(m, 0) != FixActionStageCommitPush; i++ {
		m.cycleCurrentAction(1)
	}
	if got := actionForVisibleRepo(m, 0); got != FixActionStageCommitPush {
		t.Fatalf("current action = %q, want %q before scheduling", got, FixActionStageCommitPush)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	for i := 0; i < 8 && actionForVisibleRepo(m, 0) != FixActionPush; i++ {
		m.cycleCurrentAction(1)
	}
	if got := actionForVisibleRepo(m, 0); got != FixActionPush {
		t.Fatalf("current action = %q, want %q before scheduling", got, FixActionPush)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m.applyAllSelections()
	if m.viewMode != fixViewWizard {
		t.Fatalf("view mode = %v, want %v", m.viewMode, fixViewWizard)
	}
	if len(m.wizard.Queue) != 1 {
		t.Fatalf("wizard queue len = %d, want 1 after dedupe", len(m.wizard.Queue))
	}
	if got := m.wizard.Queue[0].Action; got != FixActionStageCommitPush {
		t.Fatalf("wizard queue[0] action = %q, want %q", got, FixActionStageCommitPush)
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

func assertListViewFillsHeightAndFooterHasNoGap(t *testing.T, view string, wantHeight int) {
	t.Helper()

	if got := lipgloss.Height(view); got != wantHeight {
		t.Fatalf("list view should fill terminal height: got=%d want=%d view=%q", got, wantHeight, view)
	}

	lines := strings.Split(view, "\n")
	helpTopIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "╭") && strings.HasSuffix(trimmed, "╮") {
			helpTopIdx = i
			break
		}
	}
	if helpTopIdx <= 0 {
		t.Fatalf("footer top border not found: %q", view)
	}
	if strings.TrimSpace(lines[helpTopIdx-1]) == "" {
		t.Fatalf("expected no blank line between main panel and footer, got %q", view)
	}
	if !strings.Contains(lines[helpTopIdx-1], "╯") {
		t.Fatalf("expected main panel bottom border directly above footer, got %q", lines[helpTopIdx-1])
	}
}

func previousNonEmptyLine(lines []string, start int) string {
	for i := start; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

func firstNonEmptyLine(lines []string) string {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

func shortHelpEntries(bindings []key.Binding) []string {
	entries := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		help := binding.Help()
		keyLabel := strings.TrimSpace(help.Key)
		desc := strings.TrimSpace(help.Desc)
		if keyLabel == "" || desc == "" {
			continue
		}
		entries = append(entries, keyLabel+" "+desc)
	}
	return entries
}

func helpContains(entries []string, value string) bool {
	for _, entry := range entries {
		if entry == value {
			return true
		}
	}
	return false
}

func footerHelpContentLines(view string) []string {
	lines := strings.Split(ansi.Strip(view), "\n")
	top := -1
	bottom := -1
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if bottom < 0 && strings.HasPrefix(trimmed, "╰") && strings.HasSuffix(trimmed, "╯") {
			bottom = i
			continue
		}
		if bottom >= 0 && strings.HasPrefix(trimmed, "╭") && strings.HasSuffix(trimmed, "╮") {
			top = i
			break
		}
	}
	if top < 0 || bottom <= top {
		return nil
	}
	out := make([]string, 0, bottom-top-1)
	for i := top + 1; i < bottom; i++ {
		line := strings.TrimSpace(lines[i])
		line = strings.TrimPrefix(line, "│")
		line = strings.TrimSuffix(line, "│")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
