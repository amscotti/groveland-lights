// Package web serves the application. Features live in internal/web/<feature>.go
// files and register themselves via init(), so no two tasks need to touch the same
// shared file (plan §7 "shared files" rule).
package web

import (
	"log/slog"
	"net/http"

	assets "github.com/amscotti/groveland-lights"
	"github.com/amscotti/groveland-lights/internal/config"
)

// route is one mux registration. Method pattern per Go 1.22+ ServeMux.
type route struct {
	pattern string
	handler http.HandlerFunc
}

// routes accumulates init()-time registrations.
var routes []route

// register is called from feature files' init() functions.
func register(r route) { routes = append(routes, r) }

// Server carries shared dependencies for handlers.
type Server struct {
	Cfg    config.Config
	Log    *slog.Logger
	tpl    *templates
	routes []route
	mux    *http.ServeMux
}

// New assembles the server: templates, middleware and all registered routes.
func New(cfg config.Config, log *slog.Logger) (*Server, error) {
	tpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	s := &Server{Cfg: cfg, Log: log, tpl: tpl, routes: routes}
	return s, nil
}

// Handler builds the full middleware chain over the mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets.Static())))

	for _, rt := range s.routes {
		mux.HandleFunc(rt.pattern, rt.handler)
	}
	s.mux = mux

	var h http.Handler = mux
	h = s.withRequestID(h)
	h = s.withLogging(h)
	h = s.withRecover(h)
	h = withSecurityHeaders(h)
	h = s.withServer(h)
	return h
}

// Routes exposes registrations for the HTMX contract test.
func (s *Server) Routes() []route { return s.routes }

// --- middleware ---

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' https://challenges.cloudflare.com; "+
				"style-src 'self'; img-src 'self' data: blob:; worker-src 'self'; "+
				"connect-src 'self' https://tiles.openfreemap.org; "+
				"frame-src https://challenges.cloudflare.com; base-uri 'none'; "+
				"form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(self), camera=(self)")
		next.ServeHTTP(&statusRecorder{ResponseWriter: w, status: 200}, r)
	})
}

func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.Log.Error("panic", "err", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set("X-Request-Id", id)
		r = r.WithContext(withRequestIDCtx(r.Context(), id))
		next.ServeHTTP(w, r)
	})
}

// withLogging logs method, route pattern (never the raw path), status, duration
// and request id. No IPs, emails or coordinates (plan §5.4). The pattern is
// resolved via the mux (outer middleware sees the pre-match request).
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := nowFunc()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		pattern := r.Pattern
		if pattern == "" && s.mux != nil {
			if _, p := s.mux.Handler(r); p != "" {
				pattern = p
			}
		}
		s.Log.Info("request",
			"method", r.Method,
			"pattern", pattern,
			"status", rec.status,
			"dur_ms", nowFunc().Sub(start).Milliseconds(),
			"req_id", requestIDOf(r.Context()),
		)
	})
}

// --- home page ---

type homeData struct {
	Title    string
	TestEnv  bool
	EnvLabel string
}

func init() {
	register(route{pattern: "GET /{$}", handler: home})
}

func home(w http.ResponseWriter, r *http.Request) {
	s := serverFrom(r.Context())
	data := homeData{Title: "Groveland Lights", TestEnv: s.Cfg.IsTest(), EnvLabel: string(s.Cfg.AppEnv)}
	if err := s.tpl.renderPage(w, "index.html", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
