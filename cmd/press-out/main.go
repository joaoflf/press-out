package main

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"press-out/internal/config"
	"press-out/internal/ffmpeg"
	"press-out/internal/handler"
	"press-out/internal/pipeline"
	"press-out/internal/pipeline/stages"
	"press-out/internal/sse"
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

	if version, err := ffmpeg.Probe(); err != nil {
		slog.Warn("ffmpeg not available — video processing stages will be skipped", "error", err)
	} else {
		slog.Info("ffmpeg available", "version", version)
	}

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

	// Parse base layout + partials as a shared foundation.
	base, err := template.ParseGlob("web/templates/layouts/*.html")
	if err != nil {
		slog.Error("failed to parse layout templates", "error", err)
		os.Exit(1)
	}
	if partials, _ := filepath.Glob("web/templates/partials/*.html"); len(partials) > 0 {
		base, err = base.ParseGlob("web/templates/partials/*.html")
		if err != nil {
			slog.Error("failed to parse partial templates", "error", err)
			os.Exit(1)
		}
	}

	// Clone base for each page so {{define "content"}} blocks don't conflict.
	pages, err := filepath.Glob("web/templates/pages/*.html")
	if err != nil {
		slog.Error("failed to glob page templates", "error", err)
		os.Exit(1)
	}
	tmplMap := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		name := filepath.Base(page)
		clone, err := base.Clone()
		if err != nil {
			slog.Error("failed to clone base template", "page", name, "error", err)
			os.Exit(1)
		}
		tmplMap[name], err = clone.ParseFiles(page)
		if err != nil {
			slog.Error("failed to parse page template", "page", name, "error", err)
			os.Exit(1)
		}
	}

	// Derive project root (directory containing pyproject.toml / scripts/).
	projectRoot, err := os.Getwd()
	if err != nil {
		slog.Error("failed to get working directory", "error", err)
		os.Exit(1)
	}

	broker := sse.NewBroker()
	pipelineStages := pipeline.DefaultStages()
	pipelineStages[0] = &stages.PoseStage{ProjectRoot: projectRoot} // Replace stub with real pose stage
	pipelineStages[1] = &stages.TrimStage{}                         // Replace stub with real trim stage
	pipelineStages[2] = &stages.CropStage{}                         // Replace stub with real crop stage
	pipelineStages[3] = &stages.SkeletonStage{}                      // Replace stub with real skeleton stage

	pl := pipeline.New(pipelineStages, broker)

	srv := &handler.Server{
		Queries:   queries,
		Templates: tmplMap,
		DataDir:   cfg.DataDir,
		Pipeline:  pl,
		Broker:    broker,
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
