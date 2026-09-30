package app

import (
	"os"
	"path/filepath"
	"testing"

	"net.basov.omngo/backend/internal/storage"
)

// AssetsRefreshed drives the one-time clear of the WebView cache on
// Android (MainActivity.java). It must answer true only for a start that
// wrote a file. A start with no change must keep the cache, because a
// clear makes each page of the application load again from the server.
func TestAssetsRefreshedReportsOnlyAStartThatWroteAFile(t *testing.T) {
	dir := t.TempDir()
	a := &App{StorageDir: dir}

	// A first start installs each version-dependent asset.
	a.refreshEmbeddedAssets()
	if !AssetsRefreshed() {
		t.Error("a first start installs the shipped assets, thus the flag must be true")
	}

	// The stamp is now the app version. The next start does no work.
	a.refreshEmbeddedAssets()
	if AssetsRefreshed() {
		t.Error("a start that changed no file must report false")
	}

	// A version change alone is not sufficient. Each file on disk is already
	// the content of this build. Nothing is written, and the cache stays.
	if err := os.WriteFile(filepath.Join(dir, storage.AssetsVersionFilename), []byte("0.0.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.refreshEmbeddedAssets()
	if AssetsRefreshed() {
		t.Error("a version change that wrote no file must report false")
	}

	// A version change that finds a different copy on disk writes the
	// file again, thus the flag must be true.
	target := filepath.Join(dir, filepath.FromSlash("html/js/OMN-Go/omn-go-core.js"))
	if err := os.WriteFile(target, []byte("// older extract\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, storage.AssetsVersionFilename), []byte("0.0.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.refreshEmbeddedAssets()
	if !AssetsRefreshed() {
		t.Error("a start that replaced an asset must report true")
	}
}
