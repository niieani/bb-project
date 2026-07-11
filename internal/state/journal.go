package state

import (
	"bb-project/internal/domain"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func AppendJournal(p Paths, e domain.JournalEvent, max int) error {
	if max < 1 {
		return fmt.Errorf("journal.max_entries must be >= 1")
	}
	if err := EnsureDir(p.JournalDir()); err != nil {
		return err
	}
	events, err := LoadJournalFile(p.JournalPath(e.Machine))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	events = append(events, e)
	if len(events) > max {
		events = events[len(events)-max:]
	}
	tmp := p.JournalPath(e.Machine) + ".tmp"
	defer os.Remove(tmp)
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, v := range events {
		if err := enc.Encode(v); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.JournalPath(e.Machine)); err != nil {
		return err
	}
	if dir, err := os.Open(p.JournalDir()); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
func LoadJournalFile(path string) ([]domain.JournalEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []domain.JournalEvent{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.TrimSpace(s.Text()) == "" {
			continue
		}
		var e domain.JournalEvent
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
		}
		out = append(out, e)
	}
	return out, s.Err()
}
func LoadAllJournals(p Paths) ([]domain.JournalEvent, error) {
	entries, err := os.ReadDir(p.JournalDir())
	if errors.Is(err, os.ErrNotExist) {
		return []domain.JournalEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []domain.JournalEvent{}
	for _, v := range entries {
		if v.IsDir() || !strings.HasSuffix(v.Name(), ".jsonl") {
			continue
		}
		events, err := LoadJournalFile(filepath.Join(p.JournalDir(), v.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].Machine < out[j].Machine
		}
		return out[i].At.After(out[j].At)
	})
	return out, nil
}
