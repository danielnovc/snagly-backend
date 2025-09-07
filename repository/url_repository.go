package repository

import (
	"database/sql"
	"fmt"
	"time"

	"distrack/database"
	"distrack/models"
)

type URLRepository struct{}

func NewURLRepository() *URLRepository {
	return &URLRepository{}
}

// AddURLToTrack adds a new URL to track
func (r *URLRepository) AddURLToTrack(url, name string) (*models.TrackedURL, error) {
	query := `
		INSERT INTO tracked_urls (url, name, created_at, updated_at, retry_count)
		VALUES ($1, $2, $3, $3, 0)
		RETURNING id, url, name, current_price, original_price, currency, last_checked, last_failed_at, retry_count, next_retry_at, created_at, updated_at, is_active
	`

	var trackedURL models.TrackedURL
	now := time.Now()
	err := database.DB.QueryRow(query, url, name, now).Scan(
		&trackedURL.ID, &trackedURL.URL, &trackedURL.Name,
		&trackedURL.CurrentPrice, &trackedURL.OriginalPrice, &trackedURL.Currency,
		&trackedURL.LastChecked, &trackedURL.LastFailedAt, &trackedURL.RetryCount,
		&trackedURL.NextRetryAt, &trackedURL.CreatedAt, &trackedURL.UpdatedAt, &trackedURL.IsActive,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to add URL to track: %v", err)
	}

	return &trackedURL, nil
}

// GetTrackedURLs returns all tracked URLs
func (r *URLRepository) GetTrackedURLs() ([]models.TrackedURL, error) {
	query := `
		SELECT id, url, name, current_price, original_price, currency, last_checked, last_failed_at, retry_count, next_retry_at, created_at, updated_at, is_active
		FROM tracked_urls
		WHERE is_active = true
		ORDER BY created_at DESC
	`

	rows, err := database.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get tracked URLs: %v", err)
	}
	defer rows.Close()

	var urls []models.TrackedURL
	for rows.Next() {
		var url models.TrackedURL
		err := rows.Scan(
			&url.ID, &url.URL, &url.Name,
			&url.CurrentPrice, &url.OriginalPrice, &url.Currency,
			&url.LastChecked, &url.LastFailedAt, &url.RetryCount,
			&url.NextRetryAt, &url.CreatedAt, &url.UpdatedAt, &url.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan URL: %v", err)
		}
		urls = append(urls, url)
	}

	return urls, nil
}

// GetURLByID returns a tracked URL by ID
func (r *URLRepository) GetURLByID(id int) (*models.TrackedURL, error) {
	query := `
		SELECT id, url, name, current_price, original_price, currency, last_checked, last_failed_at, retry_count, next_retry_at, created_at, updated_at, is_active
		FROM tracked_urls
		WHERE id = $1 AND is_active = true
	`

	var url models.TrackedURL
	err := database.DB.QueryRow(query, id).Scan(
		&url.ID, &url.URL, &url.Name,
		&url.CurrentPrice, &url.OriginalPrice, &url.Currency,
		&url.LastChecked, &url.LastFailedAt, &url.RetryCount,
		&url.NextRetryAt, &url.CreatedAt, &url.UpdatedAt, &url.IsActive,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("URL not found")
		}
		return nil, fmt.Errorf("failed to get URL: %v", err)
	}

	return &url, nil
}

// UpdateURLPrice updates the price information for a tracked URL
func (r *URLRepository) UpdateURLPrice(id int, priceData *models.PriceData) error {
	query := `
		UPDATE tracked_urls
		SET current_price = $2, original_price = $3, currency = $4, last_checked = $5, updated_at = $6
		WHERE id = $1
	`

	_, err := database.DB.Exec(query, id, priceData.CurrentPrice, priceData.OriginalPrice, priceData.Currency, time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to update URL price: %v", err)
	}

	return nil
}

