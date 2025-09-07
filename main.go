package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"distrack/database"
	"distrack/handlers"
	"distrack/middleware"
	"distrack/models"
	"distrack/repository"
	"distrack/scraper"
	"distrack/scheduler"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"github.com/joho/godotenv"
)

// Metrics struct for basic monitoring
type Metrics struct {
	Timestamp     time.Time `json:"timestamp"`
	Uptime        string    `json:"uptime"`
	Goroutines    int       `json:"goroutines"`
	MemoryUsage   string    `json:"memory_usage"`
	ActiveURLs    int       `json:"active_urls"`
	TotalChecks   int       `json:"total_checks"`
	SuccessRate   float64   `json:"success_rate"`
	LastCheck     time.Time `json:"last_check"`
	OCRHealth     string    `json:"ocr_health"`
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize database
	if err := database.InitDatabase(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.CloseDatabase()

	// Create tables
	if err := database.CreateTables(); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	// Initialize repositories
	urlRepo := repository.NewURLRepository()
	alertRepo := repository.NewAlertRepository()
	prefRepo := repository.NewPreferenceRepository()

	// Initialize scraper
	scraper, err := scraper.NewHybridPriceScraper()
	if err != nil {
		log.Fatalf("Failed to create scraper: %v", err)
	}
	defer scraper.Close()

	// Initialize handlers
	h := handlers.NewHandlers(urlRepo, alertRepo, prefRepo)

	// Initialize and start price checker
	priceChecker, err := scheduler.NewPriceChecker()
	if err != nil {
		log.Fatalf("Failed to create price checker: %v", err)
	}
	priceChecker.Start()
	defer priceChecker.Stop()

	// Initialize and start retry service
	// Create retry service functions
	retryFuncs := &scheduler.RetryServiceFuncs{
		GetURLsForRetry: func() ([]interface{}, error) {
			urls, err := h.GetURLRepo().GetURLsForRetry()
			if err != nil {
				return nil, err
			}
			// Convert []models.TrackedURL to []interface{}
			interfaces := make([]interface{}, len(urls))
			for i, url := range urls {
				interfaces[i] = url
			}
			return interfaces, nil
		},
		ScrapePrice: func(url string, urlID int) (interface{}, error) {
			return h.GetScraper().ScrapePriceWithHybridMethod(url, urlID)
		},
		MarkPriceCheckFailed: func(urlID int) error {
			return h.GetURLRepo().MarkPriceCheckFailed(urlID)
		},
		MarkPriceCheckSuccess: func(urlID int) error {
			return h.GetURLRepo().MarkPriceCheckSuccess(urlID)
		},
		UpdateURLPrice: func(urlID int, priceData interface{}) error {
			return h.GetURLRepo().UpdateURLPrice(urlID, priceData.(*models.PriceData))
		},
		AddPriceHistory: func(urlID int, priceData interface{}) error {
			return h.GetURLRepo().AddPriceHistory(urlID, priceData.(*models.PriceData))
		},
		CheckAlerts: func(urlID int, price float64) ([]interface{}, error) {
			alerts, err := h.GetAlertRepo().CheckAlerts(urlID, price)
			if err != nil {
				return nil, err
			}
			// Convert []models.PriceAlert to []interface{}
			interfaces := make([]interface{}, len(alerts))
			for i, alert := range alerts {
				interfaces[i] = alert
			}
			return interfaces, nil
		},
	}
	retryService := scheduler.NewRetryService(retryFuncs)
	retryService.Start()
	defer retryService.Stop()



	// Setup router
	r := mux.NewRouter()

	// Apply global middleware
	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.RateLimitMiddleware(middleware.DefaultConfig))
	r.Use(middleware.APIKeyMiddleware(false)) // API key not required for health checks

	// Health and monitoring endpoints (no auth required)
	r.HandleFunc("/health", healthCheck).Methods("GET")
	r.HandleFunc("/metrics", getMetrics).Methods("GET")
	r.HandleFunc("/status", getStatus).Methods("GET")
	r.HandleFunc("/docs", serveAPIDocs).Methods("GET")

	// API v1 routes with authentication
	apiV1 := r.PathPrefix("/api/v1").Subrouter()
	apiV1.Use(middleware.APIKeyMiddleware(true)) // API key required for v1 endpoints
	
	// URL management
	apiV1.HandleFunc("/urls", h.AddURLToTrack).Methods("POST")
	apiV1.HandleFunc("/urls", h.GetTrackedURLs).Methods("GET")
	apiV1.HandleFunc("/urls/{id}", h.GetURLDetails).Methods("GET")
	apiV1.HandleFunc("/urls/{id}", h.DeleteTrackedURL).Methods("DELETE")
	apiV1.HandleFunc("/urls/{id}/check", h.CheckPriceNow).Methods("POST")
	apiV1.HandleFunc("/urls/{id}/check-async", h.CheckPriceNowAsync).Methods("POST")
	apiV1.HandleFunc("/urls/{id}/choice", h.HandleUserChoice).Methods("POST")
	apiV1.HandleFunc("/urls/{id}/history", h.GetPriceHistory).Methods("GET")
	
	// Price feedback system
	apiV1.HandleFunc("/price-alternative/{feedback_id}", h.GetAlternativePrice).Methods("GET")
	apiV1.HandleFunc("/price-feedback", h.SubmitPriceFeedback).Methods("POST")
	
	// Price alerts
	apiV1.HandleFunc("/urls/{id}/alerts", h.SetPriceAlert).Methods("POST")
	apiV1.HandleFunc("/urls/{id}/alerts", h.GetPriceAlerts).Methods("GET")
	apiV1.HandleFunc("/urls/{id}/alerts/{alertId}", h.DeletePriceAlert).Methods("DELETE")
	
	// Task management
	apiV1.HandleFunc("/tasks/{taskId}", h.GetTaskStatus).Methods("GET")
	apiV1.HandleFunc("/tasks/stats", h.GetTaskStats).Methods("GET")
	
	// API Key management
	apiV1.HandleFunc("/api-keys/generate", h.GenerateAPIKey).Methods("POST")
	apiV1.HandleFunc("/api-keys/info", h.GetAPIKeyInfo).Methods("GET")
	
	// Debug endpoints
	apiV1.HandleFunc("/debug/screenshot", h.DebugScreenshot).Methods("POST")

	// Legacy API routes (redirect to v1)
	legacyAPI := r.PathPrefix("/api").Subrouter()
	legacyAPI.Use(middleware.APIKeyMiddleware(true))
	legacyAPI.HandleFunc("/urls", redirectToV1).Methods("GET", "POST")
	legacyAPI.HandleFunc("/urls/{id}", redirectToV1).Methods("GET", "POST", "DELETE")
	legacyAPI.HandleFunc("/urls/{id}/check", redirectToV1).Methods("POST")
	legacyAPI.HandleFunc("/urls/{id}/check-async", redirectToV1).Methods("POST")
	legacyAPI.HandleFunc("/urls/{id}/choice", redirectToV1).Methods("POST")
	legacyAPI.HandleFunc("/urls/{id}/alerts", redirectToV1).Methods("GET", "POST")
	legacyAPI.HandleFunc("/urls/{id}/alerts/{alertId}", redirectToV1).Methods("DELETE")
	legacyAPI.HandleFunc("/tasks/{taskId}", redirectToV1).Methods("GET")
	legacyAPI.HandleFunc("/tasks/stats", redirectToV1).Methods("GET")
	legacyAPI.HandleFunc("/debug/screenshot", redirectToV1).Methods("POST")

	// CORS configuration
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:3000,http://10.0.2.2:3000"
	}

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{allowedOrigins},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	log.Printf("🌐 Server starting on port %s", port)
	log.Printf("📋 API Documentation:")
	log.Printf("   GET  /health - Health check")
	log.Printf("   GET  /metrics - System metrics")
	log.Printf("   GET  /status - Detailed status")
	log.Printf("   GET  /docs - API documentation")
	log.Printf("   POST /api/v1/urls - Add URL to track")
	log.Printf("   GET  /api/v1/urls - Get all tracked URLs")
	log.Printf("   POST /api/v1/urls/{id}/check - Check price now")
	log.Printf("   POST /api/v1/urls/{id}/choice - Save user choice")
	log.Printf("   ⚠️  /api/* - Legacy endpoints (deprecated, redirect to v1)")

	// Start server
	log.Fatal(http.ListenAndServe(host+":"+port, c.Handler(r)))
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":     "distrack",
		"status":      "healthy",
		"timestamp":   time.Now(),
		"version":     "2.0.0",
		"api_version": "v1",
		"endpoints": map[string]string{
			"health":     "/health",
			"metrics":    "/metrics",
			"status":     "/status",
			"docs":       "/docs",
			"api_v1":     "/api/v1",
			"legacy_api": "/api (deprecated)",
		},
		"rate_limits": map[string]interface{}{
			"free": map[string]interface{}{
				"requests_per_minute": 60,
				"requests_per_day":    1000,
			},
			"basic": map[string]interface{}{
				"requests_per_minute": 300,
				"requests_per_day":    50000,
			},
			"pro": map[string]interface{}{
				"requests_per_minute": 1000,
				"requests_per_day":    200000,
			},
			"enterprise": map[string]interface{}{
				"requests_per_minute": 5000,
				"requests_per_day":    1000000,
			},
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func getMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Get active URLs count
	urlRepo := repository.NewURLRepository()
	urls, err := urlRepo.GetTrackedURLs()
	activeURLs := 0
	if err == nil {
		activeURLs = len(urls)
	}

	// Use default values since metrics package is not available
	startTime := time.Now().Add(-24 * time.Hour) // Assume 24 hours uptime
	lastCheckTime := time.Now().Add(-1 * time.Hour) // Assume last check was 1 hour ago

	metricsData := Metrics{
		Timestamp:   time.Now(),
		Uptime:      time.Since(startTime).String(),
		Goroutines:  runtime.NumGoroutine(),
		MemoryUsage: fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024),
		ActiveURLs:  activeURLs,
		TotalChecks: 0, // Default value
		SuccessRate: 0.0, // Default value
		LastCheck:   lastCheckTime,
		OCRHealth:   "healthy", // You can add OCR health check here
	}

	writeJSON(w, http.StatusOK, metricsData)
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	urlRepo := repository.NewURLRepository()
	urls, err := urlRepo.GetTrackedURLs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get URLs")
		return
	}

	// Count URLs with prices
	urlsWithPrices := 0
	for _, url := range urls {
		if url.HasPrice() {
			urlsWithPrices++
		}
	}

	// Use default values since metrics package is not available
	startTime := time.Now().Add(-24 * time.Hour) // Assume 24 hours uptime
	lastCheckTime := time.Now().Add(-1 * time.Hour) // Assume last check was 1 hour ago

	status := map[string]interface{}{
		"timestamp":        time.Now(),
		"uptime":          time.Since(startTime).String(),
		"total_urls":      len(urls),
		"urls_with_prices": urlsWithPrices,
		"success_rate":    "0.0%", // Default value
		"last_check":      lastCheckTime,
		"system_health":   "healthy",
	}

	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// serveAPIDocs serves the API documentation
func serveAPIDocs(w http.ResponseWriter, r *http.Request) {
	// In a production environment, you might want to serve Swagger UI or a static HTML file
	// For now, we'll redirect to the markdown documentation
	http.Redirect(w, r, "/docs/api_documentation.md", http.StatusMovedPermanently)
}

// redirectToV1 redirects legacy API calls to v1 endpoints
func redirectToV1(w http.ResponseWriter, r *http.Request) {
	// Extract the path from the legacy API call
	path := r.URL.Path
	// Remove the /api prefix and add /api/v1
	newPath := "/api/v1" + strings.TrimPrefix(path, "/api")
	
	// Add query parameters if they exist
	if r.URL.RawQuery != "" {
		newPath += "?" + r.URL.RawQuery
	}
	
	// Set deprecation warning header
	w.Header().Set("X-API-Deprecation-Warning", "This endpoint is deprecated. Please use /api/v1 endpoints instead.")
	w.Header().Set("X-API-Version", "v1")
	
	// Redirect to the new v1 endpoint
	http.Redirect(w, r, newPath, http.StatusMovedPermanently)
}
