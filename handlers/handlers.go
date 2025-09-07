package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"distrack/config"
	"distrack/models"
	"distrack/repository"
	"distrack/scraper"
	"distrack/scheduler"
	"distrack/services"

	"github.com/gorilla/mux"
)

type Handlers struct {
	urlRepo      *repository.URLRepository
	alertRepo    *repository.AlertRepository
	prefRepo     *repository.PreferenceRepository
	scraper      *scraper.HybridPriceScraper
	taskManager  *scheduler.TaskManager
}

func NewHandlers(urlRepo *repository.URLRepository, alertRepo *repository.AlertRepository, prefRepo *repository.PreferenceRepository) *Handlers {
	hybridScraper, err := scraper.NewHybridPriceScraper()
	if err != nil {
		log.Printf("Failed to initialize hybrid scraper: %v", err)
	}

	handlers := &Handlers{
		urlRepo:   urlRepo,
		alertRepo: alertRepo,
		prefRepo:  prefRepo,
		scraper:   hybridScraper,
	}

	// Initialize task manager with 5 max workers
	handlers.taskManager = scheduler.NewTaskManager(handlers.performPriceCheck, 5)

	return handlers
}

// Close closes the handlers and scrapers
func (h *Handlers) Close() {
	if h.taskManager != nil {
		h.taskManager.Stop()
	}
	if h.scraper != nil {
		h.scraper.Close()
	}
}

// GetURLRepo returns the URL repository
func (h *Handlers) GetURLRepo() *repository.URLRepository {
	return h.urlRepo
}

// GetAlertRepo returns the alert repository
func (h *Handlers) GetAlertRepo() *repository.AlertRepository {
	return h.alertRepo
}

	// GetScraper returns the scraper
	func (h *Handlers) GetScraper() *scraper.HybridPriceScraper {
		return h.scraper
	}

	// GetTaskManager returns the task manager
func (h *Handlers) GetTaskManager() *scheduler.TaskManager {
	return h.taskManager
}

// performPriceCheck performs the actual price checking (used by TaskManager)
func (h *Handlers) performPriceCheck(urlID int) (*models.PriceData, error) {
	// Get URL details
	url, err := h.urlRepo.GetURLByID(urlID)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL details: %v", err)
	}
	
	// Check if URL can be retried
	if !url.CanRetry() {
		nextRetryStr := "Unknown"
		if url.NextRetryAt != nil {
			nextRetryStr = url.NextRetryAt.Format("15:04")
		}
		return nil, fmt.Errorf("price check failed recently. Next retry available at %s", nextRetryStr)
	}
	
	// Perform the price check using the scraper
	priceData, err := h.scraper.ScrapePriceWithHybridMethod(url.URL, urlID)
	if err != nil {
		// Mark as failed and schedule retry
		if retryErr := h.urlRepo.MarkPriceCheckFailed(urlID); retryErr != nil {
			log.Printf("❌ Failed to mark price check as failed: %v", retryErr)
		}
		return nil, fmt.Errorf("failed to scrape price: %v", err)
	}
	
	// Mark as successful
	if retryErr := h.urlRepo.MarkPriceCheckSuccess(urlID); retryErr != nil {
		log.Printf("❌ Failed to mark price check as successful: %v", retryErr)
	}
	
	// Update URL with new price
	if err := h.urlRepo.UpdateURLPrice(urlID, priceData); err != nil {
		return nil, fmt.Errorf("failed to update URL price: %v", err)
	}
	
	// Add to price history
	if err := h.urlRepo.AddPriceHistory(urlID, priceData); err != nil {
		log.Printf("❌ Failed to add price history: %v", err)
	}
	
	// Check for alerts
	triggeredAlerts, err := h.alertRepo.CheckAlerts(urlID, priceData.CurrentPrice)
	if err != nil {
		log.Printf("❌ Failed to check alerts: %v", err)
	}
	
	if len(triggeredAlerts) > 0 {
		log.Printf("🔔 %d alerts triggered for URL ID %d", len(triggeredAlerts), urlID)
	}
	
	return priceData, nil
}

