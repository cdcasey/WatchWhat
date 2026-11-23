package server_test

import (
	"github.com/chriscasey/watchwhat/internal/tmdb/tmdbtest"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestFullVsFragment(t *testing.T) {
	h := newHarness(t)
	tests := []struct {
		name    string
		headers map[string]string
		doc     bool
	}{
		{"plain", nil, true},
		{"htmx", map[string]string{"HX-Request": "true"}, false},
		{"boosted", map[string]string{"HX-Request": "true", "HX-Boosted": "true"}, true},
		{"history restore", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(req{method: http.MethodGet, path: "/watching", headers: tc.headers})
			body := rec.Body.String()
			if rec.Code != 200 {
				t.Fatalf("status %d", rec.Code)
			}
			if !strings.Contains(rec.Header().Get("Vary"), "HX-Request") {
				t.Error("missing Vary: HX-Request")
			}
			if isDocument(body) != tc.doc {
				t.Fatalf("document=%v want %v", isDocument(body), tc.doc)
			}
			if tc.doc {
				mustContain(t, body, `id="main"`, `id="nav"`, `id="toasts"`, `name="htmx-config"`, `/static/css/app.css?v=`, `/static/js/htmx.min.js?v=`)
			} else {
				mustContain(t, body, "<title>Watching · WatchWhat</title>", `hx-swap-oob="true"`, `aria-current="page"`)
			}
		})
	}
}

func TestNavActiveSection(t *testing.T) {
	h := newHarness(t)
	body := h.get("/backlog").Body.String()
	re := regexp.MustCompile(`<a[^>]*href="/backlog"[^>]*aria-current="page"`)
	if !re.MatchString(body) {
		t.Fatal("Backlog not marked active")
	}
	for _, s := range []string{"/watching", "/completed", "/dropped", "/schedule", "/search"} {
		mustContain(t, body, `href="`+s+`"`)
	}
}

func TestAssetsSameOrigin(t *testing.T) {
	h := newHarness(t)
	body := h.get("/").Body.String()
	for _, m := range regexp.MustCompile(`(?:src|href)="([^"]+\.(?:js|css)[^"]*)"`).FindAllStringSubmatch(body, -1) {
		if !strings.HasPrefix(m[1], "/static/") {
			t.Errorf("asset not same-origin: %s", m[1])
		}
	}
	css := regexp.MustCompile(`href="(/static/css/app\.css\?v=[0-9a-f]{8})"`).FindStringSubmatch(body)
	if css == nil {
		t.Fatal("no versioned stylesheet")
	}
	rec := h.get(css[1])
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestNotFoundIsFullHTML(t *testing.T) {
	h := newHarness(t)
	for _, p := range []string{"/nope", "/media/999999", "/media/abc"} {
		rec := h.get(p)
		body := rec.Body.String()
		if rec.Code != 404 || !isDocument(body) || !strings.Contains(body, `href="/"`) {
			t.Errorf("%s: code=%d doc=%v", p, rec.Code, isDocument(body))
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: content-type %q", p, ct)
		}
	}
}

func TestMethodNotAllowedPreserved(t *testing.T) {
	h := newHarness(t)
	if rec := h.do(req{method: http.MethodPost, path: "/watching"}); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestHTMXErrorIsToast(t *testing.T) {
	h := newHarness(t)
	h.fake.Set(func(f *tmdbtest.Fake) { f.Down = true })
	rec := h.hx(http.MethodGet, "/search/results", url.Values{"q": {"shogun"}})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("HX-Retarget") != "#toasts" || rec.Header().Get("HX-Reswap") != "beforeend" {
		t.Fatalf("headers: %v", rec.Header())
	}
	body := rec.Body.String()
	if isDocument(body) || !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "TMDB is unreachable") {
		t.Fatalf("body: %s", body)
	}
	// Every HTMX error is retargeted, including 404s.
	rec = h.hx(http.MethodPut, "/episodes/424242/watched", url.Values{"ctx": {"watching"}})
	if rec.Code != 404 || rec.Header().Get("HX-Retarget") != "#toasts" {
		t.Fatalf("404: code=%d retarget=%q", rec.Code, rec.Header().Get("HX-Retarget"))
	}
}

func TestInternalErrorHidesDetail(t *testing.T) {
	h := newHarness(t)
	h.app.DB.Close()
	for _, htmx := range []bool{false, true} {
		rec := h.do(req{method: http.MethodGet, path: "/watching", htmx: htmx})
		body := rec.Body.String()
		if rec.Code != 500 || !strings.Contains(body, "Something went wrong") || strings.Contains(body, "database is closed") {
			t.Fatalf("htmx=%v code=%d body=%s", htmx, rec.Code, body)
		}
	}
	if !strings.Contains(h.logs.String(), "database is closed") {
		t.Fatal("error detail not logged")
	}
}

func TestHealthz(t *testing.T) {
	h := newHarness(t)
	if rec := h.get("/healthz"); rec.Code != 200 {
		t.Fatalf("healthy: %d", rec.Code)
	}
	h.app.DB.Close()
	if rec := h.get("/healthz"); rec.Code != 503 {
		t.Fatalf("closed DB: %d", rec.Code)
	}
}

func TestNoAuthPrompt(t *testing.T) {
	h := newHarness(t)
	for _, p := range []string{"/", "/watching", "/backlog", "/completed", "/dropped", "/schedule", "/search"} {
		rec := h.get(p)
		if rec.Code != 200 {
			t.Errorf("%s: %d", p, rec.Code)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s set cookies", p)
		}
	}
}
