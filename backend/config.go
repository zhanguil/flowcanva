package main

import (
	"bufio"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host                string
	Port                string
	DBPath              string
	DevMode             bool
	Embedded            bool
	ConsoleDir          string
	CanvasDir           string
	UploadDir           string
	AdminDevURL         string
	CanvasDevURL        string
	VectorEngineBaseURL string
	VectorEngineAPIKey  string
	AssistantModel      string
	ImageModelFast      string
	ImageModelPro       string
	ImageModelEdit      string
	ImageProvider       string
	MaxConcurrentJobs   int
	FalAPIKey           string
	FalBaseURL          string
	FalImageEditModel   string
}

func loadConfig() Config {
	loadEnvFile(".env")
	embedded := os.Getenv("EMBEDDED") == "true"
	if !embedded {
		embedded = embedHasContent()
	}
	maxConcurrentJobs, _ := strconv.Atoi(envOrDefault("MAX_CONCURRENT_JOBS", "4"))
	if maxConcurrentJobs < 1 {
		maxConcurrentJobs = 4
	}
	return Config{
		Host:                envOrDefault("SERVER_HOST", "0.0.0.0"),
		Port:                envOrDefault("PORT", "6789"),
		DBPath:              envOrDefault("DB_PATH", "./data.db"),
		DevMode:             os.Getenv("DEV_MODE") == "true",
		Embedded:            embedded,
		ConsoleDir:          envOrDefault("CONSOLE_DIR", "../frontend-admin/dist"),
		CanvasDir:           envOrDefault("CANVAS_DIR", "../frontend-canvas/dist"),
		UploadDir:           envOrDefault("UPLOAD_DIR", "./uploads"),
		AdminDevURL:         envOrDefault("ADMIN_DEV_URL", "http://localhost:5174"),
		CanvasDevURL:        envOrDefault("CANVAS_DEV_URL", "http://localhost:5173"),
		VectorEngineBaseURL: os.Getenv("VECTORENGINE_BASE_URL"),
		VectorEngineAPIKey:  os.Getenv("VECTORENGINE_API_KEY"),
		AssistantModel:      os.Getenv("ASSISTANT_MODEL"),
		ImageModelFast:      envOrDefault("IMAGE_MODEL_FAST", "gemini-3.1-flash-image-preview"),
		ImageModelPro:       envOrDefault("IMAGE_MODEL_PRO", "gemini-3-pro-image-preview"),
		ImageModelEdit:      envOrDefault("IMAGE_MODEL_EDIT", "gpt-image-2"),
		ImageProvider:       envOrDefault("IMAGE_PROVIDER", "vectorengine"),
		MaxConcurrentJobs:   maxConcurrentJobs,
		FalAPIKey:           os.Getenv("FAL_KEY"),
		FalBaseURL:          envOrDefault("FAL_BASE_URL", "https://fal.run"),
		FalImageEditModel:   envOrDefault("FAL_IMAGE_EDIT_MODEL", "fal-ai/qwen-image-edit-2511"),
	}
}

func (c Config) ListenAddress() string {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		host = "0.0.0.0"
	}
	port := strings.TrimPrefix(strings.TrimSpace(c.Port), ":")
	return net.JoinHostPort(host, port)
}

func (c Config) PortNumber() string {
	return strings.TrimPrefix(strings.TrimSpace(c.Port), ":")
}

func embedHasContent() bool {
	f, err := AdminEmbedFS.Open("admin-dist/index.html")
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = value[1 : len(value)-1]
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func initLogger() *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