// HealthCheck returns a simple health check response
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "distrack",
		"version":   "2.0.0",
	}
	writeJSON(w, http.StatusOK, response)
}

// AddURLToTrack adds a new URL to track
func (h *Handlers) AddURLToTrack(w http.ResponseWriter, r *http.Request) {
	var req models.AddURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.URL == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "URL and name are required")
		return
	}

	// Add URL to database
	trackedURL, err := h.urlRepo.AddURLToTrack(req.URL, req.Name)
	if err != nil {
		log.Printf("Failed to add URL to track: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to add URL to track")
		return
	}

	// Try to get initial price with hybrid scraper
	go func() {
		if h.scraper != nil {
			priceData, err := h.scraper.ScrapePriceWithHybridMethod(req.URL, trackedURL.ID)
			if err != nil {
				log.Printf("Failed to get initial price for %s: %v", req.URL, err)
				return
			}

			// Update URL with price data
			if err := h.urlRepo.UpdateURLPrice(trackedURL.ID, priceData); err != nil {
				log.Printf("Failed to update URL price: %v", err)
				return
			}

			// Add to price history
			if err := h.urlRepo.AddPriceHistory(trackedURL.ID, priceData); err != nil {
				log.Printf("Failed to add price history: %v", err)
			}

			log.Printf("Initial price for %s: $%.2f %s (method: %s, confidence: %.2f)", 
				req.Name, priceData.CurrentPrice, priceData.Currency, 
				priceData.ExtractionMethod, priceData.Confidence)
		}
	}()

	writeJSON(w, http.StatusCreated, trackedURL)
}

// GetTrackedURLs returns all tracked URLs
func (h *Handlers) GetTrackedURLs(w http.ResponseWriter, r *http.Request) {
	urls, err := h.urlRepo.GetTrackedURLs()
	if err != nil {
		log.Printf("Failed to get tracked URLs: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get tracked URLs")
		return
	}

	// Ensure we always return an array, even if empty
	if urls == nil {
		urls = []models.TrackedURL{}
	}

	writeJSON(w, http.StatusOK, urls)
}

// GetURLDetails returns details for a specific URL
func (h *Handlers) GetURLDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	url, err := h.urlRepo.GetURLByID(id)
	if err != nil {
		if err.Error() == "URL not found" {
			writeError(w, http.StatusNotFound, "URL not found")
			return
		}
		log.Printf("Failed to get URL details: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get URL details")
		return
	}

	writeJSON(w, http.StatusOK, url)
}

// DeleteTrackedURL deletes a tracked URL
func (h *Handlers) DeleteTrackedURL(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	if err := h.urlRepo.DeleteTrackedURL(id); err != nil {
		log.Printf("Failed to delete tracked URL: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete tracked URL")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "URL deleted successfully"})
}

// GetPriceHistory returns price history for a URL
func (h *Handlers) GetPriceHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	// Get limit from query params
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	history, err := h.urlRepo.GetPriceHistory(id, limit)
	if err != nil {
		log.Printf("Failed to get price history: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get price history")
		return
	}

	writeJSON(w, http.StatusOK, history)
}

