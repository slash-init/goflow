package service

import (
	"context"
	"fmt"

	"github.com/slash-init/goflow/internal/repository"
	"github.com/slash-init/goflow/internal/workflow"
)

type CreateWorkflowInput struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type WorkflowService struct {
	engine *workflow.Engine
	repo   *repository.WorkflowRepository
}

func NewWorkflowService(engine *workflow.Engine, repo *repository.WorkflowRepository) *WorkflowService {
	return &WorkflowService{engine: engine, repo: repo}
}

func (s *WorkflowService) StartOrderWorkflow(ctx context.Context, input CreateWorkflowInput) (*repository.Workflow, error) {
	if input.Input["order_id"] == nil {
		return nil, fmt.Errorf("order_id is required")
	}
	if input.Input["amount"] == nil {
		return nil, fmt.Errorf("amount is required")
	}

	compRelease := "inventory:release"
	compRefund := "payment:refund"

	steps := []workflow.StepDefinition{
		{
			Name:                 "Reserve Inventory",
			TaskType:             "inventory:reserve",
			Input:                input.Input,
			CompensationTaskType: &compRelease,
		},
		{
			Name:                 "Charge Payment",
			TaskType:             "payment:charge",
			Input:                input.Input,
			CompensationTaskType: &compRefund,
		},
		{
			Name:     "Send Confirmation",
			TaskType: "notification:send",
			Input:    input.Input,
		},
	}

	return s.engine.StartWorkflow(ctx, input.Name, steps, input.Input)
}

func (s *WorkflowService) GetWorkflow(id string) (*repository.Workflow, []repository.WorkflowStep, error) {
	wf, steps, err := s.repo.GetByID(id)
	if err != nil {
		return nil, nil, fmt.Errorf("workflow not found: %w", err)
	}
	return wf, steps, nil
}

func (s *WorkflowService) GetAllWorkflows() ([]repository.Workflow, error) {
	return s.repo.GetAll()
}
