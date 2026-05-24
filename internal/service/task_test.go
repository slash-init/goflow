package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/slash-init/goflow/internal/service"
)

func TestCreateTask_MissingType(t *testing.T) {
	svc := service.NewTaskService(nil, nil)

	_, err := svc.CreateTask(service.CreateTaskInput{
		Type:    "",
		Payload: map[string]any{},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task type is required")
}

func TestCreateTask_DefaultsApplied(t *testing.T) {
	input := service.CreateTaskInput{
		Type:    "email:send",
		Payload: map[string]any{"to": "test@example.com"},
	}

	if input.MaxRetries == 0 {
		input.MaxRetries = 3
	}
	if input.Queue == "" {
		input.Queue = "default"
	}

	assert.Equal(t, 3, input.MaxRetries)
	assert.Equal(t, "default", input.Queue)
}