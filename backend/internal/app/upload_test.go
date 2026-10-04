package app

// ----------------------------------------------------------------------
// The upload endpoints
// ----------------------------------------------------------------------
//
// /api/upload takes an image. Each tree of config.UserFileTrees has an
// upload of its own, for example /api/upload_json for a JSON file.
// Each one writes a file of the client into the storage directory. A
// fault here can thus put a file in the wrong place or lose one.
//
// handlers_test.go tests saveUploadedFile alone. The tests below send each
// request through registerRoutes, the same as a real client. They thus
// also cover the route, the admin check, the answer that the editor
// inserts, and the status code of each failure.
//

import (
	"bytes"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
)

// uplApp makes a test application with the real route table.
func uplApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	a.Router = http.NewServeMux()
	a.registerRoutes(a.Router)
	return a
}

// uplPost sends one file in a multipart form to route. The request comes
// from the loopback address, thus it has the admin role.
func uplPost(t *testing.T, a *App, route, field, name string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, route, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.RemoteAddr = "127.0.0.1:40000"
	rec := httptest.NewRecorder()
	a.Router.ServeHTTP(rec, req)
	return rec
}

// uplFiles answers the path of each regular file under the storage
// directory, relative to it, except config.json.
func uplFiles(t *testing.T, a *App) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(a.StorageDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(a.StorageDir, p)
		if rel != "config.json" {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// An image upload writes the bytes to html/images and answers the img
// element that the editor inserts into the note. The element carries the
// class that gives the image its default width.
func TestUploadImageWritesTheFileAndAnswersAnImgElement(t *testing.T) {
	a := uplApp(t)
	payload := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a}
	rec := uplPost(t, a, "/api/upload", "image", "pic.png", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	want := "\n<img src=\"/images/pic.png\" alt=\"pic.png\" class=\"omn-imported-image\" />\n"
	if rec.Body.String() != want {
		t.Errorf("answer = %q, want %q", rec.Body.String(), want)
	}
	got, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "images", "pic.png"))
	if err != nil {
		t.Fatalf("the image is not on disk: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("the bytes on disk differ from the upload")
	}
}

// The name of the file goes into an HTML attribute. A quote or an
// ampersand in the name must not break the element.
func TestUploadImageEscapesTheNameInTheElement(t *testing.T) {
	a := uplApp(t)
	rec := uplPost(t, a, "/api/upload", "image", `a"b&c.png`, []byte("x"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `src="/images/a&#34;b&amp;c.png"`) {
		t.Errorf("the name is not escaped in src: %q", body)
	}
	if strings.Contains(body, `a"b`) {
		t.Errorf("a raw quote reached the element: %q", body)
	}
}

// A file name with a path must not write outside html/images. The
// multipart reader keeps the last element of the name. This test holds
// that rule for each form of path that a client can send.
func TestUploadImageStaysInTheImagesDirectory(t *testing.T) {
	for _, name := range []string{
		"../../md/Welcome.png",
		"/etc/evil.png",
		"sub/dir/deep.png",
	} {
		a := uplApp(t)
		before := len(uplFiles(t, a))
		rec := uplPost(t, a, "/api/upload", "image", name, []byte("x"))
		if rec.Code != http.StatusOK {
			t.Errorf("%q: status %d, want 200: %s", name, rec.Code, rec.Body.String())
			continue
		}
		files := uplFiles(t, a)
		if len(files) != before+1 {
			t.Errorf("%q: %d files after the upload, want %d: %v", name, len(files), before+1, files)
		}
		want := "html/images/" + filepath.Base(name)
		found := false
		for _, f := range files {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: no file at %s. Files: %v", name, want, files)
		}
	}
}

// A type that is not on the list gives 400 with the reason, and it
// writes nothing. The editor shows the reason to the person.
func TestUploadImageRejectsAnUnknownType(t *testing.T) {
	a := uplApp(t)
	before := uplFiles(t, a)
	rec := uplPost(t, a, "/api/upload", "image", "run.sh", []byte("#!/bin/sh"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `".sh" is not allowed`) {
		t.Errorf("the answer does not give the reason: %q", rec.Body.String())
	}
	if after := uplFiles(t, a); len(after) != len(before) {
		t.Errorf("a rejected upload wrote a file: %v", after)
	}
}

// The size limit comes from max_upload_size_mb in the configuration, and
// not from a constant. A file over the limit gives 400 and writes nothing.
func TestUploadImageUsesTheConfiguredSizeLimit(t *testing.T) {
	a := uplApp(t)
	a.config.Update(func(c *config.Config) { c.MaxUploadSizeMB = 1 })
	before := uplFiles(t, a)

	rec := uplPost(t, a, "/api/upload", "image", "big.png", make([]byte, 1<<20+1))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "too large") {
		t.Errorf("the answer does not give the reason: %q", rec.Body.String())
	}
	if after := uplFiles(t, a); len(after) != len(before) {
		t.Errorf("a rejected upload wrote a file: %v", after)
	}

	rec = uplPost(t, a, "/api/upload", "image", "fits.png", make([]byte, 1<<20))
	if rec.Code != http.StatusOK {
		t.Errorf("a file of exactly the limit: status %d, want 200", rec.Code)
	}
}

// A fault of the device gives 500 with a general message. The body must
// not carry the path or the error text, because a LAN client can read
// the body. The log gets the detail.
func TestUploadImageHidesAServerFault(t *testing.T) {
	a := uplApp(t)
	// A regular file where the images directory must be. MkdirAll then
	// fails, which stands for a full disk or a permission fault.
	images := filepath.Join(a.StorageDir, "html", "images")
	if err := os.RemoveAll(images); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(images, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	rec := uplPost(t, a, "/api/upload", "image", "pic.png", []byte("x"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "Upload failed" {
		t.Errorf("answer = %q, want only %q", got, "Upload failed")
	}
}

// A client that is not local needs the admin role. The route answers 401
// and writes nothing.
func TestUploadNeedsTheAdminRoleFromTheLAN(t *testing.T) {
	a := uplApp(t)
	before := uplFiles(t, a)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("image", "pic.png")
	fw.Write([]byte("x"))
	mw.Close()
	routes := []string{"/api/upload"}
	for _, tree := range config.UserFileTrees {
		routes = append(routes, tree.Upload)
	}
	for _, route := range routes {
		req := httptest.NewRequest(http.MethodPost, route, bytes.NewReader(buf.Bytes()))
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.RemoteAddr = "192.168.1.50:40000"
		rec := httptest.NewRecorder()
		a.Router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s from the LAN with no session: status %d, want 401", route, rec.Code)
		}
	}
	if after := uplFiles(t, a); len(after) != len(before) {
		t.Errorf("a refused upload wrote a file: %v", after)
	}
}

// A JSON upload writes to html/user_json and answers a Markdown link.
// Both .json and .jsonl are accepted.
func TestUploadJSONWritesTheFileAndAnswersALink(t *testing.T) {
	for _, name := range []string{"data.json", "log.jsonl"} {
		a := uplApp(t)
		payload := []byte(`{"a":1}`)
		rec := uplPost(t, a, "/api/upload_json", "file", name, payload)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200: %s", name, rec.Code, rec.Body.String())
			continue
		}
		want := "\n[" + name + "](/user_json/" + name + ")\n"
		if rec.Body.String() != want {
			t.Errorf("%s: answer = %q, want %q", name, rec.Body.String(), want)
		}
		got, err := os.ReadFile(filepath.Join(a.StorageDir, "html", "user_json", name))
		if err != nil || !bytes.Equal(got, payload) {
			t.Errorf("%s: the file on disk is wrong: %q, %v", name, got, err)
		}
	}
}

// Each tree of config.UserFileTrees takes each of its extensions. It writes
// the file below html/<tree> and answers a Markdown link to it. A GET of the
// link then answers the bytes as text. The Android WebView thus shows a
// contact or a calendar, and it does not start a download.
func TestUploadUserFileWritesEachTree(t *testing.T) {
	for _, tree := range config.UserFileTrees {
		for _, ext := range tree.Exts {
			a := uplApp(t)
			name := "data" + ext
			payload := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
			rec := uplPost(t, a, tree.Upload, "file", name, payload)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: status %d, want 200: %s", name, rec.Code, rec.Body.String())
				continue
			}
			link := "/" + tree.Dir + "/" + name
			if want := "\n[" + name + "](" + link + ")\n"; rec.Body.String() != want {
				t.Errorf("%s: answer = %q, want %q", name, rec.Body.String(), want)
			}
			got, err := os.ReadFile(filepath.Join(a.StorageDir, "html", tree.Dir, name))
			if err != nil || !bytes.Equal(got, payload) {
				t.Errorf("%s: the file on disk is wrong: %q, %v", name, got, err)
			}

			get := httptest.NewRecorder()
			a.Router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, link, nil))
			if get.Code != http.StatusOK || !bytes.Equal(get.Body.Bytes(), payload) {
				t.Errorf("GET %s: status %d, body %q", link, get.Code, get.Body.String())
			}
			ct := get.Header().Get("Content-Type")
			if ext != ".json" && ct != "text/plain; charset=utf-8" {
				t.Errorf("GET %s: Content-Type = %q, want text/plain", link, ct)
			}
		}
	}
}

