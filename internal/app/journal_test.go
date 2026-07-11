package app

import (
	"bb-project/internal/domain"
	"bb-project/internal/state"
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRunLogOutputFiltersAndLimits(t *testing.T) {
	t.Parallel()
	p := journalFixture(t)

	tests := []struct {
		name string
		opts LogOptions
		want string
	}{
		{
			name: "plain newest first with explicit limit",
			opts: LogOptions{Limit: 2},
			want: "1970-01-01 00:00:03Z  b  sync_run  newest\n1970-01-01 00:00:02Z  a  converged  software/web\n",
		},
		{
			name: "combined machine and repo filters",
			opts: LogOptions{Machine: "a", Repo: "api", JSON: true},
			want: "[\n  {\n    \"at\": \"1970-01-01T00:00:01Z\",\n    \"machine\": \"a\",\n    \"event\": \"pushed\",\n    \"repo_key\": \"software/api\"\n  }\n]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			a := New(p, &out, &bytes.Buffer{})
			if code, err := a.RunLog(tt.opts); err != nil || code != 0 {
				t.Fatalf("RunLog() = (%d, %v)", code, err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("output:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestRunLogDefaultLimitIsFifty(t *testing.T) {
	t.Parallel()
	p := state.NewPaths(t.TempDir())
	for i := range 51 {
		if err := state.AppendJournal(p, domain.JournalEvent{At: time.Unix(int64(i), 0).UTC(), Machine: "a", Event: "sync_run"}, 100); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if code, err := New(p, &out, &bytes.Buffer{}).RunLog(LogOptions{}); err != nil || code != 0 {
		t.Fatalf("RunLog() = (%d, %v)", code, err)
	}
	if got := strings.Count(out.String(), "sync_run"); got != 50 {
		t.Fatalf("event count = %d, want 50", got)
	}
}

func TestRunLogRepoSelectorAcrossMachines(t *testing.T) {
	t.Parallel()
	p := journalFixture(t)
	m := state.BootstrapMachine("b", "b", time.Now())
	m.Repos = []domain.MachineRepoRecord{{RepoKey: "software/api", Name: "api", Path: "/machine-b/software/api"}}
	if err := state.SaveMachine(p, m); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code, err := New(p, &out, &bytes.Buffer{}).RunLog(LogOptions{Repo: "api"}); err != nil || code != 0 {
		t.Fatalf("RunLog() = (%d, %v)", code, err)
	}
	if got := out.String(); !strings.Contains(got, "software/api") {
		t.Fatalf("output = %q", got)
	}
	out.Reset()
	if code, err := New(p, &out, &bytes.Buffer{}).RunLog(LogOptions{Repo: "/machine-b/software/api"}); err != nil || code != 0 {
		t.Fatalf("RunLog(path) = (%d, %v)", code, err)
	}
	if got := out.String(); !strings.Contains(got, "software/api") {
		t.Fatalf("path-selected output = %q", got)
	}
}

func TestRunLogSelectorErrors(t *testing.T) {
	t.Parallel()
	p := journalFixture(t)
	m := state.BootstrapMachine("b", "b", time.Now())
	m.Repos = []domain.MachineRepoRecord{{RepoKey: "work/api", Name: "api"}}
	if err := state.SaveMachine(p, m); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		opts LogOptions
		want string
	}{
		{name: "ambiguous", opts: LogOptions{Repo: "api"}, want: "ambiguous"},
		{name: "not found", opts: LogOptions{Repo: "missing"}, want: "could not be resolved"},
		{name: "invalid limit", opts: LogOptions{Limit: -1}, want: "limit must be >= 1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, err := New(p, &bytes.Buffer{}, &bytes.Buffer{}).RunLog(tt.opts)
			if code != 2 || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunLog() = (%d, %v), want error containing %q", code, err, tt.want)
			}
		})
	}
}

func journalFixture(t *testing.T) state.Paths {
	t.Helper()
	p := state.NewPaths(t.TempDir())
	events := []domain.JournalEvent{
		{At: time.Unix(1, 0).UTC(), Machine: "a", Event: "pushed", RepoKey: "software/api"},
		{At: time.Unix(3, 0).UTC(), Machine: "b", Event: "sync_run", Detail: "newest"},
		{At: time.Unix(2, 0).UTC(), Machine: "a", Event: "converged", RepoKey: "software/web"},
	}
	for _, e := range events {
		if err := state.AppendJournal(p, e, 50); err != nil {
			t.Fatal(err)
		}
	}
	m := state.BootstrapMachine("a", "a", time.Now())
	m.Repos = []domain.MachineRepoRecord{{RepoKey: "software/api", Name: "api"}, {RepoKey: "software/web", Name: "web"}}
	if err := state.SaveMachine(p, m); err != nil {
		t.Fatal(err)
	}
	return p
}
