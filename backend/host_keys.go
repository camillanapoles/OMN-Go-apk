package backend

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	cryptossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ----------------------------------------------------------------------
// The host keys of the git servers
// ----------------------------------------------------------------------
//
// The first connection to a git server stores its host key in
// <StorageDir>/known_hosts. Each later connection must show the same key.
// A changed key stops the sync, and the person decides with the fingerprint.
// See doc/decisions/0019-trust-the-host-key-on-first-use.md.

// knownHostsFilename is in StorageDir, and NOT under html/, where the server
// would serve it. gitignorePatterns keeps it out of the sync.
const knownHostsFilename = "known_hosts"

// hostKeyState holds the one changed key that waits for the decision of the
// person. mu also guards each read and write of the file.
type hostKeyState struct {
	mu      sync.Mutex
	pending *hostKeyChange
}

// hostKeyChange tells the page what changed. Host is in the form of the
// known_hosts file, for example "example.com" or "[example.com]:2222".
type hostKeyChange struct {
	Host        string `json:"host"`
	Known       string `json:"known"`
	Fingerprint string `json:"fingerprint"`
	key         cryptossh.PublicKey
}

// errHostKeyChanged is the error of a connection to a server with a changed
// key. The sync answers it with the status word "host_key_changed".
type errHostKeyChanged struct{ change hostKeyChange }

func (e *errHostKeyChanged) Error() string {
	return fmt.Sprintf("the host key of %s changed: this device knows %s, the server shows %s",
		e.change.Host, e.change.Known, e.change.Fingerprint)
}

func (a *App) knownHostsPath() string {
	return filepath.Join(a.StorageDir, knownHostsFilename)
}

// knownHostKeys answers the stored keys of host. The caller holds mu.
func (a *App) knownHostKeys(host string) []cryptossh.PublicKey {
	data, err := os.ReadFile(a.knownHostsPath())
	if err != nil {
		return nil
	}
	var keys []cryptossh.PublicKey
	for len(data) > 0 {
		_, hosts, key, _, rest, err := cryptossh.ParseKnownHosts(data)
		if err != nil {
			break
		}
		for _, h := range hosts {
			if h == host {
				keys = append(keys, key)
			}
		}
		data = rest
	}
	return keys
}

// hostKeyCallback trusts the key of a new server and stores it. For a known
// server it accepts only a stored key.
func (a *App) hostKeyCallback() cryptossh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key cryptossh.PublicKey) error {
		host := knownhosts.Normalize(hostname)
		a.hostKeys.mu.Lock()
		defer a.hostKeys.mu.Unlock()

		known := a.knownHostKeys(host)
		for _, k := range known {
			if bytes.Equal(k.Marshal(), key.Marshal()) {
				return nil
			}
		}
		fingerprint := cryptossh.FingerprintSHA256(key)
		if len(known) == 0 {
			if err := a.writeHostKey(host, key); err != nil {
				return fmt.Errorf("store the host key of %s: %w", host, err)
			}
			a.logInfof(logSync, "trusted the host key of %s at the first connection: %s", host, fingerprint)
			return nil
		}
		change := hostKeyChange{Host: host, Known: cryptossh.FingerprintSHA256(known[0]),
			Fingerprint: fingerprint, key: key}
		a.hostKeys.pending = &change
		a.logErrf(logSync, "refused %s: the host key changed from %s to %s", host, change.Known, fingerprint)
		return &errHostKeyChanged{change: change}
	}
}

// writeHostKey replaces each stored key of host with key. The caller holds
// mu.
func (a *App) writeHostKey(host string, key cryptossh.PublicKey) error {
	data, err := os.ReadFile(a.knownHostsPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var out bytes.Buffer
	for _, line := range strings.SplitAfter(string(data), "\n") {
		_, hosts, _, _, _, perr := cryptossh.ParseKnownHosts([]byte(line))
		if perr == nil && len(hosts) == 1 && hosts[0] == host {
			continue
		}
		out.WriteString(line)
	}
	if out.Len() > 0 && !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
		out.WriteString("\n")
	}
	out.WriteString(knownhosts.Line([]string{host}, key) + "\n")
	return os.WriteFile(a.knownHostsPath(), out.Bytes(), 0600)
}

// hostKeyAlgorithms answers the key algorithms of the stored keys of host.
// The server must then show a key of a stored type. A server with more than
// one key thus does not look like a changed key.
func (a *App) hostKeyAlgorithms(host string) []string {
	a.hostKeys.mu.Lock()
	defer a.hostKeys.mu.Unlock()
	var algos []string
	for _, k := range a.knownHostKeys(host) {
		if k.Type() == cryptossh.KeyAlgoRSA {
			algos = append(algos, cryptossh.KeyAlgoRSASHA512, cryptossh.KeyAlgoRSASHA256)
		}
		algos = append(algos, k.Type())
	}
	return algos
}

// sshHostOf answers the host of a git URL in the form of the known_hosts
// file, or "" for a URL that does not use SSH.
func sshHostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "ssh" || u.Hostname() == "" {
			return ""
		}
		port := u.Port()
		if port == "" {
			port = "22"
		}
		return knownhosts.Normalize(net.JoinHostPort(u.Hostname(), port))
	}
	// The scp form, "user@host:path" or "host:path". A local path has no
	// colon, and a drive letter such as "C:" is one character.
	rest := raw[strings.LastIndex(raw, "@")+1:]
	colon := strings.Index(rest, ":")
	if colon <= 1 || strings.ContainsAny(rest[:colon], "/\\") {
		return ""
	}
	return knownhosts.Normalize(rest[:colon])
}

// hostKeyText is the line of the Config page for one git URL.
func (a *App) hostKeyText(rawURL string) string {
	host := sshHostOf(rawURL)
	if host == "" {
		return ""
	}
	a.hostKeys.mu.Lock()
	defer a.hostKeys.mu.Unlock()
	keys := a.knownHostKeys(host)
	if len(keys) == 0 {
		return "Server key: not known yet. The first sync stores it."
	}
	return "Server key: " + cryptossh.FingerprintSHA256(keys[0])
}

// handleTrustHostKey answers POST /api/sync/trust-host-key. It stores the
// changed key that waits, when host and fingerprint name that key. The page
// sends the fingerprint that the person saw, thus a key that changed again
// in the meantime is not stored.
func (a *App) handleTrustHostKey(w http.ResponseWriter, r *http.Request) {
	host, fingerprint := r.FormValue("host"), r.FormValue("fingerprint")
	a.hostKeys.mu.Lock()
	defer a.hostKeys.mu.Unlock()
	p := a.hostKeys.pending
	if p == nil || p.Host != host || p.Fingerprint != fingerprint {
		a.writeJSONError(w, http.StatusConflict, "no changed key of this server waits for a decision")
		return
	}
	if err := a.writeHostKey(p.Host, p.key); err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.hostKeys.pending = nil
	a.logInfof(logSync, "trusted the new host key of %s: %s", host, fingerprint)
	a.writeJSON(w, http.StatusOK, jsonStatus{Status: "success"})
}

// hostKeyAnswer is the answer of /api/sync for a changed host key.
type hostKeyAnswer struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	hostKeyChange
}

// hostKeyChangeOf finds a changed host key in the chain of err.
func hostKeyChangeOf(err error) (hostKeyChange, bool) {
	var hk *errHostKeyChanged
	if errors.As(err, &hk) {
		return hk.change, true
	}
	return hostKeyChange{}, false
}
