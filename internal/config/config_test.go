package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear any env vars that might interfere
	os.Unsetenv("PORT")
	os.Unsetenv("DATA_DIR")
	os.Unsetenv("DB_PATH")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "./data")
	}
	if cfg.DBPath != "./data/press-out.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "./data/press-out.db")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "3000")
	t.Setenv("DATA_DIR", "/tmp/data")
	t.Setenv("DB_PATH", "/tmp/data/test.db")

	cfg := Load()

	if cfg.Port != "3000" {
		t.Errorf("Port = %q, want %q", cfg.Port, "3000")
	}
	if cfg.DataDir != "/tmp/data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/tmp/data")
	}
	if cfg.DBPath != "/tmp/data/test.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/data/test.db")
	}
}
