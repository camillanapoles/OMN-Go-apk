package gitsync

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	cryptossh "golang.org/x/crypto/ssh"
	"net.basov.omngo/backend/internal/config"
)

// hkKey makes a new ed25519 host key.
func hkKey(t *testing.T) cryptossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := cryptossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// hkTrust calls POST /api/sync/trust-host-key.
func hkTrust(a *testApp, host, fingerprint string) *httptest.ResponseRecorder {
	form := url.Values{"host": {host}, "fingerprint": {fingerprint}}
	r := httptest.NewRequest(http.MethodPost, "/api/sync/trust-host-key", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.handleTrustHostKey(w, r)
	return w
}

// Finding X3. The first key of a server is stored. A later connection with
// the same key passes, and a changed key fails until the person trusts it.
// See doc/decisions/0019-trust-the-host-key-on-first-use.md.
func TestHostKeyIsTrustedOnFirstUse(t *testing.T) {
	a := newTestApp(t)
	cb := a.gitSync().HostKeyCallback()
	first, second := hkKey(t).PublicKey(), hkKey(t).PublicKey()

	if err := cb("git.example:22", nil, first); err != nil {
		t.Fatalf("the first key was refused: %v", err)
	}
	st, err := os.Stat(a.gitSync().KnownHostsPath())
	if err != nil {
		t.Fatalf("known_hosts was not written: %v", err)
	}
	if runtimeSupportsFileMode() && st.Mode().Perm() != 0600 {
		t.Errorf("known_hosts has mode %v, want 0600", st.Mode().Perm())
	}
	if err := cb("git.example:22", nil, first); err != nil {
		t.Errorf("the stored key was refused: %v", err)
	}

	err = cb("git.example:22", nil, second)
	change, ok := HostKeyChangeOf(err)
	if !ok {
		t.Fatalf("a changed key gave %v, want ErrHostKeyChanged", err)
	}
	if change.Host != "git.example" || change.Known != cryptossh.FingerprintSHA256(first) ||
		change.Fingerprint != cryptossh.FingerprintSHA256(second) {
		t.Errorf("the change is %+v", change)
	}

	if w := hkTrust(a, "git.example", cryptossh.FingerprintSHA256(first)); w.Code != http.StatusConflict {
		t.Errorf("a trust of another fingerprint answered %d, want 409", w.Code)
	}
	if w := hkTrust(a, "git.example", change.Fingerprint); w.Code != http.StatusOK {
		t.Fatalf("the trust answered %d %s", w.Code, w.Body.String())
	}
	if err := cb("git.example:22", nil, second); err != nil {
		t.Errorf("the trusted new key was refused: %v", err)
	}
	if _, ok := HostKeyChangeOf(cb("git.example:22", nil, first)); !ok {
		t.Error("the old key still passes after the trust of the new key")
	}
	if w := hkTrust(a, "git.example", change.Fingerprint); w.Code != http.StatusConflict {
		t.Errorf("a second trust answered %d, want 409", w.Code)
	}
}

// The known_hosts file must stay out of the sync. A pull must not change
// the keys that this device trusts.
func TestKnownHostsStaysOutOfGit(t *testing.T) {
	if !slices.Contains(GitignorePatterns, KnownHostsFilename) {
		t.Errorf("GitignorePatterns has no line for %s", KnownHostsFilename)
	}
}

func TestSSHHostOf(t *testing.T) {
	for raw, want := range map[string]string{
		"git@github.com:me/notes.git":            "github.com",
		"github.com:me/notes.git":                "github.com",
		"  git@host.lan:notes.git  ":             "host.lan",
		"git@10.0.0.5:notes.git":                 "10.0.0.5",
		"ssh://git@example.com/notes.git":        "example.com",
		"ssh://git@example.com:22/notes.git":     "example.com",
		"ssh://git@example.com:2222/notes.git":   "[example.com]:2222",
		"ssh://git@[2001:db8::1]:2222/notes.git": "[2001:db8::1]:2222",
		"https://ann:pw@example.com/notes.git":   "",
		"file:///srv/notes.git":                  "",
		"/home/ann/remote.git":                   "",
		"./relative/remote.git":                  "",
		`C:\repos\notes.git`:                     "",
		"":                                       "",
	} {
		if got := SSHHostOf(raw); got != want {
			t.Errorf("SSHHostOf(%q) = %q, want %q", raw, got, want)
		}
	}
}

// A server with an RSA key can sign with rsa-sha2. The list must name those
// algorithms, or the handshake fails with a stored RSA key.
func TestHostKeyAlgorithmsFollowTheStoredKeys(t *testing.T) {
	a := newTestApp(t)
	if got := a.gitSync().HostKeyAlgorithms("git.example"); len(got) != 0 {
		t.Errorf("an unknown host gave %v, want no list", got)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := cryptossh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.gitSync().HostKeyCallback()("git.example:22", nil, pub); err != nil {
		t.Fatal(err)
	}
	got := a.gitSync().HostKeyAlgorithms("git.example")
	for _, want := range []string{cryptossh.KeyAlgoRSASHA512, cryptossh.KeyAlgoRSASHA256, cryptossh.KeyAlgoRSA} {
		if !slices.Contains(got, want) {
			t.Errorf("the list %v has no %s", got, want)
		}
	}
}

// hkServer starts an SSH server on the loopback address with hostKey. It
// accepts each client and refuses each channel. A handshake is all that a
// host key test needs.
func hkServer(t *testing.T, hostKey cryptossh.Signer) string {
	t.Helper()
	cfg := &cryptossh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(hostKey)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := cryptossh.NewServerConn(conn, cfg)
				if err != nil {
					conn.Close()
					return
				}
				go cryptossh.DiscardRequests(reqs)
				for ch := range chans {
					ch.Reject(cryptossh.Prohibited, "no git here")
				}
			}()
		}
	}()
	return l.Addr().String()
}

