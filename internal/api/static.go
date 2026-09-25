package api

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed assets/*
var embeddedAssets embed.FS

func embeddedStatic() fs.FS {
	value, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		return embeddedAssets
	}
	return value
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if s.staticFS == nil {
		http.NotFound(w, r)
		return
	}
	requestPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if requestPath == "." || requestPath == "" {
		s.serveIndex(w, r)
		return
	}
	if file, err := s.staticFS.Open(requestPath); err == nil {
		_ = file.Close()
		switch {
		case requestPath == "manifest.webmanifest":
			data, readErr := fs.ReadFile(s.staticFS, requestPath)
			if readErr != nil {
				http.NotFound(w, r)
				return
			}
			text := string(data)
			if s.profileName != "" && s.profileName != "v1" {
				label := "mctrl " + strings.ToUpper(s.profileName)
				text = strings.ReplaceAll(text, `"name": "mctrl"`, `"name": "`+label+`"`)
				text = strings.ReplaceAll(text, `"short_name": "mctrl"`, `"short_name": "`+label+`"`)
			}
			w.Header().Set("Content-Type", "application/manifest+json")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(text))
			return
		case requestPath == "sw.js":
			data, readErr := fs.ReadFile(s.staticFS, requestPath)
			if readErr != nil {
				http.NotFound(w, r)
				return
			}
			text := string(data)
			if s.profileName != "" && s.profileName != "v1" {
				text = strings.ReplaceAll(text, "mctrl-shell-", "mctrl-shell-"+s.profileName+"-")
			}
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(text))
			return
		case strings.HasPrefix(requestPath, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r)
		return
	}
	// The UI is a single-page application. Unknown non-API routes resolve to
	// index.html so browser navigation and PWA reloads remain usable.
	s.serveIndex(w, r)
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(s.staticFS, "index.html")
	if err != nil {
		http.Error(w, "web assets unavailable", http.StatusInternalServerError)
		return
	}
	runtimeName := s.profileName
	if runtimeName == "" {
		runtimeName = "v1"
	}
	if runtimeName != "v1" {
		data = []byte(strings.ReplaceAll(string(data), `content="v1"`, `content="`+runtimeName+`"`))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
