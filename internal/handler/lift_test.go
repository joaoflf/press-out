package handler

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"press-out/internal/pipeline"
	"press-out/internal/sse"
	"press-out/internal/storage"
	"press-out/internal/storage/sqlc"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schemaDir := filepath.Join(tmpDir, "schema")
	os.MkdirAll(schemaDir, 0755)
	migration := `CREATE TABLE IF NOT EXISTS lifts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		lift_type TEXT NOT NULL,
		created_at TEXT NOT NULL,
		coaching_cue TEXT,
		coaching_diagnosis TEXT
	);`
	os.WriteFile(filepath.Join(schemaDir, "001_initial.sql"), []byte(migration), 0644)
	if err := storage.RunMigrations(db, schemaDir); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	queries := sqlc.New(db)

	base := template.Must(template.New("base.html").Parse(`<!DOCTYPE html><html>{{block "content" .}}{{end}}</html>`))
	template.Must(base.New("lift-list-item").Parse(
		`<a href="/lifts/{{.ID}}" class="lift-item"><span class="type">{{.DisplayType}}</span><span class="date">{{.DisplayDate}}</span>{{if .HasThumbnail}}<img src="/data/lifts/{{.ID}}/thumbnail.jpg">{{end}}</a>`))
	template.Must(base.New("upload-modal").Parse(`<dialog id="upload-modal"></dialog>`))
	template.Must(base.New("video-player").Parse(
		`<div class="relative w-full sticky top-0 z-10 bg-black"><video class="w-full" controls playsinline preload="metadata" src="{{.VideoSrc}}"></video></div>`))

	listClone := template.Must(base.Clone())
	template.Must(listClone.New("lift-list.html").Parse(
		`{{template "base.html" .}}{{define "content"}}<div>{{if .Empty}}<p>No lifts yet</p>{{else}}{{range .Lifts}}{{template "lift-list-item" .}}{{end}}{{end}}<button onclick="document.getElementById('upload-modal').showModal()">Upload Lift</button></div>{{template "upload-modal" .}}{{end}}`))

	detailClone := template.Must(base.Clone())
	template.Must(detailClone.New("lift-detail.html").Parse(
		`{{template "base.html" .}}{{define "content"}}<div><a href="/" class="back-btn" aria-label="Back to lift list">Back</a><h1>{{.DisplayType}}</h1><p>{{.DisplayDate}}</p>{{template "video-player" .}}<div id="coaching-section"></div><div id="phase-timeline-section"></div><div id="metrics-section"></div></div>{{end}}`))

	tmplMap := map[string]*template.Template{
		"lift-list.html":   listClone,
		"lift-detail.html": detailClone,
	}

	return &Server{
		Queries:   queries,
		Templates: tmplMap,
		DataDir:   tmpDir,
	}
}

func TestHandleListLiftsEmpty(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "No lifts yet") {
		t.Error("expected empty state message")
	}
	if !strings.Contains(body, "Upload Lift") {
		t.Error("expected upload button")
	}
}

