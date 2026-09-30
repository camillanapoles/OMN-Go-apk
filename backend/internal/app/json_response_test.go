package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An error answer carries the status code, the JSON content type and the
// shape of section 1.4 of doc/API.md.
func TestWriteJSONErrorShape(t *testing.T) {
	w := httptest.NewRecorder()
	newTestApp(t).writeJSONError(w, http.StatusBadRequest, `bad "name"`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("the code is %d, want 400", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("the content type is %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
	}
	if body["status"] != "error" || body["message"] != `bad "name"` || len(body) != 2 {
		t.Errorf("the body is %v", body)
	}
}

// writeJSON is the one JSON writer. A second encoder on a ResponseWriter
// fails this test.
func TestOnlyWriteJSONEncodesAnAnswer(t *testing.T) {
	for _, f := range productionGoFiles(t) {
		if f == "internal/render/json_response.go" {
			continue
		}
		src, err := readBackendFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "json.NewEncoder(w)") {
			t.Errorf("%s encodes an answer. Call a.writeJSON.", f)
		}
	}
}
