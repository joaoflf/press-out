package main

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"press-out/internal/config"
	"press-out/internal/handler"
	"press-out/internal/storage"
	"press-out/internal/storage/sqlc"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()
	slog.Info("config loaded",
		"port", cfg.Port,
		"data_dir", cfg.DataDir,
		"db_path", cfg.DBPath,
	)

	db, err := storage.NewDB(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := storage.RunMigrations(db, "sql/schema"); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	queries := sqlc.New(db)

	tmpl, err := template.ParseGlob("web/templates/layouts/*.html")
	if err != nil {
		slog.Error("failed to parse layout templates", "error", err)
		os.Exit(1)
	}
	tmpl, err = tmpl.ParseGlob("web/templates/pages/*.html")
	if err != nil {
		slog.Error("failed to parse page templates", "error", err)
		os.Exit(1)
	}
	if partials, _ := filepath.Glob("web/templates/partials/*.html"); len(partials) > 0 {
		tmpl, err = tmpl.ParseGlob("web/templates/partials/*.html")
		if err != nil {
			slog.Error("failed to parse partial templates", "error", err)
			os.Exit(1)
		}
	}

	srv := &handler.Server{
		Queries:   queries,
		Templates: tmpl,
		DataDir:   cfg.DataDir,
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	addr := ":" + cfg.Port
	slog.Info("server starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
