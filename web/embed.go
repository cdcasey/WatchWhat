// Package web embeds static assets and serves them with content-hashed URLs.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var embedded embed.FS

// Static is the embedded static/ tree.
var Static = must(fs.Sub(embedded, "static"))

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// Manifest maps asset paths (relative to static/) to content hashes.
type Manifest struct {
	fsys   fs.FS
	hashes map[string]string
}

// NewManifest hashes every file in fsys (first 8 hex chars of SHA-256).
func NewManifest(fsys fs.FS) (Manifest, error) {
	m := Manifest{fsys: fsys, hashes: map[string]string{}}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		m.hashes[p] = hex.EncodeToString(sum[:])[:8]
		return nil
	})
	return m, err
}

// URL returns the versioned URL for an asset, e.g. /static/css/app.css?v=ab12cd34.
func (m Manifest) URL(p string) string {
	p = strings.TrimPrefix(p, "/")
	if h, ok := m.hashes[p]; ok {
		return "/static/" + p + "?v=" + h
	}
	return "/static/" + p
}

// Handler serves the manifest's FS under /static/. Versioned requests are
// cached immutably; unversioned ones must revalidate.
func (m Manifest) Handler() http.Handler {
	files := http.StripPrefix("/static/", http.FileServerFS(m.fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r) // no directory listings
			return
		}
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// Alias serves the asset p at a fixed root URL (e.g. /favicon.ico, which
// browsers request on their own). It can't be versioned, so cache for a day.
func (m Manifest) Alias(p string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFileFS(w, r, m.fsys, p)
	})
}
