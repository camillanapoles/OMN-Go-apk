package backend

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (a *App) getConfigPageBody() string {
	cfg := a.GetConfig() // snapshot under RLock; render against the copy

	for len(cfg.GitServers) < maxGitServers {
		cfg.GitServers = append(cfg.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(cfg.GitServers)+1)})
	}

	// The view carries no secret. See gitServerView in templates.go.
	view := configPageView{
		ServerPort:         cfg.ServerPort,
		Author:             cfg.Author,
		UseInternalEd:      cfg.UseInternalEd,
		DesktopExtCmd:      cfg.DesktopExtCmd,
		Theme:              cfg.Theme,
		ShareLAN:           cfg.ShareLAN,
		Hostname:           cfg.Hostname,
		PruneDepth:         cfg.BackupPruneDepth,
		MaxUploadSizeMB:    cfg.MaxUploadSizeMB,
		EnableIntentURI:    cfg.EnableIntentURI,
		EnableTermuxIntent: cfg.EnableTermuxIntent,
		AndroidFullscreen:  cfg.AndroidFullscreen,
		SearchEnabled:      cfg.SearchEnabled,
		SearchKinds:        normalizeSearchKinds(cfg.SearchKinds),
		SearchBundled:      cfg.SearchBundled,
		SearchScope:        cfg.SearchScope,
		SearchIndexStatus:  a.searchIndexStatus(),
		LogDebug:           cfg.LogDebug,
		LogInfo:            cfg.LogInfo,
		LogTags:            normalizeLogTags(cfg.LogTags),
	}
	for i, gs := range cfg.GitServers {
		view.GitServers = append(view.GitServers, gitServerView{
			Index:  i,
			Slot:   i + 1,
			Active: cfg.ActiveGitIndex == i,
			Name:   gs.Name,
			URL:    gs.URL,
		})
	}

	return renderConfigPage(view)
}

func (a *App) getExternalEditPageBody(fileName string, viewURL string) string {
	view := externalEditView{
		Cmd:      a.GetConfig().DesktopExtCmd,
		FileName: fileName,
		ViewURL:  viewURL,
	}
	return renderExternalEditPage(view)
}

// configFormMaxMemory is the defaultMaxMemory of net/http.
const configFormMaxMemory = 32 << 20

// configFieldSent reports whether a POST to /api/config carries one field. A
// field that the request does not carry keeps its value. See
// doc/decisions/0014-change-only-the-settings-that-a-request-names.md. A
// browser sends nothing for a clear checkbox. The Config page thus names its
// checkboxes in the hidden field config_fields, and each name there counts as
// sent.
func configFieldSent(r *http.Request) func(field string) bool {
	if r.Form == nil {
		// ParseMultipartForm fills r.Form for multipart and for urlencoded
		// bodies. For urlencoded, it reports ErrNotMultipart, and that is not
		// a failure.
		_ = r.ParseMultipartForm(configFormMaxMemory)
	}

	governed := map[string]bool{}
	for _, group := range r.Form["config_fields"] {
		for _, field := range strings.Split(group, ",") {
			if field = strings.TrimSpace(field); field != "" {
				governed[field] = true
			}
		}
	}

	return func(field string) bool {
		if _, ok := r.Form[field]; ok {
			return true
		}
		return governed[field]
	}
}

// handleConfig answers GET and POST on /api/config. A GET answers the whole
// Config with each password, for "Show passwords". It is admin only.
func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.writeJSON(w, http.StatusOK, a.GetConfig())

	case http.MethodPost:
		prev := a.GetConfig()

		sent := configFieldSent(r)

		var next Config
		a.WithConfig(func(c *Config) {
			applyConfigForm(c, r, sent)
			applyGitServerForm(c, r, sent)
			next = *c
		})

		if err := a.persistConfig(next); err != nil {
			http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
			return
		}
		a.applyConfigChange(prev, next)

		if next.ShareLAN != prev.ShareLAN {
			// saveConfig in omn-go-config.js reads this exact word and then
			// calls /api/restart.
			w.Write([]byte("RestartRequired"))
			return
		}
		w.Write([]byte("Saved"))

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// persistConfig writes one configuration to config.json. It runs OUTSIDE the
// configuration lock, thus a file write does not stop a reader. The caller
// passes the snapshot that it took inside the lock.
func (a *App) persistConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		a.logErrf(logConfig, "persistConfig: failed to marshal the configuration: %v", err)
		return err
	}
	configPath := filepath.Join(a.StorageDir, "config.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		a.logErrf(logConfig, "persistConfig: failed to write %s: %v", configPath, err)
		return err
	}
	return nil
}

