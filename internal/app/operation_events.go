package app

import (
	"encoding/json"

	"bb-project/internal/domain"
)

type OperationEvent struct {
	Event      string `json:"event"`
	Operation  string `json:"operation"`
	Repository string `json:"repository,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Message    string `json:"message"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
	Completed  *int   `json:"completed,omitempty"`
	Total      *int   `json:"total,omitempty"`
}

func withOperationCounts(event OperationEvent, completed, total int) OperationEvent {
	event.Completed = &completed
	event.Total = &total
	return event
}

type syncOperationProgress struct {
	app       *App
	enabled   bool
	completed int
	total     int
	started   map[string]bool
	finished  map[string]bool
}

func newSyncOperationProgress(app *App, enabled bool, total int) *syncOperationProgress {
	return &syncOperationProgress{app: app, enabled: enabled, total: total, started: map[string]bool{}, finished: map[string]bool{}}
}

func (p *syncOperationProgress) start(repository, phase, message string) {
	if p == nil || p.started[repository] {
		return
	}
	p.started[repository] = true
	p.app.emitOperationEvent(p.enabled, withOperationCounts(OperationEvent{Event: "repository_started", Operation: "sync", Repository: repository, Phase: phase, Message: message}, p.completed, p.total))
}

func (p *syncOperationProgress) progress(repository, phase, message string) {
	if p == nil {
		return
	}
	p.start(repository, phase, message)
	p.app.emitOperationEvent(p.enabled, withOperationCounts(OperationEvent{Event: "progress", Operation: "sync", Repository: repository, Phase: phase, Message: message}, p.completed, p.total))
}

func (p *syncOperationProgress) finish(repository string, record *domain.MachineRepoRecord) {
	if p == nil || p.finished[repository] {
		return
	}
	p.start(repository, "complete", "Checking repository")
	p.finished[repository] = true
	p.completed++
	event := OperationEvent{Event: "repository_finished", Operation: "sync", Repository: repository, Phase: "complete", Message: "Repository sync completed", Result: "success"}
	if record != nil && (record.State == domain.RepoStateBlocked || record.State == domain.RepoStatePending) {
		event.Message = "Repository needs attention"
		event.Result = "failure"
		event.Error = repositoryFailureDetail([]domain.MachineRepoRecord{*record}, repository)
	}
	p.app.emitOperationEvent(p.enabled, withOperationCounts(event, p.completed, p.total))
}

func (p *syncOperationProgress) fail(repository string, err error) error {
	if p == nil || p.finished[repository] {
		return err
	}
	p.start(repository, "complete", "Checking repository")
	p.finished[repository] = true
	p.completed++
	p.app.emitOperationEvent(p.enabled, withOperationCounts(OperationEvent{Event: "repository_finished", Operation: "sync", Repository: repository, Phase: "complete", Message: "Repository sync failed", Result: "failure", Error: err.Error()}, p.completed, p.total))
	return err
}

func (a *App) emitOperationEvent(enabled bool, event OperationEvent) {
	if !enabled {
		return
	}
	_ = json.NewEncoder(a.Stdout).Encode(event)
}
