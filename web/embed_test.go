package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestURLChangesWithContent(t *testing.T) {
	a, _ := NewManifest(fstest.MapFS{"css/app.css": {Data: []byte("body{}")}})
	b, _ := NewManifest(fstest.MapFS{"css/app.css": {Data: []byte("body{color:red}")}})
	ua, ub := a.URL("css/app.css"), b.URL("css/app.css")
	if !strings.HasPrefix(ua, "/static/css/app.css?v=") || ua == ub {
		t.Fatalf("urls: %q %q", ua, ub)
	}
}

func TestCacheHeaders(t *testing.T) {
	m, _ := NewManifest(fstest.MapFS{"js/x.js": {Data: []byte("1")}})
	h := m.Handler()
	for _, tc := range []struct{ url, want string }{
		{m.URL("js/x.js"), "public, max-age=31536000, immutable"},
		{"/static/js/x.js", "no-cache"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != tc.want {
			t.Errorf("%s: code=%d cache=%q", tc.url, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestEmbeddedAssetsPresent(t *testing.T) {
	m, err := NewManifest(Static)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.URL("js/htmx.min.js"), "?v=") {
		t.Fatal("htmx.min.js not embedded")
	}
}
