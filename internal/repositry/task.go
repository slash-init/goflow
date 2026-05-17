package repositry

import (
	"time"
)

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusDead       TaskStatus = "dead"
)

// task maps directly to the tasks table
type Task struct {
	ID         string     `db:"id"`
	Type       string     `db:"type"`
	Payload    []byte     `db:"payload"`
	Status     TaskStatus `db:"status"`
	Priority   int        `db:"priority"`
	MaxRetries int        `db:"max_retries"`
	Retries    int        `db:"retries"`
	LastError  *string    `db:"last_error"` // pointer = nullable
	RunAt      time.Time  `db:"run_at"`
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"`
}
