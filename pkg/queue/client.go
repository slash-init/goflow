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
    TypeAlwaysFail = "task:fail"
)

type Client struct {
    asynq *asynq.Client
    inspector *asynq.Inspector
}

func NewClient(redisAddr string) *Client {
    opt := asynq.RedisClientOpt{Addr: redisAddr}
    return &Client{
        asynq: asynq.NewClient(opt),
        inspector: asynq.NewInspector(opt),
    }
}

func (c *Client) Close() error {
    c.inspector.Close()
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

// --- Dead-letter queue operations ---

type DeadTask struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Payload     string    `json:"payload"`
	Queue       string    `json:"queue"`
	LastError   string    `json:"last_error"`
	LastFailedAt time.Time `json:"last_failed_at"`
}

func (c *Client) ListDeadTasks(queue string) ([]*DeadTask, error) {
    tasks, err := c.inspector.ListArchivedTasks(queue, asynq.PageSize(100))
    if err != nil {
        return nil, fmt.Errorf("could not list dead tasks: %w", err)
    }

    var result []*DeadTask
    for _, t := range tasks {
        result = append(result, &DeadTask{
            ID:           t.ID,
            Type:         t.Type,
            Payload:      string(t.Payload),
            Queue:        t.Queue,
            LastError:    t.LastErr,
            LastFailedAt: t.LastFailedAt,
        })
    }
    return result, nil
}

func (c *Client) RetryDeadTask(queue, taskID string) error {
    err := c.inspector.RunTask(queue, taskID)
    if err != nil {
		return fmt.Errorf("could not retry task: %w", err)
	}
	return nil
}

func (c *Client) DeleteDeadTask(queue, taskID string) error {
	err := c.inspector.DeleteTask(queue, taskID)
	if err != nil {
		return fmt.Errorf("could not delete task: %w", err)
	}
	return nil
}