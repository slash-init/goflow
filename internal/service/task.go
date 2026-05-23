package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/slash-init/goflow/internal/repository"
	"github.com/slash-init/goflow/pkg/queue"
	queuepkg "github.com/slash-init/goflow/pkg/queue"
)

type CreateTaskInput struct {
	Type         string         `json:"type"`
	Payload      map[string]any `json:"payload"`
	Priority     int            `json:"priority"`
	MaxRetries   int            `json:"max_retries"`
	DelaySeconds int            `json:"delay_seconds"`
	Queue        string         `json:"queue"`
}

type TaskService struct {
	repo   *repository.TaskRepository
	client *queue.Client
}

func NewTaskService(repo *repository.TaskRepository, client *queue.Client) *TaskService {
	return &TaskService{repo: repo, client: client}
}

func (s *TaskService) CreateTask(input CreateTaskInput) (*repository.Task, error) {
	if input.Type == "" {
		return nil, fmt.Errorf("task type is required")
	}
	if input.MaxRetries == 0 {
		input.MaxRetries = 3
	}
	if input.Queue == "" {
		input.Queue = "default"
	}

	payloadBytes, err := json.Marshal(input.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	runAt := time.Now()
	if input.DelaySeconds > 0 {
		runAt = runAt.Add(time.Duration(input.DelaySeconds) * time.Second)
	}

	// Save to PostgreSQL first
	task := &repository.Task{
		Type:       input.Type,
		Payload:    payloadBytes,
		Priority:   input.Priority,
		MaxRetries: input.MaxRetries,
		RunAt:      runAt,
	}
	if err := s.repo.Create(task); err != nil {
		return nil, fmt.Errorf("could not save task: %w", err)
	}

	// Then enqueue to Redis/Asynq
	opts := []asynq.Option{
		asynq.Queue(input.Queue),
		asynq.MaxRetry(input.MaxRetries),
	}
	if input.DelaySeconds > 0 {
		opts = append(opts, asynq.ProcessIn(time.Duration(input.DelaySeconds)*time.Second))
	}
	if err := s.client.EnqueueTask(context.Background(), input.Type, input.Payload, opts...); err != nil {
		return nil, fmt.Errorf("could not enqueue task: %w", err)
	}

	return task, nil
}

func (s *TaskService) GetAllTasks() ([]repository.Task, error) {
	return s.repo.GetAll()
}

func (s *TaskService) GetTaskByID(id string) (*repository.Task, error) {
	task, err := s.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}
	return task, nil
}

func (s *TaskService) ListDeadTasks(queue string) ([]*queuepkg.DeadTask, error) {
	if queue == "" {
		queue = "default"
	}
	return s.client.ListDeadTasks(queue)
}

func (s *TaskService) RetryDeadTask(queue, taskID string) error {
	if queue == "" {
		queue = "default"
	}
	return s.client.RetryDeadTask(queue, taskID)
}

func (s *TaskService) DeleteDeadTask(queue, taskID string) error {
	if queue == "" {
		queue = "default"
	}
	return s.client.DeleteDeadTask(queue, taskID)
}
