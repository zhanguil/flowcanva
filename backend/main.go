package main

import (
	"log/slog"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	cfg := loadConfig()
	logger := initLogger()
	slog.SetDefault(logger)

	logger.Info("starting server",
		"listen_address", cfg.ListenAddress(),
		"dev_mode", cfg.DevMode,
		"db_path", cfg.DBPath,
	)

	db, err := initDB(cfg.DBPath, logger)
	if err != nil {
		logger.Error("database init failed", "error", err)
		panic(err)
	}
	defer db.Close()

	vectorEngine := NewVectorEngineProvider(cfg.VectorEngineBaseURL, cfg.VectorEngineAPIKey, nil)
	var imageProvider ImageProvider
	if cfg.DevMode && cfg.ImageProvider == "mock" {
		imageProvider = NewMockImageProvider()
		logger.Info("mock image provider enabled")
	}
	h := &Handler{
		db: db, log: logger, vectorEngine: vectorEngine,
		assistantModel:  cfg.AssistantModel,
		imageModelFast:  cfg.ImageModelFast,
		imageModelPro:   cfg.ImageModelPro,
		imageModelEdit:  cfg.ImageModelEdit,
		imageProvider:   imageProvider,
		jobQueue:        NewJobQueue(cfg.MaxConcurrentJobs),
		devMode:         cfg.DevMode,
		generationDebug: &GenerationDebugStore{},
		uploadDir:       cfg.UploadDir,
	}
	h.generationProviders = map[string]GenerationProvider{"legacy": NewLegacyProvider(h)}
	if cfg.FalAPIKey != "" {
		h.generationProviders["fal"] = NewFalProvider(cfg.FalBaseURL, cfg.FalAPIKey, cfg.FalImageEditModel, cfg.UploadDir, nil)
		logger.Info("fal generation provider enabled", "model", cfg.FalImageEditModel)
	}
	h.resumeQueuedRecipeJobs()
	r := setupRouter(h, cfg)

	logger.Info("server ready", "listen_address", cfg.ListenAddress())

	if cfg.Embedded {
		go func() {
			time.Sleep(800 * time.Millisecond)
			url := "http://127.0.0.1:" + cfg.PortNumber() + "/canvas"
			logger.Info("opening browser", "url", url)
			openBrowser(url)
		}()
	}

	if err := r.Run(cfg.ListenAddress()); err != nil {
		logger.Error("server failed", "error", err)
		panic(err)
	}
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}
