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
	os.Unsetenv("MEDIAPIPE_API_KEY")

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
	if cfg.MediaPipeAPIKey != "" {
		t.Errorf("MediaPipeAPIKey = %q, want empty", cfg.MediaPipeAPIKey)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "3000")
	t.Setenv("DATA_DIR", "/tmp/data")
	t.Setenv("DB_PATH", "/tmp/data/test.db")
	t.Setenv("MEDIAPIPE_API_KEY", "test-key")

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
	if cfg.MediaPipeAPIKey != "test-key" {
		t.Errorf("MediaPipeAPIKey = %q, want %q", cfg.MediaPipeAPIKey, "test-key")
	}
}
