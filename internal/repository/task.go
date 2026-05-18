package repository

import (
	"time"

	"github.com/jmoiron/sqlx"
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
	ID         string     `db:"id"` //db:"..." tags tell sqlx which struct field matches which SQL column
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

type TaskRepository struct { //instead of passing the DB around everywhere, store it inside a struct and put all task queries on methods.
	db *sqlx.DB
}

func NewTaskRepository(db *sqlx.DB) *TaskRepository { //constructor for the repo
	return &TaskRepository{db: db} //returns a struct with the DB attached
}

func (r *TaskRepository) Create(task *Task) error {
	query := `
	INSERT INTO tasks (type, payload, priority, max_retries, run_at)
	VALUES (:type, :payload, :priority, :max_retries, :run_at)
	RETURNING id, status, retries, created_at, updated_at
	`

	rows, err := r.db.NamedQuery(query, task) //uses a named query, so sqlx matches struct fields to named SQL placeholders.
	if err != nil {
		return err
	}
	defer rows.Close() //cleans up the result set

	if rows.Next() {
		return rows.StructScan(task) //reads the returned row back into the same task struct
	}
	return nil
}

func (r *TaskRepository) GetAll() ([]Task, error) {
	var tasks []Task
	query := `SELECT * FROM tasks ORDER BY created_at DESC`
	err := r.db.Select(&tasks, query)
	return tasks, err
}

func (r *TaskRepository) GetByID(id string) (*Task, error) {
	var task Task
	query := `SELECT * FROM tasks WHERE id = $1`
	err := r.db.Get(&task, query, id)
	if err != nil {
		return nil, err
	}
	return &task, nil
}