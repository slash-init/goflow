package repository

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type WorkflowStatus string
type StepStatus string

const (
	WorkflowStatusPending      WorkflowStatus = "pending"
	WorkflowStatusRunning      WorkflowStatus = "running"
	WorkflowStatusCompleted    WorkflowStatus = "completed"
	WorkflowStatusFailed       WorkflowStatus = "failed"
	WorkflowStatusCompensating WorkflowStatus = "compensating"
	WorkflowStatusCompensated  WorkflowStatus = "compensated"

	StepStatusPending      StepStatus = "pending"
	StepStatusRunning      StepStatus = "running"
	StepStatusCompleted    StepStatus = "completed"
	StepStatusFailed       StepStatus = "failed"
	StepStatusCompensating StepStatus = "compensating"
	StepStatusCompensated  StepStatus = "compensated"
	StepStatusSkipped      StepStatus = "skipped"
)

type Workflow struct {
	ID          string         `db:"id"`
	Name        string         `db:"name"`
	Status      WorkflowStatus `db:"status"`
	Input       []byte         `db:"input"`
	CurrentStep int            `db:"current_step"`
	LastError   *string        `db:"last_error"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

type WorkflowStep struct {
	ID                   string     `db:"id"`
	WorkflowID           string     `db:"workflow_id"`
	StepIndex            int        `db:"step_index"`
	Name                 string     `db:"name"`
	TaskType             string     `db:"task_type"`
	Status               StepStatus `db:"status"`
	Input                []byte     `db:"input"`
	Output               []byte     `db:"output"`
	LastError            *string    `db:"last_error"`
	CompensationTaskType *string    `db:"compensation_task_type"`
	StartedAt            *time.Time `db:"started_at"`
	CompletedAt          *time.Time `db:"completed_at"`
	CreatedAt            time.Time  `db:"created_at"`
}

type WorkflowRepository struct {
	db *sqlx.DB
}

func NewWorkflowRepository(db *sqlx.DB) *WorkflowRepository {
	return &WorkflowRepository{db: db}
}

func (r *WorkflowRepository) Create(wf *Workflow, steps []WorkflowStep) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Use QueryRowx instead of NamedQuery to avoid the pq RETURNING bug
	err = tx.QueryRowx(`
		INSERT INTO workflows (name, status, input)
		VALUES ($1, $2, $3)
		RETURNING id, current_step, created_at, updated_at
	`, wf.Name, wf.Status, wf.Input).StructScan(wf)
	if err != nil {
		return fmt.Errorf("could not insert workflow: %w", err)
	}

	for i := range steps {
		steps[i].WorkflowID = wf.ID
		err = tx.QueryRowx(`
			INSERT INTO workflow_steps
				(workflow_id, step_index, name, task_type, input, compensation_task_type)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, status, created_at
		`,
			steps[i].WorkflowID,
			steps[i].StepIndex,
			steps[i].Name,
			steps[i].TaskType,
			steps[i].Input,
			steps[i].CompensationTaskType,
		).StructScan(&steps[i])
		if err != nil {
			return fmt.Errorf("could not insert step %d: %w", i, err)
		}
	}

	return tx.Commit()
}

func (r *WorkflowRepository) GetByID(id string) (*Workflow, []WorkflowStep, error) {
	var wf Workflow
	if err := r.db.Get(&wf, `SELECT * FROM workflows WHERE id = $1`, id); err != nil {
		return nil, nil, err
	}

	var steps []WorkflowStep
	if err := r.db.Select(&steps,
		`SELECT * FROM workflow_steps WHERE workflow_id = $1 ORDER BY step_index`,
		id,
	); err != nil {
		return nil, nil, err
	}

	return &wf, steps, nil
}

func (r *WorkflowRepository) GetAll() ([]Workflow, error) {
	var workflows []Workflow
	err := r.db.Select(&workflows, `SELECT * FROM workflows ORDER BY created_at DESC`)
	return workflows, err
}

func (r *WorkflowRepository) UpdateWorkflowStatus(id string, status WorkflowStatus, currentStep int, lastError *string) error {
	_, err := r.db.Exec(`
		UPDATE workflows
		SET status = $1, current_step = $2, last_error = $3, updated_at = NOW()
		WHERE id = $4
	`, status, currentStep, lastError, id)
	return err
}

func (r *WorkflowRepository) UpdateStepStatus(stepID string, status StepStatus, output []byte, lastError *string) error {
	_, err := r.db.Exec(`
		UPDATE workflow_steps
		SET status     = $1::step_status,
		    output     = $2,
		    last_error = $3,
		    started_at   = CASE WHEN $1 = 'running'
		                        THEN NOW()
		                        ELSE started_at END,
		    completed_at = CASE WHEN $1 IN ('completed','failed','compensated','compensating')
		                        THEN NOW()
		                        ELSE completed_at END
		WHERE id = $4
	`, status, output, lastError, stepID)
	return err
}
