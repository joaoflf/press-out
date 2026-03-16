package config

import "os"

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port           string
	DataDir        string
	DBPath         string
	MediaPipeAPIKey string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		Port:           envOrDefault("PORT", "8080"),
		DataDir:        envOrDefault("DATA_DIR", "./data"),
		DBPath:         envOrDefault("DB_PATH", "./data/press-out.db"),
		MediaPipeAPIKey: os.Getenv("MEDIAPIPE_API_KEY"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
