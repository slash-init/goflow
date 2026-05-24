package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// Task metrics
	TasksEnqueued = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goflow_tasks_enqueued_total",
			Help: "Total number of tasks enqueued",
		},
		[]string{"type", "queue"},
	)

	TasksProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goflow_tasks_processed_total",
			Help: "Total number of tasks processed",
		},
		[]string{"type", "status"}, // status: success | failure
	)

	TaskDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "goflow_task_duration_seconds",
			Help:    "Task processing duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)

	DeadTasks = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "goflow_dead_tasks_total",
			Help: "Current number of tasks in dead-letter queue",
		},
		[]string{"queue"},
	)

	// Workflow metrics
	WorkflowsStarted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goflow_workflows_started_total",
			Help: "Total number of workflows started",
		},
		[]string{"name"},
	)

	WorkflowsCompleted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goflow_workflows_completed_total",
			Help: "Total number of workflows completed",
		},
		[]string{"name", "status"}, // status: completed | compensated | failed
	)

	WorkflowDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "goflow_workflow_duration_seconds",
			Help:    "Workflow execution duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"name"},
	)

	// HTTP metrics (request count + latency per route)
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "goflow_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status_code"},
	)
)

func Init() {
	prometheus.MustRegister(
		TasksEnqueued,
		TasksProcessed,
		TaskDuration,
		DeadTasks,
		WorkflowsStarted,
		WorkflowsCompleted,
		WorkflowDuration,
		HTTPRequestDuration,
	)
}
