package backend

// ----------------------------------------------------------------------
// The three build pipelines must agree
// ----------------------------------------------------------------------
//
// OMN-Go has three builds. The local build uses Dockerfile.base and
// Dockerfile. GitHub and GitLab use Dockerfile.ci. F-Droid uses its own
// recipe, and metadata/net.basov.omngo.fdroid.yml holds a copy of it.
//
// The maintainer keeps the three builds as similar as possible. The
// F-Droid recipe is hard to change. The tests below read the files and
// compare three values: the Android API level, the Go version and the NDK.
// A difference in the Android API level can make an APK that installs on
// Android 6 and then fails to load its library.
//
// The tests read the LAST version in the F-Droid file. An older build
// block keeps the values of its own release. When the recipe on the
// F-Droid server changes, copy it into the repository. These tests then
// show each difference.

import (
	"regexp"
	"strings"
	"testing"
)

var (
	pipeMinSdkRe     = regexp.MustCompile(`(?m)^\s*minSdk\s+(\d+)\s*$`)
	pipeNdkGradleRe  = regexp.MustCompile(`(?m)^\s*ndkVersion\s+"([^"]+)"`)
	pipeAndroidAPIRe = regexp.MustCompile(`-androidapi\s+(\d+)`)
	pipeGoImageRe    = regexp.MustCompile(`(?m)^FROM golang:([0-9.]+)-bookworm\b`)
	pipeNdkDockerRe  = regexp.MustCompile(`"ndk;([^"]+)"`)
	pipeGoSrclibRe   = regexp.MustCompile(`go@go([0-9.]+)`)
	pipeNdkRecipeRe  = regexp.MustCompile(`(?m)^\s*ndk:\s*(\S+)`)
)

// pipeRead reads one file of the repository, or fails the test.
func pipeRead(t *testing.T, rel string) string {
	t.Helper()
	text, err := readRepoFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return text
}

// pipeAll answers the first group of each match of re in text.
func pipeAll(re *regexp.Regexp, text string) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

// pipeLastRecipeBlocks answers the build blocks of the last versionName
// in the F-Droid file. Each ABI has its own block, thus there are
// several blocks for one version.
func pipeLastRecipeBlocks(t *testing.T) []string {
	t.Helper()
	recipe := pipeRead(t, "metadata/net.basov.omngo.fdroid.yml")
	builds := recipe
	if i := strings.Index(recipe, "\nBuilds:"); i >= 0 {
		builds = recipe[i:]
	}
	if i := strings.Index(builds, "\nAutoUpdateMode:"); i >= 0 {
		builds = builds[:i]
	}
	var blocks []string
	for _, part := range strings.Split(builds, "\n  - versionName:")[1:] {
		blocks = append(blocks, "versionName:"+part)
	}
	if len(blocks) == 0 {
		t.Fatal("the F-Droid file holds no build block")
	}
	name := func(b string) string {
		return strings.TrimSpace(strings.SplitN(strings.TrimPrefix(b, "versionName:"), "\n", 2)[0])
	}
	last := name(blocks[len(blocks)-1])
	var out []string
	for _, b := range blocks {
		if name(b) == last {
			out = append(out, b)
		}
	}
	return out
}

// pipeSame fails the test when the values differ from want.
func pipeSame(t *testing.T, what, where, want string, got []string) {
	t.Helper()
	if len(got) == 0 {
		t.Errorf("%s: %s names no value, want %s", what, where, want)
	}
	for _, g := range got {
		if g != want {
			t.Errorf("%s: %s says %s, want %s", what, where, g, want)
		}
	}
}

// The Android API level of the native library must be minSdk. The
// library of a higher level can fail to load on the lowest Android that
// installs the APK. minSdk 23 is Android 6.
func TestPipelinesAgreeOnTheAndroidAPILevel(t *testing.T) {
	gradle := pipeRead(t, "android/app/build.gradle")
	m := pipeMinSdkRe.FindStringSubmatch(gradle)
	if m == nil {
		t.Fatal("android/app/build.gradle names no minSdk")
	}
	want := m[1]
	for _, f := range []string{"Dockerfile", "Dockerfile.ci"} {
		pipeSame(t, "-androidapi", f, want, pipeAll(pipeAndroidAPIRe, pipeRead(t, f)))
	}
	for _, b := range pipeLastRecipeBlocks(t) {
		pipeSame(t, "-androidapi", "the last F-Droid version", want, pipeAll(pipeAndroidAPIRe, b))
	}
}

// The Docker image and the F-Droid srclib must name the same Go version.
// A second Go version can build a different binary from the same source.
func TestPipelinesAgreeOnTheGoVersion(t *testing.T) {
	var got []string
	for _, f := range []string{"Dockerfile.base", "Dockerfile.ci"} {
		got = append(got, pipeAll(pipeGoImageRe, pipeRead(t, f))...)
	}
	if len(got) == 0 {
		t.Fatal("no Docker file names a golang image")
	}
	want := got[0]
	pipeSame(t, "the Go version", "the Docker files", want, got)
	for _, b := range pipeLastRecipeBlocks(t) {
		pipeSame(t, "the Go version", "the last F-Droid version", want, pipeAll(pipeGoSrclibRe, b))
	}
}

// The NDK of the Gradle file, of the Docker images and of the F-Droid
// recipe must be the same. The NDK builds the native library.
func TestPipelinesAgreeOnTheNDK(t *testing.T) {
	m := pipeNdkGradleRe.FindStringSubmatch(pipeRead(t, "android/app/build.gradle"))
	if m == nil {
		t.Fatal("android/app/build.gradle names no ndkVersion")
	}
	want := m[1]
	for _, f := range []string{"Dockerfile.base", "Dockerfile.ci"} {
		pipeSame(t, "the NDK", f, want, pipeAll(pipeNdkDockerRe, pipeRead(t, f)))
	}
	for _, b := range pipeLastRecipeBlocks(t) {
		pipeSame(t, "the NDK", "the last F-Droid version", want, pipeAll(pipeNdkRecipeRe, b))
	}
}
