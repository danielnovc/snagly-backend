package repository

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"distrack/database"
	"distrack/models"
)

// APIKeyRepository handles API key database operations
type APIKeyRepository struct {
	db *sql.DB
}

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository() *APIKeyRepository {
	return &APIKeyRepository{
		db: database.DB,
	}
}

// CreateAPIKey creates a new API key in the database
func (r *APIKeyRepository) CreateAPIKey(apiKey *models.APIKey) error {
	// Hash the API key for storage
	hash := sha256.Sum256([]byte(apiKey.Key))
	keyHash := hex.EncodeToString(hash[:])

	query := `
		INSERT INTO api_keys (key_hash, key_prefix, user_id, plan, max_daily, max_monthly, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	var id int
	var createdAt time.Time
	err := r.db.QueryRow(query, keyHash, apiKey.Prefix, apiKey.UserID, apiKey.Plan, apiKey.MaxDaily, apiKey.MaxMonthly, apiKey.IsActive).Scan(&id, &createdAt)
	if err != nil {
		return fmt.Errorf("failed to create API key: %v", err)
	}

	apiKey.ID = id
	apiKey.CreatedAt = createdAt
	return nil
}

// GetAPIKeyByHash retrieves an API key by its hash
func (r *APIKeyRepository) GetAPIKeyByHash(keyHash string) (*models.APIKey, error) {
	query := `
		SELECT id, key_hash, key_prefix, user_id, plan, daily_usage, monthly_usage, 
		       max_daily, max_monthly, is_active, created_at, last_used, updated_at
		FROM api_keys 
		WHERE key_hash = $1
	`

	apiKey := &models.APIKey{}
	err := r.db.QueryRow(query, keyHash).Scan(
		&apiKey.ID, &apiKey.KeyHash, &apiKey.Prefix, &apiKey.UserID, &apiKey.Plan,
		&apiKey.DailyUsage, &apiKey.MonthlyUsage, &apiKey.MaxDaily, &apiKey.MaxMonthly,
		&apiKey.IsActive, &apiKey.CreatedAt, &apiKey.LastUsed, &apiKey.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("API key not found")
		}
		return nil, fmt.Errorf("failed to get API key: %v", err)
	}

	return apiKey, nil
}

// GetAPIKeyByKey retrieves an API key by the actual key (hashes it first)
func (r *APIKeyRepository) GetAPIKeyByKey(apiKey string) (*models.APIKey, error) {
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])
	return r.GetAPIKeyByHash(keyHash)
}

// UpdateAPIKeyUsage updates the usage statistics for an API key
func (r *APIKeyRepository) UpdateAPIKeyUsage(apiKey *models.APIKey) error {
	query := `
		UPDATE api_keys 
		SET daily_usage = $1, monthly_usage = $2, last_used = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $4
	`

	_, err := r.db.Exec(query, apiKey.DailyUsage, apiKey.MonthlyUsage, apiKey.LastUsed, apiKey.ID)
	if err != nil {
		return fmt.Errorf("failed to update API key usage: %v", err)
	}

	return nil
}

// DeactivateAPIKey deactivates an API key
func (r *APIKeyRepository) DeactivateAPIKey(apiKeyID int) error {
	query := `UPDATE api_keys SET is_active = FALSE, updated_at = CURRENT_TIMESTAMP WHERE id = $1`
	_, err := r.db.Exec(query, apiKeyID)
	if err != nil {
		return fmt.Errorf("failed to deactivate API key: %v", err)
	}
	return nil
}

// GetAPIKeysByUser retrieves all API keys for a specific user
func (r *APIKeyRepository) GetAPIKeysByUser(userID string) ([]*models.APIKey, error) {
	query := `
		SELECT id, key_hash, key_prefix, user_id, plan, daily_usage, monthly_usage, 
		       max_daily, max_monthly, is_active, created_at, last_used, updated_at
		FROM api_keys 
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys: %v", err)
	}
	defer rows.Close()

	var apiKeys []*models.APIKey
	for rows.Next() {
		apiKey := &models.APIKey{}
		err := rows.Scan(
			&apiKey.ID, &apiKey.KeyHash, &apiKey.Prefix, &apiKey.UserID, &apiKey.Plan,
			&apiKey.DailyUsage, &apiKey.MonthlyUsage, &apiKey.MaxDaily, &apiKey.MaxMonthly,
			&apiKey.IsActive, &apiKey.CreatedAt, &apiKey.LastUsed, &apiKey.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %v", err)
		}
		apiKeys = append(apiKeys, apiKey)
	}

	return apiKeys, nil
}

// ResetDailyUsage resets daily usage for all API keys
func (r *APIKeyRepository) ResetDailyUsage() error {
	query := `UPDATE api_keys SET daily_usage = 0, updated_at = CURRENT_TIMESTAMP`
	_, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to reset daily usage: %v", err)
	}
	return nil
}

// ResetMonthlyUsage resets monthly usage for all API keys
func (r *APIKeyRepository) ResetMonthlyUsage() error {
	query := `UPDATE api_keys SET monthly_usage = 0, updated_at = CURRENT_TIMESTAMP`
	_, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to reset monthly usage: %v", err)
	}
	return nil
}
