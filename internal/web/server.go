// Package web serves the Snip dashboard and the public redirects.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/claretnnamocha/snip/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Config configures the web server.
type Config struct {
	// BaseURL is the public origin short links are shown with, e.g. https://snip.example.ng
	BaseURL  string
	Location *time.Location
	Logger   *slog.Logger
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
}

// Server handles HTTP requests.
type Server struct {
	store   *store.Store
	cfg     Config
	pages   map[string]*template.Template
	cache   *linkCache
	mux     *http.ServeMux
	flashMu sync.Mutex
	flash   *flash
}

type flash struct {
	Kind    string // "success" or "error"
	Message string
}

// New builds a Server.
func New(st *store.Store, cfg Config) (*Server, error) {
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	s := &Server{store: st, cfg: cfg, cache: newLinkCache(1000), mux: http.NewServeMux()}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	static, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	s.mux.HandleFunc("GET /{$}", s.handleDashboard)
	s.mux.HandleFunc("GET /links/new", s.handleNewLink)
	s.mux.HandleFunc("POST /links", s.handleCreateLink)
	s.mux.HandleFunc("GET /links/{id}", s.handleShowLink)
	s.mux.HandleFunc("GET /links/{id}/edit", s.handleEditLink)
	s.mux.HandleFunc("POST /links/{id}", s.handleUpdateLink)
	s.mux.HandleFunc("POST /links/{id}/delete", s.handleDeleteLink)
	s.mux.HandleFunc("POST /links/{id}/reset", s.handleResetStats)

	s.mux.HandleFunc("GET /{alias}", s.handleRedirect)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	s.mux.ServeHTTP(rec, r)
	s.cfg.Logger.Debug("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (s *Server) parseTemplates() error {
	funcs := template.FuncMap{
		"date": func(t time.Time) string {
			return t.In(s.cfg.Location).Format("2 Jan 2006")
		},
		"datetime": func(t time.Time) string {
			return t.In(s.cfg.Location).Format("2 Jan 2006, 15:04")
		},
		"day": func(iso string) string {
			d, err := time.Parse(time.DateOnly, iso)
			if err != nil {
				return iso
			}
			return d.Format("2 Jan 2006")
		},
		"shortURL": s.shortURL,
		"host": func(raw string) string {
			if u, err := url.Parse(raw); err == nil && u.Host != "" {
				return u.Host
			}
			return raw
		},
	}

	s.pages = make(map[string]*template.Template)
	for _, page := range []string{"dashboard", "form", "show", "message"} {
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/"+page+".html")
		if err != nil {
			return fmt.Errorf("parse %s template: %w", page, err)
		}
		s.pages[page] = t
	}
	return nil
}

func (s *Server) shortURL(alias string) string {
	return s.cfg.BaseURL + "/" + alias
}

func (s *Server) setFlash(kind, message string) {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	s.flash = &flash{Kind: kind, Message: message}
}

func (s *Server) takeFlash() *flash {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	f := s.flash
	s.flash = nil
	return f
}

// render executes a page template with the shared layout data.
func (s *Server) render(w http.ResponseWriter, status int, page string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["Flash"] = s.takeFlash()
	data["BaseURL"] = s.cfg.BaseURL

	var buf strings.Builder
	if err := s.pages[page].Execute(&buf, data); err != nil {
		s.serverError(w, fmt.Errorf("render %s: %w", page, err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, buf.String())
}

func (s *Server) message(w http.ResponseWriter, status int, title, body string) {
	s.render(w, status, "message", map[string]any{"Title": title, "Body": body})
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.cfg.Logger.Error("request failed", "error", err)
	http.Error(w, "Something went wrong on our side. Please try again.", http.StatusInternalServerError)
}

func (s *Server) notFound(w http.ResponseWriter) {
	s.message(w, http.StatusNotFound, "Link not found", "We could not find that link. Check the address and try again.")
}
