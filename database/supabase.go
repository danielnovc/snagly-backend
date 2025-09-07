package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

var DB *sql.DB

// InitDatabase initializes the database connection
func InitDatabase() error {
	// Get database connection string from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}

	var err error
	DB, err = sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Test the connection
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	log.Println("Successfully connected to database")
	return nil
}

// CreateTables creates the necessary tables if they don't exist
func CreateTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS tracked_urls (
			id SERIAL PRIMARY KEY,
			url TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			current_price DECIMAL(10,2),
			original_price DECIMAL(10,2),
			currency VARCHAR(3) DEFAULT 'USD',
			last_checked TIMESTAMP,
			last_failed_at TIMESTAMP,
			retry_count INTEGER DEFAULT 0,
			next_retry_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			is_active BOOLEAN DEFAULT TRUE
		)`,
		`CREATE TABLE IF NOT EXISTS price_history (
			id SERIAL PRIMARY KEY,
			url_id INTEGER REFERENCES tracked_urls(id) ON DELETE CASCADE,
			price DECIMAL(10,2) NOT NULL,
			currency VARCHAR(3) DEFAULT 'USD',
			discount_percentage DECIMAL(5,2) DEFAULT 0,
			original_price DECIMAL(10,2),
			checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS price_alerts (
			id SERIAL PRIMARY KEY,
			url_id INTEGER REFERENCES tracked_urls(id) ON DELETE CASCADE,
			target_price DECIMAL(10,2) NOT NULL,
			alert_type VARCHAR(20) NOT NULL CHECK (alert_type IN ('price_drop', 'percentage_drop')),
			percentage DECIMAL(5,2),
			is_active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			triggered_at TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS scraping_preferences (
			url_id INTEGER PRIMARY KEY REFERENCES tracked_urls(id) ON DELETE CASCADE,
			method VARCHAR(20) NOT NULL CHECK (method IN ('network', 'ocr', 'hybrid')),
			confidence DECIMAL(3,2) DEFAULT 0.0,
			success_rate DECIMAL(3,2) DEFAULT 0.0,
			last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id SERIAL PRIMARY KEY,
			key_hash VARCHAR(64) NOT NULL UNIQUE,
			key_prefix VARCHAR(20) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			plan VARCHAR(20) NOT NULL CHECK (plan IN ('free', 'basic', 'pro', 'enterprise')),
			daily_usage INTEGER DEFAULT 0,
			monthly_usage INTEGER DEFAULT 0,
			max_daily INTEGER NOT NULL,
			max_monthly INTEGER NOT NULL,
			is_active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE INDEX IF NOT EXISTS idx_tracked_urls_retry ON tracked_urls (last_failed_at, next_retry_at, retry_count) 
		WHERE last_failed_at IS NOT NULL AND retry_count < 5`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys (key_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys (user_id)`,

	}

	for _, query := range queries {
		_, err := DB.Exec(query)
		if err != nil {
			return fmt.Errorf("failed to create table: %v", err)
		}
	}

	return nil
}

// CloseDatabase closes the database connection
func CloseDatabase() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
