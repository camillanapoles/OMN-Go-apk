package repocheck

// ----------------------------------------------------------------------
// The three copies of the table of user files
// ----------------------------------------------------------------------
//
// config.UserFileTrees is the authority. The editor and the Android share
// path cannot read Go, thus each holds a copy: USER_FILE_UPLOADS in
// omn-go-editor.js and USER_FILE_TREES in OmnText.java.
//
// A copy that differs is a silent fault. The editor sends a dropped
// calendar to the image upload, or the share path sends a contact to a
// route that refuses it. The tests below run the real JavaScript
// and the real Java, and they compare each answer with the Go table.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"net.basov.omngo/backend/internal/config"
)

// The editor must send each extension of each tree to the upload route of
// that tree, and each other file to the image upload.
//
// It skips with no node, the same as TestHeaderPortAgreesWithTheRealJavaScript.
func TestUserFileTreesHaveTheirCopies_JavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on this machine. The build image has one and runs this test.")
	}
	script := `
const { load } = require('./frontend/test/dom-stub.js');
const editor = load('omn-go-editor.js');
process.stdout.write(JSON.stringify(editor.userFileUploads));
`
	cmd := exec.Command(node, "-e", script)
	cmd.Dir = backendDir
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, raw)
	}
	var got []struct {
		URL  string   `json:"url"`
		Exts []string `json:"exts"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("node did not answer the table: %v\n%s", err, raw)
	}
	if len(got) != len(config.UserFileTrees) {
		t.Fatalf("USER_FILE_UPLOADS in omn-go-editor.js has %d row(s), and config.UserFileTrees has %d",
			len(got), len(config.UserFileTrees))
	}
	for i, tree := range config.UserFileTrees {
		if got[i].URL != tree.Upload || !reflect.DeepEqual(got[i].Exts, tree.Exts) {
			t.Errorf("row %d of USER_FILE_UPLOADS is %q %v, and config.UserFileTrees has %q %v",
				i, got[i].URL, got[i].Exts, tree.Upload, tree.Exts)
		}
	}
}

// The Android share path must send each file to the upload route of its
// tree. The server does the rest. See handleUploadUserFile.
//
// The test writes a small class that prints the Java table. It skips with no
// JDK, the same as TestJavaUnitTests.
func TestUserFileTreesHaveTheirCopies_Java(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("no javac on this machine. The Docker gate has a JDK and runs this test.")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("no java on this machine")
	}

	const probe = `package net.basov.omngo;

public final class UserFileProbe {
    public static void main(String[] args) {
        for (OmnText.UserFileTree tree : OmnText.USER_FILE_TREES) {
            System.out.println("tree " + tree.upload + " " + String.join(",", tree.exts));
        }
    }
}
`
	dir := t.TempDir()
	probePath := filepath.Join(dir, "UserFileProbe.java")
	if err := os.WriteFile(probePath, []byte(probe), 0644); err != nil {
		t.Fatal(err)
	}
	args := []string{"-encoding", "UTF-8", "-d", dir, probePath}
	for _, rel := range pureJavaSources {
		args = append(args, filepath.Join(repoRoot, filepath.FromSlash(rel)))
	}
	if compiled, cErr := exec.Command(javac, args...).CombinedOutput(); cErr != nil {
		t.Fatalf("javac failed: %v\n%s", cErr, compiled)
	}
	// Only stdout holds the answer. Some machines write a line about
	// JAVA_TOOL_OPTIONS to stderr.
	raw, err := exec.Command(java, "-cp", dir, "net.basov.omngo.UserFileProbe").Output()
	if err != nil {
		t.Fatalf("the probe failed: %v\n%s", err, raw)
	}

	var trees []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "tree "); ok {
			trees = append(trees, rest)
		}
	}
	var want []string
	for _, tree := range config.UserFileTrees {
		want = append(want, tree.Upload+" "+strings.Join(tree.Exts, ","))
	}
	if !reflect.DeepEqual(trees, want) {
		t.Errorf("USER_FILE_TREES in OmnText.java is %q, and config.UserFileTrees is %q", trees, want)
	}
}
