package app

// ----------------------------------------------------------------------
// The start of the server and the restart
// ----------------------------------------------------------------------
//
// StartServer is the entry point of the desktop application and of the
// Android application. Before these tests, no test called it. The tests
// here start a real server on a free loopback port.
//
// A server that StartServer starts has no stop. Its goroutine and its
// socket stay until the test binary ends. Each test therefore uses its own
// free port. The storage directory comes from siDir, because the
// background precompile of initStorage writes into it after the start.

import (
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// srvFreePort answers a loopback port that no socket holds at this moment.
func srvFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// srvStart calls StartServer and waits for the socket. It fails the test
// when the wait takes more than ten seconds. StartServer sends the
// standard logger to the log page, thus the cleanup puts it back.
func srvStart(t *testing.T, port int) *App {
	t.Helper()
	prev := log.Writer()
	t.Cleanup(func() { log.SetOutput(prev) })

	a := startServer(siDir(t), port)
	done := make(chan struct{})
	go func() {
		a.waitUntilReady()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("waitUntilReady did not return in ten seconds")
	}
	return a
}

// A fresh install binds the port that the flavor gives to StartServer. It
// binds the loopback address only, because LAN sharing is off by default.
// The server then answers a real HTTP request. The Android flavors use two
// different ports, and this path is how each one gets its port.
func TestStartServerServesOnTheFlavorPort(t *testing.T) {
	port := srvFreePort(t)
	a := srvStart(t, port)

	if got := a.serverPort(); got != port {
		t.Errorf("serverPort() = %d, want %d", got, port)
	}
	// main_desktop.go reads the port through the facade function.
	if got := ServerPort(); got != port {
		t.Errorf("ServerPort() = %d, want %d", got, port)
	}
	host, boundPort, _ := a.boundAddress()
	if host != "127.0.0.1" || boundPort != strconv.Itoa(port) {
		t.Errorf("the socket is on %s:%s, want 127.0.0.1:%d", host, boundPort, port)
	}

	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/Welcome.html" {
		t.Errorf("GET / gave %d to %q, want 303 to /Welcome.html",
			resp.StatusCode, resp.Header.Get("Location"))
	}
}

// When another program holds the port, the bind fails after its retries.
// WaitUntilReady must then return, and it must not wait for ever. The
// desktop application waits on it before it opens the browser. The retry
// loop takes about three seconds, thus this test takes that long.
func TestStartServerReturnsWhenThePortIsTaken(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	a := srvStart(t, port)
	if _, _, addr := a.boundAddress(); addr != "" {
		t.Errorf("the bound address is %q, want empty after a failed bind", addr)
	}
}

// The restart endpoint must answer before the restart, thus the browser
// gets the answer. A GET through the router must not restart. The test
// replaces restartHook, because the real hook stops the process.
func TestRestartAnswersAndThenRestarts(t *testing.T) {
	called := make(chan struct{}, 2)
	prev := restartHook
	restartHook = func(*App) { called <- struct{}{} }
	t.Cleanup(func() { restartHook = prev })

	a := newTestApp(t)

	rec := routeReq(a, http.MethodGet, "/api/restart")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status %d, want 405", rec.Code)
	}

	rec = routeReq(a, http.MethodPost, "/api/restart")
	if rec.Code != http.StatusOK || rec.Body.String() != "Restarting" {
		t.Errorf("POST: status %d, body %q, want 200 and Restarting", rec.Code, rec.Body.String())
	}
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("the restart hook did not run after the POST")
	}
	select {
	case <-called:
		t.Error("the restart hook ran two times. The GET must not start it.")
	case <-time.After(700 * time.Millisecond):
	}
}

// Before StartServer, the two functions of main_desktop.go answer at once.
// WaitUntilReady must not block, and ServerPort answers 0.
func TestFacadeAnswersBeforeStartServer(t *testing.T) {
	stClearRunning()
	t.Cleanup(stClearRunning)
	done := make(chan struct{})
	go func() {
		WaitUntilReady()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitUntilReady blocked with no server")
	}
	if got := ServerPort(); got != 0 {
		t.Errorf("ServerPort() = %d before StartServer, want 0", got)
	}
}
