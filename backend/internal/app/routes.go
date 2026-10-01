package app

import (
	"net/http"
	"strings"
)

// access is the role that a route asks of a caller on another machine. The
// device itself is always admin. See doc/decisions/0018-keep-one-role.md.
type access int

const (
	open      access = iota // no role
	admin                   // authMiddleware answers 401
	adminPage               // pageHandler answers a refusal page
)

// methodHandler is the handler of one method of a route.
type methodHandler struct {
	method string
	h      http.HandlerFunc
}

func get(h http.HandlerFunc) methodHandler  { return methodHandler{http.MethodGet, h} }
func post(h http.HandlerFunc) methodHandler { return methodHandler{http.MethodPost, h} }

// routeDoc is one row of the route table of doc/API.md. A routeTable that
// also has the method document gets each row. TestAPIRouteTable reads them.
type routeDoc struct {
	methods []string
	path    string
	who     access
	answer  string // what the route answers, for example "JSON"
}

type routeDocTable interface {
	document(routeDoc)
}

// route registers each handler of hs on path, for its method. The pattern
// "GET /x" also takes HEAD. The bare path answers 405 for another method.
// route puts authMiddleware before an admin handler. See
// doc/decisions/0016-give-each-route-one-method.md.
func (a *App) route(mux routeTable, path string, who access, answer string, hs ...methodHandler) {
	var methods []string
	for _, m := range hs {
		h := m.h
		if who == admin {
			h = a.authMiddleware(h)
		}
		mux.HandleFunc(m.method+" "+path, h)
		methods = append(methods, m.method)
	}
	mux.HandleFunc(path, refuseMethod(methods...))
	if d, ok := mux.(routeDocTable); ok {
		d.document(routeDoc{methods, path, who, answer})
	}
}

// refuseMethod answers 405, and the Allow header names the methods.
func refuseMethod(methods ...string) http.HandlerFunc {
	var allow []string
	for _, m := range methods {
		allow = append(allow, m)
		if m == http.MethodGet {
			allow = append(allow, http.MethodHead)
		}
	}
	header := strings.Join(allow, ", ")
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", header)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
