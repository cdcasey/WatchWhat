package server

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (a *App) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" || r.URL.Path == "/favicon.ico" {
			return
		}
		a.Log.Debug("request", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"htmx", isHTMX(r), "dur", time.Since(start).Round(time.Microsecond))
	})
}

func (a *App) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				a.Log.Error("panic", "path", r.URL.Path, "panic", p, "stack", string(debug.Stack()))
				a.renderError(w, r, fmt.Errorf("panic: %v", p))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// vary marks every dynamic response as varying on HX-Request so caches never
// serve a fragment for a direct load.
func vary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Add("Vary", "HX-Request")
		}
		next.ServeHTTP(w, r)
	})
}

// notFoundWriter swaps the mux's plain-text 404 for the HTML error page.
type notFoundWriter struct {
	http.ResponseWriter
	app       *App
	r         *http.Request
	swallowed bool
}

func (w *notFoundWriter) WriteHeader(code int) {
	if code == http.StatusNotFound {
		w.swallowed = true
		w.Header().Del("X-Content-Type-Options")
		w.app.renderError(w.ResponseWriter, w.r, errNotFound)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundWriter) Write(b []byte) (int, error) {
	if w.swallowed {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
