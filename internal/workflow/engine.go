package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slash-init/goflow/internal/repository"
	"github.com/slash-init/goflow/pkg/logger"
	"github.com/slash-init/goflow/pkg/metrics"
	"github.com/slash-init/goflow/pkg/queue"
	"go.uber.org/zap"
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

func (e *Engine) execute(ctx context.Context, workflowID string) {
	start := time.Now()

	wf, steps, err := e.repo.GetByID(workflowID)
	if err != nil {
		logger.Error("could not load workflow",
			zap.String("workflow_id", workflowID),
			zap.Error(err),
		)
		return
	}

	metrics.WorkflowsStarted.WithLabelValues(wf.Name).Inc()
	logger.Info("workflow started",
		zap.String("workflow_id", wf.ID),
		zap.String("name", wf.Name),
	)

	for i := range steps {
		step := &steps[i]

		if err := e.repo.UpdateStepStatus(step.ID, repository.StepStatusRunning, nil, nil); err != nil {
			logger.Error("failed to update step status", zap.String("step_id", step.ID), zap.Error(err))
		}
		if err := e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusRunning, i, nil); err != nil {
			logger.Error("failed to update workflow status", zap.String("workflow_id", wf.ID), zap.Error(err))
		}

		logger.Info("step started",
			zap.String("workflow_id", wf.ID),
			zap.String("step", step.Name),
			zap.Int("index", i),
		)

		handler, ok := e.handlers[step.TaskType]
		if !ok {
			errMsg := fmt.Sprintf("no handler registered for task type: %s", step.TaskType)
			e.failWorkflow(wf, steps, i, errMsg)
			return
		}

		var stepInput map[string]any
		if err := json.Unmarshal(step.Input, &stepInput); err != nil {
			logger.Error("failed to unmarshal step input", zap.String("step_id", step.ID), zap.Error(err))
			e.failWorkflow(wf, steps, i, err.Error())
			return
		}

		output, err := handler(ctx, stepInput)
		if err != nil {
			errMsg := err.Error()
			logger.Error("step failed",
				zap.String("workflow_id", wf.ID),
				zap.String("step", step.Name),
				zap.Error(err),
			)
			if err := e.repo.UpdateStepStatus(step.ID, repository.StepStatusFailed, nil, &errMsg); err != nil {
				logger.Error("failed to update step status", zap.String("step_id", step.ID), zap.Error(err))
			}
			e.failWorkflow(wf, steps, i, errMsg)
			return
		}

		outputBytes, _ := json.Marshal(output)
		if err := e.repo.UpdateStepStatus(step.ID, repository.StepStatusCompleted, outputBytes, nil); err != nil {
			logger.Error("failed to update step status", zap.String("step_id", step.ID), zap.Error(err))
		}
		logger.Info("step completed",
			zap.String("workflow_id", wf.ID),
			zap.String("step", step.Name),
		)
	}

	if err := e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusCompleted, len(steps), nil); err != nil {
		logger.Error("failed to update workflow status", zap.String("workflow_id", wf.ID), zap.Error(err))
	}
	metrics.WorkflowsCompleted.WithLabelValues(wf.Name, "completed").Inc()
	metrics.WorkflowDuration.WithLabelValues(wf.Name).Observe(time.Since(start).Seconds())

	logger.Info("workflow completed",
		zap.String("workflow_id", wf.ID),
		zap.String("name", wf.Name),
		zap.Duration("duration", time.Since(start)),
	)
}

func (e *Engine) failWorkflow(wf *repository.Workflow, steps []repository.WorkflowStep, failedAt int, errMsg string) {
	if err := e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusCompensating, failedAt, &errMsg); err != nil {
		logger.Error("failed to update workflow status", zap.String("workflow_id", wf.ID), zap.Error(err))
	}

	logger.Warn("workflow compensating",
		zap.String("workflow_id", wf.ID),
		zap.String("name", wf.Name),
		zap.String("error", errMsg),
		zap.Int("failed_at_step", failedAt),
	)

	for i := failedAt - 1; i >= 0; i-- {
		step := steps[i]
		if step.CompensationTaskType == nil {
			continue
		}

		handler, ok := e.handlers[*step.CompensationTaskType]
		if !ok {
			logger.Warn("no compensation handler",
				zap.String("task_type", *step.CompensationTaskType),
			)
			continue
		}

		if err := e.repo.UpdateStepStatus(step.ID, repository.StepStatusCompensating, nil, nil); err != nil {
			logger.Error("failed to update step status", zap.String("step_id", step.ID), zap.Error(err))
		}

		var stepInput map[string]any
		if err := json.Unmarshal(step.Input, &stepInput); err != nil {
			logger.Error("failed to unmarshal step input", zap.String("step_id", step.ID), zap.Error(err))
			continue
		}

		if _, err := handler(context.Background(), stepInput); err != nil {
			logger.Error("compensation failed",
				zap.String("step", step.Name),
				zap.Error(err),
			)
		} else {
			if err := e.repo.UpdateStepStatus(step.ID, repository.StepStatusCompensated, nil, nil); err != nil {
				logger.Error("failed to update step status", zap.String("step_id", step.ID), zap.Error(err))
			}
			logger.Info("step compensated",
				zap.String("workflow_id", wf.ID),
				zap.String("step", step.Name),
			)
		}
	}

	if err := e.repo.UpdateWorkflowStatus(wf.ID, repository.WorkflowStatusCompensated, failedAt, &errMsg); err != nil {
		logger.Error("failed to update workflow status", zap.String("workflow_id", wf.ID), zap.Error(err))
	}
	metrics.WorkflowsCompleted.WithLabelValues(wf.Name, "compensated").Inc()
}