// The contacts application names a file after the person, thus the name
// holds a space. A space ends the target of a Markdown link. The answer
// escapes the name in the target, and the escaped link reaches the file.
func TestUploadUserFileEscapesTheNameInTheLink(t *testing.T) {
	a := uplApp(t)
	rec := uplPost(t, a, "/api/upload_contacts", "file", "Ann Lee (1).vcf", []byte("BEGIN:VCARD\r\nEND:VCARD\r\n"))
	const want = "\n[Ann Lee (1).vcf](/user_contacts/Ann%20Lee%20%281%29.vcf)\n"
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Fatalf("status %d, answer = %q, want %q", rec.Code, rec.Body.String(), want)
	}
	get := httptest.NewRecorder()
	a.Router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/user_contacts/Ann%20Lee%20%281%29.vcf", nil))
	if get.Code != http.StatusOK || !strings.HasPrefix(get.Body.String(), "BEGIN:VCARD") {
		t.Errorf("the escaped link answers status %d, body %q", get.Code, get.Body.String())
	}
}

// A tree takes only its own extensions. A calendar sent to the contacts
// route gives 400 and writes no file.
func TestUploadUserFileRejectsTheFileOfAnotherTree(t *testing.T) {
	for _, tree := range config.UserFileTrees {
		for _, other := range config.UserFileTrees {
			if other.Dir == tree.Dir {
				continue
			}
			a := uplApp(t)
			before := uplFiles(t, a)
			name := "data" + other.Exts[0]
			rec := uplPost(t, a, tree.Upload, "file", name, []byte("x"))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s sent to %s: status %d, want 400", name, tree.Upload, rec.Code)
			}
			if after := uplFiles(t, a); len(after) != len(before) {
				t.Errorf("%s sent to %s wrote a file: %v", name, tree.Upload, after)
			}
		}
	}
}

