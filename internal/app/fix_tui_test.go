package app

import (
	"testing"

	"github.com/charmbracelet/bubbles/table"

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
	before := m.table.Rows()[0][4]
	if before == FixActionPush {
		t.Fatalf("expected cycled action not to be default %q", FixActionPush)
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

	if got := m.table.Rows()[0][4]; got != FixActionPush {
		t.Fatalf("fallback action = %q, want %q", got, FixActionPush)
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
	}
	m.rebuildTable("")
	return m
}