// CheckPriceNow performs price checking with hybrid scraper
func (h *Handlers) CheckPriceNow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	// Get URL details
	url, err := h.urlRepo.GetURLByID(id)
	if err != nil {
		if err.Error() == "URL not found" {
			writeError(w, http.StatusNotFound, "URL not found")
			return
		}
		log.Printf("Failed to get URL: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get URL")
		return
	}

	// Check if URL can be retried now
	if !url.CanRetry() {
		nextRetryStr := "Unknown"
		if url.NextRetryAt != nil {
			nextRetryStr = url.NextRetryAt.Format("15:04")
		}
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf("Price check failed recently. Next retry available at %s", nextRetryStr))
		return
	}

	// Scrape price with hybrid scraper
	if h.scraper == nil {
		writeError(w, http.StatusInternalServerError, "Scraper not available")
		return
	}

	priceData, err := h.scraper.ScrapePriceWithHybridMethod(url.URL, id)
	if err != nil {
		log.Printf("Failed to scrape price for %s: %v", url.Name, err)
		
		// Mark as failed and schedule retry
		if retryErr := h.urlRepo.MarkPriceCheckFailed(id); retryErr != nil {
			log.Printf("Failed to mark price check as failed: %v", retryErr)
		}
		
		// Calculate next retry time
		nextRetry := time.Now().Add(url.GetRetryDelay())
		nextRetryStr := nextRetry.Format("15:04")
		
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to scrape price. Will retry at %s (attempt %d/5)", nextRetryStr, url.RetryCount+1))
		return
	}

	// Mark as successful (reset retry count)
	if retryErr := h.urlRepo.MarkPriceCheckSuccess(id); retryErr != nil {
		log.Printf("Failed to mark price check as successful: %v", retryErr)
	}

	// Update URL with new price
	oldPrice := url.GetCurrentPrice()
	if err := h.urlRepo.UpdateURLPrice(id, priceData); err != nil {
		log.Printf("Failed to update URL price: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to update URL price")
		return
	}

	// Add to price history
	if err := h.urlRepo.AddPriceHistory(id, priceData); err != nil {
		log.Printf("Failed to add price history: %v", err)
	}

	// Validate price change before updating and triggering alerts
	priceChangeReason := url.GetPriceChangeReason(priceData.CurrentPrice)
	isRealistic := url.IsPriceChangeRealistic(priceData.CurrentPrice)
	
	// Log price changes with validation
	log.Printf("Price check for %s: $%.2f %s (method: %s, confidence: %.2f)", 
		url.Name, priceData.CurrentPrice, priceData.Currency,
		priceData.ExtractionMethod, priceData.Confidence)

	if oldPrice > 0 && priceData.CurrentPrice != oldPrice {
		change := priceData.CurrentPrice - oldPrice
		changePercent := (change / oldPrice) * 100

		if isRealistic {
			if change < 0 {
				log.Printf("📉 Price DROPPED: $%.2f → $%.2f (%.1f%%) - %s", oldPrice, priceData.CurrentPrice, changePercent, priceChangeReason)
			} else {
				log.Printf("📈 Price INCREASED: $%.2f → $%.2f (+%.1f%%) - %s", oldPrice, priceData.CurrentPrice, changePercent, priceChangeReason)
			}
		} else {
			log.Printf("⚠️  UNREALISTIC PRICE CHANGE: $%.2f → $%.2f (%.1f%%) - %s", oldPrice, priceData.CurrentPrice, changePercent, priceChangeReason)
		}
	}

	if priceData.DiscountPercentage > 0 {
		log.Printf("💰 Discount: %.1f%% off", priceData.DiscountPercentage)
	}

	// Only check for alerts if the price change is realistic
	var triggeredAlerts []models.PriceAlert
	if isRealistic {
		triggeredAlerts, err = h.alertRepo.CheckAlerts(id, priceData.CurrentPrice)
		if err != nil {
			log.Printf("Failed to check alerts: %v", err)
		}
		
		// Log triggered alerts
		if len(triggeredAlerts) > 0 {
			log.Printf("🔔 %d alerts triggered for %s", len(triggeredAlerts), url.Name)
			for _, alert := range triggeredAlerts {
				log.Printf("  - Alert: %s (%.1f%%)", alert.AlertType, alert.Percentage)
			}
		}
	} else {
		log.Printf("🚫 Skipping alert checks due to unrealistic price change")
	}

	// Generate feedback ID for user confirmation
	feedbackID := fmt.Sprintf("feedback_%d_%d", id, time.Now().Unix())
	
	// Create response with both prices for user feedback
	response := models.PriceCheckResponse{
		URLID:         id,
		PrimaryPrice:  priceData,
		NeedsFeedback: true,
		FeedbackID:    feedbackID,
		HasAlternative: false, // Will be set to true if user rejects primary
		CheckedAt:     time.Now(),
	}
	
	writeJSON(w, http.StatusOK, response)
}