// The whole path. A sync through go-git stores the key of a new server. A
// changed key gives the status word host_key_changed with both fingerprints.
func TestSyncAnswersAChangedHostKey(t *testing.T) {
	first := hkKey(t)
	addr := hkServer(t, first)
	a := gsApp(t, "ssh://git@"+addr+"/notes.git")

	w := ghSync(t, a, url.Values{"action": {"pull"}})
	if strings.Contains(w.Body.String(), "host_key_changed") {
		t.Fatalf("the first connection gave %s", w.Body.String())
	}
	host := SSHHostOf("ssh://git@" + addr + "/notes.git")
	if !strings.Contains(a.gitSync().HostKeyText("ssh://git@"+addr+"/notes.git"), cryptossh.FingerprintSHA256(first.PublicKey())) {
		t.Fatalf("the first connection stored no key for %s", host)
	}

	second := hkKey(t)
	a.Config.Update(func(c *config.Config) { c.GitServers[0].URL = "ssh://git@" + hkServer(t, second) + "/notes.git" })
	// The new server listens on another port. The test gives it the host
	// entry of the first server, thus the key looks changed.
	newHost := SSHHostOf(a.Config.Get().GitServers[0].URL)
	if err := a.gitSync().WriteHostKey(newHost, first.PublicKey()); err != nil {
		t.Fatal(err)
	}

	w = ghSync(t, a, url.Values{"action": {"pull"}})
	var body struct {
		Status, Host, Known, Fingerprint string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %s", w.Body.String())
	}
	if body.Status != "host_key_changed" || body.Host != newHost ||
		body.Known != cryptossh.FingerprintSHA256(first.PublicKey()) ||
		body.Fingerprint != cryptossh.FingerprintSHA256(second.PublicKey()) {
		t.Errorf("the answer is %s", w.Body.String())
	}
}
