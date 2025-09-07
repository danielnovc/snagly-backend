package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// APIKey represents an API key with associated limits
type APIKey struct {
	Key         string
	Plan        string
	RateLimit   int
	DailyLimit  int
	ValidUntil  time.Time
	IsActive    bool
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	DefaultRateLimit  int
	DefaultDailyLimit int
	PlanLimits        map[string]PlanLimit
}

// PlanLimit defines limits for different subscription plans
type PlanLimit struct {
	RateLimit  int
	DailyLimit int
}

// Default rate limiting configuration
var DefaultConfig = RateLimitConfig{
	DefaultRateLimit:  100, // requests per minute
	DefaultDailyLimit: 10000, // requests per day
	PlanLimits: map[string]PlanLimit{
		"free": {
			RateLimit:  60,  // 1 request per second
			DailyLimit: 1000,
		},
		"basic": {
			RateLimit:  300,  // 5 requests per second
			DailyLimit: 50000,
		},
		"pro": {
			RateLimit:  1000, // ~17 requests per second
			DailyLimit: 200000,
		},
		"enterprise": {
			RateLimit:  5000, // ~83 requests per second
			DailyLimit: 1000000,
		},
	},
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(config RateLimitConfig) func(http.Handler) http.Handler {
	// For now, implement a simple rate limiting approach
	// In production, you'd want to use Redis or a proper rate limiting library
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract API key from header
			apiKey := extractAPIKey(r)
			
			// Determine plan based on API key (default to free plan)
			plan := "free"
			if apiKey != "" {
				// In a real implementation, you'd validate the API key and get the plan
				// For now, we'll use a simple heuristic
				if strings.HasPrefix(apiKey, "pro_") {
					plan = "pro"
				} else if strings.HasPrefix(apiKey, "basic_") {
					plan = "basic"
				} else if strings.HasPrefix(apiKey, "enterprise_") {
					plan = "enterprise"
				} else if strings.HasPrefix(apiKey, "snag_") {
					// Snagly app gets pro plan access
					plan = "pro"
				}
			}

			// Get the rate limit for the plan
			rateLimit := config.PlanLimits[plan].RateLimit
			if rateLimit == 0 {
				rateLimit = config.DefaultRateLimit
			}

			// Add rate limit headers (simplified implementation)
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rateLimit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", rateLimit-1)) // Simplified
			w.Header().Set("X-RateLimit-Reset", time.Now().Add(time.Minute).Format(time.RFC3339))

			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyMiddleware validates API keys
func APIKeyMiddleware(required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip API key validation for health checks and documentation
			if r.URL.Path == "/health" || r.URL.Path == "/docs" || strings.HasPrefix(r.URL.Path, "/docs/") {
				next.ServeHTTP(w, r)
				return
			}

			apiKey := extractAPIKey(r)
			
			if required && apiKey == "" {
				http.Error(w, "API key required", http.StatusUnauthorized)
				return
			}

			if apiKey != "" {
				// In a real implementation, validate the API key
				if !isValidAPIKey(apiKey) {
					http.Error(w, "Invalid API key", http.StatusUnauthorized)
					return
				}
			}

			// Add API key info to context
			ctx := context.WithValue(r.Context(), "api_key", apiKey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractAPIKey extracts the API key from the request
func extractAPIKey(r *http.Request) string {
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

// isValidAPIKey validates an API key (placeholder implementation)
func isValidAPIKey(apiKey string) bool {
	// In a real implementation, you'd validate against a database
	// For now, just check if it's not empty and has a valid format
	if apiKey == "" {
		return false
	}

	// Simple format validation
	if len(apiKey) < 10 {
		return false
	}

	// Check if it's a test key
	if strings.HasPrefix(apiKey, "test_") {
		return true
	}

	// Check if it's a valid plan key
	validPrefixes := []string{"free_", "basic_", "pro_", "enterprise_", "snag_"}
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(apiKey, prefix) {
			return true
		}
	}

	return false
}

// LoggingMiddleware logs API requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		next.ServeHTTP(wrapped, r)
		
		duration := time.Since(start)
		
		// Log the request
		apiKey := extractAPIKey(r)
		if apiKey != "" {
			apiKey = apiKey[:8] + "..." // Truncate for logging
		}
		
		// In a real implementation, you'd use a proper logger
		// For now, we'll just print to stdout
		if r.URL.Path != "/health" && r.URL.Path != "/metrics" {
			// Only log non-health check requests
			log.Printf("API Request: %s %s (API Key: %s, Duration: %v)", r.Method, r.URL.Path, apiKey, duration)
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}
