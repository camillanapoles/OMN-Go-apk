package render

import (
	"encoding/json"
	"net/http"

	"net.basov.omngo/backend/internal/logx"
)

// JSONStatus is the answer {"status", "message"}. Message has no omitempty,
// thus a sync success also sends "message": "".
type JSONStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// WriteJSON answers with one JSON value and the HTTP status code. It is the
// one JSON writer of the backend. Section 1.4 of doc/API.md lists each
// endpoint that uses it. The encoder escapes a quote in a message, thus the
// body stays valid JSON.
func WriteJSON(w http.ResponseWriter, code int, v any, log logx.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Errf("encode the JSON answer: %v", err)
	}
}
