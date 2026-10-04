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
	"slices"
	"strings"
	"time"

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

// saveUploadedFile does the work of handleUpload. It checks the file field
// against allowedExt and maxBytes, and copies it to destDir. It returns each failure, thus a full disk is not a success. An
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

// readUserFile reads the file of an upload to a tree of config.UserFileTrees,
// and it answers the name and the bytes. A form gives the file in the field
// "file". Each other body is the bytes of the file, and ?name= is its name.
// The Android share path sends that form. See ShareIn.java.
func readUserFile(r *http.Request, tree config.UserFileTree, maxBytes int64, now time.Time) (string, []byte, error) {
	var body io.Reader = r.Body
	name := r.URL.Query().Get("name")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		// 10 MB is the part of the form that stays in memory. It is not the
		// size limit.
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			return "", nil, fmt.Errorf("parse form: %w", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return "", nil, fmt.Errorf("read upload: %w", err)
		}
		defer file.Close()
		if header.Size > maxBytes {
			return "", nil, &uploadRejected{msg: fmt.Sprintf("file too large (%.2f MB, limit is %.2f MB)", float64(header.Size)/(1<<20), float64(maxBytes)/(1<<20))}
		}
		body, name = file, header.Filename
	}

	name = userFileName(name, tree, now)
	ext := strings.ToLower(filepath.Ext(name))
	if !slices.Contains(tree.Exts, ext) {
		return "", nil, &uploadRejected{msg: fmt.Sprintf("file type %q is not allowed (allowed: %s)", ext, strings.Join(tree.Exts, ", "))}
	}
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return "", nil, &uploadRejected{msg: fmt.Sprintf("file too large (limit is %.2f MB)", float64(maxBytes)/(1<<20))}
	}
	return name, data, nil
}

// userFileName changes the name that a client gives into a file name of the
// tree. It keeps only the part after the last separator, thus the file cannot
// leave the tree. An empty name gets the time. A name with no extension gets
// the first extension of the tree. The contacts application of Android gives
// the name of the person, with no extension.
func userFileName(raw string, tree config.UserFileTree, now time.Time) string {
	name := strings.ReplaceAll(raw, "\\", "/")
	name = strings.TrimSpace(name[strings.LastIndexByte(name, '/')+1:])
	if name == "" {
		name = "shared-" + now.UTC().Format("20060102T150405Z")
	}
	if strings.LastIndexByte(name, '.') <= 0 {
		name += tree.Exts[0]
	}
	return name
}

// handleUploadUserFile answers the upload handler of one tree of
// config.UserFileTrees. The answer is a Markdown link. The target of the
// link holds the name with escapes. A contact file often has a space in its
// name, and a space ends the target of a Markdown link.
//
// ?incoming=1 also puts a line for the file on the incoming index. The
// editor, the receive box and the Android share path send it. A note script
// that keeps its data in a JSON file sends each change to this handler, and
// it does not. Its writes thus add no line.
func (a *App) handleUploadUserFile(tree config.UserFileTree) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		filename, data, err := readUserFile(r, tree, a.maxUploadBytes(), now)
		if err == nil {
			dir := a.layout().HTML(tree.Dir)
			if err = os.MkdirAll(dir, 0755); err == nil {
				err = os.WriteFile(filepath.Join(dir, filename), data, 0644)
			}
		}
		if err != nil {
			a.writeUploadError(w, "handleUploadUserFile "+tree.Dir, err)
			return
		}
		if r.URL.Query().Get("incoming") == "1" {
			// The file is on disk. A fault of the index must not fail an
			// upload that worked.
			if err := a.addIncomingFile(tree.Dir+"/"+filename, now); err != nil {
				a.log(logx.Upload).Errf("%s is saved, but the incoming index has no line for it: %v", filename, err)
			}
		}
		w.Write(fmt.Appendf(nil, "\n[%s](/%s/%s)\n", filename, tree.Dir, url.PathEscape(filename)))
	}
}
