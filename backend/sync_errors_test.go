package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"net.basov.omngo/backend/internal/gitsync"
)

// TestSyncErrorStatus pins the Phase 4 typed-sentinel mapping. Each sync
// sentinel maps to its wire status. An unrelated error and nil both map to
// ok=false, which is a plain "error". The match also survives a wrap with
// %w, and that is the whole point of the move off string comparison.
func TestSyncErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
		wantOK     bool
	}{
		{"conflict", gitsync.ErrSyncConflict, "conflict", true},
		{"push conflict", gitsync.ErrPushConflict, "push_conflict", true},
		{"needs message", gitsync.ErrCommitMessageRequired, "needs_commit_message", true},
		{"wrapped conflict", fmt.Errorf("pull step: %w", gitsync.ErrSyncConflict), "conflict", true},
		{"generic error", errors.New("disk exploded"), "", false},
		{"nil", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, ok := gitsync.SyncErrorStatus(tt.err)
			if ok != tt.wantOK || status != tt.wantStatus {
				t.Errorf("gitsync.SyncErrorStatus(%v) = (%q, %v), want (%q, %v)",
					tt.err, status, ok, tt.wantStatus, tt.wantOK)
			}
			// A mapped error must carry a non-empty user message. An
			// unmapped one must not.
			if ok && msg == "" {
				t.Errorf("mapped error %v produced an empty message", tt.err)
			}
			if !ok && msg != "" {
				t.Errorf("unmapped error %v produced message %q, want empty", tt.err, msg)
			}
		})
	}
}

// TestSyncConflictErrorWrapsSentinel pins that the file-carrying conflict
// error still reads as ErrSyncConflict everywhere the state machine looks,
// which is errors.Is and gitsync.SyncErrorStatus. Its file list stays reachable
// with errors.As, and that list survives a wrap with %w.
func TestSyncConflictErrorWrapsSentinel(t *testing.T) {
	ce := &gitsync.SyncConflictError{Files: []string{"md/A.md", "md/B.md"}}

	if !errors.Is(ce, gitsync.ErrSyncConflict) {
		t.Error("gitsync.SyncConflictError does not unwrap to ErrSyncConflict")
	}
	if status, _, ok := gitsync.SyncErrorStatus(ce); !ok || status != "conflict" {
		t.Errorf("gitsync.SyncErrorStatus(gitsync.SyncConflictError) = (%q, %v), want (conflict, true)", status, ok)
	}

	wrapped := fmt.Errorf("pull: %w", error(ce))
	var got *gitsync.SyncConflictError
	if !errors.As(wrapped, &got) {
		t.Fatal("errors.As could not recover gitsync.SyncConflictError through a %w wrap")
	}
	if len(got.Files) != 2 || got.Files[0] != "md/A.md" || got.Files[1] != "md/B.md" {
		t.Errorf("recovered Files = %v, want [md/A.md md/B.md]", got.Files)
	}
}

// TestSyncConflictAnswerShape pins the wire shape that the conflict modal
// reads. That shape is status "conflict", the message, and a files array
// that is ALWAYS present. The array is [] and never null, also when there
// is no per-file conflict, thus the frontend can iterate it with no nil
// guard.
func TestSyncConflictAnswerShape(t *testing.T) {
	a := newTestApp(t)

	// With files.
	rec := httptest.NewRecorder()
	a.writeJSON(rec, http.StatusOK, gitsync.NewSyncConflict("Fast-forward not possible.", []string{"md/A.md"}))
	var body struct {
		Status  string   `json:"status"`
		Message string   `json:"message"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Status != "conflict" || body.Message == "" || len(body.Files) != 1 || body.Files[0] != "md/A.md" {
		t.Errorf("unexpected body: %+v", body)
	}

	// Nil files must serialize as [] (never null) and never omit the key.
	rec2 := httptest.NewRecorder()
	a.writeJSON(rec2, http.StatusOK, gitsync.NewSyncConflict("diverged", nil))
	raw := rec2.Body.String()
	if !contains(raw, `"files":[]`) {
		t.Errorf("nil files did not serialize as []: %s", raw)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestSyncSentinelsAreDistinct guards that the three sentinels are not
// accidentally the same value (which would collapse their wire statuses).
func TestSyncSentinelsAreDistinct(t *testing.T) {
	all := []error{gitsync.ErrSyncConflict, gitsync.ErrPushConflict, gitsync.ErrCommitMessageRequired}
	for i := range all {
		for j := range all {
			if i != j && errors.Is(all[i], all[j]) {
				t.Errorf("sentinels %d and %d are not distinct", i, j)
			}
		}
	}
}
