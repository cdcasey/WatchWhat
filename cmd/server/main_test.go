package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	code := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(code)
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]

	if got := healthcheck(port); got != 0 { // ":PORT" form, like WATCHWHAT_ADDR
		t.Fatalf("healthy: exit %d", got)
	}
	code = http.StatusServiceUnavailable
	if got := healthcheck(port); got != 1 {
		t.Fatalf("503: exit %d", got)
	}
	srv.Close()
	if got := healthcheck(port); got != 1 {
		t.Fatalf("down: exit %d", got)
	}
	if got := healthcheck("garbage"); got != 1 {
		t.Fatalf("bad addr: exit %d", got)
	}
}