// applyConfigChange starts the work that a saved change needs. Each change
// applies at once, except share_lan, because the socket binds one time at the
// start.
func (a *App) applyConfigChange(prev, next Config) {
	a.applyLogFilter(next)

	if !next.SearchEnabled {
		a.dropSearchIndex()
		return
	}
	if searchIndexNeedsRebuild(prev, next) {
		go a.rebuildSearchIndex()
	}
}

// searchIndexNeedsRebuild reports whether a saved change makes the global
// index wrong: search was off before, or the kinds that the index covers
// changed. A rebuild reads each note. The caller tests SearchEnabled first.
func searchIndexNeedsRebuild(prev, next Config) bool {
	if !prev.SearchEnabled {
		return true
	}
	if prev.SearchBundled != next.SearchBundled {
		return true
	}
	// Compare after normalization. A nil list and the default list are the
	// same set.
	return strings.Join(normalizeSearchKinds(prev.SearchKinds), ",") !=
		strings.Join(normalizeSearchKinds(next.SearchKinds), ",")
}

// handleRestart restarts the process, thus the start-time values come from
// the saved config. It answers first and exits on a delayed goroutine. On
// Android, it calls os.Exit(0), and START_STICKY starts ServerService again.
// On the desktop, it starts a new copy with OMN_GO_RESTARTED=1, thus no
// second browser tab opens.
func (a *App) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	a.logInfof(logRestart, "restart requested via /api/restart")
	w.Write([]byte("Restarting"))

	go func() {
		time.Sleep(500 * time.Millisecond) // let the response flush
		restartHook(a)
	}()
}

// restartHook stops this process. On the desktop it first starts a new
// process. handleRestart calls the hook after it answers. A test replaces
// the hook, because the real hook stops the test binary too.
var restartHook = (*App).restartProcess

func (a *App) restartProcess() {
	if runtime.GOOS == "android" {
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		a.logErrf(logRestart, "cannot locate own executable, not restarting: %v", err)
		return
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "OMN_GO_RESTARTED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		// A working old instance is better than none.
		a.logErrf(logRestart, "failed to start replacement process, keeping current one: %v", err)
		return
	}
	a.logInfof(logRestart, "replacement process started (pid %d), exiting", cmd.Process.Pid)
	os.Exit(0)
}

// resolveAndroidEditName makes the name for the omngo://edit intent.
// MainActivity picks md/ or html/ by the ".md" suffix alone, thus a page must
// become baseName + ".md". A file that is not a page keeps its name. It has
// no runtime.GOOS test, thus each platform can test it.
func resolveAndroidEditName(name, baseName string, isPage bool) string {
	if isPage {
		return baseName + ".md"
	}
	return name
}

func (a *App) handleEditExternal(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)
	filePath := htmlPath
	if isPage {
		filePath = mdPath
	} else {
		// This route applies the two rules of serveEditor, because a client
		// can call it directly. First: no editor for a file that is not text.
		if !a.editableFileType(name) {
			a.serveNotEditable(w, r, name)
			return
		}
		// Second: put a shipped html/ asset on disk, or the external editor
		// opens a path with no file.
		a.materializeAsset("/" + filepath.ToSlash(name))
	}

	if runtime.GOOS == "android" {
		editName := resolveAndroidEditName(name, baseName, isPage)
		w.Header().Set("Location", "omngo://edit?name="+url.QueryEscape(editName))
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	var cmd *exec.Cmd
	cmdStr := strings.TrimSpace(a.GetConfig().DesktopExtCmd)

	if cmdStr == "" {
		switch runtime.GOOS {
		case "linux":
			cmd = exec.Command("xdg-open", filePath)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
		case "darwin":
			cmd = exec.Command("open", filePath)
		}
	} else {
		parts := strings.Fields(cmdStr)
		if len(parts) > 0 {
			args := append(parts[1:], filePath)
			cmd = exec.Command(parts[0], args...)
		}
	}

	if cmd != nil {
		err := cmd.Start()
		if err != nil {
			a.logErrf(logEdit, "Failed to run external editor: %v", err)
		}
	} else {
		a.logErrf(logEdit, "Failed to run external editor: no command configured")
	}

	writeHTMLHeader(w)
	viewURL := name
	if isPage {
		viewURL = baseName + ".html"
	}
	waitBody := a.getExternalEditPageBody(name, viewURL)
	compiledWait := a.compilePageWithBody(name, fmt.Appendf(nil, "Title: Refresh %s\nDate: %s\nCategory: Action\n\n", name, time.Now().Format("2006-01-02 15:04:05")), waitBody)
	w.Write(a.injectRuntimeVars(compiledWait))
}