// uplPostRaw sends the bytes of a file as the body of the request, the same
// as ShareIn.java. query holds the name of the file and the other values.
func uplPostRaw(t *testing.T, a *App, route, query string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, route+"?"+query, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.RemoteAddr = "127.0.0.1:40000"
	rec := httptest.NewRecorder()
	a.Router.ServeHTTP(rec, req)
	return rec
}

// uplIncoming answers the Markdown of the incoming index, or "".
func uplIncoming(a *App) string {
	data, _ := os.ReadFile(a.layout().MD("incoming", "incoming.md"))
	return string(data)
}

// ?incoming=1 puts a line for the file on the Incoming notes page, for each
// tree. The text of the link is the path below the storage directory. The
// page that the server sends then holds a link that reaches the file.
func TestUploadUserFileWithIncomingAddsALine(t *testing.T) {
	a := uplApp(t)
	for _, tree := range config.UserFileTrees {
		name := "Ann Lee (1)" + tree.Exts[0]
		rec := uplPost(t, a, tree.Upload+"?incoming=1", "file", name, []byte("data"))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", tree.Upload, rec.Code, rec.Body.String())
		}
		target := "/" + tree.Dir + "/Ann%20Lee%20%281%29" + tree.Exts[0]
		line := "· [html/" + tree.Dir + "/" + name + "](" + target + ")\n"
		if idx := uplIncoming(a); !strings.Contains(idx, line) {
			t.Errorf("%s: the incoming index has no line %q:\n%s", tree.Dir, line, idx)
		}

		page := httptest.NewRecorder()
		// refresh=true compiles the page again. The time of a file has
		// the step of the kernel clock, and this loop is faster.
		req := httptest.NewRequest(http.MethodGet, "/incoming/incoming.html?refresh=true", nil)
		req.RemoteAddr = "127.0.0.1:40000"
		a.Router.ServeHTTP(page, req)
		link := `<a href="` + target + `">html/` + tree.Dir + "/" + name + "</a>"
		if !strings.Contains(page.Body.String(), link) {
			t.Errorf("%s: the Incoming notes page has no link %q", tree.Dir, link)
		}
		file := httptest.NewRecorder()
		a.Router.ServeHTTP(file, httptest.NewRequest(http.MethodGet, target, nil))
		if file.Code != http.StatusOK || file.Body.String() != "data" {
			t.Errorf("GET %s: status %d, body %q", target, file.Code, file.Body.String())
		}
	}
	if n := strings.Count(uplIncoming(a), `* <span class="omn-incoming-when">`); n != len(config.UserFileTrees) {
		t.Errorf("the incoming index has %d line(s), want %d", n, len(config.UserFileTrees))
	}
}

