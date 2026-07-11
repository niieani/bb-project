package state

import (
	"bb-project/internal/domain"
	"testing"
	"time"
)

func TestJournalPrunesOldest(t *testing.T) {
	t.Parallel()
	p := NewPaths(t.TempDir())
	for i := 0; i < 3; i++ {
		if err := AppendJournal(p, domain.JournalEvent{At: time.Unix(int64(i), 0), Machine: "m", Event: "sync_run"}, 2); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadJournalFile(p.JournalPath("m"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].At.Unix() != 1 || got[1].At.Unix() != 2 {
		t.Fatalf("events=%+v", got)
	}
}

func TestLoadAllJournalsOrdersTimestampTiesByMachine(t *testing.T) {
	t.Parallel()
	p := NewPaths(t.TempDir())
	at := time.Unix(1, 0).UTC()
	for _, machine := range []string{"b", "a"} {
		if err := AppendJournal(p, domain.JournalEvent{At: at, Machine: machine, Event: "sync_run"}, 2); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadAllJournals(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Machine != "a" || got[1].Machine != "b" {
		t.Fatalf("events = %+v", got)
	}
}
