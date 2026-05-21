package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/slash-init/goflow/internal/api"
	"github.com/slash-init/goflow/internal/repository"
	"github.com/slash-init/goflow/internal/service"
	"github.com/slash-init/goflow/internal/worker"
	"github.com/slash-init/goflow/pkg/config"
	"github.com/slash-init/goflow/pkg/database"
	"github.com/slash-init/goflow/pkg/queue"
)

func main() {
	cfg := config.Load()
	db := database.NewPostgresDB(cfg)
	defer db.Close() //closes DB connection when main() exits

    queueClient := queue.NewClient(cfg.RedisAddr)
    defer queueClient.Close()

	// Wire up layers
	taskRepo := repository.NewTaskRepository(db)
	taskSvc := service.NewTaskService(taskRepo, queueClient)
	taskHandler := api.NewTaskHandler(taskSvc)

	// Start worker in background goroutine
	workerSrv := worker.NewWorkerServer(cfg.RedisAddr)
	go worker.RegisterHandlers(workerSrv) // <-- this is a goroutine

	//create a new Fiber app
	app := fiber.New(fiber.Config{
		// Fiber will return 503 to new requests during shutdown
		// giving in-flight requests time to complete
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})

	//register a route
	app.Get("/health", api.HealthHandler)

    v1 := app.Group("/api/v1")
    v1.Post("/tasks", taskHandler.CreateTask)
    v1.Get("/tasks", taskHandler.GetAllTasks)
    v1.Get("/tasks/dead", taskHandler.ListDeadTasks)          // before /:id
    v1.Post("/tasks/dead/:id/retry", taskHandler.RetryDeadTask)
    v1.Delete("/tasks/dead/:id", taskHandler.DeleteDeadTask)
    v1.Get("/tasks/:id", taskHandler.GetTaskByID)             // after /dead

	// Start HTTP server in a goroutine so it doesn't block
	go func() {
		log.Printf("Server starting on :%s", cfg.AppPort)
		if err := app.Listen(":" + cfg.AppPort); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Block here until we recieve a shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit // execution pauses here until signal arrives

	log.Println("Shutdown signal recieved - draining...")

	// Give everything 30 seconds to finish
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown Fiber (stops accepting new requests, waits for in-flight)
	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	// Shutdown Asynq worker (waits for running tasks to finish)
	workerSrv.Shutdown()

	log.Println("Shutdown complete.")
}