func TestHandleListLiftsWithData(t *testing.T) {
	srv := setupTestServer(t)

	_, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("CreateLift: %v", err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if strings.Contains(body, "No lifts yet") {
		t.Error("should not show empty state when lifts exist")
	}
	if !strings.Contains(body, "Snatch") {
		t.Error("expected lift type in response")
	}
}

// --- Story 1.2: Upload tests ---

func createMultipartRequest(t *testing.T, filename, liftType string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("video", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := writer.WriteField("lift_type", liftType); err != nil {
		t.Fatalf("write field: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/lifts", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestHandleCreateLift_ValidUpload(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	content := []byte("fake video content")
	req := createMultipartRequest(t, "test.mp4", "snatch", content)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}

	// Verify DB record
	lifts, err := srv.Queries.ListLifts(context.Background())
	if err != nil {
		t.Fatalf("list lifts: %v", err)
	}
	if len(lifts) != 1 {
		t.Fatalf("expected 1 lift, got %d", len(lifts))
	}
	if lifts[0].LiftType != "snatch" {
		t.Errorf("expected lift type snatch, got %s", lifts[0].LiftType)
	}

	// Verify file persisted to correct path
	filePath := storage.LiftFile(srv.DataDir, lifts[0].ID, storage.FileOriginal)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if string(data) != string(content) {
		t.Error("persisted file content mismatch")
	}
}

func TestHandleCreateLift_MovFile(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	content := []byte("mov video content")
	req := createMultipartRequest(t, "video.mov", "clean_and_jerk", content)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}

	lifts, _ := srv.Queries.ListLifts(context.Background())
	if len(lifts) != 1 {
		t.Fatalf("expected 1 lift, got %d", len(lifts))
	}
	if lifts[0].LiftType != "clean_and_jerk" {
		t.Errorf("expected clean_and_jerk, got %s", lifts[0].LiftType)
	}

	filePath := storage.LiftFile(srv.DataDir, lifts[0].ID, storage.FileOriginal)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("file should be persisted")
	}
}

func TestHandleCreateLift_InvalidFileType(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := createMultipartRequest(t, "test.txt", "snatch", []byte("not a video"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	// Verify no DB record created
	lifts, _ := srv.Queries.ListLifts(context.Background())
	if len(lifts) != 0 {
		t.Error("no lift should be created for invalid file type")
	}
}

func TestHandleCreateLift_InvalidLiftType(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := createMultipartRequest(t, "test.mp4", "deadlift", []byte("video"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateLift_NoFile(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("lift_type", "snatch")
	writer.Close()

	req := httptest.NewRequest("POST", "/lifts", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing file, got %d", w.Code)
	}
}

func TestHandleCreateLift_CleanType(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := createMultipartRequest(t, "lift.mp4", "clean", []byte("video data"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}

	lifts, _ := srv.Queries.ListLifts(context.Background())
	if len(lifts) != 1 || lifts[0].LiftType != "clean" {
		t.Errorf("expected clean lift, got %v", lifts)
	}
}

// --- Story 1.3: Browse Lift History tests ---

func TestHandleListLiftsReverseChronological(t *testing.T) {
	srv := setupTestServer(t)

	// Create lifts with different timestamps
	srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean",
		CreatedAt: "2026-03-15T00:00:00Z",
	})
	srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean_and_jerk",
		CreatedAt: "2026-02-10T00:00:00Z",
	})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// Verify all lifts present with display types
	if !strings.Contains(body, "Snatch") {
		t.Error("expected Snatch in response")
	}
	if !strings.Contains(body, "Clean &amp; Jerk") {
		t.Error("expected Clean & Jerk in response")
	}
	if !strings.Contains(body, "Clean") {
		t.Error("expected Clean in response")
	}

	// Verify reverse chronological order: Mar 15 before Feb 10 before Jan 1
	mar := strings.Index(body, "Mar 15, 2026")
	feb := strings.Index(body, "Feb 10, 2026")
	jan := strings.Index(body, "Jan 1, 2026")
	if mar == -1 || feb == -1 || jan == -1 {
		t.Fatalf("expected formatted dates, got body: %s", body)
	}
	if mar > feb || feb > jan {
		t.Error("lifts not in reverse chronological order")
	}
}

func TestHandleListLiftsDisplayType(t *testing.T) {
	srv := setupTestServer(t)

	srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean_and_jerk",
		CreatedAt: "2026-01-01T00:00:00Z",
	})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Clean &amp; Jerk") {
		t.Errorf("expected human-readable 'Clean & Jerk', got body: %s", body)
	}
}

func TestHandleListLiftsThumbnail(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create thumbnail file
	if err := storage.CreateLiftDir(srv.DataDir, lift.ID); err != nil {
		t.Fatal(err)
	}
	thumbPath := storage.LiftFile(srv.DataDir, lift.ID, storage.FileThumbnail)
	if err := os.WriteFile(thumbPath, []byte("fake jpg"), 0644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "thumbnail.jpg") {
		t.Errorf("expected thumbnail img tag, got body: %s", body)
	}
}

func TestHandleListLiftsNoThumbnail(t *testing.T) {
	srv := setupTestServer(t)

	srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "thumbnail.jpg") {
		t.Error("should not show thumbnail when file doesn't exist")
	}
}

