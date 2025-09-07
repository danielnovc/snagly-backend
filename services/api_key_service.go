package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"distrack/config"
	"distrack/models"
	"distrack/repository"
)

// APIKeyService manages API keys and their associated plans
type APIKeyService struct {
	config *config.APIConfig
	repo   *repository.APIKeyRepository
}

// APIKeyInfo represents information about an API key
type APIKeyInfo struct {
	Key         string
	Plan        string
	UserID      string
	CreatedAt   time.Time
	LastUsed    time.Time
	IsActive    bool
	DailyUsage  int
	MonthlyUsage int
	MaxDaily    int
	MaxMonthly  int
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(apiConfig *config.APIConfig) *APIKeyService {
	return &APIKeyService{
		config: apiConfig,
		repo:   repository.NewAPIKeyRepository(),
	}
}


// getPlanLimit gets the limit for a specific plan and period
func (s *APIKeyService) getPlanLimit(plan, period string) int {
	planConfig, exists := s.config.GetPlan(plan)
	if !exists {
		planConfig = s.config.GetDefaultPlan()
	}

	if period == "daily" {
		return planConfig.DailyLimit
	}
	// For monthly, we'll use daily * 30 as a simple calculation
	return planConfig.DailyLimit * 30
}

// ValidateAPIKey validates an API key and returns the associated plan
func (s *APIKeyService) ValidateAPIKey(apiKey string) (*APIKeyInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Get API key from database
	dbKey, err := s.repo.GetAPIKeyByKey(apiKey)
	if err != nil {
		return nil, fmt.Errorf("Invalid API key")
	}

	if !dbKey.IsActive {
		return nil, fmt.Errorf("API key is inactive")
	}

	// Check daily usage limit
	if dbKey.DailyUsage >= dbKey.MaxDaily {
		return nil, fmt.Errorf("Daily usage limit exceeded")
	}

	// Increment daily usage and update last used time
	dbKey.DailyUsage++
	dbKey.LastUsed = time.Now()

	// Update in database
	err = s.repo.UpdateAPIKeyUsage(dbKey)
	if err != nil {
		return nil, fmt.Errorf("Failed to update API key usage: %v", err)
	}

	// Convert to APIKeyInfo
	keyInfo := &APIKeyInfo{
		Key:         apiKey,
		Plan:        dbKey.Plan,
		UserID:      dbKey.UserID,
		CreatedAt:   dbKey.CreatedAt,
		LastUsed:    dbKey.LastUsed,
		IsActive:    dbKey.IsActive,
		DailyUsage:  dbKey.DailyUsage,
		MonthlyUsage: dbKey.MonthlyUsage,
		MaxDaily:    dbKey.MaxDaily,
		MaxMonthly:  dbKey.MaxMonthly,
	}

	return keyInfo, nil
}

// GenerateAPIKey generates a new API key for a user
func (s *APIKeyService) GenerateAPIKey(userID, plan string) (string, error) {
	// Validate plan
	if !s.config.IsValidPlan(plan) {
		return "", fmt.Errorf("Invalid plan: %s", plan)
	}

	// Generate a random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("Failed to generate random key: %v", err)
	}

	// Create a prefix based on the plan
	prefix := ""
	switch plan {
	case "free":
		prefix = "free"
	case "basic":
		prefix = "basic"
	case "pro":
		prefix = "pro"
	case "enterprise":
		prefix = "enterprise"
	default:
		prefix = "key"
	}

	// Hash the random bytes and take first 16 characters
	hash := sha256.Sum256(keyBytes)
	keySuffix := hex.EncodeToString(hash[:])[:16]

	// Combine prefix and suffix
	apiKey := prefix + "_" + keySuffix

	// Create API key model
	dbKey := &models.APIKey{
		Key:         apiKey,
		Prefix:      prefix,
		UserID:      userID,
		Plan:        plan,
		DailyUsage:  0,
		MonthlyUsage: 0,
		MaxDaily:    s.getPlanLimit(plan, "daily"),
		MaxMonthly:  s.getPlanLimit(plan, "monthly"),
		IsActive:    true,
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
	}

	// Store in database
	err := s.repo.CreateAPIKey(dbKey)
	if err != nil {
		return "", fmt.Errorf("Failed to store API key: %v", err)
	}

	return apiKey, nil
}

