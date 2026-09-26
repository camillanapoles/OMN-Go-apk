package backend

// ----------------------------------------------------------------------
// The two upload endpoints
// ----------------------------------------------------------------------
//
// /api/upload takes an image, and /api/upload_json takes a JSON file.
// Each one writes a file of the client into the storage directory. A
// fault here can thus put a file in the wrong place or lose one.
//
// handlers_test.go tests saveUploadedFile alone. The tests below send each
// request through registerRoutes, the same as a real client. They thus
// also cover the route, the admin check, the answer that the editor
// inserts, and the status code of each failure.
//
// Before these tests, handleUpload, handleUploadJSON and writeUploadError
// had no coverage.

import (
	"bytes"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	a.WithConfig(func(c *Config) { c.MaxUploadSizeMB = 1 })
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
	for _, route := range []string{"/api/upload", "/api/upload_json"} {
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

// The JSON route reads the field "file" and takes JSON types only. An
// image sent there gives 400.
func TestUploadJSONRejectsAnImage(t *testing.T) {
	a := uplApp(t)
	rec := uplPost(t, a, "/api/upload_json", "file", "pic.png", []byte("x"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}
