package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
)

// --- Email Task ---

type SendEmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

func HandleSendEmail(ctx context.Context, t *asynq.Task) error {
	var payload SendEmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	// In real life : call your email provider (SendGrid, SES, etc.)
	log.Printf("[EMAIL] Sending to=%s subject=%s", payload.To, payload.Subject)

	return nil //returning nil = success
}

// --- Report Task ---

type GenerateReportPayload struct {
	ReportType string `json:"report_type"`
	UserID     string `json:"user_id"`
}

func HandleGenerateReport(ctx context.Context, t *asynq.Task) error {
	var payload GenerateReportPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	log.Printf("[REPORT] Generating report_type=%s user_id=%s", payload.ReportType, payload.UserID)

	// Simulate work
	// time.Sleep(2 * time.Second)

	return nil
}

func HandleAlwaysFail(ctx context.Context, t *asynq.Task) error {
	log.Printf("[FAIL] This task always fails")
	return fmt.Errorf("intentional failure for testing")
}
