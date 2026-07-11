package app

import "encoding/json"

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

func (a *App) emitOperationEvent(enabled bool, event OperationEvent) {
	if !enabled {
		return
	}
	_ = json.NewEncoder(a.Stdout).Encode(event)
}
