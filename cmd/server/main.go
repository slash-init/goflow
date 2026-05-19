package main

import (
	"log"

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
	app := fiber.New()

	//register a route
	app.Get("/health", api.HealthHandler)

	api := app.Group("/api/v1")
	api.Post("/tasks", taskHandler.CreateTask)
	api.Get("/tasks", taskHandler.GetAllTasks)
	api.Get("/tasks/:id", taskHandler.GetTaskByID)

	// start server
	log.Fatal(app.Listen(":" + cfg.AppPort))
}
