package models

import "time"

// APIKey represents an API key in the database
type APIKey struct {
	ID           int       `json:"id" db:"id"`
	KeyHash      string    `json:"-" db:"key_hash"`      // Never expose the hash
	Prefix       string    `json:"prefix" db:"key_prefix"`
	UserID       string    `json:"user_id" db:"user_id"`
	Plan         string    `json:"plan" db:"plan"`
	DailyUsage   int       `json:"daily_usage" db:"daily_usage"`
	MonthlyUsage int       `json:"monthly_usage" db:"monthly_usage"`
	MaxDaily     int       `json:"max_daily" db:"max_daily"`
	MaxMonthly   int       `json:"max_monthly" db:"max_monthly"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	LastUsed     time.Time `json:"last_used" db:"last_used"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	
	// Transient fields (not stored in DB)
	Key string `json:"key,omitempty"` // Only set when creating/returning the key
}

// APIKeyInfo represents public information about an API key (without sensitive data)
type APIKeyInfo struct {
	ID           int       `json:"id"`
	Prefix       string    `json:"prefix"`
	UserID       string    `json:"user_id"`
	Plan         string    `json:"plan"`
	DailyUsage   int       `json:"daily_usage"`
	MonthlyUsage int       `json:"monthly_usage"`
	MaxDaily     int       `json:"max_daily"`
	MaxMonthly   int       `json:"max_monthly"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsed     time.Time `json:"last_used"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ToAPIKeyInfo converts APIKey to APIKeyInfo (removes sensitive data)
func (ak *APIKey) ToAPIKeyInfo() *APIKeyInfo {
	return &APIKeyInfo{
		ID:           ak.ID,
		Prefix:       ak.Prefix,
		UserID:       ak.UserID,
		Plan:         ak.Plan,
		DailyUsage:   ak.DailyUsage,
		MonthlyUsage: ak.MonthlyUsage,
		MaxDaily:     ak.MaxDaily,
		MaxMonthly:   ak.MaxMonthly,
		IsActive:     ak.IsActive,
		CreatedAt:    ak.CreatedAt,
		LastUsed:     ak.LastUsed,
		UpdatedAt:    ak.UpdatedAt,
	}
}

// APIKeyRequest represents a request to create an API key
type APIKeyRequest struct {
	UserID string `json:"user_id" validate:"required"`
	Plan   string `json:"plan" validate:"required,oneof=free basic pro enterprise"`
	Prefix string `json:"prefix,omitempty"`
}

// APIKeyResponse represents the response when creating an API key
type APIKeyResponse struct {
	APIKey    string    `json:"api_key"`
	KeyInfo   *APIKeyInfo `json:"key_info"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKeyStats represents usage statistics for an API key
type APIKeyStats struct {
	Plan map[string]interface{} `json:"plan"`
	Usage map[string]interface{} `json:"usage"`
	KeyInfo map[string]interface{} `json:"key_info"`
}
