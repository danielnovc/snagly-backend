package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// APIConfig holds API configuration settings
type APIConfig struct {
	Version           string
	BaseURL           string
	RequireAPIKey     bool
	RateLimitEnabled  bool
	LoggingEnabled    bool
	CORSEnabled       bool
	MaxRequestSize    int64
	RequestTimeout    time.Duration
	SubscriptionPlans map[string]SubscriptionPlan
}

// SubscriptionPlan defines the limits and features for each subscription tier
type SubscriptionPlan struct {
	Name           string
	RateLimit      int // requests per minute
	DailyLimit     int // requests per day
	MaxURLs        int // maximum tracked URLs
	MaxAlerts      int // maximum price alerts
	Webhooks       bool
	Priority       bool
	Price          float64 // monthly price in USD
	Features       []string
}

// DefaultAPIConfig returns the default API configuration
func DefaultAPIConfig() *APIConfig {
	return &APIConfig{
		Version:          "v1",
		BaseURL:          getEnv("API_BASE_URL", "https://api.distrack.com"),
		RequireAPIKey:    getEnvBool("API_REQUIRE_KEY", true),
		RateLimitEnabled: getEnvBool("API_RATE_LIMIT_ENABLED", true),
		LoggingEnabled:   getEnvBool("API_LOGGING_ENABLED", true),
		CORSEnabled:      getEnvBool("API_CORS_ENABLED", true),
		MaxRequestSize:   getEnvInt64("API_MAX_REQUEST_SIZE", 10*1024*1024), // 10MB
		RequestTimeout:   getEnvDuration("API_REQUEST_TIMEOUT", 30*time.Second),
		SubscriptionPlans: map[string]SubscriptionPlan{
			"free": {
				Name:       "Free",
				RateLimit:  60,
				DailyLimit: 1000,
				MaxURLs:    10,
				MaxAlerts:  5,
				Webhooks:   false,
				Priority:   false,
				Price:      0.0,
				Features: []string{
					"Basic price tracking",
					"Price alerts",
					"Price history",
					"Email notifications",
				},
			},
			"basic": {
				Name:       "Basic",
				RateLimit:  300,
				DailyLimit: 50000,
				MaxURLs:    100,
				MaxAlerts:  50,
				Webhooks:   true,
				Priority:   false,
				Price:      29.99,
				Features: []string{
					"Everything in Free",
					"Webhook support",
					"Priority support",
					"Advanced analytics",
					"Bulk operations",
				},
			},
			"pro": {
				Name:       "Professional",
				RateLimit:  1000,
				DailyLimit: 200000,
				MaxURLs:    1000,
				MaxAlerts:  500,
				Webhooks:   true,
				Priority:   true,
				Price:      99.99,
				Features: []string{
					"Everything in Basic",
					"Priority processing",
					"Custom webhook endpoints",
					"Advanced filtering",
					"Data export",
					"API usage analytics",
				},
			},
			"enterprise": {
				Name:       "Enterprise",
				RateLimit:  5000,
				DailyLimit: 1000000,
				MaxURLs:    10000,
				MaxAlerts:  5000,
				Webhooks:   true,
				Priority:   true,
				Price:      299.99,
				Features: []string{
					"Everything in Pro",
					"Custom rate limits",
					"Dedicated support",
					"Custom integrations",
					"White-label options",
					"SLA guarantees",
				},
			},
		},
	}
}

// GetPlan returns a subscription plan by name
func (c *APIConfig) GetPlan(planName string) (SubscriptionPlan, bool) {
	plan, exists := c.SubscriptionPlans[planName]
	return plan, exists
}

// GetDefaultPlan returns the free plan
func (c *APIConfig) GetDefaultPlan() SubscriptionPlan {
	return c.SubscriptionPlans["free"]
}

// IsValidPlan checks if a plan name is valid
func (c *APIConfig) IsValidPlan(planName string) bool {
	_, exists := c.SubscriptionPlans[planName]
	return exists
}

// GetPlanByAPIKey determines the plan based on API key prefix
func (c *APIConfig) GetPlanByAPIKey(apiKey string) SubscriptionPlan {
	if apiKey == "" {
		return c.GetDefaultPlan()
	}

	// Simple heuristic based on API key prefix
	// In production, this would be looked up from a database
	if len(apiKey) < 10 {
		return c.GetDefaultPlan()
	}

	if strings.HasPrefix(apiKey, "enterprise_") {
		return c.SubscriptionPlans["enterprise"]
	} else if strings.HasPrefix(apiKey, "pro_") {
		return c.SubscriptionPlans["pro"]
	} else if strings.HasPrefix(apiKey, "basic_") {
		return c.SubscriptionPlans["basic"]
	} else if strings.HasPrefix(apiKey, "test_") {
		// Test keys get pro plan access
		return c.SubscriptionPlans["pro"]
	}

	return c.GetDefaultPlan()
}

// Helper functions for environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

