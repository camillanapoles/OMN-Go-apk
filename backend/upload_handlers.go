package backend

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// imageUploadExtensions and jsonUploadExtensions list what saveUploadedFile
// accepts. MainActivity.java has a copy of both lists for its share path.
// Keep the copies the same.
var (
	imageUploadExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"}
	jsonUploadExtensions  = []string{".json", ".jsonl"}
)

// uploadRejected marks a failure that the file itself causes: a wrong type or
// too large a size. The handlers answer it with 400, and not with 500.
type uploadRejected struct{ msg string }

func (e *uploadRejected) Error() string { return e.msg }

// saveUploadedFile does the shared work of handleUpload and handleUploadJSON.
// It checks the file field against allowedExt and maxBytes, and copies it to
// destDir. It returns each failure, thus a full disk is not a success. An
// empty allowedExt, or maxBytes <= 0, skips that check, for the tests.
func (a *App) saveUploadedFile(r *http.Request, formField, destDir string, allowedExt []string, maxBytes int64) (filename string, err error) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB in-memory threshold before spilling to temp files; NOT the size cap (see maxBytes below)
		return "", fmt.Errorf("parse form: %w", err)
	}
	file, header, err := r.FormFile(formField)
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	defer file.Close()

	if len(allowedExt) > 0 {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		allowed := false
		for _, e := range allowedExt {
			if ext == e {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", &uploadRejected{msg: fmt.Sprintf("file type %q is not allowed (allowed: %s)", ext, strings.Join(allowedExt, ", "))}
		}
	}
	if maxBytes > 0 && header.Size > maxBytes {
		return "", &uploadRejected{msg: fmt.Sprintf("file too large (%.2f MB, limit is %.2f MB)", float64(header.Size)/(1<<20), float64(maxBytes)/(1<<20))}
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	destPath := filepath.Join(destDir, header.Filename)
	dest, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create destination file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		return "", fmt.Errorf("write destination file: %w", err)
	}
	return header.Filename, nil
}

// maxUploadBytes converts MaxUploadSizeMB to bytes. loadConfig always sets a
// positive value, thus the fallback below is a guard only.
func (a *App) maxUploadBytes() int64 {
	mb := a.GetConfig().MaxUploadSizeMB
	if mb <= 0 {
		mb = defaultMaxUploadSizeMB
	}
	return int64(mb) * 1024 * 1024
}

// writeUploadError answers an uploadRejected with 400 and its reason. Each
// other failure gets 500 with a general text. The answer never holds the
// detail of a server fault.
func (a *App) writeUploadError(w http.ResponseWriter, logPrefix string, err error) {
	var rejected *uploadRejected
	if errors.As(err, &rejected) {
		http.Error(w, rejected.msg, http.StatusBadRequest)
		return
	}
	a.logErrf(logUpload, "%s: %v", logPrefix, err)
	http.Error(w, "Upload failed", http.StatusInternalServerError)
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	imgDir := filepath.Join(a.StorageDir, "html", "images")
	filename, err := a.saveUploadedFile(r, "image", imgDir, imageUploadExtensions, a.maxUploadBytes())
	if err != nil {
		a.writeUploadError(w, "handleUpload", err)
		return
	}
	// Use an <img> element, because only HTML can carry the class. Use a
	// double-quoted string, because a raw string keeps "\n" as two
	// characters.
	escaped := html.EscapeString(filename)
	w.Write(fmt.Appendf(nil, "\n<img src=\"/images/%s\" alt=\"%s\" class=\"omn-imported-image\" />\n", escaped, escaped))
}

func (a *App) handleUploadJSON(w http.ResponseWriter, r *http.Request) {
	jsonDir := filepath.Join(a.StorageDir, "html", "user_json")
	filename, err := a.saveUploadedFile(r, "file", jsonDir, jsonUploadExtensions, a.maxUploadBytes())
	if err != nil {
		a.writeUploadError(w, "handleUploadJSON", err)
		return
	}
	w.Write(fmt.Appendf(nil, "\n[%s](/user_json/%s)\n", filename, filename))
}
