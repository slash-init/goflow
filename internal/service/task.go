package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/slash-init/goflow/internal/repository"
)

type CreateTaskInput struct {
	Type string `json:"type"`
	Payload map[string]any `json:"payload"`
	Priority int `json:"priority"`
	MaxRetries int `json:"max_retries"`
	DelaySeconds int `json:"delay_seconds"`
}

type TaskService struct {
	repo *repository.TaskRepository
}

func NewTaskService(repo *repository.TaskRepository) *TaskService {
	return &TaskService{repo: repo}
}

func (s *TaskService) CreateTask(input CreateTaskInput) (*repository.Task, error) {
	//Validate
	if input.Type == "" {
		return nil, fmt.Errorf("task type is required")
	}
	if input.MaxRetries == 0 {
		input.MaxRetries = 3 //sensible default
	}

	// Serialize payload to JSON bytes
	payloadBytes, err := json.Marshal(input.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	// Calculate when to run
	runAt := time.Now()
	if input.DelaySeconds > 0 {
		runAt = runAt.Add(time.Duration(input.DelaySeconds) * time.Second)
	}

	task := &repository.Task{
		Type : input.Type,
		Payload : payloadBytes,
		Priority:  input.Priority,
		MaxRetries: input.MaxRetries,
		RunAt: runAt,
	}

	if err := s.repo.Create(task); err != nil {
		return nil, fmt.Errorf("could not create task: %w", err) //%w wraps the original error so the caller can inspect it
	}

	return task, nil
}

func (s *TaskService) GetAllTaks() ([]repository.Task, error) {
	return s.repo.GetAll()
}

func (s *TaskService) GetTaskByID(id string) (*repository.Task, error) {
	task, err := s.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}
	return task, nil
}