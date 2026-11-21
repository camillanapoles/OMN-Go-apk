package repocheck

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// handleUpload accepts one list of image extensions and ShareIn.java keeps a
// copy of the list for its share path. The two sides live in different
// languages and the compiler links nothing. A drift between them lets one
// side accept a file that the other side drops. This test pins both lists.
func TestUploadExtensionListsStayInSync(t *testing.T) {
	const goPath = "backend/internal/app/upload_handlers.go"
	const javaPath = "android/app/src/main/java/net/basov/omngo/ShareIn.java"
	goSrc, err := readRepoFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	javaSrc, err := readRepoFile(javaPath)
	if err != nil {
		t.Fatal(err)
	}
	goList := parseStringList(goSrc, `imageUploadExtensions = \[\]string\{([^}]*)\}`)
	javaList := parseStringList(javaSrc, `SHARED_IMAGE_EXT[\s\S]*?Arrays\.asList\(([^)]*)\)`)
	if len(goList) == 0 {
		t.Fatalf("%s: the parser found no imageUploadExtensions list.", goPath)
	}
	if len(javaList) == 0 {
		t.Fatalf("%s: the parser found no SHARED_IMAGE_EXT list.", javaPath)
	}
	sort.Strings(goList)
	sort.Strings(javaList)
	if strings.Join(goList, ",") != strings.Join(javaList, ",") {
		t.Errorf("the upload lists differ.\n%s has %v\n%s has %v\nKeep the copies the same.",
			goPath, goList, javaPath, javaList)
	}
}

// parseStringList finds the regex group and splits its quoted strings.
func parseStringList(src, pattern string) []string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return nil
	}
	var out []string
	for _, part := range strings.Split(m[1], ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
