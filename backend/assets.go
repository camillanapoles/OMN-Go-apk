package backend

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"net.basov.omngo/backend/frontend"
)

// ----------------------------------------------------------------------
// The refresh of the embedded files at each new version
// ----------------------------------------------------------------------
//
// The binary embeds frontend/html and the starter notes of frontend/md. A
// file reaches StorageDir in one of two ways:
//
//	- A USER file comes from the embed ONLY when it is absent.
//	  materializeAsset in serving.go extracts an html/ file at the first
//	  request, and the start extracts the starter notes one time. After
//	  that, the copy belongs to the user, for example md/Welcome.md.
//	- A VERSION-DEPENDENT file (versionDependentAssets) must match the
//	  running build: the app scripts, the app styles and the system notes.
//
// A lazy extract alone keeps the old copy of a version-dependent file after
// an upgrade. A new note in a new release would also never appear. At each
// change of APP_VERSION, refreshEmbeddedAssets writes the embedded copy of
// each listed file. It first moves a copy on disk that differs to
// asset_backups/<previous-version>/, thus nothing is lost. See
// doc/decisions/0006-replace-the-application-files-at-each-new-version.md.

// assetsVersionFilename holds the APP_VERSION of the last refresh. It is in
// StorageDir beside config.json, and NOT under html/, where the server would
// send it and the sync would carry it.
const assetsVersionFilename = "assets_version"

// assetsRefreshed tells whether refreshEmbeddedAssets wrote a file in this
// process. The Android WebView keeps scripts and styles in its own disk
// cache, thus new pages can use old scripts after an update. The Android
// layer reads this value through AssetsRefreshed.
var assetsRefreshed atomic.Bool

// AssetsRefreshed tells whether this start installed or replaced a
// version-dependent file. gomobile exports it. MainActivity.java calls it
// before the first loadUrl, and on true it calls WebView.clearCache(true) one
// time. A start with no change keeps the cache.
func AssetsRefreshed() bool {
	return assetsRefreshed.Load()
}

// backupLabelSanitizer keeps a backup directory name safe, whatever an old
// version stamp holds.
var backupLabelSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// versionDependentAssets lists the StorageDir-relative files that ship with
// OMN-Go and must match the running build. The embedded source of each is
// "frontend/" plus the path. Each file that is NOT here belongs to the user,
// and a version change leaves it alone.
//
// EACH FILE BELOW html/ IS IN AN OMN-Go DIRECTORY.
// TestEveryAppAssetIsUnderOMNGo holds that rule. The two user files stay at
// html/js/omn-go-custom.js and html/css/omn-go-custom.css. legacyAssetURL in
// serving.go answers a request for an old path from the new place. See
// doc/decisions/0007-keep-the-application-files-in-omn-go-directories.md.
var versionDependentAssets = []string{
	"html/js/OMN-Go/omn-go-compat.js",
	"html/js/OMN-Go/omn-go-core.js",
	"html/js/OMN-Go/omn-go-editor.js",
	"html/js/OMN-Go/omn-go-sse.js",
	"html/js/OMN-Go/omn-go-config.js",
	"html/js/OMN-Go/omn-go-sync.js",
	"html/js/OMN-Go/omn-go-bookmark.js",
	"html/js/OMN-Go/omn-go-search.js",
	"html/js/OMN-Go/omn-go-logs.js",
	"html/js/OMN-Go/omn-go-status.js",
	"html/js/OMN-Go/Bookmarker.js",
	"html/js/OMN-Go/auto-render.min.js",
	"html/js/OMN-Go/katex.min.js",
	"html/js/OMN-Go/highlight.min.js",
	"html/css/OMN-Go/omn-go-core.css",
	"html/css/OMN-Go/Bookmarker.css",
	"html/css/OMN-Go/omn-go-logs.css",
	"html/css/OMN-Go/omn-go-status.css",
	"html/css/OMN-Go/highlight.default.min.css",
	"html/css/OMN-Go/katex.min.css",
	"md/AndroidIntents.md",
	"md/BookmarksHowTo.md",
	"md/Database.md",
	"md/Editor.md",
	"md/ScriptRules.md",
	"md/SQLImport.md",
	"md/UserManual.md",
}

// retiredAssets lists the StorageDir-relative paths that this build does not
// own any more. They are the old place of each moved file, and
// html/css/markdown.css, which no page loaded. THE LIST ONLY GROWS, because
// an install can skip versions.
//
// removeRetiredAssets deletes each copy on disk. gitignorePatterns does not
// name these paths, thus a copy that stays would become a TRACKED file at the
// next commit.
var retiredAssets = []string{
	"html/js/omn-go-compat.js",
	"html/js/omn-go-core.js",
	"html/js/omn-go-editor.js",
	"html/js/omn-go-sse.js",
	"html/js/Bookmarker.js",
	"html/js/auto-render.min.js",
	"html/js/katex.min.js",
	"html/js/highlight.min.js",
	"html/css/omn-go-core.css",
	"html/css/Bookmarker.css",
	"html/css/highlight.default.min.css",
	"html/css/katex.min.css",
	"html/css/markdown.css",
}

// retiredFonts adds the old place of each web font to retiredAssets. A font
// is not version-dependent: materializeAsset writes it when a page asks for
// it. The list comes from the embedded tree, thus a new font needs no change
// here.
var retiredFonts = func() []string {
	entries, err := frontend.Static.ReadDir("html/css/OMN-Go/fonts")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, "html/css/fonts/"+e.Name())
		}
	}
	return out
}()