// An upload with no incoming=1 adds no line. A note script that keeps its
// data in a JSON file sends each change to /api/upload_json. See
// frontend/md/Test/OMN-Go/JSONBasedCounter.md. A line for each write would
// fill the Incoming notes page.
func TestUploadUserFileWithoutIncomingAddsNoLine(t *testing.T) {
	a := uplApp(t)
	before := uplIncoming(a)
	for i := 0; i < 3; i++ {
		if rec := uplPost(t, a, "/api/upload_json", "file", "local-counter.json", []byte(`{"n":1}`)); rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
	}
	if after := uplIncoming(a); after != before || strings.Contains(after, "local-counter") {
		t.Errorf("an upload with no incoming=1 changed the incoming index:\n%s", after)
	}
}

// The Android share path sends the bytes as the body and the name in ?name=.
// The server gives the file a safe name with an extension of the tree, saves
// it and answers the link.
func TestUploadUserFileTakesARawBody(t *testing.T) {
	for _, c := range []struct {
		why, route, name, want string
	}{
		{"a plain name", "/api/upload_calendars", "invite.ics", "invite.ics"},
		{"the contacts application gives the name of the person", "/api/upload_contacts", "Ann Lee", "Ann Lee.vcf"},
		{"a path keeps only its last part", "/api/upload_contacts", "../../md/Ann.vcf", "Ann.vcf"},
		{"a path of Windows", "/api/upload_json", `C:\dir\data.json`, "data.json"},
		{"upper case in the extension", "/api/upload_calendars", "OLD.VCS", "OLD.VCS"},
	} {
		a := uplApp(t)
		rec := uplPostRaw(t, a, c.route, "incoming=1&name="+url.QueryEscape(c.name), []byte("data"))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d: %s", c.why, rec.Code, rec.Body.String())
			continue
		}
		dir := strings.Replace(c.route, "/api/upload_", "user_", 1)
		got, err := os.ReadFile(filepath.Join(a.StorageDir, "html", dir, c.want))
		if err != nil || string(got) != "data" {
			t.Errorf("%s: html/%s/%s is wrong: %q, %v", c.why, dir, c.want, got, err)
		}
		if !strings.Contains(uplIncoming(a), "[html/"+dir+"/"+c.want+"](") {
			t.Errorf("%s: the incoming index has no line for %s:\n%s", c.why, c.want, uplIncoming(a))
		}
		if files := uplFiles(t, a); len(files) != 2 {
			t.Errorf("%s: the upload wrote %v, want the file and the incoming index only", c.why, files)
		}
	}

	// A share with no name gets the time as its name.
	a := uplApp(t)
	rec := uplPostRaw(t, a, "/api/upload_contacts", "", []byte("data"))
	if rec.Code != http.StatusOK || !regexp.MustCompile(`^\n\[shared-\d{8}T\d{6}Z\.vcf\]\(/user_contacts/shared-\d{8}T\d{6}Z\.vcf\)\n$`).MatchString(rec.Body.String()) {
		t.Errorf("a body with no name: status %d, answer %q", rec.Code, rec.Body.String())
	}
}

// A raw body has no declared size, thus the server counts the bytes. A file
// over the limit, and a file of another type, give 400 and write nothing.
func TestUploadUserFileRefusesABadRawBody(t *testing.T) {
	a := uplApp(t)
	a.config.Update(func(c *config.Config) { c.MaxUploadSizeMB = 1 })
	before := uplFiles(t, a)

	big := bytes.Repeat([]byte("x"), 1<<20+1)
	if rec := uplPostRaw(t, a, "/api/upload_contacts", "incoming=1&name=big.vcf", big); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "file too large") {
		t.Errorf("a body over the limit: status %d, answer %q", rec.Code, rec.Body.String())
	}
	if rec := uplPostRaw(t, a, "/api/upload_contacts", "incoming=1&name=invite.ics", []byte("x")); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), `file type ".ics" is not allowed (allowed: .vcf)`) {
		t.Errorf("a calendar sent to the contacts route: status %d, answer %q", rec.Code, rec.Body.String())
	}
	if rec := uplPostRaw(t, a, "/api/upload_contacts", "incoming=1&name=..", []byte("x")); rec.Code != http.StatusBadRequest {
		t.Errorf("the name \"..\": status %d, want 400", rec.Code)
	}
	if after := uplFiles(t, a); len(after) != len(before) {
		t.Errorf("a refused upload wrote a file: %v", after)
	}
}

// The JSON route reads the field "file" and takes JSON types only. An
// image sent there gives 400.
func TestUploadJSONRejectsAnImage(t *testing.T) {
	a := uplApp(t)
	rec := uplPost(t, a, "/api/upload_json", "file", "pic.png", []byte("x"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}