// SetPriceAlert creates a new price alert
func (h *Handlers) SetPriceAlert(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	var req models.SetAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.AlertType == "" {
		writeError(w, http.StatusBadRequest, "Alert type is required")
		return
	}

	if req.AlertType == "price_drop" && req.TargetPrice <= 0 {
		writeError(w, http.StatusBadRequest, "Target price is required for price drop alerts")
		return
	}

	if req.AlertType == "percentage_drop" && req.Percentage <= 0 {
		writeError(w, http.StatusBadRequest, "Percentage is required for percentage drop alerts")
		return
	}

	// Create alert
	alert, err := h.alertRepo.SetPriceAlert(id, req.TargetPrice, req.AlertType, req.Percentage)
	if err != nil {
		log.Printf("Failed to set price alert: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to set price alert")
		return
	}

	writeJSON(w, http.StatusCreated, alert)
}

// GetPriceAlerts returns all alerts for a URL
func (h *Handlers) GetPriceAlerts(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	alerts, err := h.alertRepo.GetPriceAlerts(id)
	if err != nil {
		log.Printf("Failed to get price alerts: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get price alerts")
		return
	}

	writeJSON(w, http.StatusOK, alerts)
}

// DeletePriceAlert deletes a price alert
func (h *Handlers) DeletePriceAlert(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	alertID, err := strconv.Atoi(vars["alertId"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid alert ID")
		return
	}

	if err := h.alertRepo.DeletePriceAlert(alertID); err != nil {
		log.Printf("Failed to delete price alert: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete price alert")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Alert deleted successfully"})
}

// HandleUserChoice saves the user's choice when prices don't match
func (h *Handlers) HandleUserChoice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	var choice models.UserChoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&choice); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if choice.URLID != id || choice.ChosenPrice <= 0 || choice.ChosenSource == "" || choice.ChosenMethod == "" {
		writeError(w, http.StatusBadRequest, "Invalid choice data")
		return
	}

	// Save the user's choice
	if h.scraper == nil {
		writeError(w, http.StatusInternalServerError, "Scraper not available")
		return
	}

	err = h.scraper.SaveUserChoice(id, &choice)
	if err != nil {
		log.Printf("Failed to save user choice: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to save user choice")
		return
	}

	// Update the URL with the chosen price
	priceData := &models.PriceData{
		CurrentPrice:     choice.ChosenPrice,
		OriginalPrice:    choice.ChosenPrice,
		Currency:         "USD",
		DiscountPercentage: 0.0,
		IsOnSale:         false,
		Source:           choice.ChosenSource,
		ExtractionMethod: choice.ChosenMethod,
		Confidence:       0.9, // High confidence for user choice
	}

	if err := h.urlRepo.UpdateURLPrice(id, priceData); err != nil {
		log.Printf("Failed to update URL price after user choice: %v", err)
	}

	if err := h.urlRepo.AddPriceHistory(id, priceData); err != nil {
		log.Printf("Failed to add price history after user choice: %v", err)
	}

	response := map[string]interface{}{
		"message": "User choice saved successfully",
		"choice":  choice,
		"updated_at": time.Now(),
	}

	writeJSON(w, http.StatusOK, response)
}



// DebugScreenshot takes a screenshot of a URL for debugging purposes
func (h *Handlers) DebugScreenshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "URL is required")
		return
	}

	if h.scraper == nil {
		writeError(w, http.StatusInternalServerError, "Scraper not available")
		return
	}

	// Create a new page for screenshot
	page := h.scraper.GetBrowser().MustPage(req.URL)
	defer page.MustClose()

	// Set viewport to 1600x1200 and wait for page to load
	page.MustSetViewport(1600, 1200, 1.0, false)
	page.MustWaitLoad()
	time.Sleep(8 * time.Second) // Wait for dynamic content
	page.MustWaitStable()
	time.Sleep(3 * time.Second)

	// Take screenshot of visible area
	screenshotBytes, err := page.Screenshot(false, nil)
	if err != nil {
		log.Printf("Failed to take screenshot: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to take screenshot")
		return
	}

	// Encode to base64
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshotBytes)

	response := map[string]interface{}{
		"success":    true,
		"screenshot": screenshotBase64,
		"url":        req.URL,
		"timestamp":  time.Now(),
	}

	writeJSON(w, http.StatusOK, response)
}

