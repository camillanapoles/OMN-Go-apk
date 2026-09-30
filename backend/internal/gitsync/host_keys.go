package gitsync

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	cryptossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
)

// ----------------------------------------------------------------------
// The host keys of the git servers
// ----------------------------------------------------------------------
//
// The first connection to a git server stores its host key in
// <StorageDir>/known_hosts. Each later connection must show the same key.
// A changed key stops the sync, and the person decides with the fingerprint.
// See doc/decisions/0019-trust-the-host-key-on-first-use.md.

// KnownHostsFilename is in StorageDir, and NOT under html/, where the server
// would serve it. GitignorePatterns keeps it out of the sync.
const KnownHostsFilename = "known_hosts"

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

// ErrHostKeyChanged is the error of a connection to a server with a changed
// key. The sync answers it with the status word "host_key_changed".
type ErrHostKeyChanged struct{ change hostKeyChange }

func (e *ErrHostKeyChanged) Error() string {
	return fmt.Sprintf("the host key of %s changed: this device knows %s, the server shows %s",
		e.change.Host, e.change.Known, e.change.Fingerprint)
}

func (svc Service) KnownHostsPath() string {
	return svc.Layout.File(KnownHostsFilename)
}

// knownHostKeys answers the stored keys of host. The caller holds mu.
func (svc Service) knownHostKeys(host string) []cryptossh.PublicKey {
	data, err := os.ReadFile(svc.KnownHostsPath())
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

// HostKeyCallback trusts the key of a new server and stores it. For a known
// server it accepts only a stored key.
func (svc Service) HostKeyCallback() cryptossh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key cryptossh.PublicKey) error {
		host := knownhosts.Normalize(hostname)
		svc.State.hostKeys.mu.Lock()
		defer svc.State.hostKeys.mu.Unlock()

		known := svc.knownHostKeys(host)
		for _, k := range known {
			if bytes.Equal(k.Marshal(), key.Marshal()) {
				return nil
			}
		}
		fingerprint := cryptossh.FingerprintSHA256(key)
		if len(known) == 0 {
			if err := svc.writeHostKey(host, key); err != nil {
				return fmt.Errorf("store the host key of %s: %w", host, err)
			}
			svc.Log(logx.Sync).Infof("trusted the host key of %s at the first connection: %s", host, fingerprint)
			return nil
		}
		change := hostKeyChange{Host: host, Known: cryptossh.FingerprintSHA256(known[0]),
			Fingerprint: fingerprint, key: key}
		svc.State.hostKeys.pending = &change
		svc.Log(logx.Sync).Errf("refused %s: the host key changed from %s to %s", host, change.Known, fingerprint)
		return &ErrHostKeyChanged{change: change}
	}
}

// writeHostKey replaces each stored key of host with key. The caller holds
// mu.
func (svc Service) writeHostKey(host string, key cryptossh.PublicKey) error {
	data, err := os.ReadFile(svc.KnownHostsPath())
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
	return os.WriteFile(svc.KnownHostsPath(), out.Bytes(), 0600)
}

// HostKeyAlgorithms answers the key algorithms of the stored keys of host.
// The server must then show a key of a stored type. A server with more than
// one key thus does not look like a changed key.
func (svc Service) HostKeyAlgorithms(host string) []string {
	svc.State.hostKeys.mu.Lock()
	defer svc.State.hostKeys.mu.Unlock()
	var algos []string
	for _, k := range svc.knownHostKeys(host) {
		if k.Type() == cryptossh.KeyAlgoRSA {
			algos = append(algos, cryptossh.KeyAlgoRSASHA512, cryptossh.KeyAlgoRSASHA256)
		}
		algos = append(algos, k.Type())
	}
	return algos
}

// SSHHostOf answers the host of a git URL in the form of the known_hosts
// file, or "" for a URL that does not use SSH.
func SSHHostOf(raw string) string {
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

// HostKeyText is the line of the Config page for one git URL.
func (svc Service) HostKeyText(rawURL string) string {
	host := SSHHostOf(rawURL)
	if host == "" {
		return ""
	}
	svc.State.hostKeys.mu.Lock()
	defer svc.State.hostKeys.mu.Unlock()
	keys := svc.knownHostKeys(host)
	if len(keys) == 0 {
		return "Server key: not known yet. The first sync stores it."
	}
	return "Server key: " + cryptossh.FingerprintSHA256(keys[0])
}

// HandleTrustHostKey answers POST /api/sync/trust-host-key. It stores the
// changed key that waits, when host and fingerprint name that key. The page
// sends the fingerprint that the person saw, thus a key that changed again
// in the meantime is not stored.
func (svc Service) HandleTrustHostKey(w http.ResponseWriter, r *http.Request) {
	host, fingerprint := r.FormValue("host"), r.FormValue("fingerprint")
	svc.State.hostKeys.mu.Lock()
	defer svc.State.hostKeys.mu.Unlock()
	p := svc.State.hostKeys.pending
	if p == nil || p.Host != host || p.Fingerprint != fingerprint {
		svc.writeJSONError(w, http.StatusConflict, "no changed key of this server waits for a decision")
		return
	}
	if err := svc.writeHostKey(p.Host, p.key); err != nil {
		svc.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc.State.hostKeys.pending = nil
	svc.Log(logx.Sync).Infof("trusted the new host key of %s: %s", host, fingerprint)
	svc.writeJSON(w, http.StatusOK, render.JSONStatus{Status: "success"})
}

// hostKeyAnswer is the answer of /api/sync for a changed host key.
type hostKeyAnswer struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	hostKeyChange
}

// HostKeyChangeOf finds a changed host key in the chain of err.
func HostKeyChangeOf(err error) (hostKeyChange, bool) {
	var hk *ErrHostKeyChanged
	if errors.As(err, &hk) {
		return hk.change, true
	}
	return hostKeyChange{}, false
}

// WriteHostKey trusts key for host. It holds the lock of the known_hosts file
// for the write. See writeHostKey.
func (svc Service) WriteHostKey(host string, key cryptossh.PublicKey) error {
	svc.State.hostKeys.mu.Lock()
	defer svc.State.hostKeys.mu.Unlock()
	return svc.writeHostKey(host, key)
}
