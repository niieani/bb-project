package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

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
	m.table.SetCursor(0)
	m.cycleCurrentAction(1)
	firstRepoAction := m.table.Rows()[0][4]

	m.table.SetCursor(1)
	m.cycleCurrentAction(0)
	secondRepoAction := m.table.Rows()[1][4]

	m.table.SetCursor(0)
	m.cycleCurrentAction(0)
	if got := m.table.Rows()[0][4]; got != firstRepoAction {
		t.Fatalf("row 0 action changed unexpectedly: got %q want %q", got, firstRepoAction)
	}
	if got := m.table.Rows()[1][4]; got != secondRepoAction {
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
	m.table.SetCursor(0)
	m.cycleCurrentAction(1)
	m.cycleCurrentAction(1)
	before := m.table.Rows()[0][4]
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
	m.rebuildTable("/repos/api")

	if got := m.table.Rows()[0][4]; !strings.Contains(got, "-") {
		t.Fatalf("fallback action = %q, want default no-op", got)
	}
}

func newFixTUIModelForTest(repos []fixRepoState) *fixTUIModel {
	columns := []table.Column{
		{Title: "Repo", Width: 24},
		{Title: "Branch", Width: 20},
		{Title: "State", Width: 12},
		{Title: "Reasons", Width: 32},
		{Title: "Selected Fix", Width: 22},
	}
	tbl := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	m := &fixTUIModel{
		repos:          append([]fixRepoState(nil), repos...),
		ignored:        map[string]bool{},
		selectedAction: map[string]int{},
		table:          tbl,
		keys:           defaultFixTUIKeyMap(),
		help:           help.New(),
	}
	m.rebuildTable("")
	return m
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

	if got := m.table.Rows()[0][4]; !strings.Contains(got, "-") {
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
	m.table.SetCursor(0)

	m.cycleCurrentAction(1)
	if got := m.table.Rows()[0][4]; strings.Contains(got, "-") {
		t.Fatalf("expected first cycle to move off no-op, got %q", got)
	}

	m.cycleCurrentAction(-1)
	if got := m.table.Rows()[0][4]; !strings.Contains(got, "-") {
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

	view := m.table.View()
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

func TestFixTUIRowsUsePlainTextCells(t *testing.T) {
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
	}
	m := newFixTUIModelForTest(repos)
	m.table.SetCursor(0)
	m.cycleCurrentAction(1)

	rows := m.table.Rows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	for idx, cell := range rows[0] {
		if strings.Contains(cell, "\x1b") {
			t.Fatalf("cell %d includes ANSI escape sequence: %q", idx, cell)
		}
	}
}

func TestFixTUIResizeExpandsWideColumns(t *testing.T) {
	t.Parallel()

	repos := []fixRepoState{
		{
			Record: domain.MachineRepoRecord{
				Name:      "condu",
				Path:      "/repos/condu",
				RepoID:    "github.com/you/condu",
				OriginURL: "git@github.com:you/condu.git",
				Upstream:  "origin/main",
				Ahead:     1,
			},
			Meta: &domain.RepoMetadataFile{RepoID: "github.com/you/condu", AutoPush: false},
		},
	}
	m := newFixTUIModelForTest(repos)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 26})

	cols := m.table.Columns()
	if cols[3].Width <= 32 {
		t.Fatalf("reasons width = %d, want > 32 in wide viewport", cols[3].Width)
	}
	if cols[4].Width <= 22 {
		t.Fatalf("selected-fix width = %d, want > 22 in wide viewport", cols[4].Width)
	}
}
