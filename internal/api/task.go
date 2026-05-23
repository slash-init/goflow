package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/slash-init/goflow/internal/service"
)

type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

func (h *TaskHandler) CreateTask(c *fiber.Ctx) error {
	var input service.CreateTaskInput

	// Parse request body
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	task, err := h.svc.CreateTask(input)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(task)
}

func (h *TaskHandler) GetAllTasks(c *fiber.Ctx) error {
	tasks, err := h.svc.GetAllTasks()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "could not fetch tasks",
		})
	}

	return c.JSON(fiber.Map{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func (h *TaskHandler) GetTaskByID(c *fiber.Ctx) error {
	id := c.Params("id") // like req.params.id in Express

	task, err := h.svc.GetTaskByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(task)
}

func (h *TaskHandler) ListDeadTasks(c *fiber.Ctx) error {
	queue := c.Query("queue", "default") // ?queue=critical

	tasks, err := h.svc.ListDeadTasks(queue)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// return empty array not null when no dead tasks
	if tasks == nil {
		return c.JSON(fiber.Map{
			"queue": queue,
			"count": 0,
			"tasks": []interface{}{},
		})
	}

	return c.JSON(fiber.Map{
		"queue": queue,
		"count": len(tasks),
		"tasks": tasks,
	})
}

func (h *TaskHandler) RetryDeadTask(c *fiber.Ctx) error {
	taskID := c.Params("id")
	queue := c.Query("queue", "default")

	if err := h.svc.RetryDeadTask(queue, taskID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "task re-queued successfully",
		"id":      taskID,
	})
}

func (h *TaskHandler) DeleteDeadTask(c *fiber.Ctx) error {
	taskID := c.Params("id")
	queue := c.Query("queue", "default")

	if err := h.svc.DeleteDeadTask(queue, taskID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "task deleted",
		"id":      taskID,
	})
}
