package handler

import (
	"html/template"
	"net/http"

	"press-out/internal/storage/sqlc"
)

// Server holds dependencies for HTTP handlers.
type Server struct {
	Queries   *sqlc.Queries
	Templates *template.Template
	DataDir   string
}

// RegisterRoutes sets up all HTTP routes on the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Pages
	mux.HandleFunc("GET /{$}", s.HandleListLifts)

	// Lift CRUD
	mux.HandleFunc("POST /lifts", s.HandleCreateLift)
	mux.HandleFunc("GET /lifts/{id}", s.HandleGetLift)
	mux.HandleFunc("DELETE /lifts/{id}", s.HandleDeleteLift)

	// SSE
	mux.HandleFunc("GET /lifts/{id}/events", s.HandleLiftEvents)

	// HTMX partials
	mux.HandleFunc("GET /lifts/{id}/coaching", s.HandleLiftCoaching)
	mux.HandleFunc("GET /lifts/{id}/status", s.HandleLiftStatus)

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
}
