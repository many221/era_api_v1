//go:build !cgo

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

const (
	defaultPort         = "8080"
	defaultReadTimeout  = 30 * time.Second
	defaultWriteTimeout = 30 * time.Second
	shutdownTimeout     = 10 * time.Second
	templateDir         = "internal/templates" // Directory for HTML templates
	startupBanner      = `
╔═══════════════════════════════════════════╗
║           ERA API v1 Server               ║
║     Election Results Aggregator API       ║
╚═══════════════════════════════════════════╝
`
)

type ServerConfig struct {
	port      string
	templates *template.Template
	logger    *slog.Logger
}

func main() {
	// Print startup banner
	fmt.Print(startupBanner)

	// Initialize structured logger with more detailed options
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	}))
	slog.SetDefault(logger)

	// Add startup information
	logger.Info("server initialization",
		"go_version", runtime.Version(),
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
		"cpu_cores", runtime.NumCPU(),
	)

	// Load HTML templates
	tmpl, err := template.ParseGlob(filepath.Join(templateDir, "*.html"))
	if err != nil {
		if os.IsNotExist(err) {
			// Create templates directory if it doesn't exist
			if err := os.MkdirAll(templateDir, 0755); err != nil {
				logger.Error("failed to create templates directory", 
					"path", templateDir,
					"error", err)
				os.Exit(1)
			}
			
			// Create a default template
			defaultTemplate := filepath.Join(templateDir, "default.html")
			defaultContent := `
<!DOCTYPE html>
<html>
<head>
    <title>ERA API - Election Results</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .results { border: 1px solid #ddd; padding: 15px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.Title}}</h1>
        <div class="results">
            {{.Content}}
        </div>
    </div>
</body>
</html>`
			
			if err := os.WriteFile(defaultTemplate, []byte(defaultContent), 0644); err != nil {
				logger.Error("failed to create default template",
					"path", defaultTemplate,
					"error", err)
				os.Exit(1)
			}
			
			// Try loading templates again
			tmpl, err = template.ParseGlob(filepath.Join(templateDir, "*.html"))
			if err != nil {
				logger.Error("failed to load templates after creation", "error", err)
				os.Exit(1)
			}
			
			logger.Info("created default template", "path", defaultTemplate)
		} else {
			logger.Error("failed to load templates", "error", err)
			os.Exit(1)
		}
	}

	// Initialize server config
	config := &ServerConfig{
		port:      getEnvOrDefault("PORT", defaultPort),
		templates: tmpl,
		logger:    logger,
	}

	// Create new server mux
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("POST /api/v1/process", corsMiddleware(handleProcess))
	mux.HandleFunc("GET /health", healthCheck)
	
	// Create server with timeouts
	server := &http.Server{
		Addr:         ":" + config.port,
		Handler:      requestLogger(mux, config.logger),
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}

	// Start server
	startServer(server, config)
}

// requestLogger middleware for logging requests
func requestLogger(handler http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		handler.ServeHTTP(w, r)
		
		logger.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
		)
	})
}

func startServer(server *http.Server, config *ServerConfig) {
	serverErrors := make(chan error, 1)
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		config.logger.Info("starting server", "port", config.port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		config.logger.Error("server error", "error", err)
	case sig := <-shutdown:
		config.logger.Info("shutdown signal received", "signal", sig)
	}

	gracefulShutdown(server, config.logger)
}

func gracefulShutdown(server *http.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		if err := server.Close(); err != nil {
			logger.Error("server close failed", "error", err)
		}
	}

	logger.Info("server stopped")
}

// corsMiddleware adds CORS headers to responses
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Update CORS to allow Vue.js dev server
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5174")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// handleProcess is a temporary placeholder for the process handler
func handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the request body
	var req struct {
		CountyName  string `json:"countyName"`
		FileLink    string `json:"fileLink"`
		ContentType string `json:"contentType"` // "candidate" or "measure"
		ParseMethod string `json:"parseMethod"` // "zip", "html", "pdf", "xml"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// For now, return a sample response
	response := map[string]interface{}{
		"html": fmt.Sprintf(`
			<div class="election-results">
				<h2>%s Results</h2>
				<div class="results-container">
					<p>Processing %s data from %s using %s parser</p>
				</div>
			</div>
		`, req.CountyName, req.ContentType, req.FileLink, req.ParseMethod),
		"error": nil,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func printStartupInstructions() {
	fmt.Println("\nTo run with XCode tools bypass, use one of these commands:")
	fmt.Println("\n1. For development:")
	fmt.Println("   CGO_ENABLED=1 go run cmd/server/main.go")
	fmt.Println("\n2. For building:")
	fmt.Println("   CGO_ENABLED=1 go build -o era_api cmd/server/main.go")
	fmt.Println("\n3. For production:")
	fmt.Println("   CGO_ENABLED=1 GOOS=darwin go build -o era_api cmd/server/main.go")
	fmt.Println("\nEnvironment variables:")
	fmt.Println("- PORT: Server port (default: 8080)")
	fmt.Println("- LOG_LEVEL: Logging level (default: info)")
	fmt.Println()
}
