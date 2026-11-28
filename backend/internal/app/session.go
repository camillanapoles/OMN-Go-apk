package app

// ----------------------------------------------------------------------
// The session cookie
// ----------------------------------------------------------------------
//
// A client on the network must not name its own role. See
// doc/decisions/0001-sign-the-session-cookie.md.
//
// The server writes the role, the expiry time and an HMAC of the two. It
// accepts a cookie only when the key of this install makes the same HMAC. The
// key is 32 random bytes in <StorageDir>/session_secret. It is not a field of
// Config, because GET /api/config and the Config page use the whole Config.
// gitsync.GitignorePatterns keeps it out of the sync.
//
// There are two cookies:
//
//	session_role       HttpOnly, signed, the only thing the server reads
//	session_role_hint  readable, NOT signed, display only
//
// A note can hold a script, and a script that reads session_role could send
// it to another machine. checkSession in omn-go-api.js needs to know about
// the login, thus it reads the hint. A client that changes the hint changes
// only its own page.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/logx"
)

// roleAdmin is the one role. An empty string means "no role", and no cookie
// holds it. See doc/decisions/0018-keep-one-role.md.
const roleAdmin = "admin"

const (
	sessionCookieName = "session_role"

	// sessionHintCookieName is the plain role for the page. The server never
	// reads it.
	sessionHintCookieName = "session_role_hint"

	// sessionSecretFilename is in StorageDir, and NOT under html/, where the
	// server would serve it.
	sessionSecretFilename = "session_secret"

	// sessionKeyBytes is the output size of SHA-256. A longer key adds
	// nothing.
	sessionKeyBytes = 32

	// sessionTTL is the length of a login. The device itself needs no login.
	sessionTTL = 30 * 24 * time.Hour
)

// sessionSecret returns the HMAC key of this install. It reads the key one
// time. A first start makes the key and writes it with mode 0600. A damaged
// file gets a new key, and each person must enter the password again. When the write fails,
// the key stays in memory until the process stops.
func (a *App) sessionSecret() []byte {
	a.sessionOnce.Do(func() {
		path := a.layout().File(sessionSecretFilename)

		if data, err := os.ReadFile(path); err == nil {
			key, decErr := hex.DecodeString(strings.TrimSpace(string(data)))
			if decErr == nil && len(key) == sessionKeyBytes {
				a.sessionKey = key
				return
			}
			a.log(logx.Session).Errf("%s does not hold a valid key, a new key replaces it", path)
		}

		key := make([]byte, sessionKeyBytes)
		if _, err := rand.Read(key); err != nil {
			// With no random source, write no key. Each remote request then
			// gets 401. A guessable key would be worse.
			a.log(logx.Session).Errf("no random source for the session key: %v", err)
			return
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0600); err != nil {
			a.log(logx.Session).Errf("failed to write %s, each session ends at the next start: %v", path, err)
		}
		a.sessionKey = key
	})
	return a.sessionKey
}

