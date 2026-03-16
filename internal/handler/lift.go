package handler

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"press-out/internal/storage"
	"press-out/internal/storage/sqlc"
)

// LiftListData holds template data for the lift list page.
type LiftListData struct {
	Lifts []LiftItem
	Empty bool
}

// LiftItem represents a lift in the list view.
type LiftItem struct {
	ID           int64
	LiftType     string
	CreatedAt    string
	DisplayType  string
	DisplayDate  string
	HasThumbnail bool
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
		thumbPath := storage.LiftFile(s.DataDir, l.ID, storage.FileThumbnail)
		_, thumbErr := os.Stat(thumbPath)
		data.Lifts = append(data.Lifts, LiftItem{
			ID:           l.ID,
			LiftType:     l.LiftType,
			CreatedAt:    l.CreatedAt,
			DisplayType:  formatLiftType(l.LiftType),
			DisplayDate:  formatDate(l.CreatedAt),
			HasThumbnail: thumbErr == nil,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.Templates["lift-list.html"].Execute(w, data); err != nil {
		slog.Error("failed to render template", "error", err)
	}
}

const maxUploadSize = 300 * 1024 * 1024 // 300MB

// HandleCreateLift handles POST /lifts — upload a video with lift type.
func (s *Server) HandleCreateLift(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			http.Error(w, "File too large (max 300MB)", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	liftType := r.FormValue("lift_type")
	if liftType != "snatch" && liftType != "clean" && liftType != "clean_and_jerk" {
		http.Error(w, "Invalid lift type", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		http.Error(w, "No video file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !isValidVideo(header.Filename, header.Header.Get("Content-Type")) {
		http.Error(w, "Invalid file type (must be mp4 or mov)", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	lift, err := s.Queries.CreateLift(r.Context(), sqlc.CreateLiftParams{
		LiftType:  liftType,
		CreatedAt: now,
	})
	if err != nil {
		slog.Error("failed to create lift record", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := storage.CreateLiftDir(s.DataDir, lift.ID); err != nil {
		slog.Error("failed to create lift dir", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	dstPath := storage.LiftFile(s.DataDir, lift.ID, storage.FileOriginal)
	dst, err := os.Create(dstPath)
	if err != nil {
		slog.Error("failed to create file", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		slog.Error("failed to save file", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	slog.Info("lift uploaded", "id", lift.ID, "type", liftType)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func isValidVideo(filename, contentType string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".mp4" || ext == ".mov" {
		return true
	}
	mt, _, _ := mime.ParseMediaType(contentType)
	return mt == "video/mp4" || mt == "video/quicktime"
}

// formatLiftType returns a human-readable lift type label.
func formatLiftType(lt string) string {
	switch lt {
	case "snatch":
		return "Snatch"
	case "clean":
		return "Clean"
	case "clean_and_jerk":
		return "Clean & Jerk"
	default:
		return lt
	}
}

// formatDate converts an RFC3339 timestamp to a human-readable date.
func formatDate(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("Jan 2, 2006")
}

// LiftDetailData holds template data for the lift detail page.
type LiftDetailData struct {
	ID          int64
	LiftType    string
	DisplayType string
	DisplayDate string
	VideoSrc    string
}

// HandleGetLift handles GET /lifts/{id} — renders the lift detail page.
func (s *Server) HandleGetLift(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	lift, err := s.Queries.GetLift(r.Context(), id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	videoFile := bestVideoFile(s.DataDir, lift.ID)

	data := LiftDetailData{
		ID:          lift.ID,
		LiftType:    lift.LiftType,
		DisplayType: formatLiftType(lift.LiftType),
		DisplayDate: formatDate(lift.CreatedAt),
		VideoSrc:    fmt.Sprintf("/data/lifts/%d/%s", lift.ID, videoFile),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.Templates["lift-detail.html"].Execute(w, data); err != nil {
		slog.Error("failed to render template", "error", err)
	}
}

// bestVideoFile returns the best available video filename for a lift,
// checking in priority order: cropped > trimmed > original.
func bestVideoFile(dataDir string, liftID int64) string {
	for _, f := range []string{storage.FileCropped, storage.FileTrimmed, storage.FileOriginal} {
		if _, err := os.Stat(storage.LiftFile(dataDir, liftID, f)); err == nil {
			return f
		}
	}
	return storage.FileOriginal
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
