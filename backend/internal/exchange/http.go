package exchange

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"net.basov.omngo/backend/internal/logx"
)

// ----------------------------------------------------------------------
// The HTTP handlers
// ----------------------------------------------------------------------
//
// Both endpoints are ADMIN-ONLY. An import writes files, and an export is a
// way out of the note tree. The device itself passes authMiddleware. A person
// reads an error answer as a toast on Android or as a line on the incoming page.

// HandleExportNote answers GET /api/export/note?name=<note> with the Markdown
// of the note, FileName: set, as a download. ShareOut.java gives the bytes to
// the share sheet.
func (svc Service) HandleExportNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		svc.writeJSONError(w, http.StatusBadRequest, "no note named")
		return
	}

	data, filename, err := svc.ExportNoteSource(name)
	if err != nil {
		if os.IsNotExist(err) {
			svc.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no note %q", name))
			return
		}
		svc.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// The description goes in a header, for the message that carries the
	// file. Android needs the bytes and the text in one answer.
	// flattenExportName keeps only A-Za-z0-9._-, thus the file name needs no
	// quotes.
	if desc := noteDescription(string(data)); desc != "" {
		w.Header().Set(headerDescription, base64.StdEncoding.EncodeToString([]byte(desc)))
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write(data)
}

// HandleImportNote answers POST /api/import/note?name=<display name>. Android
// posts raw bytes, and the desktop posts a form file. The sanitizer also
// checks ?name=, the fallback for a note with no FileName: line.
func (svc Service) HandleImportNote(w http.ResponseWriter, r *http.Request) {

	limit := svc.MaxUploadBytes
	displayName := r.URL.Query().Get("name")
	var content []byte

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(limit); err != nil {
			svc.writeJSONError(w, http.StatusBadRequest, "cannot read the upload: "+err.Error())
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			svc.writeJSONError(w, http.StatusBadRequest, "no file in the upload")
			return
		}
		defer file.Close()
		if content, err = readImportBody(file, limit); err != nil {
			svc.writeJSONError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		if displayName == "" && header != nil {
			displayName = header.Filename
		}
	} else {
		var err error
		if content, err = readImportBody(r.Body, limit); err != nil {
			svc.writeJSONError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
	}

	res, err := svc.ImportNote(content, displayName, time.Now())
	if res.Name == "" {
		// The import wrote nothing. This is the only real failure.
		svc.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	out := map[string]string{
		"status": "success",
		"name":   res.Name,
		"base":   res.Base,
		"url":    "/" + res.Name + ".html",
	}
	if err != nil {
		// The note is on disk. Only its index line is missing. A failure
		// report would make the user send the note again, and a second copy
		// repairs nothing.
		out["warning"] = err.Error()
		svc.Log(logx.Exchange).Errf("%v", err)
	}
	svc.Log(logx.Exchange).Infof("imported %s", res.Name)
	svc.writeJSON(w, http.StatusOK, out)
}

// readImportBody reads at most limit bytes, and it answers an error when the
// body has more. Do NOT use readCapped of internal/search/search.go, because it
// cuts the file, and half a note is not a useful import.
func readImportBody(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("the note is larger than the upload limit of %d MB",
			limit/(1024*1024))
	}
	return data, nil
}
