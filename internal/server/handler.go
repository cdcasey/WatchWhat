package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/chriscasey/watchwhat/internal/catalog"
	"github.com/chriscasey/watchwhat/internal/schedule"
	"github.com/chriscasey/watchwhat/internal/tmdb"
	"github.com/chriscasey/watchwhat/internal/tracking"
	"github.com/chriscasey/watchwhat/internal/views/components"
	"github.com/chriscasey/watchwhat/internal/views/layout"
	"github.com/chriscasey/watchwhat/internal/views/pages"
)

type appHandler func(w http.ResponseWriter, r *http.Request) error

// HTTPError carries a status and a user-safe message. Err is logged, never shown.
type HTTPError struct {
	Status int
	Msg    string
	Err    error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%d %s: %v", e.Status, e.Msg, e.Err)
	}
	return fmt.Sprintf("%d %s", e.Status, e.Msg)
}

func (e *HTTPError) Unwrap() error { return e.Err }

func badRequest(msg string) error { return &HTTPError{Status: http.StatusBadRequest, Msg: msg} }

var errNotFound = &HTTPError{Status: http.StatusNotFound, Msg: "That page doesn't exist."}

func (a *App) h(fn appHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			a.renderError(w, r, err)
		}
	})
}

// classify maps an error to a status and a user-safe message.
func classify(err error) (int, string) {
	var he *HTTPError
	var se *tmdb.StatusError
	switch {
	case errors.As(err, &he):
		return he.Status, he.Msg
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound, "Not found."
	case errors.Is(err, tmdb.ErrUnavailable):
		return http.StatusBadGateway, "TMDB is unreachable. Try again in a moment."
	case errors.As(err, &se) && se.Code == http.StatusNotFound:
		return http.StatusNotFound, "TMDB doesn't know that title."
	case errors.As(err, &se):
		return http.StatusBadGateway, "TMDB returned an error."
	case errors.Is(err, schedule.ErrNotSchedulable):
		return http.StatusUnprocessableEntity, "Only Backlog or Watching TV shows can be scheduled."
	case errors.Is(err, catalog.ErrInvalid), errors.Is(err, tracking.ErrInvalid):
		return http.StatusBadRequest, "Invalid request."
	}
	return http.StatusInternalServerError, "Something went wrong."
}

// renderError writes an HTML error. HTMX requests get a toast retargeted to
// #toasts so error markup can never land inside the triggering element.
func (a *App) renderError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		return // client went away
	}
	status, msg := classify(err)
	if status >= 500 {
		a.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "status", status, "err", err)
	} else {
		a.Log.Info("request rejected", "method", r.Method, "path", r.URL.Path, "status", status, "err", err)
	}

	var c templ.Component
	if wantsFragment(r) {
		w.Header().Set("HX-Retarget", "#toasts")
		w.Header().Set("HX-Reswap", "beforeend")
		c = components.Toast(msg)
	} else {
		c = layout.Base(a.meta(http.StatusText(status), ""), pages.Error(status, msg))
	}
	var buf bytes.Buffer
	if rerr := c.Render(r.Context(), &buf); rerr != nil {
		a.Log.Error("rendering error page", "err", rerr)
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Del("Content-Length")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// wantsFragment: HTMX requests get content only, except boosted requests and
// history-restore requests, which need a full document.
func wantsFragment(r *http.Request) bool {
	return isHTMX(r) && r.Header.Get("HX-Boosted") != "true" && r.Header.Get("HX-History-Restore-Request") != "true"
}

func (a *App) meta(title, active string) layout.Meta {
	return layout.Meta{Title: title, Active: active, CSS: a.Assets.URL("css/app.css"), JS: a.Assets.URL("js/htmx.min.js"), Icon: a.Assets.URL("favicon.svg")}
}

// render writes a navigable view: a full document or the content fragment.
func (a *App) render(w http.ResponseWriter, r *http.Request, title, active string, content templ.Component) error {
	m := a.meta(title, active)
	if wantsFragment(r) {
		return a.fragment(w, r, layout.Fragment(m, content))
	}
	return a.fragment(w, r, layout.Base(m, content))
}

// fragment renders c into a buffer first so a render error still yields a clean error response.
func (a *App) fragment(w http.ResponseWriter, r *http.Request, c templ.Component) error {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write(buf.Bytes())
	return err
}
