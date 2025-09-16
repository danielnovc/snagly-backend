package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"distrack/models"
)

// PriceCheckFunc is a function type for checking prices
type PriceCheckFunc func(urlID int) (*models.PriceData, error)

// DualPriceCheckFunc is a function type for checking prices with dual response
type DualPriceCheckFunc func(urlID int) (*models.PriceCheckResponse, error)

// TaskManager manages async price checking tasks
type TaskManager struct {
	tasks           map[string]*models.PriceCheckTask
	taskQueue       chan *models.PriceCheckTask
	workers         int
	maxWorkers      int
	priceCheckFunc  PriceCheckFunc
	dualCheckFunc   DualPriceCheckFunc
	mutex           sync.RWMutex
	stopChan        chan bool
}

// NewTaskManager creates a new task manager
func NewTaskManager(priceCheckFunc PriceCheckFunc, dualCheckFunc DualPriceCheckFunc, maxWorkers int) *TaskManager {
	tm := &TaskManager{
		tasks:          make(map[string]*models.PriceCheckTask),
		taskQueue:      make(chan *models.PriceCheckTask, 100), // Buffer for 100 tasks
		workers:        0,
		maxWorkers:     maxWorkers,
		priceCheckFunc: priceCheckFunc,
		dualCheckFunc:  dualCheckFunc,
		stopChan:       make(chan bool),
	}
	
	go tm.processTasks()
	log.Printf("🚀 Task manager started with %d max workers", maxWorkers)
	return tm
}

// SubmitTask submits a new price check task
func (tm *TaskManager) SubmitTask(urlID int) *models.PriceCheckTask {
	task := models.NewPriceCheckTask(urlID)
	
	tm.mutex.Lock()
	tm.tasks[task.ID] = task
	tm.mutex.Unlock()
	
	// Submit to queue
	select {
	case tm.taskQueue <- task:
		log.Printf("📝 Task %s submitted for URL ID %d", task.ID, urlID)
	default:
		task.Fail("Task queue is full")
		log.Printf("❌ Failed to submit task %s - queue full", task.ID)
	}
	
	return task
}

// GetTask returns a task by ID
func (tm *TaskManager) GetTask(taskID string) (*models.PriceCheckTask, bool) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	task, exists := tm.tasks[taskID]
	return task, exists
}

// GetActiveTasks returns all active tasks
func (tm *TaskManager) GetActiveTasks() []*models.PriceCheckTask {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	var activeTasks []*models.PriceCheckTask
	for _, task := range tm.tasks {
		if task.IsActive() {
			activeTasks = append(activeTasks, task)
		}
	}
	
	return activeTasks
}

// GetCompletedTasks returns all completed tasks (for cleanup)
func (tm *TaskManager) GetCompletedTasks() []*models.PriceCheckTask {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	var completedTasks []*models.PriceCheckTask
	for _, task := range tm.tasks {
		if task.IsCompleted() {
			completedTasks = append(completedTasks, task)
		}
	}
	
	return completedTasks
}

// CleanupOldTasks removes completed tasks older than specified duration
func (tm *TaskManager) CleanupOldTasks(maxAge time.Duration) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	cutoff := time.Now().Add(-maxAge)
	for taskID, task := range tm.tasks {
		if task.IsCompleted() && task.CreatedAt.Before(cutoff) {
			delete(tm.tasks, taskID)
			log.Printf("🧹 Cleaned up old task: %s", taskID)
		}
	}
}

// processTasks processes tasks from the queue
func (tm *TaskManager) processTasks() {
	ticker := time.NewTicker(5 * time.Second) // Cleanup every 5 seconds
	defer ticker.Stop()
	
	for {
		select {
		case task := <-tm.taskQueue:
			// Start a new worker if we haven't reached max
			if tm.workers < tm.maxWorkers {
				tm.workers++
				go tm.worker(task)
			} else {
				// Re-queue the task if we're at max workers
				go func() {
					time.Sleep(1 * time.Second) // Wait a bit before re-queuing
					select {
					case tm.taskQueue <- task:
						log.Printf("🔄 Re-queued task %s (max workers reached)", task.ID)
					default:
						task.Fail("System overloaded, unable to process task")
						log.Printf("❌ Failed to re-queue task %s", task.ID)
					}
				}()
			}
			
		case <-ticker.C:
			// Periodic cleanup
			tm.CleanupOldTasks(1 * time.Hour) // Keep tasks for 1 hour
			
		case <-tm.stopChan:
			log.Println("🛑 Task manager stopped")
			return
		}
	}
}

// worker processes a single task
func (tm *TaskManager) worker(task *models.PriceCheckTask) {
	defer func() {
		tm.workers--
		log.Printf("👷 Worker finished, active workers: %d", tm.workers)
	}()
	
	log.Printf("👷 Worker started processing task %s for URL ID %d", task.ID, task.URLID)
	
	// Start the task
	task.Start()
	
	// Simulate progress updates
	for i := 0; i <= 100; i += 20 {
		task.Progress = i
		task.Message = fmt.Sprintf("Processing... %d%%", i)
		time.Sleep(200 * time.Millisecond)
	}

	// Perform the actual price check (get dual response for frontend)
	priceResponse, err := tm.dualCheckFunc(task.URLID)
	if err != nil {
		task.Fail("Price check failed: " + err.Error())
		return
	}
	
	// Validate that we have some price data
	if priceResponse.PrimaryPrice == nil && priceResponse.AlternativePrice == nil {
		task.Fail("No price data found in response")
		return
	}
	
	// Complete the task with dual response
	task.CompleteWithDualResponse(priceResponse)
	
	log.Printf("✅ Task %s completed successfully in %v", task.ID, task.Duration())
}

// Stop stops the task manager
func (tm *TaskManager) Stop() {
	close(tm.stopChan)
	log.Println("🛑 Task manager stopping...")
}

// GetStats returns task manager statistics
func (tm *TaskManager) GetStats() map[string]interface{} {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	stats := map[string]interface{}{
		"total_tasks":    len(tm.tasks),
		"active_workers": tm.workers,
		"max_workers":    tm.maxWorkers,
		"queue_size":     len(tm.taskQueue),
	}
	
	// Count tasks by status
	statusCounts := make(map[string]int)
	for _, task := range tm.tasks {
		status := string(task.Status)
		statusCounts[status]++
	}
	stats["tasks_by_status"] = statusCounts
	
	return stats
}
