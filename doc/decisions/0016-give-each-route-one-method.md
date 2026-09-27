# 0016. Give each route one method

* Status: accepted
* Version: 26.09.102
* Code: `registerRoutes`, `route` and `refuseMethod` in `backend/server.go`

## Context

`http.ServeMux` sent each method to each route. Eleven handlers checked
`r.Method` themselves, and the other routes took any method. `r.FormValue`
reads the query string, thus a `GET` with the name and the content in the
address saved a note. A link or an image on another site could send that
`GET` from the browser of the device. The device is always admin, thus
the note changed with no question to the person.

The handlers that checked the method did it in three ways. Some answered
JSON, some answered `GET only`, and some answered `Method Not Allowed`.

## Decision

* Each route names its method in the pattern, for example
  `POST /api/save`. A route that reads takes `GET`. A route that writes
  takes `POST`. `/api/config` takes both, with one handler for each.
* `route` also registers the bare path. That path answers `405`, and the
  `Allow` header names the method of the route.
* No handler checks `r.Method`.
* The catch-all `/` and the asset trees take each method. These routes
  write nothing.

## Rejected alternatives

* **The method patterns alone.** `ServeMux` sends `405` only when no
  pattern matches the path. The catch-all `/` matches each path, thus a
  `GET` on `POST /api/save` went to `serveFrontend`.
* **A check of `r.Method` in each handler.** A new handler can forget the
  check, and the route table does not show the method.

## Consequences

* A form on another site can still send a `POST`. A check of `Origin` and
  `Host` must stop that.
* A wrong method gets `405` before `authMiddleware` runs. The answer tells
  a caller nothing about the data.
* `TestEachMethodRouteRefusesAnotherMethod` reads the routes from
  `registerRoutes`, thus a new route needs no new test line.
  `TestGetDoesNotSaveANote` keeps the probe of the `GET` fault.
