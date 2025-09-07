package models

import (
	"time"
)

// TaskStatus represents the status of an async task
type TaskStatus string

const (
	TaskStatusQueued     TaskStatus = "queued"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// PriceCheckTask represents an async price checking task
type PriceCheckTask struct {
	ID          string                 `json:"id"`
	URLID       int                    `json:"url_id"`
	Status      TaskStatus             `json:"status"`
	Progress    int                    `json:"progress"` // 0-100
	Message     string                 `json:"message"`
	Result      *PriceData             `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// NewPriceCheckTask creates a new price check task
func NewPriceCheckTask(urlID int) *PriceCheckTask {
	return &PriceCheckTask{
		ID:        generateTaskID(),
		URLID:     urlID,
		Status:    TaskStatusQueued,
		Progress:  0,
		Message:   "Task queued for processing",
		CreatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// UpdateProgress updates the task progress
func (t *PriceCheckTask) UpdateProgress(progress int, message string) {
	t.Progress = progress
	t.Message = message
}

// Start marks the task as processing
func (t *PriceCheckTask) Start() {
	t.Status = TaskStatusProcessing
	t.Progress = 0
	t.Message = "Starting price check..."
	now := time.Now()
	t.StartedAt = &now
}

// Complete marks the task as completed with result
func (t *PriceCheckTask) Complete(result *PriceData) {
	t.Status = TaskStatusCompleted
	t.Progress = 100
	t.Message = "Price check completed successfully"
	t.Result = result
	now := time.Now()
	t.CompletedAt = &now
}

// Fail marks the task as failed with error
func (t *PriceCheckTask) Fail(error string) {
	t.Status = TaskStatusFailed
	t.Progress = 0
	t.Message = "Price check failed"
	t.Error = error
	now := time.Now()
	t.CompletedAt = &now
}

// IsCompleted returns true if the task is in a final state
func (t *PriceCheckTask) IsCompleted() bool {
	return t.Status == TaskStatusCompleted || t.Status == TaskStatusFailed
}

// IsActive returns true if the task is still running
func (t *PriceCheckTask) IsActive() bool {
	return t.Status == TaskStatusQueued || t.Status == TaskStatusProcessing
}

// Duration returns the duration of the task
func (t *PriceCheckTask) Duration() time.Duration {
	if t.StartedAt == nil {
		return 0
	}
	
	endTime := time.Now()
	if t.CompletedAt != nil {
		endTime = *t.CompletedAt
	}
	
	return endTime.Sub(*t.StartedAt)
}

// generateTaskID generates a unique task ID
func generateTaskID() string {
	return "task_" + time.Now().Format("20060102150405") + "_" + randomString(8)
}

// randomString generates a random string of specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
