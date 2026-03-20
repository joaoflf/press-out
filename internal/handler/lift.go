package handler

import (
	"context"
	"encoding/json"
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

	"press-out/internal/pose"
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
	Processing   bool
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
			Processing:   s.Broker != nil && s.Broker.IsProcessing(l.ID),
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

	// Save optional keypoints.json from client-side pose estimation.
	saveKeypoints(r, s.DataDir, lift.ID)

	slog.Info("lift uploaded", "id", lift.ID, "type", liftType)

	if s.Pipeline != nil {
		go s.Pipeline.Run(context.Background(), lift.ID, s.DataDir)
	}

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

// saveKeypoints reads the optional "keypoints" multipart field, validates it
// as a pose.Result JSON, and writes it to the lift directory. Any failure is
// logged but does not abort the upload (graceful degradation).
func saveKeypoints(r *http.Request, dataDir string, liftID int64) {
	kpFile, _, err := r.FormFile("keypoints")
	if err != nil {
		return // field absent — normal when client-side estimation was skipped
	}
	defer kpFile.Close()

	data, err := io.ReadAll(kpFile)
	if err != nil {
		slog.Warn("failed to read keypoints field", "lift_id", liftID, "error", err)
		return
	}

	var result pose.Result
	if err := json.Unmarshal(data, &result); err != nil {
		slog.Warn("invalid keypoints JSON", "lift_id", liftID, "error", err)
		return
	}
	if result.SourceWidth == 0 || result.SourceHeight == 0 || len(result.Frames) == 0 {
		slog.Warn("keypoints JSON missing required fields", "lift_id", liftID)
		return
	}

	kpPath := storage.LiftFile(dataDir, liftID, storage.FileKeypoints)
	if err := os.WriteFile(kpPath, data, 0644); err != nil {
		slog.Error("failed to write keypoints.json", "lift_id", liftID, "error", err)
	}
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
	Processing  bool
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
		Processing:  s.Broker != nil && s.Broker.IsProcessing(lift.ID),
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

// HandleDeleteLift handles DELETE /lifts/{id} — removes lift record and files.
func (s *Server) HandleDeleteLift(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Verify lift exists before deleting.
	if _, err := s.Queries.GetLift(r.Context(), id); err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Delete DB record first.
	if err := s.Queries.DeleteLift(r.Context(), id); err != nil {
		slog.Error("failed to delete lift record", "id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Remove lift directory and all files. Log but don't fail if removal errors.
	if err := storage.RemoveLiftDir(s.DataDir, id); err != nil {
		slog.Error("failed to remove lift directory", "id", id, "error", err)
	}

	slog.Info("lift deleted", "id", id)

	// HTMX redirect via header.
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

// HandleLiftEvents handles GET /lifts/{id}/events — streams SSE pipeline progress.
func (s *Server) HandleLiftEvents(w http.ResponseWriter, r *http.Request) {
	if s.Broker == nil {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.Broker.Subscribe(id)
	defer s.Broker.Unsubscribe(id, ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, event.Data)
			flusher.Flush()
		}
	}
}

// HandleLiftCoaching handles GET /lifts/{id}/coaching (stub for later stories).
func (s *Server) HandleLiftCoaching(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

// HandleLiftStatus handles GET /lifts/{id}/status (stub for later stories).
func (s *Server) HandleLiftStatus(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}
