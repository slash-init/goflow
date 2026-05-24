// @title           GoFlow API
// @version         1.0
// @description     Distributed task queue and workflow engine
// @host            localhost:8080
// @BasePath        /api/v1

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"
	fiberSwagger "github.com/swaggo/fiber-swagger"
    _ "github.com/slash-init/goflow/docs"

	"github.com/slash-init/goflow/internal/api"
	"github.com/slash-init/goflow/internal/repository"
	"github.com/slash-init/goflow/internal/service"
	"github.com/slash-init/goflow/internal/worker"
	"github.com/slash-init/goflow/internal/workflow"
	"github.com/slash-init/goflow/pkg/config"
	"github.com/slash-init/goflow/pkg/database"
	"github.com/slash-init/goflow/pkg/logger"
	"github.com/slash-init/goflow/pkg/metrics"
	"github.com/slash-init/goflow/pkg/queue"
)

func main() {
	cfg := config.Load()

	// Init logger and metrics
	logger.Init(cfg.AppEnv)
	defer logger.Log.Sync()
	metrics.Init()

	db := database.NewPostgresDB(cfg)
	defer db.Close() //closes DB connection when main() exits

	queueClient := queue.NewClient(cfg.RedisAddr)
	defer queueClient.Close()

	// Wire up layers
	taskRepo := repository.NewTaskRepository(db)
	taskSvc := service.NewTaskService(taskRepo, queueClient)
	taskHandler := api.NewTaskHandler(taskSvc)

	// Workflow engine
	workflowRepo := repository.NewWorkflowRepository(db)
	workflowEngine := workflow.NewEngine(workflowRepo, queueClient)
	workflowSvc := service.NewWorkflowService(workflowEngine, workflowRepo)
	workflowHandler := api.NewWorkflowHandler(workflowSvc)

	// Start worker in background goroutine
	workerSrv := worker.NewWorkerServer(cfg.RedisAddr)
	go worker.RegisterHandlers(workerSrv) // <-- this is a goroutine

	//create a new Fiber app
	app := fiber.New(fiber.Config{
		// Fiber will return 503 to new requests during shutdown
		// giving in-flight requests time to complete
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})

	// Middleware
	app.Use(api.LoggerMiddleware())

	// Prometheus metrics endpoint
	app.Get("/metrics", func(c *fiber.Ctx) error {
		fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())(c.Context())
		return nil
	})

	// Routes
	app.Get("/health", api.HealthHandler)
	app.Get("/swagger/*", fiberSwagger.WrapHandler)

	v1 := app.Group("/api/v1")
	v1.Post("/tasks", taskHandler.CreateTask)
	v1.Get("/tasks", taskHandler.GetAllTasks)
	v1.Get("/tasks/dead", taskHandler.ListDeadTasks) // before /:id
	v1.Post("/tasks/dead/:id/retry", taskHandler.RetryDeadTask)
	v1.Delete("/tasks/dead/:id", taskHandler.DeleteDeadTask)
	v1.Get("/tasks/:id", taskHandler.GetTaskByID) // after /dead

	// Workflow routes
	v1.Post("/workflows", workflowHandler.StartWorkflow)
	v1.Get("/workflows", workflowHandler.GetAllWorkflows)
	v1.Get("/workflows/:id", workflowHandler.GetWorkflow)

	// Start HTTP server in a goroutine so it doesn't block
	go func() {
		logger.Info("server starting", zap.String("port", cfg.AppPort))
		if err := app.Listen(":" + cfg.AppPort); err != nil {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// Block here until we recieve a shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit // execution pauses here until signal arrives

	logger.Info("Shutdown signal recieved - draining...")

	// Give everything 30 seconds to finish
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown Fiber (stops accepting new requests, waits for in-flight)
	if err := app.ShutdownWithContext(ctx); err != nil {
		logger.Error("HTTP shutdown error", zap.Error(err))
	}

	// Shutdown Asynq worker (waits for running tasks to finish)
	workerSrv.Shutdown()

	logger.Info("Shutdown complete.")
}
