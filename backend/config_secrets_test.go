package backend

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// The secrets of the Config page
// ----------------------------------------------------------------------
//
// The HTML of the Config page holds no password and no SSH key. An empty
// box means "keep the stored value". These tests hold the two halves of
// that rule: the page carries no secret, and a request that omits a secret
// changes none. See doc/decisions/0005-keep-each-secret-out-of-the-config-page.md.

// secretValues are the values that the tests below plant in the
// configuration. Each one is long and unusual, thus a match in the page
// is a real leak and not a coincidence.
var secretValues = map[string]string{
	"admin":  "ADMIN-SECRET-8f2a1c",
	"sshKey": "-----BEGIN OPENSSH PRIVATE KEY-----KEY-SECRET-11ff22",
	"keyPwd": "KEYPASS-SECRET-73dd10",
}

// secretsApp builds an application whose configuration holds each value
// of secretValues, in the admin password and git slot 0.
func secretsApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	a.WithConfig(func(c *Config) {
		c.AdminPassword = secretValues["admin"]
		c.GitServers = make([]GitServerConfig, maxGitServers)
		c.GitServers[0].Name = "primary"
		c.GitServers[0].URL = "git@host:notes.git"
		c.GitServers[0].SSHKeyData = secretValues["sshKey"]
		c.GitServers[0].Password = secretValues["keyPwd"]
	})
	return a
}

