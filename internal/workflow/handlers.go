package workflow

import (
	"context"
	"fmt"
	"log"
)

// StepDefinition describes one step in a workflow.
type StepDefinition struct {
	Name                 string
	TaskType             string
	Input                map[string]any
	CompensationTaskType *string // nil = no compensation needed
}

// Helper to make compensation pointer cleaner at call sites
func withCompensation(taskType string) *string {
	return &taskType
}

// registerBuiltinHandlers wires up the demo handlers.
func (e *Engine) registerBuiltinHandlers() {
	e.RegisterHandler("inventory:reserve", handleReserveInventory)
	e.RegisterHandler("inventory:release", handleReleaseInventory)
	e.RegisterHandler("payment:charge", handleChargePayment)
	e.RegisterHandler("payment:refund", handleRefundPayment)
	e.RegisterHandler("notification:send", handleSendNotification)
}

func handleReserveInventory(ctx context.Context, input map[string]any) (map[string]any, error) {
	orderID := input["order_id"]
	log.Printf("[INVENTORY] Reserving inventory for order %v", orderID)
	// Real impl: call inventory service or update DB
	return map[string]any{"reservation_id": "res_123", "order_id": orderID}, nil
}

func handleReleaseInventory(ctx context.Context, input map[string]any) (map[string]any, error) {
	orderID := input["order_id"]
	log.Printf("[INVENTORY] Releasing inventory for order %v", orderID)
	return map[string]any{"released": true}, nil
}

func handleChargePayment(ctx context.Context, input map[string]any) (map[string]any, error) {
	orderID := input["order_id"]
	amount := input["amount"]
	log.Printf("[PAYMENT] Charging %.2f for order %v", amount, orderID)

	// Simulate a failure when amount > 1000 so we can test compensation
	if amt, ok := amount.(float64); ok && amt > 1000 {
		return nil, fmt.Errorf("payment declined: amount %.2f exceeds limit", amt)
	}

	return map[string]any{"transaction_id": "txn_456", "charged": amount}, nil
}

func handleRefundPayment(ctx context.Context, input map[string]any) (map[string]any, error) {
	orderID := input["order_id"]
	log.Printf("[PAYMENT] Refunding payment for order %v", orderID)
	return map[string]any{"refunded": true}, nil
}

func handleSendNotification(ctx context.Context, input map[string]any) (map[string]any, error) {
	orderID := input["order_id"]
	log.Printf("[NOTIFICATION] Sending confirmation for order %v", orderID)
	return map[string]any{"notified": true}, nil
}
