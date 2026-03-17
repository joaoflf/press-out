package config

import "os"

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port    string
	DataDir string
	DBPath  string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		Port:    envOrDefault("PORT", "8080"),
		DataDir: envOrDefault("DATA_DIR", "./data"),
		DBPath:  envOrDefault("DB_PATH", "./data/press-out.db"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
