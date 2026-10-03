package app

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// imageUploadExtensions lists what handleUpload accepts. ShareIn.java has a
// copy of the list for its share path. Keep the copies the same.
// config.UserFileTrees holds the lists of each other upload.
var imageUploadExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"}

// uploadRejected marks a failure that the file itself causes: a wrong type or
// too large a size. The handlers answer it with 400, and not with 500.
type uploadRejected struct{ msg string }

func (e *uploadRejected) Error() string { return e.msg }

// saveUploadedFile does the shared work of handleUpload and
// handleUploadUserFile.
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

// writeUploadError answers an uploadRejected with 400 and its reason. Each
// other failure gets 500 with a general text. The answer never holds the
// detail of a server fault.
func (a *App) writeUploadError(w http.ResponseWriter, logPrefix string, err error) {
	var rejected *uploadRejected
	if errors.As(err, &rejected) {
		http.Error(w, rejected.msg, http.StatusBadRequest)
		return
	}
	a.log(logx.Upload).Errf("%s: %v", logPrefix, err)
	http.Error(w, "Upload failed", http.StatusInternalServerError)
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	imgDir := a.layout().HTML("images")
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

// handleUploadUserFile answers the upload handler of one tree of
// config.UserFileTrees. The answer is a Markdown link. The target of the
// link holds the name with escapes. A contact file often has a space in its
// name, and a space ends the target of a Markdown link.
func (a *App) handleUploadUserFile(tree config.UserFileTree) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir := a.layout().HTML(tree.Dir)
		filename, err := a.saveUploadedFile(r, "file", dir, tree.Exts, a.maxUploadBytes())
		if err != nil {
			a.writeUploadError(w, "handleUploadUserFile "+tree.Dir, err)
			return
		}
		w.Write(fmt.Appendf(nil, "\n[%s](/%s/%s)\n", filename, tree.Dir, url.PathEscape(filename)))
	}
}
