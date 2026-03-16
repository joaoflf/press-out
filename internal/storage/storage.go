package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// File name constants for lift artifacts.
const (
	FileOriginal  = "original.mp4"
	FileTrimmed   = "trimmed.mp4"
	FileCropped   = "cropped.mp4"
	FileSkeleton  = "skeleton.mp4"
	FileThumbnail = "thumbnail.jpg"
	FileKeypoints = "keypoints.json"
)

// LiftDir returns the directory path for a given lift's files.
func LiftDir(dataDir string, liftID int64) string {
	return filepath.Join(dataDir, "lifts", fmt.Sprintf("%d", liftID))
}

// LiftFile returns the full file path for a given lift artifact.
func LiftFile(dataDir string, liftID int64, filename string) string {
	return filepath.Join(LiftDir(dataDir, liftID), filename)
}

// CreateLiftDir creates the directory for a lift's files.
func CreateLiftDir(dataDir string, liftID int64) error {
	return os.MkdirAll(LiftDir(dataDir, liftID), 0755)
}

// RemoveLiftDir removes the entire directory for a lift's files.
func RemoveLiftDir(dataDir string, liftID int64) error {
	return os.RemoveAll(LiftDir(dataDir, liftID))
}
