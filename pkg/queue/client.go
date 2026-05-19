package queue

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/hibiken/asynq"
)

const (
    TypeSendEmail    = "email:send"
    TypeGenerateReport = "report:generate"
)

type Client struct {
    asynq *asynq.Client
}

func NewClient(redisAddr string) *Client {
    return &Client{
        asynq: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
    }
}

func (c *Client) Close() error {
    return c.asynq.Close()
}

// EnqueueTask sends a task to the queue
func (c *Client) EnqueueTask(ctx context.Context, taskType string, payload any, opts ...asynq.Option) error {
    payloadBytes, err := json.Marshal(payload)
    if err != nil {
        return fmt.Errorf("could not marshal payload: %w", err)
    }

    task := asynq.NewTask(taskType, payloadBytes, opts...)
    _, err = c.asynq.EnqueueContext(ctx, task)
    return err
}

// Helper: enqueue with delay
func (c *Client) EnqueueDelayed(ctx context.Context, taskType string, payload any, delay time.Duration) error {
    return c.EnqueueTask(ctx, taskType, payload, asynq.ProcessIn(delay))
}

// Helper: enqueue with priority queue
func (c *Client) EnqueueWithPriority(ctx context.Context, taskType string, payload any, queue string) error {
    return c.EnqueueTask(ctx, taskType, payload, asynq.Queue(queue))
}