// GenerateAPIKeyWithPrefix generates a new API key with a custom prefix
func (s *APIKeyService) GenerateAPIKeyWithPrefix(userID, plan, prefix string) (string, error) {
	// Validate plan
	if !s.config.IsValidPlan(plan) {
		return "", fmt.Errorf("Invalid plan: %s", plan)
	}

	// Generate a random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("Failed to generate random key: %v", err)
	}

	// Hash the random bytes and take first 16 characters
	hash := sha256.Sum256(keyBytes)
	keySuffix := hex.EncodeToString(hash[:])[:16]

	// Combine custom prefix and suffix
	apiKey := prefix + "_" + keySuffix

	// Create API key model
	dbKey := &models.APIKey{
		Key:         apiKey,
		Prefix:      prefix,
		UserID:      userID,
		Plan:        plan,
		DailyUsage:  0,
		MonthlyUsage: 0,
		MaxDaily:    s.getPlanLimit(plan, "daily"),
		MaxMonthly:  s.getPlanLimit(plan, "monthly"),
		IsActive:    true,
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
	}

	// Store in database
	err := s.repo.CreateAPIKey(dbKey)
	if err != nil {
		return "", fmt.Errorf("Failed to store API key: %v", err)
	}

	return apiKey, nil
}

// GetAPIKeyInfo returns information about an API key
func (s *APIKeyService) GetAPIKeyInfo(apiKey string) (*APIKeyInfo, error) {
	dbKey, err := s.repo.GetAPIKeyByKey(apiKey)
	if err != nil {
		return nil, fmt.Errorf("API key not found")
	}

	keyInfo := &APIKeyInfo{
		Key:         apiKey,
		Plan:        dbKey.Plan,
		UserID:      dbKey.UserID,
		CreatedAt:   dbKey.CreatedAt,
		LastUsed:    dbKey.LastUsed,
		IsActive:    dbKey.IsActive,
		DailyUsage:  dbKey.DailyUsage,
		MonthlyUsage: dbKey.MonthlyUsage,
		MaxDaily:    dbKey.MaxDaily,
		MaxMonthly:  dbKey.MaxMonthly,
	}

	return keyInfo, nil
}

// DeactivateAPIKey deactivates an API key
func (s *APIKeyService) DeactivateAPIKey(apiKey string) error {
	dbKey, err := s.repo.GetAPIKeyByKey(apiKey)
	if err != nil {
		return fmt.Errorf("API key not found")
	}

	return s.repo.DeactivateAPIKey(dbKey.ID)
}

// GetUsageStats returns usage statistics for an API key
func (s *APIKeyService) GetUsageStats(apiKey string) (map[string]interface{}, error) {
	dbKey, err := s.repo.GetAPIKeyByKey(apiKey)
	if err != nil {
		return nil, fmt.Errorf("API key not found")
	}

	planConfig, _ := s.config.GetPlan(dbKey.Plan)

	stats := map[string]interface{}{
		"plan": map[string]interface{}{
			"name":           planConfig.Name,
			"rate_limit":     planConfig.RateLimit,
			"daily_limit":    planConfig.DailyLimit,
			"max_urls":       planConfig.MaxURLs,
			"max_alerts":     planConfig.MaxAlerts,
			"webhooks":       planConfig.Webhooks,
			"priority":       planConfig.Priority,
			"monthly_price":  planConfig.Price,
		},
		"usage": map[string]interface{}{
			"daily_usage":    dbKey.DailyUsage,
			"monthly_usage":  dbKey.MonthlyUsage,
			"max_daily":      dbKey.MaxDaily,
			"max_monthly":    dbKey.MaxMonthly,
			"daily_remaining": dbKey.MaxDaily - dbKey.DailyUsage,
			"monthly_remaining": dbKey.MaxMonthly - dbKey.MonthlyUsage,
		},
		"key_info": map[string]interface{}{
			"created_at":     dbKey.CreatedAt,
			"last_used":      dbKey.LastUsed,
			"is_active":      dbKey.IsActive,
		},
	}

	return stats, nil
}

// ResetDailyUsage resets the daily usage counter (should be called daily)
func (s *APIKeyService) ResetDailyUsage() error {
	return s.repo.ResetDailyUsage()
}

// ResetMonthlyUsage resets the monthly usage counter (should be called monthly)
func (s *APIKeyService) ResetMonthlyUsage() error {
	return s.repo.ResetMonthlyUsage()
}