// CheckPriceNowAsync starts an async price check and returns a task ID
func (h *Handlers) CheckPriceNowAsync(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID")
		return
	}

	// Get URL details to validate it exists
	url, err := h.urlRepo.GetURLByID(id)
	if err != nil {
		if err.Error() == "URL not found" {
			writeError(w, http.StatusNotFound, "URL not found")
			return
		}
		log.Printf("Failed to get URL: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to get URL")
		return
	}

	// Submit task to task manager
	task := h.taskManager.SubmitTask(id)
	
	log.Printf("🚀 Async price check started for %s (URL ID: %d, Task ID: %s)", url.Name, id, task.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"task_id": task.ID,
		"status":  "queued",
		"message": "Price check queued for processing",
		"url_id":  id,
		"url_name": url.Name,
	})
}

// GetTaskStatus returns the status of an async task
func (h *Handlers) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	task, exists := h.taskManager.GetTask(taskID)
	if !exists {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// GetTaskStats returns statistics about the task manager
func (h *Handlers) GetTaskStats(w http.ResponseWriter, r *http.Request) {
	stats := h.taskManager.GetStats()
	
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stats":     stats,
		"timestamp": time.Now(),
	})
}

// API Key Management

// GenerateAPIKey generates a new API key
func (h *Handlers) GenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Plan   string `json:"plan"`
		Prefix string `json:"prefix,omitempty"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	
	if req.Plan == "" {
		req.Plan = "free" // Default to free plan
	}

	// Create a simple API key service instance for now
	// In production, this would be injected as a dependency
	apiKeyService := newAPIKeyService()
	
	var apiKey string
	var err error
	
	if req.Prefix != "" {
		apiKey, err = apiKeyService.GenerateAPIKeyWithPrefix(req.UserID, req.Plan, req.Prefix)
	} else {
		apiKey, err = apiKeyService.GenerateAPIKey(req.UserID, req.Plan)
	}
	
	if err != nil {
		log.Printf("Failed to generate API key: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to generate API key")
		return
	}

	response := map[string]interface{}{
		"api_key":    apiKey,
		"plan":       req.Plan,
		"user_id":    req.UserID,
		"created_at": time.Now(),
		"message":    "API key generated successfully",
	}

	writeJSON(w, http.StatusCreated, response)
}

// GetAPIKeyInfo returns information about an API key
func (h *Handlers) GetAPIKeyInfo(w http.ResponseWriter, r *http.Request) {
	// Extract API key from request
	apiKey := extractAPIKeyFromRequest(r)
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "API key is required")
		return
	}

	// Create API key service instance
	apiKeyService := newAPIKeyService()
	
	stats, err := apiKeyService.GetUsageStats(apiKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "API key not found or invalid")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// Helper function to create API key service (temporary implementation)
func newAPIKeyService() *services.APIKeyService {
	// This is a temporary implementation
	// In production, you'd inject this as a dependency
	return services.NewAPIKeyService(config.DefaultAPIConfig())
}

// Helper function to extract API key from request
func extractAPIKeyFromRequest(r *http.Request) string {
	// Check Authorization header first
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
		if strings.HasPrefix(auth, "ApiKey ") {
			return strings.TrimPrefix(auth, "ApiKey ")
		}
	}

	// Check X-API-Key header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	// Check query parameter
	if apiKey := r.URL.Query().Get("api_key"); apiKey != "" {
		return apiKey
	}

	return ""
}

// GetAlternativePrice returns the alternative price when user rejects primary price
func (h *Handlers) GetAlternativePrice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	feedbackID := vars["feedback_id"]
	
	if feedbackID == "" {
		writeError(w, http.StatusBadRequest, "Feedback ID is required")
		return
	}
	
	// For now, we'll extract URL ID from feedback ID and re-scrape with alternative method
	// In a production system, you'd store the alternative price data with the feedback ID
	
	// Parse feedback ID to get URL ID (format: feedback_{url_id}_{timestamp})
	parts := strings.Split(feedbackID, "_")
	if len(parts) < 3 {
		writeError(w, http.StatusBadRequest, "Invalid feedback ID format")
		return
	}
	
	urlID, err := strconv.Atoi(parts[1])
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid URL ID in feedback ID")
		return
	}
	
	// Get URL details
	url, err := h.urlRepo.GetURLByID(urlID)
	if err != nil {
		writeError(w, http.StatusNotFound, "URL not found")
		return
	}
	
	// Re-scrape with alternative method (force network extraction for Amazon, YOLO+OCR for others)
	if h.scraper == nil {
		writeError(w, http.StatusInternalServerError, "Scraper not available")
		return
	}
	
	// Force alternative method based on domain
	alternativePriceData, err := h.scraper.ScrapePriceWithAlternativeMethod(url.URL, urlID)
	if err != nil {
		log.Printf("Failed to get alternative price for %s: %v", url.Name, err)
		writeError(w, http.StatusInternalServerError, "Failed to get alternative price")
		return
	}
	
	response := models.PriceCheckResponse{
		URLID:            urlID,
		AlternativePrice: alternativePriceData,
		NeedsFeedback:    true,
		FeedbackID:       feedbackID,
		HasAlternative:   true,
		CheckedAt:        time.Now(),
	}
	
	writeJSON(w, http.StatusOK, response)
}

// SubmitPriceFeedback processes user feedback on price detection
func (h *Handlers) SubmitPriceFeedback(w http.ResponseWriter, r *http.Request) {
	var req models.PriceFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	
	// Validate request
	if req.FeedbackID == "" || req.URLID == 0 {
		writeError(w, http.StatusBadRequest, "Feedback ID and URL ID are required")
		return
	}
	
	// Get current scraping preference
	pref, err := h.prefRepo.GetPreference(req.URLID)
	if err != nil {
		// Create new preference if it doesn't exist
		pref = &models.ScrapingPreference{
			URLID:       req.URLID,
			Method:      "hybrid", // Default method
			Confidence:  0.5,
			SuccessRate: 0.5,
			LastUsed:    time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	}
	
	// Update preference based on user feedback
	updatedMethod := pref.Method
	confidenceUpdated := false
	
	if req.PrimaryCorrect {
		// User confirmed primary price - increase confidence for current method
		pref.SuccessRate = min(1.0, pref.SuccessRate + 0.1)
		confidenceUpdated = true
		log.Printf("✅ User confirmed primary price for URL %d, method: %s", req.URLID, pref.Method)
	} else if req.AlternativeCorrect {
		// User confirmed alternative price - switch to alternative method
		// Determine alternative method based on current method
		if strings.Contains(pref.Method, "yolo") || strings.Contains(pref.Method, "ocr") {
			updatedMethod = "network"
		} else {
			updatedMethod = "yolo_ocr"
		}
		
		pref.Method = updatedMethod
		pref.SuccessRate = min(1.0, pref.SuccessRate + 0.1)
		confidenceUpdated = true
		log.Printf("✅ User confirmed alternative price for URL %d, switching to method: %s", req.URLID, updatedMethod)
	} else {
		// Both prices were wrong - decrease confidence
		pref.SuccessRate = max(0.0, pref.SuccessRate - 0.1)
		confidenceUpdated = true
		log.Printf("❌ User rejected both prices for URL %d, decreasing confidence", req.URLID)
	}
	
	pref.LastUsed = time.Now()
	pref.UpdatedAt = time.Now()
	
	// Save updated preference
	if err := h.prefRepo.SavePreference(pref.URLID, pref.Method, pref.SuccessRate); err != nil {
		log.Printf("Failed to save preference: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to save preference")
		return
	}
	
	response := models.PriceFeedbackResponse{
		Success:           true,
		Message:           "Feedback processed successfully",
		UpdatedMethod:     updatedMethod,
		ConfidenceUpdated: confidenceUpdated,
	}
	
	writeJSON(w, http.StatusOK, response)
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
