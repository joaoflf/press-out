package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sub", "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Verify the parent directory was created
	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Fatal("data dir was not created")
	}

	// Verify WAL mode
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestRunMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Create a schema dir with a migration
	schemaDir := filepath.Join(tmpDir, "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		t.Fatal(err)
	}
	migration := `CREATE TABLE IF NOT EXISTS lifts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		lift_type TEXT NOT NULL,
		created_at TEXT NOT NULL,
		coaching_cue TEXT,
		coaching_diagnosis TEXT
	);`
	if err := os.WriteFile(filepath.Join(schemaDir, "001_initial.sql"), []byte(migration), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(db, schemaDir); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify the table exists by inserting a row
	_, err = db.Exec("INSERT INTO lifts (lift_type, created_at) VALUES ('snatch', '2026-01-01T00:00:00Z')")
	if err != nil {
		t.Fatalf("insert into lifts: %v", err)
	}
}
