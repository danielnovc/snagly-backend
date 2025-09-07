-- Add retry fields to tracked_urls table
ALTER TABLE tracked_urls 
ADD COLUMN last_failed_at TIMESTAMP,
ADD COLUMN retry_count INTEGER DEFAULT 0,
ADD COLUMN next_retry_at TIMESTAMP;

-- Create index for retry queries
CREATE INDEX idx_tracked_urls_retry ON tracked_urls (last_failed_at, next_retry_at, retry_count) 
WHERE last_failed_at IS NOT NULL AND retry_count < 5;