// /Config.html needs no login, thus a caller on the LAN can read the
// source of the page. It must therefore hold no password and no SSH key.
func TestConfigPageCarriesNoSecret(t *testing.T) {
	a := secretsApp(t)
	page := a.getConfigPageBody()

	for name, value := range secretValues {
		if strings.Contains(page, value) {
			t.Errorf("the Config page carries the %s value in its HTML", name)
		}
	}
	// The name and the URL of a slot are not secrets, and the page needs
	// them. A page with no name box would say that the boxes are gone
	// rather than that the secrets are gone.
	for _, want := range []string{"primary", "git@host:notes.git", `name="admin_password"`, `name="git_key_0"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the Config page lost %q", want)
		}
	}
}

// Each secret box carries data-secret. omn-go-sse.js reads that attribute
// to find the boxes that it must remove from a save. A box that loses the
// attribute silently clears its value at the next save.
func TestEverySecretBoxIsMarked(t *testing.T) {
	a := secretsApp(t)
	page := a.getConfigPageBody()

	want := []string{"admin_password"}
	for i := 0; i < maxGitServers; i++ {
		want = append(want, "git_key_"+itoa(i), "git_pass_"+itoa(i))
	}
	for _, name := range want {
		if !strings.Contains(page, `data-secret="`+name+`"`) {
			t.Errorf("the box %q carries no data-secret attribute", name)
		}
	}

	// No secret box may carry a value attribute that is not empty.
	valueRe := regexp.MustCompile(`data-secret="[^"]*"[^>]*value="([^"]+)"`)
	if m := valueRe.FindStringSubmatch(page); m != nil {
		t.Errorf("a secret box carries the value %q", m[1])
	}
}

// A save that changes the name of a slot alone must keep the SSH key and
// the key password. The page does not carry them, thus the request does
// not carry them either.
func TestConfigPostKeepsAnUnsentGitSecret(t *testing.T) {
	a := secretsApp(t)

	// Exactly what the Config page sends after the change: the name and
	// the URL of each slot, and no key and no password.
	form := url.Values{
		"git_name_0": {"renamed"},
		"git_url_0":  {"git@host:notes.git"},
	}
	postForm(t, a.handleConfigPost, "/api/config", form)

	cfg := a.GetConfig()
	if cfg.GitServers[0].Name != "renamed" {
		t.Errorf("the name is %q, want renamed", cfg.GitServers[0].Name)
	}
	if cfg.GitServers[0].SSHKeyData != secretValues["sshKey"] {
		t.Errorf("the SSH key changed to %q", cfg.GitServers[0].SSHKeyData)
	}
	if cfg.GitServers[0].Password != secretValues["keyPwd"] {
		t.Errorf("the key password changed to %q", cfg.GitServers[0].Password)
	}
}

// A reader who empties a revealed box asks for an empty value. The box is
// then dirty, thus omn-go-sse.js sends it, thus the server must write it.
func TestConfigPostClearsASentGitSecret(t *testing.T) {
	a := secretsApp(t)

	postForm(t, a.handleConfigPost, "/api/config", url.Values{
		"git_key_0":  {""},
		"git_pass_0": {""},
	})

	cfg := a.GetConfig()
	if cfg.GitServers[0].SSHKeyData != "" {
		t.Errorf("a sent and empty git_key_0 did not clear the key: %q", cfg.GitServers[0].SSHKeyData)
	}
	if cfg.GitServers[0].Password != "" {
		t.Errorf("a sent and empty git_pass_0 did not clear the password: %q", cfg.GitServers[0].Password)
	}
	// The name and the URL were not sent, thus they stay.
	if cfg.GitServers[0].Name != "primary" {
		t.Errorf("the name changed to %q", cfg.GitServers[0].Name)
	}
}

// A new key reaches the slot, and it reaches that slot alone.
func TestConfigPostWritesANewGitKey(t *testing.T) {
	a := secretsApp(t)
	a.WithConfig(func(c *Config) { c.GitServers[1].SSHKeyData = "SLOT-ONE-KEY" })

	postForm(t, a.handleConfigPost, "/api/config", url.Values{
		"git_key_0": {"A-NEW-KEY"},
	})

	cfg := a.GetConfig()
	if cfg.GitServers[0].SSHKeyData != "A-NEW-KEY" {
		t.Errorf("slot 0 holds %q, want the new key", cfg.GitServers[0].SSHKeyData)
	}
	if cfg.GitServers[1].SSHKeyData != "SLOT-ONE-KEY" {
		t.Errorf("slot 1 changed to %q", cfg.GitServers[1].SSHKeyData)
	}
}

// The same rule covers the two passwords of the Network screen. An
// omitted field keeps the stored password, and a sent and empty field
// clears it. The second half is what a person does to remove a password.
func TestConfigPostPasswordFollowsTheSentRule(t *testing.T) {
	a := secretsApp(t)
	postForm(t, a.handleConfigPost, "/api/config", url.Values{"author": {"Ann"}})
	if got := a.GetConfig().AdminPassword; got != secretValues["admin"] {
		t.Errorf("an omitted admin_password changed to %q", got)
	}

	postForm(t, a.handleConfigPost, "/api/config", url.Values{"admin_password": {""}})
	if got := a.GetConfig().AdminPassword; got != "" {
		t.Errorf("a sent and empty admin_password did not clear it: %q", got)
	}
}

// omn-go-config.js removes each box that carries data-secret and no
// data-dirty from the FormData. The page and the script must therefore
// agree on the attribute name. This test reads both files and compares
// them, the same as TestFoldTableHasAFrontendCopy.
//
// The code is in omn-go-config.js, and not in omn-go-sse.js, which every
// note loads. The Config page is the only reader of this code.
func TestSecretAttributeHasAFrontendReader(t *testing.T) {
	const scriptPath = "frontend/html/js/OMN-Go/omn-go-config.js"
	raw, err := staticFS.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("%s is not embedded: %v", scriptPath, err)
	}
	script := string(raw)

	for _, want := range []string{"[data-secret]", "dataset.dirty", "fd.delete("} {
		if !strings.Contains(script, want) {
			t.Errorf("omn-go-config.js no longer holds %q. The Config page then sends "+
				"an empty password on each save, and each save clears it.", want)
		}
	}
	if !strings.Contains(configPageTmpl, "omnGoRevealSecrets") {
		t.Error("the Config page has no button that reads the passwords back")
	}
	if !strings.Contains(script, "window.omnGoRevealSecrets") {
		t.Error("omn-go-config.js exports no omnGoRevealSecrets, thus the button does nothing")
	}

	// The page must LOAD the file. The two checks above pass on a file
	// that no page reads. A Config page with no script is a page where
	// no button works.
	if !strings.Contains(configPageTmpl, `<script src="/js/OMN-Go/omn-go-config.js"></script>`) {
		t.Error("config_page.html does not load omn-go-config.js")
	}

	// And the code must be gone from the file that every note loads.
	// Leaving a copy there is how two implementations of one rule start.
	sse, err := staticFS.ReadFile("frontend/html/js/OMN-Go/omn-go-sse.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"omnGoRevealSecrets", "window.saveConfig"} {
		if strings.Contains(string(sse), gone) {
			t.Errorf("omn-go-sse.js still holds %q. Every note carries that file, "+
				"and only the Config page runs this code.", gone)
		}
	}
}

// An old config.json can still hold guest_password. The load ignores the
// key, and the next save removes it. See doc/decisions/0018-keep-one-role.md.
func TestOldGuestPasswordIsDropped(t *testing.T) {
	a := newTestApp(t)
	path := filepath.Join(a.StorageDir, "config.json")
	old := `{"admin_password":"adminpw","guest_password":"GUEST-OLD-5c1d"}`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}
	a.loadConfig(a.StorageDir)
	if got := a.GetConfig().AdminPassword; got != "adminpw" {
		t.Fatalf("the admin password is %q after the load, want adminpw", got)
	}
	if err := a.persistConfig(a.GetConfig()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "guest_password") {
		t.Errorf("the save kept the guest password:\n%s", data)
	}
}