// retiredAssetDirs lists the directories that the removal can leave empty.
// os.Remove refuses a directory that still holds a file of the user.
var retiredAssetDirs = []string{
	"html/css/fonts",
}

// removeRetiredAssets deletes each path of retiredAssets. A copy that differs
// from the shipped bytes goes to asset_backups/<previous>/ first, thus the
// work of a person stays. It runs one time for each version change, under the
// version stamp of refreshEmbeddedAssets.
func (a *App) removeRetiredAssets(backupDir string) int {
	removed := 0
	for _, rel := range append(append([]string(nil), retiredAssets...), retiredFonts...) {
		diskPath := a.layout().file(filepath.FromSlash(rel))
		diskData, rerr := os.ReadFile(diskPath)
		if rerr != nil {
			continue // absent, which is the normal state after the first run
		}

		// An unchanged old copy holds the bytes that this build ships at the
		// NEW place. A copy that differs is the work of a person.
		if !bytes.Equal(diskData, embeddedTwinOf(rel)) {
			backupPath := filepath.Join(backupDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err == nil {
				if err := os.WriteFile(backupPath, diskData, 0644); err == nil {
					a.log(logAssets).infof("kept your copy of %s at %s", rel, backupPath)
				} else {
					a.log(logAssets).errf("cannot back up %s: %v", rel, err)
					continue // do not delete work that has no copy
				}
			} else {
				a.log(logAssets).errf("cannot make the backup directory for %s: %v", rel, err)
				continue
			}
		}

		if err := os.Remove(diskPath); err != nil {
			a.log(logAssets).errf("cannot remove the old %s: %v", rel, err)
			continue
		}
		a.log(logAssets).infof("removed the old %s", rel)
		removed++
	}

	for _, rel := range retiredAssetDirs {
		dirPath := a.layout().file(filepath.FromSlash(rel))
		if err := os.Remove(dirPath); err == nil {
			a.log(logAssets).infof("removed the empty directory %s", rel)
		}
	}
	return removed
}

// embeddedTwinOf answers the bytes that this build ships for the file that
// was at rel: the same name below OMN-Go/. It answers nil for a dropped file,
// thus each copy of such a file goes to the backup.
func embeddedTwinOf(rel string) []byte {
	dir, name := path.Split(rel)
	data, err := frontend.Static.ReadFile(dir + "OMN-Go/" + name)
	if err != nil {
		return nil
	}
	return data
}

func (a *App) refreshEmbeddedAssets() {
	// The flag reports the work of this start only.
	assetsRefreshed.Store(false)

	verFile := a.layout().file(assetsVersionFilename)
	prevRaw, _ := os.ReadFile(verFile) // missing file => "" => first run
	prev := strings.TrimSpace(string(prevRaw))
	if prev == APP_VERSION {
		return
	}

	prevLabel := prev
	if prevLabel == "" {
		// This is an install with no version stamp, or with a wiped one.
		prevLabel = "unknown"
	}
	prevLabel = backupLabelSanitizer.ReplaceAllString(prevLabel, "_")

	backupDir := a.layout().assetBackups(prevLabel)

	// Delete the old copies BEFORE the install loop. A reader of the storage
	// directory must never see two copies of one script. See retiredAssets.
	refreshed := a.removeRetiredAssets(backupDir)

	for _, rel := range versionDependentAssets {
		embedData, eerr := frontend.Static.ReadFile(rel)
		if eerr != nil {
			// The list names the file, but this build does not embed it.
			a.log(logAssets).errf("%s not embedded in this build: %v", rel, eerr)
			continue
		}
		diskPath := a.layout().file(filepath.FromSlash(rel))

		diskData, rerr := os.ReadFile(diskPath)
		if rerr == nil && bytes.Equal(diskData, embedData) {
			continue // already current - nothing to do
		}
		if rerr != nil && !os.IsNotExist(rerr) {
			a.log(logAssets).errf("cannot read %s: %v", diskPath, rerr)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
			a.log(logAssets).errf("skip %s: cannot create dir: %v", rel, err)
			continue
		}

		// Save a copy on disk that differs before the loop overwrites it: it
		// is an older extract or an edit of the user. Never write without a
		// backup. A MISSING file needs no backup.
		existed := rerr == nil
		if existed {
			bakPath := filepath.Join(backupDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(bakPath), 0755); err != nil {
				a.log(logAssets).errf("skip %s: cannot create backup dir: %v", rel, err)
				continue
			}
			if err := os.WriteFile(bakPath, diskData, 0644); err != nil {
				a.log(logAssets).errf("skip %s: backup failed: %v", rel, err)
				continue
			}
		}

		if err := os.WriteFile(diskPath, embedData, 0644); err != nil {
			a.log(logAssets).errf("write of %s failed: %v", rel, err)
			continue
		}
		refreshed++
		if existed {
			a.log(logAssets).infof("refreshed %s (previous copy saved to asset_backups/%s/%s)", rel, prevLabel, rel)
		} else {
			a.log(logAssets).infof("installed %s from this build", rel)
		}
	}

	// Write the stamp AFTER the loop. When the process stops during a
	// refresh, the next start runs it again, and the loop skips an equal
	// file.
	if err := os.WriteFile(verFile, []byte(APP_VERSION+"\n"), 0644); err != nil {
		a.log(logAssets).errf("cannot write version stamp %s: %v", verFile, err)
	}
	if refreshed > 0 {
		// AssetsRefreshed tells the Android layer to clear the WebView cache
		// one time.
		assetsRefreshed.Store(true)
		a.log(logAssets).infof("%d embedded asset(s) refreshed for v%s (previous: %s)", refreshed, APP_VERSION, prevLabel)
	}
}
