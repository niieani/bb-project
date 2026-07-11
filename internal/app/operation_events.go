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
}

func (a *App) emitOperationEvent(enabled bool, event OperationEvent) {
	if !enabled {
		return
	}
	_ = json.NewEncoder(a.Stdout).Encode(event)
}