// resolveNewPageTarget resolves a new page name the same way as a link on the
// source page. A bare name is a sibling of source, a leading "/" is absolute,
// and a target with a directory stays. It first trims a stray slash at each
// end of source.
func (a *App) resolveNewPageTarget(source, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return target
	}

	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}

	if strings.Contains(target, "/") {
		return path.Clean(target)
	}

	source = strings.Trim(strings.TrimSpace(source), "/")
	if source == "" {
		return target
	}

	dir := path.Dir(source)
	if dir == "." {
		return target
	}
	return dir + "/" + target
}

// handleLogin changes a password into the two session cookies. Only a caller
// on the network needs it. The comparison is constant-time, and an EMPTY
// configured password matches nothing.
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	cfg := a.GetConfig()
	pwd := r.FormValue("password")

	role := ""
	switch {
	case passwordMatches(pwd, cfg.AdminPassword):
		role = roleAdmin
	case passwordMatches(pwd, cfg.GuestPassword):
		role = roleGuest
	}
	if role == "" {
		if cfg.AdminPassword == "" && cfg.GuestPassword == "" {
			a.logErrf(logSession, "login refused: config.json holds no password, thus no caller on the network can log in")
		}
		http.Error(w, "Invalid", http.StatusUnauthorized)
		return
	}

	signed, hint := a.newSessionCookies(role)
	if signed == nil {
		// The install has no key. See sessionSecret. An unsigned cookie is
		// not an option.
		a.logErrf(logSession, "login refused: this install has no session key")
		http.Error(w, "Login unavailable", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, signed)
	http.SetCookie(w, hint)
	w.Write([]byte("OK"))
}

func passwordMatches(submitted, configured string) bool {
	if configured == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(submitted), []byte(configured)) == 1
}

func (a *App) handleQuickNote(w http.ResponseWriter, r *http.Request) {
	note := r.FormValue("note")
	if note == "" {
		return
	}
	path := filepath.Join(a.StorageDir, "md", "QuickNotes.md")
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")

	insertIdx := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" { // Find first blank line ending Pelican header
			insertIdx = i + 1
			break
		}
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("\n---\n##### %s\n%s\n", timestamp, note)

	newContent := append(lines[:insertIdx], append([]string{entry}, lines[insertIdx:]...)...)
	fullMarkdown := strings.Join(newContent, "\n")
	fullMarkdown = a.ensureHeaderModified(fullMarkdown, "Quick Notes")
	os.WriteFile(path, []byte(fullMarkdown), 0644)

	// renderAndCache is the only writer of html/*.html. See render_cache.go.
	if _, err := a.renderAndCache("QuickNotes", []byte(fullMarkdown)); err != nil {
		a.logErrf(logPage, "handleQuickNote: %v", err)
	}

	w.Write([]byte("Saved"))
}