// DeleteTrackedURL deletes a tracked URL
func (r *URLRepository) DeleteTrackedURL(id int) error {
	query := `UPDATE tracked_urls SET is_active = false WHERE id = $1`
	_, err := database.DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tracked URL: %v", err)
	}
	return nil
}

// AddPriceHistory adds a price point to the history
func (r *URLRepository) AddPriceHistory(urlID int, priceData *models.PriceData) error {
	query := `
		INSERT INTO price_history (url_id, price, currency, discount_percentage, original_price, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := database.DB.Exec(query, urlID, priceData.CurrentPrice, priceData.Currency, priceData.DiscountPercentage, priceData.OriginalPrice, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add price history: %v", err)
	}

	return nil
}

// GetPriceHistory returns price history for a URL
func (r *URLRepository) GetPriceHistory(urlID int, limit int) ([]models.PriceHistory, error) {
	if limit <= 0 {
		limit = 50 // default limit
	}

	query := `
		SELECT id, url_id, price, currency, discount_percentage, original_price, checked_at
		FROM price_history
		WHERE url_id = $1
		ORDER BY checked_at DESC
		LIMIT $2
	`

	rows, err := database.DB.Query(query, urlID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get price history: %v", err)
	}
	defer rows.Close()

	var history []models.PriceHistory
	for rows.Next() {
		var entry models.PriceHistory
		err := rows.Scan(
			&entry.ID, &entry.URLID, &entry.Price, &entry.Currency,
			&entry.DiscountPercentage, &entry.OriginalPrice, &entry.CheckedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan price history: %v", err)
		}
		history = append(history, entry)
	}

	return history, nil
}

// MarkPriceCheckFailed marks a price check as failed and schedules retry
func (r *URLRepository) MarkPriceCheckFailed(id int) error {
	query := `
		UPDATE tracked_urls 
		SET last_failed_at = $1, retry_count = retry_count + 1, next_retry_at = $2, updated_at = $1
		WHERE id = $3
	`

	now := time.Now()
	nextRetry := now.Add(time.Duration(10) * time.Minute) // Start with 10 minutes

	_, err := database.DB.Exec(query, now, nextRetry, id)
	if err != nil {
		return fmt.Errorf("failed to mark price check as failed: %v", err)
	}

	return nil
}

// MarkPriceCheckSuccess resets retry count when price check succeeds
func (r *URLRepository) MarkPriceCheckSuccess(id int) error {
	query := `
		UPDATE tracked_urls 
		SET last_failed_at = NULL, retry_count = 0, next_retry_at = NULL, updated_at = $1
		WHERE id = $2
	`

	now := time.Now()
	_, err := database.DB.Exec(query, now, id)
	if err != nil {
		return fmt.Errorf("failed to mark price check as successful: %v", err)
	}

	return nil
}

// GetURLsForRetry returns URLs that should be retried
func (r *URLRepository) GetURLsForRetry() ([]models.TrackedURL, error) {
	query := `
		SELECT id, url, name, current_price, original_price, currency, last_checked, last_failed_at, retry_count, next_retry_at, created_at, updated_at, is_active
		FROM tracked_urls
		WHERE is_active = true 
		AND last_failed_at IS NOT NULL 
		AND (next_retry_at IS NULL OR next_retry_at <= $1)
		AND retry_count < 5
		ORDER BY next_retry_at ASC
	`

	now := time.Now()
	rows, err := database.DB.Query(query, now)
	if err != nil {
		return nil, fmt.Errorf("failed to get URLs for retry: %v", err)
	}
	defer rows.Close()

	var urls []models.TrackedURL
	for rows.Next() {
		var url models.TrackedURL
		err := rows.Scan(
			&url.ID, &url.URL, &url.Name,
			&url.CurrentPrice, &url.OriginalPrice, &url.Currency,
			&url.LastChecked, &url.LastFailedAt, &url.RetryCount,
			&url.NextRetryAt, &url.CreatedAt, &url.UpdatedAt, &url.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan URL: %v", err)
		}
		urls = append(urls, url)
	}

	return urls, nil
}
