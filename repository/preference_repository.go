package repository

import (
	"fmt"
	"time"

	"distrack/database"
	"distrack/models"
)

type PreferenceRepository struct{}

func NewPreferenceRepository() *PreferenceRepository {
	return &PreferenceRepository{}
}

// GetPreference returns the preferred scraping method for a URL
func (r *PreferenceRepository) GetPreference(urlID int) (*models.ScrapingPreference, error) {
	query := `
		SELECT url_id, method, confidence, success_rate, last_used, created_at, updated_at
		FROM scraping_preferences
		WHERE url_id = $1
	`

	var preference models.ScrapingPreference
	err := database.DB.QueryRow(query, urlID).Scan(
		&preference.URLID, &preference.Method, &preference.Confidence,
		&preference.SuccessRate, &preference.LastUsed, &preference.CreatedAt, &preference.UpdatedAt,
	)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil // No preference found
		}
		return nil, fmt.Errorf("failed to get preference: %v", err)
	}

	return &preference, nil
}

// SavePreference saves or updates the preferred scraping method for a URL
func (r *PreferenceRepository) SavePreference(urlID int, method string, confidence float64) error {
	query := `
		INSERT INTO scraping_preferences (url_id, method, confidence, success_rate, last_used, updated_at)
		VALUES ($1, $2, $3, 1.0, $4, $4)
		ON CONFLICT (url_id) DO UPDATE SET
			method = EXCLUDED.method,
			confidence = EXCLUDED.confidence,
			last_used = EXCLUDED.last_used,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	_, err := database.DB.Exec(query, urlID, method, confidence, now)
	if err != nil {
		return fmt.Errorf("failed to save preference: %v", err)
	}

	return nil
}

// UpdateSuccessRate updates the success rate for a URL's preferred method
func (r *PreferenceRepository) UpdateSuccessRate(urlID int, success bool) error {
	query := `
		UPDATE scraping_preferences 
		SET success_rate = CASE 
			WHEN success THEN LEAST(success_rate + 0.1, 1.0)
			ELSE GREATEST(success_rate - 0.1, 0.0)
		END,
		updated_at = $2
		WHERE url_id = $1
	`

	now := time.Now()
	_, err := database.DB.Exec(query, urlID, now)
	if err != nil {
		return fmt.Errorf("failed to update success rate: %v", err)
	}

	return nil
}
