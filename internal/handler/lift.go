package handler

import (
	"log/slog"
	"net/http"
)

// LiftListData holds template data for the lift list page.
type LiftListData struct {
	Lifts []LiftItem
	Empty bool
}

// LiftItem represents a lift in the list view.
type LiftItem struct {
	ID        int64
	LiftType  string
	CreatedAt string
}

// HandleListLifts renders the lift list page at GET /.
func (s *Server) HandleListLifts(w http.ResponseWriter, r *http.Request) {
	lifts, err := s.Queries.ListLifts(r.Context())
	if err != nil {
		slog.Error("failed to list lifts", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := LiftListData{
		Empty: len(lifts) == 0,
	}
	for _, l := range lifts {
		data.Lifts = append(data.Lifts, LiftItem{
			ID:        l.ID,
			LiftType:  l.LiftType,
			CreatedAt: l.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.Templates.ExecuteTemplate(w, "lift-list.html", data); err != nil {
		slog.Error("failed to render template", "error", err)
	}
}

// HandleCreateLift handles POST /lifts (stub for Story 1.2).
func (s *Server) HandleCreateLift(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

// HandleGetLift handles GET /lifts/{id} (stub for Story 1.3).
func (s *Server) HandleGetLift(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

// HandleDeleteLift handles DELETE /lifts/{id} (stub for later stories).
func (s *Server) HandleDeleteLift(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

// HandleLiftEvents handles GET /lifts/{id}/events SSE (stub for later stories).
func (s *Server) HandleLiftEvents(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

// HandleLiftCoaching handles GET /lifts/{id}/coaching (stub for later stories).
func (s *Server) HandleLiftCoaching(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

// HandleLiftStatus handles GET /lifts/{id}/status (stub for later stories).
func (s *Server) HandleLiftStatus(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}
