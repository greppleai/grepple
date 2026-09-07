package server

import (
	"fmt"
	"net/http"
)

// Config holds runtime options for the HTTP server.
type Config struct {
	Addr    string
	Timeout int
}

// Server wraps the standard library HTTP server.
type Server struct {
	cfg    Config
	routes map[string]http.HandlerFunc
}

// NewServer builds a Server from the given config.
func NewServer(cfg Config) *Server {
	return &Server{cfg: cfg, routes: map[string]http.HandlerFunc{}}
}

// Handle registers a handler for a path.
func (s *Server) Handle(path string, fn http.HandlerFunc) {
	s.routes[path] = fn
}

// ListenAndServe starts the server and blocks.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	for path, fn := range s.routes {
		mux.HandleFunc(path, fn)
	}
	fmt.Printf("listening on %s\n", s.cfg.Addr)
	return http.ListenAndServe(s.cfg.Addr, mux)
}
