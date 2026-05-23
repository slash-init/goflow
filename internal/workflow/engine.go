package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/slash-init/goflow/internal/repository"
	"github.com/slash-init/goflow/pkg/queue"
)

// StepHandler is a function that does the actual work for a step.
// Returns output data or an error.
type StepHandler func(ctx context.Context, input map[string]any) (map[string]any, error)

// Engine orchestrates workflow execution.
type Engine struct {
	repo     *repository.WorkflowRepository
	client   *queue.Client
	handlers map[string]StepHandler
}

func NewEngine(repo *repository.WorkflowRepository, client *queue.Client) *Engine {
	e := &Engine{
		repo:     repo,
		client:   client,
		handlers: make(map[string]StepHandler),
	}
	e.registerBuiltinHandlers()
	return e
}

// RegisterHandler lets you plug in a handler for a task type.
func (e *Engine) RegisterHandler(taskType string, fn StepHandler) {
	e.handlers[taskType] = fn
}

// StartWorkflow creates and immediately begins executing a workflow.
func (e *Engine) StartWorkflow(ctx context.Context, name string, stepDefs []StepDefinition, input map[string]any) (*repository.Workflow, error) {
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	wf := &repository.Workflow{
		Name:   name,
		Status: repository.WorkflowStatusRunning,
		Input:  inputBytes,
	}

	steps := make([]repository.WorkflowStep, len(stepDefs))
	for i, def := range stepDefs {
		inputBytes, _ := json.Marshal(def.Input)
		steps[i] = repository.WorkflowStep{
			StepIndex:            i,
			Name:                 def.Name,
			TaskType:             def.TaskType,
			Input:                inputBytes,
			CompensationTaskType: def.CompensationTaskType,
		}
	}

	if err := e.repo.Create(wf, steps); err != nil {
		return nil, fmt.Errorf("could not create workflow: %w", err)
	}

	// Execute in background so the HTTP response returns immediately
	go e.execute(context.Background(), wf.ID)

	return wf, nil
}

// execute runs all steps in order.
func (e *Engine) execute(ctx context.Context, workflowID string) {
	wf, steps, err := e.repo.GetByID(workflowID)
	if err != nil {
		log.Printf("[WORKFLOW] Could not load workflow %s: %v", workflowID, err)
		return
	}

	log.Printf("[WORKFLOW] Starting %s (id=%s)", wf.Name, wf.ID)

	for i := range steps {
		step := &steps[i]

		// Mark step as running
		e.repo.UpdateStepStatus(step.ID, repository.StepStatusRunning, nil, nil)
		e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusRunning, i, nil)

		log.Printf("[WORKFLOW] Step %d/%d: %s", i+1, len(steps), step.Name)

		// Get the handler for this step's task type
		handler, ok := e.handlers[step.TaskType]
		if !ok {
			errMsg := fmt.Sprintf("no handler registered for task type: %s", step.TaskType)
			e.failWorkflow(wf, steps, i, errMsg)
			return
		}

		// Unmarshal step input
		var stepInput map[string]any
		json.Unmarshal(step.Input, &stepInput)

		// Run the handler
		output, err := handler(ctx, stepInput)
		if err != nil {
			errMsg := err.Error()
			log.Printf("[WORKFLOW] Step %d failed: %v", i+1, err)
			e.repo.UpdateStepStatus(step.ID, repository.StepStatusFailed, nil, &errMsg)
			e.failWorkflow(wf, steps, i, errMsg)
			return
		}

		// Step succeeded
		outputBytes, _ := json.Marshal(output)
		e.repo.UpdateStepStatus(step.ID, repository.StepStatusCompleted, outputBytes, nil)
		log.Printf("[WORKFLOW] Step %d completed: %s", i+1, step.Name)
	}

	// All steps done
	e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusCompleted, len(steps), nil)
	log.Printf("[WORKFLOW] Completed: %s (id=%s)", wf.Name, wf.ID)
}

// failWorkflow triggers compensation for all previously completed steps.
func (e *Engine) failWorkflow(wf *repository.Workflow, steps []repository.WorkflowStep, failedAt int, errMsg string) {
	e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusCompensating, failedAt, &errMsg)
	log.Printf("[WORKFLOW] Compensating %s from step %d", wf.Name, failedAt)

	// Run compensation in reverse order for completed steps
	for i := failedAt - 1; i >= 0; i-- {
		step := steps[i]
		if step.CompensationTaskType == nil {
			continue
		}

		handler, ok := e.handlers[*step.CompensationTaskType]
		if !ok {
			log.Printf("[WORKFLOW] No compensation handler for %s", *step.CompensationTaskType)
			continue
		}

		e.repo.UpdateStepStatus(step.ID, repository.StepStatusCompensating, nil, nil)

		var stepInput map[string]any
		json.Unmarshal(step.Input, &stepInput)

		if _, err := handler(context.Background(), stepInput); err != nil {
			log.Printf("[WORKFLOW] Compensation failed for step %d: %v", i, err)
		} else {
			e.repo.UpdateStepStatus(step.ID, repository.StepStatusCompensated, nil, nil)
			log.Printf("[WORKFLOW] Compensated step %d: %s", i, step.Name)
		}
	}

	e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusCompensated, failedAt, &errMsg)
}
