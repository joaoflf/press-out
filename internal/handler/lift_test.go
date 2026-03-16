package handler

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
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

	tmpl := template.Must(template.New("base.html").Parse(`<!DOCTYPE html><html>{{block "content" .}}{{end}}</html>`))
	template.Must(tmpl.New("lift-list.html").Parse(
		`{{template "base.html" .}}{{define "content"}}<div>{{if .Empty}}<p>No lifts yet</p>{{else}}{{range .Lifts}}<a href="/lifts/{{.ID}}">{{.LiftType}}</a>{{end}}{{end}}<a href="#">Upload Lift</a></div>{{end}}`))

	return &Server{
		Queries:   queries,
		Templates: tmpl,
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
	if !strings.Contains(body, "snatch") {
		t.Error("expected lift type in response")
	}
}