// sessionMAC makes the HMAC of "role.expiry". A role and an expiry hold no
// dot, thus no text can move across the separator.
func (a *App) sessionMAC(role string, expiry int64) string {
	key := a.sessionSecret()
	if key == nil {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(role + "." + strconv.FormatInt(expiry, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// newSessionCookies makes the two cookies of one login. signed is nil when
// the install has no key, and the caller must then answer with a fault.
func (a *App) newSessionCookies(role string) (signed, hint *http.Cookie) {
	expiry := time.Now().Add(sessionTTL).Unix()
	sig := a.sessionMAC(role, expiry)
	if sig == "" {
		return nil, nil
	}
	expires := time.Unix(expiry, 0)
	value := role + "." + strconv.FormatInt(expiry, 10) + "." + sig

	signed = &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		// SameSite is Lax, because a link from another application is a
		// normal way to arrive. The cookie has no Secure flag, because this
		// server uses HTTP. A browser never sends a Secure cookie over HTTP.
		SameSite: http.SameSiteLaxMode,
	}
	hint = &http.Cookie{
		Name:     sessionHintCookieName,
		Value:    role,
		Path:     "/",
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
	}
	return signed, hint
}

// readSessionRole returns the role that the request carries, or "". It is the
// ONE reader of the session cookie. It answers "" for no cookie, a wrong
// shape, an unknown role, an expiry that passed, a wrong HMAC and the old
// unsigned value.
func (a *App) readSessionRole(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return ""
	}
	role, expiryText, sig := parts[0], parts[1], parts[2]

	if role != roleAdmin {
		return ""
	}
	expiry, err := strconv.ParseInt(expiryText, 10, 64)
	if err != nil {
		return ""
	}

	// Test the MAC before the clock. A value that this install did not sign
	// then fails for that reason alone.
	want := a.sessionMAC(role, expiry)
	if want == "" || !hmac.Equal([]byte(sig), []byte(want)) {
		return ""
	}
	if time.Now().Unix() >= expiry {
		return ""
	}
	return role
}

// handleLogin changes a password into the two session cookies. Only a caller
// on the network needs it. The comparison is constant-time, and an EMPTY
// configured password matches nothing.
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	cfg := a.config.Get()
	pwd := r.FormValue("password")
	local := a.isLocalConnection(r)

	// An address on the network owes the wait that its failed logins made.
	// The device itself never waits, and the guard speaks before the
	// password is read. See doc/decisions/0035.
	addr := loginAddress(r)
	if !local {
		if wait := a.loginWait(addr); wait > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			http.Error(w, "Too many attempts", http.StatusTooManyRequests)
			return
		}
	}

	if !passwordMatches(pwd, cfg.AdminPassword) {
		if cfg.AdminPassword == "" {
			a.log(logx.Session).Errf("login refused: config.json holds no admin password, thus no caller on the network can log in")
		}
		if !local {
			a.loginNoteFailure(addr)
		}
		http.Error(w, "Invalid", http.StatusUnauthorized)
		return
	}

	// The network may not use the default password. See doc/decisions/0034.
	if cfg.AdminPassword == config.DefaultAdminPassword && !local {
		a.log(logx.Session).Errf("login refused: the admin password is still the default one, and the caller is not local")
		http.Error(w, "Set a new password on the device first", http.StatusForbidden)
		return
	}

	if !local {
		a.loginNoteSuccess(addr)
	}
	signed, hint := a.newSessionCookies(roleAdmin)
	if signed == nil {
		// The install has no key. See sessionSecret. An unsigned cookie is
		// not an option.
		a.log(logx.Session).Errf("login refused: this install has no session key")
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

// The login throttle. Five failed logins of one address close the door for
// a while, and each further failure doubles the wait. The state is memory
// alone, keyed by the app and by the address, and the device itself never
// enters it. See doc/decisions/0035.
type loginAttempt struct {
	failures     int
	blockedUntil time.Time
}

var loginThrottle = struct {
	sync.Mutex
	perApp map[*App]map[string]*loginAttempt
}{perApp: map[*App]map[string]*loginAttempt{}}

const loginTryLimit = 5
const loginBackoffCap = 15 * time.Minute

func loginAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginWait answers the wait that the address still owes, or zero.
func (a *App) loginWait(addr string) time.Duration {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()
	if at := loginThrottle.perApp[a][addr]; at != nil {
		if wait := time.Until(at.blockedUntil); wait > 0 {
			return wait
		}
	}
	return 0
}

// loginNoteFailure remembers one failed login and grows the wait.
func (a *App) loginNoteFailure(addr string) {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()
	m := loginThrottle.perApp[a]
	if m == nil {
		m = map[string]*loginAttempt{}
		loginThrottle.perApp[a] = m
	}
	at := m[addr]
	if at == nil {
		at = &loginAttempt{}
		m[addr] = at
	}
	at.failures++
	if at.failures >= loginTryLimit {
		wait := 30 * time.Second << (at.failures - loginTryLimit)
		if wait > loginBackoffCap || wait <= 0 {
			wait = loginBackoffCap
		}
		at.blockedUntil = time.Now().Add(wait)
	}
}

// loginNoteSuccess clears the address, and the count starts over.
func (a *App) loginNoteSuccess(addr string) {
	loginThrottle.Lock()
	defer loginThrottle.Unlock()
	if m := loginThrottle.perApp[a]; m != nil {
		delete(m, addr)
	}
}
