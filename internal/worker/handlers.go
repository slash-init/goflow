package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/slash-init/goflow/pkg/logger"
	"github.com/slash-init/goflow/pkg/metrics"
	"go.uber.org/zap"
)

// --- Email Task ---

type SendEmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

func HandleSendEmail(ctx context.Context, t *asynq.Task) error {
	start := time.Now()

	var payload SendEmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	logger.Info("processing email task",
		zap.String("to", payload.To),
		zap.String("subject", payload.Subject),
	)

	// In real life : call your email provider (SendGrid, SES, etc.)

	metrics.TasksProcessed.WithLabelValues(t.Type(), "success").Inc()
	metrics.TaskDuration.WithLabelValues(t.Type()).Observe(time.Since(start).Seconds())
	return nil
}

// --- Report Task ---

type GenerateReportPayload struct {
	ReportType string `json:"report_type"`
	UserID     string `json:"user_id"`
}

func HandleGenerateReport(ctx context.Context, t *asynq.Task) error {
	start := time.Now()

	var payload GenerateReportPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	logger.Info("generating report",
		zap.String("report_type", payload.ReportType),
		zap.String("user_id", payload.UserID),
	)

	// Simulate work
	// time.Sleep(2 * time.Second)

	metrics.TasksProcessed.WithLabelValues(t.Type(), "success").Inc()
	metrics.TaskDuration.WithLabelValues(t.Type()).Observe(time.Since(start).Seconds())
	return nil
}

func HandleAlwaysFail(ctx context.Context, t *asynq.Task) error {
	logger.Warn("task:fail handler invoked — this always fails")
	metrics.TasksProcessed.WithLabelValues(t.Type(), "failure").Inc()
	return fmt.Errorf("intentional failure for testing")
}
