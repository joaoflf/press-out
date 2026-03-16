package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLiftDir(t *testing.T) {
	got := LiftDir("/data", 42)
	want := filepath.Join("/data", "lifts", "42")
	if got != want {
		t.Errorf("LiftDir = %q, want %q", got, want)
	}
}

func TestLiftFile(t *testing.T) {
	got := LiftFile("/data", 42, FileOriginal)
	want := filepath.Join("/data", "lifts", "42", "original.mp4")
	if got != want {
		t.Errorf("LiftFile = %q, want %q", got, want)
	}
}

func TestCreateAndRemoveLiftDir(t *testing.T) {
	tmpDir := t.TempDir()

	if err := CreateLiftDir(tmpDir, 1); err != nil {
		t.Fatalf("CreateLiftDir: %v", err)
	}

	dir := LiftDir(tmpDir, 1)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("lift dir was not created")
	}

	if err := RemoveLiftDir(tmpDir, 1); err != nil {
		t.Fatalf("RemoveLiftDir: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("lift dir was not removed")
	}
}
