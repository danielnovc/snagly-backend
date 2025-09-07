package repository

import (
	"fmt"
	"time"

	"distrack/database"
	"distrack/models"
)

type AlertRepository struct{}

func NewAlertRepository() *AlertRepository {
	return &AlertRepository{}
}

// SetPriceAlert creates a new price alert
func (r *AlertRepository) SetPriceAlert(urlID int, targetPrice float64, alertType string, percentage float64) (*models.PriceAlert, error) {
	query := `
		INSERT INTO price_alerts (url_id, target_price, alert_type, percentage, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, url_id, target_price, alert_type, percentage, is_active, created_at, triggered_at
	`

	var priceAlert models.PriceAlert
	now := time.Now()
	err := database.DB.QueryRow(query, urlID, targetPrice, alertType, percentage, now).Scan(
		&priceAlert.ID, &priceAlert.URLID, &priceAlert.TargetPrice,
		&priceAlert.AlertType, &priceAlert.Percentage, &priceAlert.IsActive,
		&priceAlert.CreatedAt, &priceAlert.TriggeredAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to set price alert: %v", err)
	}

	return &priceAlert, nil
}

// GetPriceAlerts returns all alerts for a URL
func (r *AlertRepository) GetPriceAlerts(urlID int) ([]models.PriceAlert, error) {
	query := `
		SELECT id, url_id, target_price, alert_type, percentage, is_active, created_at, triggered_at
		FROM price_alerts
		WHERE url_id = $1 AND is_active = true
		ORDER BY created_at DESC
	`

	rows, err := database.DB.Query(query, urlID)
	if err != nil {
		return nil, fmt.Errorf("failed to get price alerts: %v", err)
	}
	defer rows.Close()

	var alerts []models.PriceAlert
	for rows.Next() {
		var alert models.PriceAlert
		err := rows.Scan(
			&alert.ID, &alert.URLID, &alert.TargetPrice,
			&alert.AlertType, &alert.Percentage, &alert.IsActive,
			&alert.CreatedAt, &alert.TriggeredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan price alert: %v", err)
		}
		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// DeletePriceAlert deletes a price alert
func (r *AlertRepository) DeletePriceAlert(alertID int) error {
	query := `UPDATE price_alerts SET is_active = false WHERE id = $1`
	_, err := database.DB.Exec(query, alertID)
	if err != nil {
		return fmt.Errorf("failed to delete price alert: %v", err)
	}
	return nil
}

// GetActiveAlertsForURL returns all active alerts for a URL
func (r *AlertRepository) GetActiveAlertsForURL(urlID int) ([]models.PriceAlert, error) {
	query := `
		SELECT id, url_id, target_price, alert_type, percentage, is_active, created_at, triggered_at
		FROM price_alerts
		WHERE url_id = $1 AND is_active = true AND triggered_at IS NULL
	`

	rows, err := database.DB.Query(query, urlID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active alerts: %v", err)
	}
	defer rows.Close()

	var alerts []models.PriceAlert
	for rows.Next() {
		var alert models.PriceAlert
		err := rows.Scan(
			&alert.ID, &alert.URLID, &alert.TargetPrice,
			&alert.AlertType, &alert.Percentage, &alert.IsActive,
			&alert.CreatedAt, &alert.TriggeredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan price alert: %v", err)
		}
		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// TriggerAlert marks an alert as triggered
func (r *AlertRepository) TriggerAlert(alertID int) error {
	query := `UPDATE price_alerts SET triggered_at = $1 WHERE id = $2`
	_, err := database.DB.Exec(query, time.Now(), alertID)
	if err != nil {
		return fmt.Errorf("failed to trigger alert: %v", err)
	}
	return nil
}

// CheckAlerts checks if any alerts should be triggered based on current price
func (r *AlertRepository) CheckAlerts(urlID int, currentPrice float64) ([]models.PriceAlert, error) {
	alerts, err := r.GetActiveAlertsForURL(urlID)
	if err != nil {
		return nil, err
	}

	var triggeredAlerts []models.PriceAlert
	for _, alert := range alerts {
		shouldTrigger := false

		switch alert.AlertType {
		case "price_drop":
			if currentPrice <= alert.TargetPrice {
				shouldTrigger = true
			}
		case "percentage_drop":
			// Get the original price from the URL
			url, err := NewURLRepository().GetURLByID(urlID)
			if err != nil {
				continue
			}
			
			if url.OriginalPrice.Valid && url.OriginalPrice.Float64 > 0 {
				discountPercentage := ((url.OriginalPrice.Float64 - currentPrice) / url.OriginalPrice.Float64) * 100
				if discountPercentage >= alert.Percentage {
					shouldTrigger = true
				}
			}
		}

		if shouldTrigger {
			if err := r.TriggerAlert(alert.ID); err != nil {
				continue
			}
			alert.TriggeredAt = &time.Time{}
			*alert.TriggeredAt = time.Now()
			triggeredAlerts = append(triggeredAlerts, alert)
		}
	}

	return triggeredAlerts, nil
}
