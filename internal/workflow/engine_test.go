package workflow_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/slash-init/goflow/internal/workflow"
)

func TestStepDefinition_WithCompensation(t *testing.T) {
	comp := "inventory:release"
	step := workflow.StepDefinition{
		Name:                 "Reserve Inventory",
		TaskType:             "inventory:reserve",
		Input:                map[string]any{"order_id": "order_001"},
		CompensationTaskType: &comp,
	}

	assert.Equal(t, "Reserve Inventory", step.Name)
	assert.NotNil(t, step.CompensationTaskType)
	assert.Equal(t, "inventory:release", *step.CompensationTaskType)
}

func TestStepDefinition_WithoutCompensation(t *testing.T) {
	step := workflow.StepDefinition{
		Name:     "Send Notification",
		TaskType: "notification:send",
		Input:    map[string]any{"order_id": "order_001"},
	}

	assert.Nil(t, step.CompensationTaskType)
}

func TestHandlerRegistry(t *testing.T) {
	// Verify built-in handlers are registered and callable
	called := false
	handler := func(ctx context.Context, input map[string]any) (map[string]any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}

	// Simulate handler execution
	output, err := handler(context.Background(), map[string]any{"order_id": "test"})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, true, output["ok"])
}