func (a *App) handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")

	path := filepath.Join(a.StorageDir, "md", "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	tagsList := []string{}
	for t := range strings.SplitSeq(tags, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tagsList = append(tagsList, trimmed)
		}
	}
	// A semicolon separates notes, and a comma separates tags, the same as in
	// OMN. json.MarshalIndent below escapes '<', '>' and '&', thus a note
	// cannot close the <script> of Bookmarks.md.
	notesList := []string{}
	for n := range strings.SplitSeq(notes, ";") {
		if trimmed := strings.TrimSpace(n); trimmed != "" {
			notesList = append(notesList, trimmed)
		}
	}

	type BM struct {
		Date  string   `json:"date"`
		Url   string   `json:"url"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
		Notes []string `json:"notes"`
	}
	bm := BM{Date: timestamp, Url: url, Title: title, Tags: tagsList, Notes: notesList}
	bmJson, _ := json.MarshalIndent(bm, "  ", "  ")
	entry := "  " + string(bmJson) + ",\n"

	data, err := os.ReadFile(path)
	if err == nil {
		content := string(data)
		marker := "<!-- Don't edit body below this line -->"
		if strings.Contains(content, marker) {
			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			newContent = a.ensureHeaderModified(newContent, "Incoming bookmarks")
			os.WriteFile(path, []byte(newContent), 0644)
			if _, err := a.renderAndCache("Bookmarks", []byte(newContent)); err != nil {
				a.logErrf(logPage, "handleBookmark: %v", err)
			}
		}
	}
	w.Write([]byte("Saved"))
}

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

func (a *App) handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	if !isPage {
		// The editor reads its text here. Refuse each file that serveEditor
		// refuses, because a save through a textarea damages binary content.
		if !a.editableFileType(name) {
			a.serveNotEditable(w, r, name)
			return
		}
		data, err := os.ReadFile(htmlPath)
		if err != nil {
			// The file is not on disk yet. Extract it first. A 404 would open
			// an EMPTY editor, and the first Save would erase the shipped
			// content.
			if physPath, ok := a.materializeAsset("/" + strings.TrimPrefix(filepath.ToSlash(name), "/")); ok {
				data, err = os.ReadFile(physPath)
			}
		}
		if err != nil {
			// The file is not on disk and not embedded. Answer a 404 in plain
			// text. The editor shows it, and a save then makes the file. The
			// User Manual documents that use.
			a.serveNotFound(w, r)
			return
		}
		w.Write(data)
		return
	}

	data, err := os.ReadFile(mdPath)
	if err == nil {
		w.Write(data)
		return
	}

	// Not on disk yet. Use the embedded default or an empty page, and write
	// it, thus this runs one time for each page. Log a failed write. The data
	// in memory is still correct.
	embedPath := "frontend/md/" + baseName + ".md"
	if embedData, embedErr := staticFS.ReadFile(embedPath); embedErr == nil {
		data = embedData
	} else {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		authorLine := ""
		if a.GetConfig().Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", a.GetConfig().Author)
		}
		data = []byte(fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", baseName, timestamp, authorLine))
	}

	if mkErr := os.MkdirAll(filepath.Dir(mdPath), 0755); mkErr != nil {
		a.logErrf(logPage, "handleGetNote: failed to create directory for %q: %v", baseName, mkErr)
	} else if writeErr := os.WriteFile(mdPath, data, 0644); writeErr != nil {
		a.logErrf(logPage, "handleGetNote: failed to persist new page %q: %v", baseName, writeErr)
	}

	w.Write(data)
}

func (a *App) handleNewPage(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("source")
	target := r.FormValue("target")
	title := r.FormValue("title")

	if target == "" || title == "" {
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	// A bare target resolves relative to the directory of source, the same as
	// a bare relative link on that page. "test" from "local/local" is
	// "local/test".
	rawTarget := target
	// containedName keeps each name in the md directory. See paths.go.
	target = containedName(a.resolveNewPageTarget(source, target))
	source = containedName(source)

	now := time.Now().Format("2006-01-02 15:04:05")

	targetMdPath := filepath.Join(a.StorageDir, "md", target+".md")
	if _, err := os.Stat(targetMdPath); os.IsNotExist(err) {
		authorLine := ""
		if a.GetConfig().Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", a.GetConfig().Author)
		}
		defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nModified: %s\nCategory: Notes%s\n\n", title, now, now, authorLine)
		os.MkdirAll(filepath.Dir(targetMdPath), 0755)
		os.WriteFile(targetMdPath, []byte(defaultContent), 0644)
	}

	if source != "" {
		sourceMdPath := filepath.Join(a.StorageDir, "md", source+".md")
		sourceData, err := os.ReadFile(sourceMdPath)
		if err == nil {
			content := string(sourceData)

			// The link in source must resolve to the new page. The browser
			// resolves a bare name relative to source, thus a bare rawTarget
			// stays as it is. A rawTarget with its own directory, or an
			// absolute one, gets a leading "/".
			linkHref := strings.TrimSpace(rawTarget)
			if strings.Contains(linkHref, "/") {
				linkHref = "/" + target
			}
			linkStr := fmt.Sprintf("* [%s](%s.html)", title, linkHref)

			// The new link goes below the header block, or at the top of a
			// note with no header. See header_block.go.
			hb := parseHeaderBlock(content)
			if hb.HasHeader {
				if hb.Body != "" {
					content = hb.Header + "\n\n" + linkStr + "\n" + hb.Body
				} else {
					content = hb.Header + "\n\n" + linkStr + "\n"
				}
			} else {
				content = linkStr + "\n\n" + content
			}

			content = a.ensureHeaderModified(content, source)
			os.WriteFile(sourceMdPath, []byte(content), 0644)

			if _, err := a.renderAndCache(source, []byte(content)); err != nil {
				a.logErrf(logPage, "handleNewPage: %v", err)
			}
		}
	}

	w.Write([]byte(target))
}

func (a *App) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")

	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)

	if !isPage {
		// The last of the four guards, and the one that matters, because
		// this is the write. A crafted request can pass each guard above it.
		if !a.editableFileType(name) {
			a.serveNotEditable(w, r, name)
			return
		}
		if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
			a.logErrf(logPage, "handleSaveNote: mkdir failed for %q: %v", name, err)
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(htmlPath, []byte(content), 0644); err != nil {
			a.logErrf(logPage, "handleSaveNote: write failed for %q: %v", name, err)
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}
		// A text file beside a note also goes back to md/. Git sync carries
		// the md/ copy, and it must not go stale. See note_files.go.
		a.syncNoteFileToMD(htmlPath)
		w.Write([]byte("Saved"))
		return
	}

	content = a.ensureHeaderModified(content, baseName)

	// Write the markdown source first. When the write fails, answer with a
	// failure. Do not compile content that did not reach the disk.
	if err := os.MkdirAll(filepath.Dir(mdPath), 0755); err != nil {
		a.logErrf(logPage, "handleSaveNote: mkdir failed for %q: %v", baseName, err)
		http.Error(w, "Failed to save", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(mdPath, []byte(content), 0644); err != nil {
		a.logErrf(logPage, "handleSaveNote: write failed for %q: %v", baseName, err)
		http.Error(w, "Failed to save", http.StatusInternalServerError)
		return
	}

	// The HTML is a derived cache. When its write fails, the note is safe,
	// and serveHTMLPage compiles it again at the next view because the .md is
	// newer. The save is not a failure.
	if _, err := a.renderAndCache(baseName, []byte(content)); err != nil {
		a.logErrf(logPage, "handleSaveNote: %v", err)
	}

	w.Write([]byte("Saved"))
}

func (a *App) serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" || path == "/index.html" {
		http.Redirect(w, r, "/Welcome.html", http.StatusSeeOther)
		return
	}

	// Edit intent comes first, for a page and for an asset. One editor page
	// handles each editable file.
	if r.URL.Query().Get("edit") == "true" {
		a.serveEditor(w, r, path)
		return
	}

	if strings.HasSuffix(path, ".html") {
		a.serveHTMLPage(w, r, path)
		return
	}

	a.serveStaticAsset(w, r, path)
}

func (a *App) serveHTMLPage(w http.ResponseWriter, r *http.Request, path string) {
	// requested keeps ".html" for resolvePageName, which reads the LAST
	// extension: "Draft.txt.html" is the note md/Draft.txt.md, and
	// "Draft.txt" is a file. Do not strip ".md", because "Welcome.md" is a
	// valid note name.
	requested := strings.TrimPrefix(path, "/")
	name := strings.TrimSuffix(requested, ".html")

	if name == "Config" {
		a.serveConfigPage(w)
		return
	}

	// The Tags index is stale against ALL notes, and not against its own .md.
	// See serveTagsPage.
	if name == "OMNGoTags" {
		a.serveTagsPage(w, r)
		return
	}

	// The Search page is dynamic. It has no .md and no cache. See
	// serveSearchPage.
	if name == "OMNGoSearch" {
		a.serveSearchPage(w, r)
		return
	}

	mdPath, htmlPath, name, _ := a.resolvePageName(requested)

	htmlStat, errHtml := os.Stat(htmlPath)
	mdStat, errMd := os.Stat(mdPath)

	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
		a.recompileMarkdownPage(name, mdPath, errMd)
	}

	writeHTMLHeader(w)
	data, err := os.ReadFile(htmlPath)
	if err == nil {
		w.Write(a.injectRuntimeVars(data))
	} else {
		http.ServeFile(w, r, htmlPath)
	}
}

func (a *App) recompileMarkdownPage(name, mdPath string, errMd error) {
	if os.IsNotExist(errMd) {
		embedData, err := staticFS.ReadFile("frontend/md/" + name + ".md")
		if err == nil {
			os.MkdirAll(filepath.Dir(mdPath), 0755)
			os.WriteFile(mdPath, embedData, 0644)
		} else {
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			authorLine := ""
			if a.GetConfig().Author != "" {
				authorLine = fmt.Sprintf("\nAuthor: %s", a.GetConfig().Author)
			}
			defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n", name, timestamp, authorLine)
			os.MkdirAll(filepath.Dir(mdPath), 0755)
			os.WriteFile(mdPath, []byte(defaultContent), 0644)
		}
	}

	mdContent, err := os.ReadFile(mdPath)
	if err == nil {
		// Rebuild the cache from the .md on disk. This runs on a plain VIEW,
		// thus it must not rewrite the .md. ensureHeaderModified belongs to
		// handleSaveNote alone.
		if _, err := a.renderAndCache(name, mdContent); err != nil {
			a.logErrf(logPrecompile, "recompileMarkdownPage: %v", err)
		}
	}
}

func (a *App) serveConfigPage(w http.ResponseWriter) {
	writeHTMLHeader(w)
	body := a.getConfigPageBody()
	compiled := a.compilePageWithBody("Config", []byte("Title: Config\nCategory: Settings\n\n"), body)
	w.Write(a.injectRuntimeVars(compiled))
}

// serveEditor handles each ?edit=true request: the internal editor page, or
// the external editor when the internal one is off.
func (a *App) serveEditor(w http.ResponseWriter, r *http.Request, path string) {
	relPath := strings.TrimPrefix(path, "/")

	if _, _, _, isPage := a.resolvePageName(relPath); !isPage {
		// No editor for a picture, a font, an audio file or a video file.
		// Refuse before the extraction below. A page skips this test: its
		// edit opens the markdown source.
		if !a.editableFileType(relPath) {
			a.serveNotEditable(w, r, relPath)
			return
		}
		// Put a shipped html/ asset on disk. The external editor and the
		// Android intent open the file path directly, and a missing file
		// gives them nothing to show.
		a.materializeAsset("/" + filepath.ToSlash(relPath))
	}

	if !a.GetConfig().UseInternalEd {
		http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
		return
	}

	a.renderInternalEditor(w, relPath)
}

// renderInternalEditor writes the editor page for relPath. The page fetches
// the text from /api/note, thus no view page carries a second copy.
func (a *App) renderInternalEditor(w http.ResponseWriter, relPath string) {
	_, _, baseName, isPage := a.resolvePageName(relPath)

	// name goes to /api/note and /api/save. A page sends baseName + ".md",
	// because both endpoints read the LAST extension.
	name := relPath
	viewURL := "/" + relPath
	pageExt := filepath.Ext(relPath)
	title := relPath
	if isPage {
		name = baseName + ".md"
		viewURL = "/" + baseName + ".html"
		pageExt = ".md"
		title = baseName
	}

	page := renderEditorPage(editorPageView{
		Title:   title,
		Name:    name,
		PageExt: pageExt,
		ViewURL: viewURL,
	})

	writeHTMLHeader(w)
	w.Write(a.injectRuntimeVars([]byte(page)))
}

// serveStaticAsset is the root catch-all for an embedded asset outside /js,
// /css and /json, for example favicon.ico. It shares serveEmbeddableAsset
// with those trees.
func (a *App) serveStaticAsset(w http.ResponseWriter, r *http.Request, path string) {
	a.serveEmbeddableAsset(w, r, path)
}
