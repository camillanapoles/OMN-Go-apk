package backend

import (
	"encoding/json"
	"net/http"

	"net.basov.omngo/backend/internal/logx"
)

// jsonStatus is the answer {"status", "message"}. Message has no omitempty,
// thus a sync success also sends "message": "".
type jsonStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// writeJSON answers with one JSON value and the HTTP status code. It is the
// one JSON writer of the backend. Section 1.4 of doc/API.md lists each
// endpoint that uses it. The encoder escapes a quote in a message, thus the
// body stays valid JSON.
func (a *App) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		a.log(logx.Server).Errf("encode the JSON answer: %v", err)
	}
}

// writeJSONError answers {"status": "error", "message": msg} with the HTTP
// status code.
func (a *App) writeJSONError(w http.ResponseWriter, code int, msg string) {
	a.writeJSON(w, code, jsonStatus{Status: "error", Message: msg})
}