func TestFormatLiftType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"snatch", "Snatch"},
		{"clean", "Clean"},
		{"clean_and_jerk", "Clean & Jerk"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := formatLiftType(tt.input)
		if got != tt.want {
			t.Errorf("formatLiftType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-01-01T00:00:00Z", "Jan 1, 2026"},
		{"2026-03-15T14:30:00Z", "Mar 15, 2026"},
		{"not-a-date", "not-a-date"},
	}
	for _, tt := range tests {
		got := formatDate(tt.input)
		if got != tt.want {
			t.Errorf("formatDate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDataFileServing(t *testing.T) {
	srv := setupTestServer(t)

	// Create a lift dir with a thumbnail
	liftDir := filepath.Join(srv.DataDir, "lifts", "1")
	os.MkdirAll(liftDir, 0755)
	os.WriteFile(filepath.Join(liftDir, "thumbnail.jpg"), []byte("fake jpg data"), 0644)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/data/lifts/1/thumbnail.jpg", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for thumbnail, got %d", w.Code)
	}
	if w.Body.String() != "fake jpg data" {
		t.Error("thumbnail content mismatch")
	}
}

// --- Story 1.4: View Lift Detail tests ---

func TestHandleGetLift_ValidID(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-03-15T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create original video file
	if err := storage.CreateLiftDir(srv.DataDir, lift.ID); err != nil {
		t.Fatal(err)
	}
	videoPath := storage.LiftFile(srv.DataDir, lift.ID, storage.FileOriginal)
	if err := os.WriteFile(videoPath, []byte("fake video"), 0644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", fmt.Sprintf("/lifts/%d", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "Snatch") {
		t.Error("expected lift type 'Snatch' in response")
	}
	if !strings.Contains(body, "Mar 15, 2026") {
		t.Error("expected formatted date in response")
	}
	if !strings.Contains(body, "original.mp4") {
		t.Error("expected video source in response")
	}
	if !strings.Contains(body, `href="/"`) {
		t.Error("expected back button link to /")
	}
}

func TestHandleGetLift_InvalidID(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/lifts/999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetLift_NonNumericID(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/lifts/abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetLift_BestVideoFile(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.CreateLiftDir(srv.DataDir, lift.ID); err != nil {
		t.Fatal(err)
	}

	// Create both original and cropped — should prefer cropped
	os.WriteFile(storage.LiftFile(srv.DataDir, lift.ID, storage.FileOriginal), []byte("orig"), 0644)
	os.WriteFile(storage.LiftFile(srv.DataDir, lift.ID, storage.FileCropped), []byte("crop"), 0644)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", fmt.Sprintf("/lifts/%d", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "cropped.mp4") {
		t.Errorf("expected cropped.mp4 video source, got body: %s", body)
	}
}

func TestHandleGetLift_PlaceholderSections(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", fmt.Sprintf("/lifts/%d", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	for _, id := range []string{"coaching-section", "phase-timeline-section", "metrics-section"} {
		if !strings.Contains(body, id) {
			t.Errorf("expected placeholder section %q in response", id)
		}
	}
}

// --- Story 1.5: Delete Lift tests ---

func TestHandleDeleteLift_RemovesDBRecord(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create lift directory with files
	if err := storage.CreateLiftDir(srv.DataDir, lift.ID); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(storage.LiftFile(srv.DataDir, lift.ID, storage.FileOriginal), []byte("video"), 0644)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/lifts/%d", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify HX-Redirect header
	if redirect := w.Header().Get("HX-Redirect"); redirect != "/" {
		t.Errorf("HX-Redirect=%q, want %q", redirect, "/")
	}

	// Verify DB record is gone
	_, err = srv.Queries.GetLift(context.Background(), lift.ID)
	if err == nil {
		t.Error("expected lift to be deleted from DB")
	}
}

func TestHandleDeleteLift_RemovesFiles(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create lift directory with multiple files
	if err := storage.CreateLiftDir(srv.DataDir, lift.ID); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{storage.FileOriginal, storage.FileTrimmed, storage.FileThumbnail, storage.FileKeypoints} {
		os.WriteFile(storage.LiftFile(srv.DataDir, lift.ID, f), []byte("data"), 0644)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/lifts/%d", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify directory is gone
	liftDir := storage.LiftDir(srv.DataDir, lift.ID)
	if _, err := os.Stat(liftDir); !os.IsNotExist(err) {
		t.Error("expected lift directory to be removed")
	}
}

func TestHandleDeleteLift_InvalidID(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", "/lifts/999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteLift_NonNumericID(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", "/lifts/abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteLift_AbsentFromList(t *testing.T) {
	srv := setupTestServer(t)

	// Create two lifts
	lift1, _ := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean",
		CreatedAt: "2026-02-01T00:00:00Z",
	})

	storage.CreateLiftDir(srv.DataDir, lift1.ID)
	os.WriteFile(storage.LiftFile(srv.DataDir, lift1.ID, storage.FileOriginal), []byte("v"), 0644)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Delete lift1
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/lifts/%d", lift1.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	// Now list lifts — deleted lift should be absent
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Clean") {
		t.Error("expected remaining lift 'Clean' in list")
	}
	// The deleted snatch lift should not appear. Check that there's only one lift link.
	lifts, _ := srv.Queries.ListLifts(context.Background())
	if len(lifts) != 1 {
		t.Errorf("expected 1 lift remaining, got %d", len(lifts))
	}
	if lifts[0].LiftType != "clean" {
		t.Errorf("expected remaining lift to be 'clean', got %q", lifts[0].LiftType)
	}
}

func TestHandleDeleteLift_NoOrphanedFiles(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean_and_jerk",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create all possible files
	if err := storage.CreateLiftDir(srv.DataDir, lift.ID); err != nil {
		t.Fatal(err)
	}
	allFiles := []string{
		storage.FileOriginal, storage.FileTrimmed, storage.FileCropped,
		storage.FileSkeleton, storage.FileThumbnail, storage.FileKeypoints,
	}
	for _, f := range allFiles {
		os.WriteFile(storage.LiftFile(srv.DataDir, lift.ID, f), []byte("data"), 0644)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/lifts/%d", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify no orphaned files
	liftDir := storage.LiftDir(srv.DataDir, lift.ID)
	if _, err := os.Stat(liftDir); !os.IsNotExist(err) {
		t.Error("lift directory still exists — orphaned files remain")
	}
}

// --- Story 2.4: Keypoints upload tests ---

func createMultipartRequestWithKeypoints(t *testing.T, filename, liftType string, content []byte, keypoints []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("video", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := writer.WriteField("lift_type", liftType); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if keypoints != nil {
		kpPart, err := writer.CreateFormFile("keypoints", "keypoints.json")
		if err != nil {
			t.Fatalf("create keypoints field: %v", err)
		}
		if _, err := kpPart.Write(keypoints); err != nil {
			t.Fatalf("write keypoints: %v", err)
		}
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/lifts", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

var sampleKeypointsJSON = []byte(`{
	"sourceWidth": 1920,
	"sourceHeight": 1080,
	"frames": [
		{
			"timeOffsetMs": 0,
			"boundingBox": {"left": 0.1, "top": 0.15, "right": 0.75, "bottom": 0.95},
			"keypoints": [
				{"name": "nose", "x": 0.5, "y": 0.3, "confidence": 0.95}
			]
		}
	]
}`)

func TestHandleCreateLift_IgnoresKeypointsField(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Include a keypoints field — handler should ignore it (pose is now server-side).
	req := createMultipartRequestWithKeypoints(t, "test.mp4", "snatch", []byte("fake video"), sampleKeypointsJSON)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}

	lifts, err := srv.Queries.ListLifts(context.Background())
	if err != nil || len(lifts) != 1 {
		t.Fatalf("expected 1 lift, got %d, err=%v", len(lifts), err)
	}

	// Verify video saved.
	videoPath := storage.LiftFile(srv.DataDir, lifts[0].ID, storage.FileOriginal)
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		t.Error("video file should be persisted")
	}

	// Verify keypoints.json is NOT saved (handler no longer reads this field).
	kpPath := storage.LiftFile(srv.DataDir, lifts[0].ID, storage.FileKeypoints)
	if _, err := os.Stat(kpPath); !os.IsNotExist(err) {
		t.Error("keypoints.json should NOT be saved — pose estimation is now server-side")
	}
}

func TestHandleCreateLift_WithoutKeypoints(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := createMultipartRequest(t, "test.mp4", "snatch", []byte("fake video"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}

	lifts, _ := srv.Queries.ListLifts(context.Background())
	if len(lifts) != 1 {
		t.Fatalf("expected 1 lift, got %d", len(lifts))
	}

	// keypoints.json should NOT exist
	kpPath := storage.LiftFile(srv.DataDir, lifts[0].ID, storage.FileKeypoints)
	if _, err := os.Stat(kpPath); !os.IsNotExist(err) {
		t.Error("keypoints.json should not exist when not provided")
	}
}

func TestHandleCreateLift_ExtraFieldsIgnored(t *testing.T) {
	srv := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Include arbitrary extra data in the keypoints field — handler should ignore it.
	req := createMultipartRequestWithKeypoints(t, "test.mp4", "snatch", []byte("fake video"), []byte("not json"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Upload should succeed (extra fields are ignored).
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}

	lifts, _ := srv.Queries.ListLifts(context.Background())
	if len(lifts) != 1 {
		t.Fatalf("expected 1 lift, got %d", len(lifts))
	}

	// keypoints.json should NOT exist (handler no longer reads this field).
	kpPath := storage.LiftFile(srv.DataDir, lifts[0].ID, storage.FileKeypoints)
	if _, err := os.Stat(kpPath); !os.IsNotExist(err) {
		t.Error("keypoints.json should not exist — pose estimation is now server-side")
	}
}

// --- Story 2.7: Reprocess tests ---

func TestHandleReprocess_ValidLift(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create original video file (required for pipeline)
	if err := storage.CreateLiftDir(srv.DataDir, lift.ID); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(storage.LiftFile(srv.DataDir, lift.ID, storage.FileOriginal), []byte("video"), 0644)

	broker := sse.NewBroker()
	srv.Broker = broker
	srv.Pipeline = pipeline.New(nil, broker) // no stages — Run returns quickly

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", fmt.Sprintf("/lifts/%d/reprocess", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReprocess_NonExistentLift(t *testing.T) {
	srv := setupTestServer(t)

	broker := sse.NewBroker()
	srv.Broker = broker
	srv.Pipeline = pipeline.New(nil, broker)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/lifts/999/reprocess", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleReprocess_AlreadyProcessing(t *testing.T) {
	srv := setupTestServer(t)

	lift, err := srv.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "snatch",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	broker := sse.NewBroker()
	broker.StartProcessing(lift.ID) // simulate running pipeline
	srv.Broker = broker
	srv.Pipeline = pipeline.New(nil, broker)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", fmt.Sprintf("/lifts/%d/reprocess", lift.ID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}

	broker.StopProcessing(lift.ID) // cleanup
}

func TestHandleReprocess_InvalidID(t *testing.T) {
	srv := setupTestServer(t)

	broker := sse.NewBroker()
	srv.Broker = broker
	srv.Pipeline = pipeline.New(nil, broker)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/lifts/abc/reprocess", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestIsValidVideo(t *testing.T) {
	tests := []struct {
		filename    string
		contentType string
		want        bool
	}{
		{"video.mp4", "", true},
		{"video.MP4", "", true},
		{"video.mov", "", true},
		{"video.MOV", "", true},
		{"video.txt", "", false},
		{"video.avi", "", false},
		{"video", "video/mp4", true},
		{"video", "video/quicktime", true},
		{"video", "text/plain", false},
	}

	for _, tt := range tests {
		got := isValidVideo(tt.filename, tt.contentType)
		if got != tt.want {
			t.Errorf("isValidVideo(%q, %q) = %v, want %v", tt.filename, tt.contentType, got, tt.want)
		}
	}
}
